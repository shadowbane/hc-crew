package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sessionInfo describes a saved Claude Code session found on disk.
type sessionInfo struct {
	ID    string
	Path  string
	Cwd   string // folder the session ran in
	Title string // the session's name: custom title, else AI title, else first prompt
	Named string // the name the user gave it with /rename or -n, if any
	Auto  string // Claude's own title for it: AI title, else first prompt (up to 500 runes)
}

// crewLabel is the name shown for a crew: the name the user gave main's
// session with /rename wins over the start task, which wins over the
// session's AI title or first prompt.
func crewLabel(m *Meta) string {
	label, _ := crewNames(m)
	return label
}

// crewNames returns the crew's label (see crewLabel) and Claude's own title
// for main's session, reading the transcript once.
func crewNames(m *Meta) (label, auto string) {
	if m.MainSession == "" {
		return m.Task, "" // an empty id would glob every session
	}
	s, err := findSession(m.MainSession)
	if err == nil {
		auto = s.Auto
	}
	switch {
	case err == nil && s.Named != "":
		return s.Named, auto
	case m.Task != "":
		return m.Task, auto
	case err == nil && !strings.HasPrefix(s.Title, "session "):
		return s.Title, auto
	}
	return "", auto
}

// sameTitle reports whether two titles are the same text once whitespace is
// ignored, where one ending in "…" was cut short of the other. A short label
// such as "fix" does not match "fix the deploy script".
func sameTitle(a, b string) bool {
	a, b = strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " ")
	if a == b {
		return true
	}
	cut := func(short, long string) bool {
		s, ok := strings.CutSuffix(short, "…")
		return ok && s != "" && strings.HasPrefix(long, s)
	}
	return cut(a, b) || cut(b, a)
}

// injectedOrigin reports whether a user record's turnOrigin marks a message
// that nobody typed: another session's hand-back, a task notification or a
// system note. Anything else, including "sdk" (claude -p) and names added
// later, still counts as a prompt.
func injectedOrigin(o string) bool {
	switch o {
	case "peer", "task_notification", "system":
		return true
	}
	return false
}

// findSession resolves a session id or unique id prefix to its transcript.
func findSession(idOrPrefix string) (*sessionInfo, error) {
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", idOrPrefix+"*.jsonl"))
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no Claude session %q (run claude --resume in the repo to list them)", idOrPrefix)
	case 1:
	default:
		return nil, fmt.Errorf("%q matches %d sessions; give more of the id", idOrPrefix, len(matches))
	}
	si := &sessionInfo{Path: matches[0], ID: strings.TrimSuffix(filepath.Base(matches[0]), ".jsonl")}
	f, err := os.Open(si.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	var custom, ai, first string
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var rec struct {
				Type       string `json:"type"`
				Cwd        string `json:"cwd"`
				IsMeta     bool   `json:"isMeta"`     // injected by Claude Code, not typed
				TurnOrigin string `json:"turnOrigin"` // "human" when typed; absent in older transcripts
				Message    struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			switch {
			case bytes.Contains(line, []byte(`"custom-title"`)):
				var t struct {
					CustomTitle string `json:"customTitle"`
				}
				// crew-* names are ours, not the user's
				if json.Unmarshal(line, &t) == nil && t.CustomTitle != "" && !strings.HasPrefix(t.CustomTitle, "crew-") {
					custom = t.CustomTitle
				}
				continue
			case bytes.Contains(line, []byte(`"ai-title"`)):
				var t struct {
					AITitle string `json:"aiTitle"`
				}
				if json.Unmarshal(line, &t) == nil && t.AITitle != "" {
					ai = t.AITitle
				}
				continue
			}
			if (si.Cwd == "" || si.Title == "") && json.Unmarshal(line, &rec) == nil {
				if si.Cwd == "" && rec.Cwd != "" {
					si.Cwd = rec.Cwd
				}
				if si.Title == "" && rec.Type == "user" && rec.Message.Role == "user" && !rec.IsMeta &&
					!injectedOrigin(rec.TurnOrigin) {
					var s string
					// "<" also skips messages hc pastes in from other terminals
					if json.Unmarshal(rec.Message.Content, &s) == nil && !strings.HasPrefix(strings.TrimSpace(s), "<") {
						si.Title = trunc(s, 60)
						first = trunc(s, 500)
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	si.Named = custom
	si.Auto = ai
	if si.Auto == "" {
		si.Auto = first
	}
	switch {
	case custom != "":
		si.Title = custom
	case ai != "":
		si.Title = ai
	case si.Title == "":
		si.Title = "session " + si.ID[:8]
	}
	return si, nil
}
