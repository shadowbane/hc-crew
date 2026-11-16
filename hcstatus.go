package main

import (
	"encoding/json"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// termInfo is what the dashboard needs from one hc terminal.
type termInfo struct {
	found bool
	addr  string // terminal address, e.g. t1r (Herdr only accepts this, not the name)
	state string // live, finished
	agent string // idle, working, done, blocked, error ("" if the program reports none)
	msg   string // the agent's own one-line progress note, when it gives one
}

// hcGet reads one terminal with `hc get`. Status comes either as a plain
// string or, for Claude Code, as {"app","state","msg"}.
func hcGet(name string) termInfo {
	out, err := exec.Command("hc", "get", name, "--json").Output()
	if err != nil {
		return termInfo{}
	}
	var r struct {
		Address string          `json:"address"`
		State   string          `json:"state"`
		Status  json.RawMessage `json:"status"`
	}
	if json.Unmarshal(out, &r) != nil {
		return termInfo{}
	}
	ti := termInfo{found: true, addr: r.Address, state: r.State}
	var s string
	var o struct {
		State string `json:"state"`
		Msg   string `json:"msg"`
	}
	if json.Unmarshal(r.Status, &s) == nil {
		ti.agent = s
	} else if json.Unmarshal(r.Status, &o) == nil {
		ti.agent, ti.msg = o.State, o.Msg
	}
	return ti
}

type statusMsg map[string]termInfo // role -> info
type pollMsg struct{}

func pollStatus(m *Meta) tea.Cmd {
	return func() tea.Msg {
		return statusMsg{"main": hcGet(m.MainName), "advisor": hcGet(m.AdvisorName)}
	}
}

func pollLater() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return pollMsg{} })
}
