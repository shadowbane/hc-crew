package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	cFable  = lipgloss.Color("#a78bfa")
	cOpus   = lipgloss.Color("#e8825a")
	cSonnet = lipgloss.Color("#60a5fa")
	cHaiku  = lipgloss.Color("#4ade80")
	cDim    = lipgloss.Color("#6b7280")
	cText   = lipgloss.Color("#d1d5db")
	cRed    = lipgloss.Color("#f05266")
	cGreen  = lipgloss.Color("#4ade80")

	spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)

func famColor(f string) lipgloss.Color {
	switch f {
	case "fable":
		return cFable
	case "sonnet":
		return cSonnet
	case "haiku":
		return cHaiku
	}
	return cOpus
}

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

type sub struct {
	role, fam, text, status, agent string
	now                            string // latest tool call while running
	done                           bool
}

type cpState int

const (
	cpPending cpState = iota
	cpActive
	cpDone
)

type tickMsg time.Time

type dashModel struct {
	m      *Meta
	w, h   int
	frame  int
	terms  statusMsg
	evOff  int64
	feed   []Event
	subs   map[string]*sub
	order  []string
	agents map[string]string // subagent id -> model family
	byAgt  map[string]string // subagent id -> sub key (background subagents)
	cps    map[string]cpState
	title  string // main session's own title, used when the crew has no task label
	wakes  int
	edits  int
	tests  int
	nudges int
}

func cmdDash(args []string) error {
	var m *Meta
	var err error
	if len(args) > 0 {
		m, err = loadMeta(args[0])
	} else if ms := allMetas(); len(ms) > 0 {
		m = ms[0]
	} else {
		err = errors.New("no crews yet: hc-crew start \"<task>\"")
	}
	if err != nil {
		return err
	}
	return runDash(m)
}

func runDash(m *Meta) error {
	dm := &dashModel{
		m: m, terms: statusMsg{}, subs: map[string]*sub{}, agents: map[string]string{}, byAgt: map[string]string{},
		cps: map[string]cpState{},
	}
	dm.pull()
	dm.scanSubagents()
	p := tea.NewProgram(dm, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func tick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (d *dashModel) Init() tea.Cmd { return tea.Batch(tick(), pollStatus(d.m)) }

func (d *dashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return d, tea.Quit
		}
	case tea.WindowSizeMsg:
		d.w, d.h = msg.Width, msg.Height
	case tickMsg:
		d.frame++
		d.pull()
		if d.frame%5 == 1 {
			d.scanSubagents()
		}
		if d.m.Task == "" && d.frame%25 == 1 {
			if s, err := findSession(d.m.MainSession); err == nil && !strings.HasPrefix(s.Title, "session ") {
				d.title = s.Title
			}
		}
		return d, tick()
	case pollMsg:
		return d, pollStatus(d.m)
	case statusMsg:
		d.terms = msg
		return d, pollLater()
	}
	return d, nil
}

