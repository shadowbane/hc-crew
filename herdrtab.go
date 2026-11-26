package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The Herdr tab title of a crew's main is kept in its own file, crewDir/tab,
// so that meta.json has a single kind of writer. The dashboard saves it while
// the crew runs, stop saves it before ending the terminals, and resume gives
// the new tab that title again.

func tabFile(id string) string { return filepath.Join(crewDir(id), "tab") }

// readTab is the saved tab title, or "".
func readTab(id string) string {
	b, err := os.ReadFile(tabFile(id))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// writeTab saves the tab title when it changed. It never creates the crew's
// folder, so a dashboard left open on a removed crew doesn't bring it back.
func writeTab(id, name string) {
	if _, err := os.Stat(crewDir(id)); err != nil || readTab(id) == name {
		return
	}
	tmp := tabFile(id) + ".tmp"
	if os.WriteFile(tmp, []byte(name+"\n"), 0o644) == nil {
		_ = os.Rename(tmp, tabFile(id))
	}
}

// keepTabName reports whether a tab title was given by the user: Herdr names
// a new tab after the hc terminal (crew-<id>-main) or numbers it.
func keepTabName(name string) bool {
	if name == "" || strings.HasPrefix(name, "crew-") {
		return false
	}
	return strings.Trim(name, "0123456789") != ""
}

// tabNameFor finds a tab's title in `herdr-app ls` output.
func tabNameFor(ls []byte, tabID string) string {
	var r struct {
		Spaces []struct {
			Tabs []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"tabs"`
		} `json:"spaces"`
	}
	if tabID == "" || json.Unmarshal(ls, &r) != nil {
		return ""
	}
	for _, s := range r.Spaces {
		for _, t := range s.Tabs {
			if t.ID == tabID {
				return t.Name
			}
		}
	}
	return ""
}

// hcTabID is the Herdr tab id hc reports for a terminal, or "".
func hcTabID(ctx context.Context, name string) string {
	out, err := exec.CommandContext(ctx, "hc", "get", name, "--json").Output()
	if err != nil {
		return ""
	}
	var r struct {
		Labels map[string]string `json:"labels"`
	}
	if json.Unmarshal(out, &r) != nil {
		return ""
	}
	return r.Labels["herdr.tab"]
}

// saveTab records the title of main's Herdr tab if the user gave it one.
// It gives up silently after 2 seconds or when Herdr isn't there.
func saveTab(m *Meta) {
	if _, err := exec.LookPath("herdr-app"); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	id := hcTabID(ctx, m.MainName)
	if id == "" {
		return
	}
	ls, err := exec.CommandContext(ctx, "herdr-app", "ls").Output()
	if err != nil {
		return
	}
	if name := tabNameFor(ls, id); keepTabName(name) {
		writeTab(m.ID, name)
	}
}

// resumeTabName is the title main's tab gets back on resume: the saved one,
// else the name the user gave main's session with /rename, else "" (Herdr's own).
func resumeTabName(m *Meta) string {
	if t := readTab(m.ID); t != "" {
		return t
	}
	if m.MainSession == "" {
		return ""
	}
	if s, err := findSession(m.MainSession); err == nil {
		return s.Named
	}
	return ""
}

// restoreTab renames the tab that now shows main. A failure is only logged:
// it must never fail the resume.
func restoreTab(m *Meta, name string) {
	if name == "" {
		return
	}
	for i := 0; i < 6; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		id := hcTabID(ctx, m.MainName)
		if id == "" {
			cancel()
			continue
		}
		out, err := exec.CommandContext(ctx, "herdr-app", "do", "tab.rename", "--id", id, "--name", name).CombinedOutput()
		cancel()
		herdrLog(m, "tab.rename "+id, out)
		if err == nil && !strings.Contains(string(out), `"error"`) {
			return
		}
	}
}

// mainSessionName is the -n name main starts with. A resumed session keeps the
// name the user gave it with /rename; otherwise it is crew-<id>-main.
func mainSessionName(m *Meta, resume bool) string {
	if resume && m.MainSession != "" && sessionSaved(m.MainSession) {
		if s, err := findSession(m.MainSession); err == nil && s.Named != "" {
			return s.Named
		}
	}
	return m.MainName
}
