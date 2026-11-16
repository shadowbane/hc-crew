package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Meta describes one crew run. Stored as <stateRoot>/<id>/meta.json.
type Meta struct {
	ID             string    `json:"id"`
	Task           string    `json:"task"`
	Cwd            string    `json:"cwd"`
	Created        time.Time `json:"created"`
	MainName       string    `json:"main_name"`
	AdvisorName    string    `json:"advisor_name"`
	MainSession    string    `json:"main_session"`
	AdvisorSession string    `json:"advisor_session"`
	MainModel      string    `json:"main_model"`
	AdvisorModel   string    `json:"advisor_model"`
	FromSession    bool      `json:"from_session,omitempty"` // main continues a session the user started`
}

// Event is one line of <stateRoot>/<id>/events.jsonl, written by hooks and by
// consult/advise. The dashboard renders these.
type Event struct {
	T      time.Time `json:"t"`
	Kind   string    `json:"kind"`             // task, sub_start, sub_done, bash, edit, consult, advice
	Key    string    `json:"key,omitempty"`    // tool_use_id for sub_start/sub_done
	Agent  string    `json:"agent,omitempty"`  // subagent id when the action ran inside a subagent
	Role   string    `json:"role,omitempty"`   // checker, swarm, helper; checkpoint name for consult
	Model  string    `json:"model,omitempty"`  // requested or resolved model
	Text   string    `json:"text,omitempty"`   // description, command, file, question, advice
	Detail string    `json:"detail,omitempty"` // "2 failed", "+9 -3", status
}

// stateRoot is where crews live. HC_CREW_STATE is set by start for every crew
// process, so hooks find the same folder even when the hc core's environment
// differs from the shell that started the crew.
func stateRoot() string {
	if d := os.Getenv("HC_CREW_STATE"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "hc-crew")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "hc-crew")
}

func crewDir(id string) string { return filepath.Join(stateRoot(), id) }

func newID() string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func saveMeta(m *Meta) error {
	if err := os.MkdirAll(crewDir(m.ID), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(crewDir(m.ID), "meta.json"), b, 0o644)
}

func loadMeta(id string) (*Meta, error) {
	b, err := os.ReadFile(filepath.Join(crewDir(id), "meta.json"))
	if err != nil {
		return nil, fmt.Errorf("no crew %q (%v)", id, err)
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// allMetas returns every crew, newest first.
func allMetas() []*Meta {
	ents, _ := os.ReadDir(stateRoot())
	var out []*Meta
	for _, e := range ents {
		if m, err := loadMeta(e.Name()); err == nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// envCrew returns the crew this process belongs to (set by hc-crew start).
func envCrew() (*Meta, error) {
	id := os.Getenv("CREW_ID")
	if id == "" {
		return nil, errors.New("CREW_ID is not set: this command runs inside a crew")
	}
	return loadMeta(id)
}

func appendEvent(id string, ev Event) error {
	if ev.T.IsZero() {
		ev.T = time.Now()
	}
	f, err := os.OpenFile(filepath.Join(crewDir(id), "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(ev)
	_, err = f.Write(append(b, '\n'))
	return err
}

// eventTail reads events.jsonl from offset and returns new events + new offset.
func eventTail(id string, off int64) ([]Event, int64) {
	f, err := os.Open(filepath.Join(crewDir(id), "events.jsonl"))
	if err != nil {
		return nil, off
	}
	defer f.Close()
	if _, err := f.Seek(off, 0); err != nil {
		return nil, off
	}
	var out []Event
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil { // partial last line: leave it for next time
			break
		}
		off += int64(len(line))
		var ev Event
		if json.Unmarshal(line, &ev) == nil {
			out = append(out, ev)
		}
	}
	return out, off
}