// pull reads new crew events and folds them into dashboard state.
func (d *dashModel) pull() {
	evs, off := eventTail(d.m.ID, d.evOff)
	d.evOff = off
	for _, ev := range evs {
		switch ev.Kind {
		case "sub_start":
			if _, ok := d.subs[ev.Key]; !ok {
				d.order = append(d.order, ev.Key)
			}
			d.subs[ev.Key] = &sub{role: ev.Role, fam: famOrHelper(ev.Model), text: ev.Text}
		case "sub_done":
			s := d.subs[ev.Key]
			if s == nil {
				s = &sub{text: ev.Text}
				d.subs[ev.Key] = s
				d.order = append(d.order, ev.Key)
			}
			s.status, s.role, s.fam, s.agent = ev.Detail, ev.Role, family(ev.Model), ev.Agent
			s.done = ev.Detail != "async_launched" // background: done arrives as sub_stop
			if ev.Agent != "" {
				d.agents[ev.Agent] = s.fam
				if !s.done {
					d.byAgt[ev.Agent] = ev.Key
				}
			}
		case "sub_stop":
			k, ok := d.byAgt[ev.Agent]
			if !ok {
				continue // a foreground subagent: its sub_done already covers it
			}
			delete(d.byAgt, ev.Agent)
			s := d.subs[k]
			s.done, s.status = true, "completed"
			ev = Event{T: ev.T, Kind: "sub_done", Agent: ev.Agent, Model: s.fam, Text: s.text, Detail: "completed"}
		case "edit":
			d.edits++
		case "bash":
			d.tests++
		case "nudge":
			d.nudges++
		case "consult":
			d.wakes++
			d.cps[ev.Role] = cpActive
		case "advice":
			for k, v := range d.cps {
				if v == cpActive {
					d.cps[k] = cpDone
				}
			}
		}
		if ev.Kind != "sub_start" {
			d.feed = append(d.feed, ev)
		}
	}
	if len(d.feed) > 500 {
		d.feed = d.feed[len(d.feed)-500:]
	}
}

func famOrHelper(model string) string {
	if model == "" || model == "general-purpose" {
		return "opus"
	}
	return family(model)
}

// status renders a crew terminal's agent status; live is true while working.
func (d *dashModel) status(role string) (string, bool) {
	t, ok := d.terms[role]
	switch {
	case !ok:
		return "connecting", false
	case !t.found:
		return "not running", false
	case t.state == "finished":
		return "exited", false
	case t.agent == "":
		return "starting", false
	}
	return t.agent, t.agent == "working"
}

func (d *dashModel) View() string {
	if d.w == 0 {
		return "loading…"
	}
	w := d.w
	var parts []string
	parts = append(parts, d.header(w), d.tree(w))
	parts = append(parts, d.middle(w))
	used := lipgloss.Height(strings.Join(parts, "\n"))
	feedH := d.h - used - 2
	parts = append(parts, d.feedView(w, feedH), d.footer(w))
	return strings.Join(parts, "\n")
}

func (d *dashModel) header(w int) string {
	left := fg(cOpus).Bold(true).Render("✻ hc-crew ") + fg(cDim).Render(d.m.ID+" · ")
	el := time.Since(d.m.Created).Round(time.Second).String()
	right := fg(cDim).Render(el)
	room := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	label := d.m.Task
	if label == "" {
		label = d.title
	}
	if label == "" {
		label = "(no goal yet)"
	}
	task := fg(cText).Render(trunc(label, max(room, 4)))
	return pad(left+task, right, w)
}

func (d *dashModel) tree(w int) string {
	ms, mw := d.status("main")
	as, aw := d.status("advisor")
	dot := func(c lipgloss.Color, name, what string, live bool) string {
		g := "●"
		if live {
			g = spinner[d.frame%len(spinner)]
		}
		return fg(c).Render(g+" "+name) + fg(cDim).Render(" · "+what)
	}
	nSon, nHai := d.count("checker"), d.count("swarm")
	items := []string{
		dot(cFable, "Fable", "advisor "+as, aw),
		dot(cOpus, "Opus", "builds "+ms, mw),
		dot(cSonnet, "Sonnet", fmt.Sprintf("checks ×%d", nSon), d.running("checker") > 0),
		dot(cHaiku, "Haiku", fmt.Sprintf("swarm ×%d", nHai), d.running("swarm") > 0),
	}
	line := strings.Join(items, "   ")
	if lipgloss.Width(line) > w {
		line = strings.Join(items[:2], "   ") + "\n" + strings.Join(items[2:], "   ")
	}
	return line
}

func (d *dashModel) count(role string) int {
	n := 0
	for _, s := range d.subs {
		if s.role == role {
			n++
		}
	}
	return n
}

func (d *dashModel) running(role string) int {
	n := 0
	for _, s := range d.subs {
		if s.role == role && !s.done {
			n++
		}
	}
	return n
}

