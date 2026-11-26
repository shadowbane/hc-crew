package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSession writes a transcript for id under a temporary HOME.
func fakeSession(t *testing.T, id string, lines ...string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".claude", "projects", "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMainSessionName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id := "11111111-2222-3333-4444-555555555555"
	m := &Meta{ID: "ab12", MainName: "crew-ab12-main", AdvisorName: "crew-ab12-advisor", MainSession: id}

	if got := mainSessionName(m, true); got != "crew-ab12-main" {
		t.Errorf("unsaved session: got %q", got)
	}
	// What an old resume left behind: the user's rename, then crew-ab12-main again.
	fakeSession(t, id, `{"type":"custom-title","customTitle":"crew-ab12-main"}`,
		`{"type":"custom-title","customTitle":"e-invoicing"}`, `{"type":"custom-title","customTitle":"crew-ab12-main"}`)
	if got := mainSessionName(m, true); got != "e-invoicing" {
		t.Errorf("resume after /rename: got %q, want e-invoicing", got)
	}
	if got := mainSessionName(m, false); got != "crew-ab12-main" {
		t.Errorf("fresh start: got %q", got)
	}
	fakeSession(t, id, `{"type":"custom-title","customTitle":"crew-ab12-main"}`)
	if got := mainSessionName(m, true); got != "crew-ab12-main" {
		t.Errorf("resume, never renamed: got %q", got)
	}
}

func TestTabFile(t *testing.T) {
	t.Setenv("HC_CREW_STATE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	writeTab("ab12", "einvoicing")
	if _, err := os.Stat(crewDir("ab12")); err == nil {
		t.Fatal("writeTab must not create a removed crew's folder")
	}
	if err := os.MkdirAll(crewDir("ab12"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTab("ab12", "einvoicing")
	if got := readTab("ab12"); got != "einvoicing" {
		t.Errorf("readTab = %q", got)
	}
	if _, err := os.Stat(tabFile("ab12") + ".tmp"); err == nil {
		t.Error("temp file left behind")
	}

	// resume: saved tab wins, else the /rename name, else Herdr's own.
	id := "11111111-2222-3333-4444-555555555555"
	fakeSession(t, id, `{"type":"custom-title","customTitle":"renamed"}`)
	m := &Meta{ID: "ab12", MainSession: id}
	if got := resumeTabName(m); got != "einvoicing" {
		t.Errorf("saved tab: got %q", got)
	}
	os.Remove(tabFile("ab12"))
	if got := resumeTabName(m); got != "renamed" {
		t.Errorf("no saved tab: got %q, want the /rename name", got)
	}
	if got := resumeTabName(&Meta{ID: "ab12"}); got != "" {
		t.Errorf("no session: got %q", got)
	}
}

func TestKeepTabName(t *testing.T) {
	for name, want := range map[string]bool{
		"einvoicing": true, "mcp-upload": true, "Codex 2": true,
		"": false, "3": false, "12": false, "crew-ab12-main": false,
	} {
		if got := keepTabName(name); got != want {
			t.Errorf("keepTabName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTabNameFor(t *testing.T) {
	ls := []byte(`{"spaces":[{"id":"sp1","name":"leapcount","tabs":[{"id":"tb1","name":"1","terminals":[]},
		{"id":"tb2","name":"einvoicing","terminals":[{"terminal":"t169"}]}]},
		{"id":"sp2","name":"oth","tabs":[{"id":"tb3","name":"Codex"}]}],"recent":[]}`)
	for id, want := range map[string]string{"tb2": "einvoicing", "tb3": "Codex", "tb9": "", "": ""} {
		if got := tabNameFor(ls, id); got != want {
			t.Errorf("tabNameFor(%q) = %q, want %q", id, got, want)
		}
	}
	if got := tabNameFor([]byte("not json"), "tb1"); got != "" {
		t.Errorf("bad json: got %q", got)
	}
}
