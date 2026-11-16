package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const mainPrompt = `You are the MAIN builder in an hc-crew run (crew %[1]s). You plan, write code and drive the build.

Crew rules:
1. Advisor (Fable, read-only) - consult it at exactly these checkpoints:
   - before your plan locks:            %[2]s consult plan "<plan summary + what you are unsure of>"
   - when the same error happens twice: %[2]s consult error "<the error + what you already tried>"
   - before you call the task done:     %[2]s consult done "<what changed + how you verified it>"
   The command returns at once. The reply arrives later as a message starting with "Advisor:"; the command tells you how to wait for it.
2. Checker - to verify work, spawn a subagent with model "sonnet" whose only job is to run the tests and try to break the change. It must not edit files.
3. Swarm - you do not explore the repo yourself. Any reading beyond the file you are about to edit (understanding code, grep/find sweeps, cat/sed of several files, docs, summaries) goes to subagents: subagent_type "Explore", model "haiku", 2-4 in parallel in one message, each with one narrow, self-contained question and a request for a short answer with file:line references. Reading through Bash (cat, sed, grep, find) counts as reading. You keep the plan, the decisions and the edits.
4. Slow commands (test suites, builds, installs, anything over ~30 s) never run directly in your Bash tool, which would block you. Run them in an hc terminal and wait with hc, which returns early when a message arrives:
     hc run --bg --name crew-%[1]s-<short> -- <command>
     hc wait crew-%[1]s-<short> --timeout 100      (repeat on timeout)
     hc read crew-%[1]s-<short> --lines 80
     hc kill crew-%[1]s-<short>; hc rm crew-%[1]s-<short>
   Several can run at once. A Checker subagent may run the same way.
5. Do not start other crews, and do not start other agents with hc run.`

// resumeNote is main's first message after a resume.
const resumeNote = `[hc-crew] This crew was restarted. Your crew rules are in your system prompt now, and they replace any crew rules you remember from earlier in this conversation; re-read them. If you were in the middle of a task, carry on under the current rules. Otherwise reply "ready" and wait.`

const advisorPrompt = `You are the ADVISOR in an hc-crew run (crew %[1]s). You never edit files, never run builds, never start agents.
The main builder (Opus) consults you at checkpoints: plan, error, done. Each consult includes the question and the path of the main session transcript (JSONL); read its tail or repo files only when needed.
Answer short: what was missed, the biggest risk, one concrete recommendation. For "error": root cause or rabbit hole? For "done": did the diff break anything?
Deliver every answer ONLY by running:
%[2]s advise <<'EOF'
<your advice>
EOF
Then wait for the next consult.`

func cmdStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	cwd := fs.String("cwd", "", "working directory (default: current)")
	mainModel := fs.String("main-model", "opus", "model for the main session")
	advModel := fs.String("advisor-model", "claude-fable-5-2", "model for the advisor")
	noUI := fs.Bool("no-ui", false, "start the crew without opening the dashboard")
	from := fs.String("from", "", "continue this Claude session (id or unique prefix) as the crew's main")
	if err := fs.Parse(args); err != nil {
		return err
	}
	task := strings.TrimSpace(strings.Join(fs.Args(), " "))
	var src *sessionInfo
	if *from != "" {
		var err error
		if src, err = findSession(*from); err != nil {
			return err
		}
		if *cwd == "" {
			*cwd = src.Cwd // claude --resume only finds a session from its own project folder
		}
	}
	if *cwd == "" {
		*cwd, _ = os.Getwd()
	}
	abs, err := filepath.Abs(*cwd)
	if err != nil {
		return err
	}

	id := newID()
	m := &Meta{
		ID: id, Task: task, Cwd: abs, Created: time.Now(),
		MainName: "crew-" + id + "-main", AdvisorName: "crew-" + id + "-advisor",
		MainSession: newUUID(), AdvisorSession: newUUID(),
		MainModel: *mainModel, AdvisorModel: *advModel,
	}
	mainResume := false
	if src != nil {
		m.MainSession, mainResume = src.ID, true
		m.Task = src.Title
		m.FromSession = true
		if src.Cwd != "" && src.Cwd != abs {
			fmt.Fprintf(os.Stderr, "note: session %s ran in %s; claude may not find it from %s\n", src.ID[:8], src.Cwd, abs)
		}
	}
	if err := saveMeta(m); err != nil {
		return err
	}
	_ = appendEvent(id, Event{Kind: "task", Text: m.Task})

	if err := launch(m, mainResume, false, task); err != nil {
		return err
	}
	fmt.Printf("crew %s started\n  main:    hc attach %s\n  advisor: hc attach %s\n  dash:    hc-crew dash %s\n  stop:    hc-crew stop %s\n",
		id, m.MainName, m.AdvisorName, id, id)
	return openUI(m, *noUI)
}

