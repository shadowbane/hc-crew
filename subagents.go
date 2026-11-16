package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Claude Code writes, per subagent of a session:
//   <projects>/<dir>/<session>/subagents/agent-<id>.meta.json  {toolUseId, description, model, finished, ...}
//   <projects>/<dir>/<session>/subagents/agent-<id>.jsonl      its transcript
// These are the source of truth for subagent state: they cover foreground and
// background subagents, including ones started before the crew's hooks existed.

type subMeta struct {
	ToolUseID   string `json:"toolUseId"`
	Description string `json:"description"`
	Model       string `json:"model"`
	AgentType   string `json:"agentType"`
	Finished    bool   `json:"finished"`
}

// scanSubagents folds the main session's subagent files into dashboard state.
func (d *dashModel) scanSubagents() {
	home, _ := os.UserHomeDir()
	metas, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", d.m.MainSession, "subagents", "agent-*.meta.json"))
	for _, mp := range metas {
		b, err := os.ReadFile(mp)
		if err != nil {
			continue
		}
		var sm subMeta
		if json.Unmarshal(b, &sm) != nil || sm.ToolUseID == "" {
			continue
		}
		agent := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(mp), "agent-"), ".meta.json")
		model := sm.Model
		if model == "" {
			model = sm.AgentType
		}
		s := d.subs[sm.ToolUseID]
		if s == nil {
			s = &sub{text: sm.Description, role: roleFor(model), fam: famOrHelper(model)}
			d.subs[sm.ToolUseID] = s
			d.order = append(d.order, sm.ToolUseID)
		}
		s.agent = agent
		d.agents[agent] = s.fam
		if sm.Finished {
			if !s.done {
				s.done, s.status, s.now = true, "completed", ""
			}
			continue
		}
		s.done = false
		s.now = lastToolCall(strings.TrimSuffix(mp, ".meta.json") + ".jsonl")
	}
}

// lastToolCall returns a one-line summary of the newest tool call in a
// transcript, read from its tail, e.g. "Bash: pest tests/Feature/Auth".
func lastToolCall(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	const tail = 256 << 10
	if st, err := f.Stat(); err == nil && st.Size() > tail {
		_, _ = f.Seek(st.Size()-tail, io.SeekStart)
	}
	buf, _ := io.ReadAll(f)
	lines := bytes.Split(buf, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"tool_use"`)) {
			continue
		}
		var rec struct {
			Message struct {
				Content []struct {
					Type  string          `json:"type"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &rec) != nil {
			continue // first line of the tail may be cut
		}
		for j := len(rec.Message.Content) - 1; j >= 0; j-- {
			c := rec.Message.Content[j]
			if c.Type == "tool_use" {
				return c.Name + ": " + toolArg(c.Input)
			}
		}
	}
	return "thinking"
}

// toolArg picks the most telling argument of a tool call.
func toolArg(raw json.RawMessage) string {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	for _, k := range []string{"command", "file_path", "pattern", "path", "url", "query", "description", "prompt"} {
		if v, ok := in[k].(string); ok && v != "" {
			if k == "file_path" || k == "path" {
				v = shortPath(v)
			}
			return firstLine(v)
		}
	}
	return ""
}
