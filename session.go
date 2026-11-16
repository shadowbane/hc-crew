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
	var custom, ai string
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var rec struct {
				Type    string `json:"type"`
				Cwd     string `json:"cwd"`
				Message struct {
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
				if si.Title == "" && rec.Type == "user" && rec.Message.Role == "user" {
					var s string
					if json.Unmarshal(rec.Message.Content, &s) == nil && !strings.HasPrefix(s, "<") {
						si.Title = trunc(s, 60)
					}
				}
			}
		}
		if err != nil {
			break
		}
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