// cmdResume restarts a stopped crew on its saved Claude sessions, with the
// same names, labels, hooks and crew rules. The activity log carries on.
func cmdResume(args []string) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	noUI := fs.Bool("no-ui", false, "resume without opening the dashboard")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: hc-crew resume [--no-ui] ID")
	}
	m, err := loadMeta(fs.Arg(0))
	if err != nil {
		return err
	}
	for _, n := range []string{m.MainName, m.AdvisorName} {
		if t := hcGet(n); t.found && t.state == "live" {
			return fmt.Errorf("%s is still running: use hc-crew dash %s, or hc-crew stop %s first", n, m.ID, m.ID)
		}
		_ = exec.Command("hc", "rm", n).Run() // clear a finished terminal so the name is free
	}
	note := ""
	if sessionSaved(m.MainSession) {
		note = resumeNote // a resumed conversation still remembers the crew rules it started with
	}
	if err := launch(m, true, true, note); err != nil {
		return err
	}
	_ = appendEvent(m.ID, Event{Kind: "resume", Text: "crew resumed"})
	fmt.Printf("crew %s resumed\n  main:    hc attach %s\n  advisor: hc attach %s\n", m.ID, m.MainName, m.AdvisorName)
	return openUI(m, *noUI)
}

// launch starts the advisor and main terminals. Each one either starts its
// session fresh or resumes it; prompt (if any) is sent as main's first message.
func launch(m *Meta, mainResume, advResume bool, prompt string) error {
	bin := selfPath()
	settings, err := writeSettings(m.ID, bin)
	if err != nil {
		return err
	}
	session := func(id string, resume bool) []string {
		// Claude Code writes a session to disk only after its first message; an
		// advisor that was never consulted has nothing to resume.
		if resume && sessionSaved(id) {
			return []string{"--resume", id}
		}
		return []string{"--session-id", id}
	}
	env := func(role string) []string {
		return []string{"env", "CREW_ID=" + m.ID, "CREW_ROLE=" + role, "HC_CREW_STATE=" + stateRoot(), claudeBin()}
	}

	// Advisor first, so it is ready before main's first consult.
	adv := append(env("advisor"), "--model", m.AdvisorModel)
	adv = append(adv, session(m.AdvisorSession, advResume)...)
	adv = append(adv, "-n", m.AdvisorName,
		"--append-system-prompt", fmt.Sprintf(advisorPrompt, m.ID, bin),
		"--disallowedTools", "Edit,Write,MultiEdit,NotebookEdit,Agent,Task",
		"--allowedTools", "Read,Grep,Glob,Bash("+bin+" advise:*)")
	if err := hcRun(m, "advisor", adv); err != nil {
		return fmt.Errorf("start advisor: %w", err)
	}

	main := append(env("main"), "--model", m.MainModel)
	main = append(main, session(m.MainSession, mainResume)...)
	if !m.FromSession {
		main = append(main, "-n", m.MainName) // keep the user's own name on a session brought in with --from
	}
	main = append(main, "--settings", settings,
		"--append-system-prompt", fmt.Sprintf(mainPrompt, m.ID, bin))
	if prompt != "" {
		main = append(main, prompt)
	}
	if err := hcRun(m, "main", main); err != nil {
		cmdStopQuiet(m)
		return fmt.Errorf("start main: %w", err)
	}
	return nil
}

func openUI(m *Meta, noUI bool) error {
	if noUI {
		return nil
	}
	if openInHerdr(m, selfPath()) {
		fmt.Println("opened in Herdr: main on the left, dashboard on the right")
		return nil
	}
	return runDash(m)
}