func (d *dashModel) middle(w int) string {
	if w < 70 {
		half := w / 2
		return lipgloss.JoinVertical(lipgloss.Left, d.advisorBox(w), d.mainBox(w),
			lipgloss.JoinHorizontal(lipgloss.Top, d.subBoxN("checker", half, 3), " ", d.subBoxN("swarm", w-half-1, 3)))
	}
	leftW := min(34, w/3)
	rightW := w - leftW - 1
	half := rightW / 2
	right := lipgloss.JoinVertical(lipgloss.Left,
		d.mainBox(rightW),
		lipgloss.JoinHorizontal(lipgloss.Top, d.subBox("checker", half), " ", d.subBox("swarm", rightW-half-1)))
	left := d.advisorBox(leftW)
	// Match heights so the boxes line up.
	lh, rh := lipgloss.Height(left), lipgloss.Height(right)
	if lh < rh {
		left = d.advisorBoxH(leftW, rh)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (d *dashModel) advisorBox(w int) string { return d.advisorBoxH(w, 0) }

func (d *dashModel) advisorBoxH(w, h int) string {
	st, live := d.status("advisor")
	head := "○ Fable · " + st
	if live {
		head = spinner[d.frame%len(spinner)] + " Fable · advising"
	}
	names := map[string][2]string{
		"plan":  {"before a plan locks", "what did the plan miss?"},
		"error": {"error repeats twice", "root cause or rabbit hole?"},
		"done":  {"before calling done", "did the diff break anything?"},
	}
	lines := []string{fg(cFable).Bold(true).Render(head), fg(cDim).Render("read-only · never writes code"), ""}
	hit := 0
	for _, cp := range checkpoints {
		n := names[cp]
		var mark string
		st := lipgloss.NewStyle()
		switch d.cps[cp] {
		case cpDone:
			mark, st = "✓ ", st.Foreground(cFable)
			hit++
		case cpActive:
			mark, st = "› ", st.Foreground(lipgloss.Color("#111")).Background(cFable).Bold(true)
			hit++
		default:
			mark, st = "· ", st.Foreground(cDim)
		}
		lines = append(lines, st.Render(mark+n[0]), fg(cDim).Render("  "+n[1]))
	}
	foot := pad(fg(cDim).Render("checkpoints"), fg(cFable).Render(fmt.Sprintf("%d / 3", hit)), w-4)
	body := strings.Join(lines, "\n")
	if h > 0 {
		gap := h - 2 - lipgloss.Height(body) - 1
		if gap > 0 {
			body += strings.Repeat("\n", gap)
		}
	}
	return box(body+"\n"+foot, cFable, w)
}

func (d *dashModel) mainBox(w int) string {
	st, live := d.status("main")
	g := "○"
	if live {
		g = spinner[d.frame%len(spinner)]
	}
	head := fg(cOpus).Bold(true).Render(fmt.Sprintf("%s Opus · main session · %s", g, st))
	stats := fg(cDim).Render(fmt.Sprintf("edits %d · test runs %d · subagents %d · advisor wakes %d · nudges %d", d.edits, d.tests, len(d.subs), d.wakes, d.nudges))
	body := lipgloss.PlaceHorizontal(w-4, lipgloss.Center, head)
	if msg := d.terms["main"].msg; msg != "" {
		body += "\n" + lipgloss.PlaceHorizontal(w-4, lipgloss.Center, fg(cText).Render(trunc(msg, w-6)))
	}
	return box(body+"\n"+lipgloss.PlaceHorizontal(w-4, lipgloss.Center, stats), cOpus, w)
}

func (d *dashModel) subBox(role string, w int) string { return d.subBoxN(role, w, 7) }

func (d *dashModel) subBoxN(role string, w, minLines int) string {
	c, title, empty := cSonnet, "Sonnet · checker", "tries to break it · waiting"
	if role == "swarm" {
		c, title, empty = cHaiku, fmt.Sprintf("Haiku · swarm ×%d", d.count("swarm")), "bulk reading · idle"
	}
	lines := []string{fg(c).Bold(true).Render("○ " + title)}
	var mine []*sub
	for _, k := range d.order {
		if s := d.subs[k]; s != nil && s.role == role {
			mine = append(mine, s)
		}
	}
	if len(mine) > 5 {
		mine = mine[len(mine)-5:]
	}
	if len(mine) == 0 {
		lines = append(lines, fg(cDim).Render(empty))
	}
	for _, s := range mine {
		mark := fg(c).Render(spinner[d.frame%len(spinner)])
		if s.done {
			mark = fg(c).Render("✓")
			if s.status != "" && s.status != "completed" && s.status != "async_launched" {
				mark = fg(cRed).Render("✗")
			}
		}
		lines = append(lines, mark+" "+fg(cText).Render(trunc(s.text, w-8)))
		if !s.done && s.now != "" {
			lines = append(lines, fg(cDim).Render("  ↳ "+trunc(s.now, w-10)))
		}
	}
	for len(lines) < minLines {
		lines = append(lines, "")
	}
	return box(strings.Join(lines, "\n"), c, w)
}

func (d *dashModel) feedView(w, h int) string {
	if h < 1 {
		return ""
	}
	tag := fg(cDim).Render(strings.Repeat("─", 2) + " activity " + strings.Repeat("─", max(w-12, 0)))
	var rows []string
	for _, ev := range d.feed {
		if r := d.feedRow(ev, w); r != "" {
			rows = append(rows, r)
		}
	}
	if len(rows) > h-1 {
		rows = rows[len(rows)-(h-1):]
	}
	return tag + "\n" + strings.Join(rows, "\n")
}

func (d *dashModel) feedRow(ev Event, w int) string {
	actor, c, text, det, dc := "", cOpus, ev.Text, ev.Detail, cDim
	switch ev.Kind {
	case "task":
		actor, text, det = "task", ev.Text, "started"
	case "sub_done":
		f := family(ev.Model)
		actor, c, det = f, famColor(f), "done"
		switch ev.Detail {
		case "", "completed":
		case "async_launched":
			det = "launched"
		default:
			det, dc = ev.Detail, cRed
		}
	case "bash":
		actor = "opus"
		if f, ok := d.agents[ev.Agent]; ok {
			actor = f
		} else if ev.Agent != "" {
			actor = "subagent"
		}
		c = famColor(actor)
		switch {
		case strings.Contains(det, "failed"):
			dc = cRed
		case strings.Contains(det, "passed"):
			dc = cGreen
		}
	case "resume":
		actor, c, det = "crew", cDim, "resumed"
	case "nudge":
		actor, c, det, dc = "crew", cDim, "nudged", cHaiku
	case "edit":
		actor, dc = "update", cGreen
	case "consult":
		actor, c, text, det = "→ advisor", cFable, "["+ev.Role+"] "+firstLine(ev.Text), "asked"
	case "advice":
		actor, c, text, det = "advisor", cFable, firstLine(ev.Text), "reviewed"
	default:
		return ""
	}
	left := fg(c).Render("● "+fmt.Sprintf("%-9s", actor)) + " "
	right := fg(dc).Render(det)
	room := w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	return pad(left+fg(cText).Render(trunc(text, max(room, 4))), right, w)
}

func (d *dashModel) footer(w int) string {
	s := fmt.Sprintf("q quit · hc attach %s · hc-crew stop %s", d.m.MainName, d.m.ID)
	return fg(cDim).Render(trunc(s, w))
}

func box(body string, c lipgloss.Color, w int) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c).
		Padding(0, 1).Width(max(w-2, 4)).Render(body)
}

// pad puts left and right on one line of width w.
func pad(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