func hcRun(m *Meta, role string, argv []string) error {
	name := m.MainName
	if role == "advisor" {
		name = m.AdvisorName
	}
	args := []string{"run", "--bg", "--no-parent", "--no-notify", "--json",
		"--name", name, "--cwd", m.Cwd,
		"--label", "crew=" + m.ID, "--label", "role=" + role, "--"}
	out, err := exec.Command("hc", append(args, argv...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// writeSettings writes the Claude Code settings used only by this crew's main
// session: the logging hooks and permission to run consult.
func writeSettings(id, bin string) (string, error) {
	hook := []map[string]any{{"type": "command", "command": bin + " hook", "timeout": 10}}
	s := map[string]any{
		"permissions": map[string]any{"allow": []string{"Bash(" + bin + " consult:*)",
			"Bash(hc wait:*)", "Bash(hc read:*)", "Bash(hc kill crew-" + id + "-:*)", "Bash(hc rm crew-" + id + "-:*)"}},
		"hooks": map[string]any{
			"PreToolUse":   []map[string]any{{"matcher": "Agent|Task", "hooks": hook}},
			"PostToolUse":  []map[string]any{{"matcher": "Agent|Task|Bash|Edit|Write|MultiEdit|Read|Grep|Glob", "hooks": hook}},
			"SubagentStop": []map[string]any{{"hooks": hook}},
		},
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	p := filepath.Join(crewDir(id), "settings.json")
	return p, os.WriteFile(p, b, 0o644)
}

var rePane = regexp.MustCompile(`"(pane|pane_id)"\s*:\s*"(p[0-9a-z]+)"`)

// openInHerdr gives the main terminal a tab in the Herdr window and splits the
// dashboard to its right. Returns false when there is no Herdr window to use.
func openInHerdr(m *Meta, bin string) bool {
	if _, err := exec.LookPath("herdr-app"); err != nil {
		return false
	}
	if err := exec.Command("herdr-app", "status").Run(); err != nil {
		return false
	}
	t := hcGet(m.MainName)
	if !t.found || t.addr == "" {
		fmt.Fprintf(os.Stderr, "herdr: %s not found in hc\n", m.MainName)
		return false
	}
	// herdr-app wants the terminal address, and reports errors as JSON with exit 0.
	// A terminal hc just started can take a moment to reach Herdr: retry.
	var out []byte
	var err error
	for i := 0; i < 20; i++ {
		out, err = exec.Command("herdr-app", "do", "terminal.keep", "--terminal", t.addr, "--place", "tab").CombinedOutput()
		if err == nil && !strings.Contains(string(out), `"error"`) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil || strings.Contains(string(out), `"error"`) {
		herdrLog(m, "terminal.keep (gave up)", out)
		fmt.Fprintf(os.Stderr, "herdr: could not show main (%v: %s)\n", err, strings.TrimSpace(string(out)))
		return false
	}
	herdrLog(m, "terminal.keep", out)
	// The pane id in keep's reply can be a pending one that split then rejects.
	// Retry, and from the second try ask Herdr which pane shows main.
	pane := ""
	if mm := rePane.FindStringSubmatch(string(out)); mm != nil {
		pane = mm[2]
	}
	for i := 0; i < 12; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
			if p := herdrPaneOf(t.addr); p != "" {
				pane = p
			}
		}
		split := []string{"do", "pane.split", "--direction", "right", "--argv", jsonArr(bin, "dash", m.ID)}
		if pane != "" {
			split = append(split, "--pane", pane)
		}
		out, err = exec.Command("herdr-app", split...).CombinedOutput()
		herdrLog(m, "pane.split "+pane, out)
		if err == nil && !strings.Contains(string(out), `"error"`) {
			return true
		}
	}
	fmt.Fprintf(os.Stderr, "herdr: could not open dashboard (%v: %s); see %s\n", err, strings.TrimSpace(string(out)),
		filepath.Join(crewDir(m.ID), "herdr.log"))
	return false
}

// herdrPaneOf focuses the pane showing terminal addr and returns its id.
func herdrPaneOf(addr string) string {
	if exec.Command("herdr-app", "do", "pane.focus", "--terminal", addr).Run() != nil {
		return ""
	}
	b, err := exec.Command("herdr-app", "status").Output()
	if err != nil {
		return ""
	}
	var st struct {
		Pane  string `json:"focused_pane"`
		Agent string `json:"agent"`
	}
	if json.Unmarshal(b, &st) != nil || st.Agent != addr {
		return ""
	}
	return st.Pane
}

func herdrLog(m *Meta, step string, out []byte) {
	f, err := os.OpenFile(filepath.Join(crewDir(m.ID), "herdr.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s: %s\n", time.Now().Format(time.TimeOnly), step, strings.Join(strings.Fields(string(out)), " "))
}

func jsonArr(xs ...string) string {
	b, _ := json.Marshal(xs)
	return string(b)
}

func cmdLs() error {
	ms := allMetas()
	if len(ms) == 0 {
		fmt.Println("no crews")
		return nil
	}
	for _, m := range ms {
		fmt.Printf("%s  %s  main:%-9s advisor:%-9s %s\n", m.ID, m.Created.Format("01-02 15:04"),
			hcState(m.MainName), hcState(m.AdvisorName), trunc(m.Task, 60))
	}
	return nil
}

func hcState(name string) string {
	t := hcGet(name)
	switch {
	case !t.found:
		return "gone"
	case t.state == "live" && t.agent != "":
		return t.agent
	}
	return t.state
}

func cmdStop(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: hc-crew stop ID")
	}
	m, err := loadMeta(args[0])
	if err != nil {
		return err
	}
	cmdStopQuiet(m)
	fmt.Printf("crew %s stopped (state kept in %s)\n", m.ID, crewDir(m.ID))
	return nil
}

// cmdStopQuiet ends and removes only this crew's own terminals.
func cmdStopQuiet(m *Meta) {
	for _, n := range []string{m.MainName, m.AdvisorName} {
		_ = exec.Command("hc", "kill", n).Run()
		_ = exec.Command("hc", "rm", n).Run()
	}
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// claudeBin is "claude" unless HC_CREW_CLAUDE overrides it (used by tests).
func claudeBin() string {
	if b := os.Getenv("HC_CREW_CLAUDE"); b != "" {
		return b
	}
	return "claude"
}

// sessionSaved reports whether Claude Code has written a transcript for this session id.
func sessionSaved(id string) bool {
	home, _ := os.UserHomeDir()
	m, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", id+".jsonl"))
	return len(m) > 0
}
