package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRender prints the dashboard for a fixture crew. Run with
// HC_CREW_RENDER=1 go test -run TestRender -v to look at it.
func TestRender(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HC_CREW_STATE", dir)
	m := &Meta{ID: "demo", Task: "Move the API to Rust. Done means: every test passes", Created: time.Now().Add(-3 * time.Minute),
		MainName: "crew-demo-main", AdvisorName: "crew-demo-advisor", MainSession: "x", AdvisorSession: "y"}
	if err := saveMeta(m); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []Event{
		{Kind: "task", Text: m.Task},
		{Kind: "consult", Role: "plan", Text: "Port handlers first, then auth. Unsure about session cache."},
		{Kind: "advice", Text: "Plan misses the token refresh path.\nCover it before porting auth."},
		{Kind: "sub_start", Key: "1", Role: "swarm", Model: "haiku", Text: "grep auth call sites"},
		{Kind: "sub_start", Key: "2", Role: "swarm", Model: "haiku", Text: "list route files"},
		{Kind: "sub_done", Key: "1", Agent: "h1", Role: "swarm", Model: "claude-haiku-5-5", Text: "grep auth call sites", Detail: "completed"},
		{Kind: "sub_start", Key: "3", Role: "checker", Model: "sonnet", Text: "run tests, try to break auth"},
		{Kind: "bash", Agent: "s1", Text: "npm test", Detail: "2 failed"},
		{Kind: "edit", Text: "src/auth/cache.ts", Detail: "+9 -3"},
		{Kind: "consult", Role: "error", Text: "same 2 tests fail after fix"},
		{Kind: "sub_start", Key: "4", Role: "swarm", Model: "haiku", Text: "map frontend pages"},
		{Kind: "sub_done", Key: "4", Agent: "bg1", Role: "swarm", Model: "claude-haiku-5-5", Text: "map frontend pages", Detail: "async_launched"},
		{Kind: "sub_start", Key: "5", Role: "swarm", Model: "haiku", Text: "read sign-in tests"},
		{Kind: "sub_done", Key: "5", Agent: "bg2", Role: "swarm", Model: "claude-haiku-5-5", Text: "read sign-in tests", Detail: "async_launched"},
		{Kind: "sub_stop", Agent: "bg1"},
		{Kind: "sub_stop", Agent: "h1"},
	} {
		if err := appendEvent(m.ID, ev); err != nil {
			t.Fatal(err)
		}
	}
	d := &dashModel{m: m, terms: statusMsg{
		"main":    {found: true, state: "live", agent: "working", msg: "porting auth handlers; 2 tests still failing"},
		"advisor": {found: true, state: "live", agent: "working"},
	}, subs: map[string]*sub{}, agents: map[string]string{"s1": "sonnet"}, byAgt: map[string]string{}, cps: map[string]cpState{}}
	d.pull()
	for _, w := range []int{110, 64} {
		d.Update(tea.WindowSizeMsg{Width: w, Height: 44})
		out := d.View()
		if os.Getenv("HC_CREW_RENDER") != "" {
			fmt.Println(out)
		}
		if d.cps["plan"] != cpDone || d.cps["error"] != cpActive || d.wakes != 2 || len(d.subs) != 5 {
			t.Fatalf("state: cps=%v wakes=%d subs=%d", d.cps, d.wakes, len(d.subs))
		}
		if !d.subs["4"].done || d.subs["5"].done || !d.subs["1"].done {
			t.Fatalf("background subagents: 4 done=%v (want true), 5 done=%v (want false)", d.subs["4"].done, d.subs["5"].done)
		}
	}
}

func TestIsTestCommand(t *testing.T) {
	yes := []string{"npm test", "./vendor/bin/pest tests/Feature/Auth --compact 2>&1 | tail -n 5", "rtk pest tests/Feature/Auth",
		"php artisan test --filter=Login", "go test ./...", "cd app && npm run test:unit", "APP_ENV=testing vendor/bin/phpunit"}
	no := []string{`sed -n 1,200p app/Providers/FortifyServiceProvider.php; rtk ls tests/Feature`, "cat phpunit.xml | sed -n '1,60p'",
		"rtk grep -n test composer.json", "ls tests"}
	for _, c := range yes {
		if !isTestCommand(c) {
			t.Errorf("isTestCommand(%q) = false, want true", c)
		}
	}
	for _, c := range no {
		if isTestCommand(c) {
			t.Errorf("isTestCommand(%q) = true, want false", c)
		}
	}
}

func TestTestSummary(t *testing.T) {
	cases := map[string]string{
		"Tests: 2 failed, 12 passed, 14 total": "2 failed",
		"14 passed in 0.3s":                    "14 passed",
		"Tests: 0 failed, 14 passed":           "14 passed",
		"ok  github.com/x 0.2s":                "ran",
	}
	for in, want := range cases {
		if got := testSummary(in); got != want {
			t.Errorf("testSummary(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanSubagents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := home + "/.claude/projects/-x/sess1/subagents"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(dir+"/"+name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("agent-aaa.meta.json", `{"toolUseId":"tu1","description":"Map backend login flow","model":"haiku","finished":true}`)
	write("agent-bbb.meta.json", `{"toolUseId":"tu2","description":"Verify login","model":"sonnet","finished":false}`)
	write("agent-bbb.jsonl", `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/x/a.php"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result"}]}}
{"type":"assistant","message":{"content":[{"type":"thinking"},{"type":"tool_use","name":"Bash","input":{"command":"./vendor/bin/pest tests/Feature/Auth\nmore"}}]}}
`)
	d := &dashModel{m: &Meta{MainSession: "sess1"}, subs: map[string]*sub{}, agents: map[string]string{}, byAgt: map[string]string{}}
	// tu1 was seen by hooks as a background launch that never got a stop event.
	d.subs["tu1"] = &sub{text: "Map backend login flow", role: "swarm", fam: "haiku", status: "async_launched"}
	d.order = []string{"tu1"}
	d.scanSubagents()
	if !d.subs["tu1"].done {
		t.Error("tu1: finished in meta.json, want done")
	}
	s := d.subs["tu2"]
	if s == nil || s.done || s.role != "checker" || s.now != "Bash: ./vendor/bin/pest tests/Feature/Auth …" {
		t.Fatalf("tu2 = %+v", s)
	}
}

// TestRenderReal renders a real crew: HC_CREW_REAL=<id> go test -run TestRenderReal -v
func TestRenderReal(t *testing.T) {
	id := os.Getenv("HC_CREW_REAL")
	if id == "" {
		t.Skip("set HC_CREW_REAL")
	}
	m, err := loadMeta(id)
	if err != nil {
		t.Fatal(err)
	}
	d := &dashModel{m: m, terms: statusMsg{}, subs: map[string]*sub{}, agents: map[string]string{}, byAgt: map[string]string{}, cps: map[string]cpState{}}
	d.pull()
	d.scanSubagents()
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	fmt.Println(d.View())
}

func TestFindSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := home + "/.claude/projects/-Users-x-repo"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"summary"}
{"type":"user","cwd":"/Users/x/repo","message":{"role":"user","content":"<command-name>/clear</command-name>"}}
{"type":"user","cwd":"/Users/x/repo","message":{"role":"user","content":"Improve login for superadmin"}}
{"type":"ai-title","aiTitle":"Superadmin login work"}
{"type":"custom-title","customTitle":"Lens"}
{"type":"custom-title","customTitle":"crew-7092-main"}
`
	for _, id := range []string{"0651a620-20f8-48c2-a161-72fe157ca4d1", "0699aaaa-0000-0000-0000-000000000000"} {
		if err := os.WriteFile(dir+"/"+id+".jsonl", []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := findSession("0651")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "0651a620-20f8-48c2-a161-72fe157ca4d1" || s.Cwd != "/Users/x/repo" || s.Title != "Lens" {
		t.Fatalf("got %+v", s)
	}
	if _, err := findSession("06"); err == nil {
		t.Error("ambiguous prefix: want error")
	}
	if _, err := findSession("ffff"); err == nil {
		t.Error("unknown id: want error")
	}
}

func TestIsReadCommand(t *testing.T) {
	cases := map[string]bool{
		"ls -la && cat *.md 2>/dev/null | head -80":             true,
		"cd lens-desktop && git log --oneline | head -5":        true,
		"sed -n 1,110p src/crypto/hybrid.rs; sed -n 1,60p x.rs": true,
		"grep -rn foo src | wc -l":                              true,
		`grep -n "fn \\|pub " src/k.rs | head -40; ls src`:      true,
		`grep -rn 'a|b;c' . && sed -n 1,9p x`:                   true,
		"sed -i '' s/a/b/ x.go":                                 false,
		"cargo build && ls target":                              false,
		"/Users/x/.local/bin/hc-crew consult plan \"q\"":        false,
		"go test ./...": false,
		"":              false,
	}
	for c, want := range cases {
		if got := isReadCommand(c); got != want {
			t.Errorf("isReadCommand(%q) = %v, want %v", c, got, want)
		}
	}
}

func TestNudge(t *testing.T) {
	t.Setenv("HC_CREW_STATE", t.TempDir())
	id := "abcd"
	if err := os.MkdirAll(crewDir(id), 0o755); err != nil {
		t.Fatal(err)
	}
	read := &hookInput{Event: "PostToolUse", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"cat a.go"}`)}
	sub := &hookInput{Event: "PostToolUse", ToolName: "Read", AgentID: "a1"}
	agent := &hookInput{Event: "PreToolUse", ToolName: "Agent"}
	for i := 1; i <= 3; i++ {
		if out := nudge(id, read); out != "" {
			t.Fatalf("call %d nudged early", i)
		}
		if out := nudge(id, sub); out != "" {
			t.Fatal("subagent reads must not count")
		}
	}
	out := nudge(id, read)
	if !strings.Contains(out, `"additionalContext"`) || !strings.Contains(out, "4 reading calls") {
		t.Fatalf("4th read: got %q", out)
	}
	nudge(id, agent)
	for i := 0; i < 3; i++ {
		if nudge(id, read) != "" {
			t.Fatal("streak not reset by Agent call")
		}
	}
}

func TestHCRunTests(t *testing.T) {
	t.Setenv("HC_CREW_STATE", t.TempDir())
	t.Setenv("CREW_ID", "ab12")
	if !isTestCommand("hc run --bg --name crew-ab12-t -- php artisan test --parallel") {
		t.Error("hc run of a test suite not seen as a test")
	}
	if isTestCommand("hc run --bg --name crew-ab12-b -- cargo build") {
		t.Error("hc run of a build seen as a test")
	}
	name, prog, bg := hcRunParts(`cd app && hc run --bg --name crew-ab12-t --cwd /x -- go test ./...`)
	if name != "crew-ab12-t" || prog != "go test ./..." || !bg {
		t.Errorf("hcRunParts = %q %q %v", name, prog, bg)
	}
	if got := hcReadName("hc read --lines 80 crew-ab12-t"); got != "crew-ab12-t" {
		t.Errorf("hcReadName = %q", got)
	}
	start := &hookInput{Event: "PostToolUse", ToolName: "Bash",
		ToolInput: json.RawMessage(`{"command":"hc run --bg --name crew-ab12-t -- pest"}`)}
	ev, ok := hookEvent(start)
	if !ok || ev.Detail != "running" || ev.Key != "crew-ab12-t" {
		t.Fatalf("start event = %+v %v", ev, ok)
	}
	read := &hookInput{Event: "PostToolUse", ToolName: "Bash",
		ToolInput:    json.RawMessage(`{"command":"hc read crew-ab12-t --lines 80"}`),
		ToolResponse: json.RawMessage(`{"stdout":"Tests: 2 failed, 40 passed"}`)}
	ev, ok = hookEvent(read)
	if !ok || ev.Detail != "2 failed" || ev.Text != "pest" {
		t.Fatalf("read event = %+v %v", ev, ok)
	}
	if _, ok = hookEvent(read); ok {
		t.Error("second read of the same terminal logged again")
	}
}

func TestBlockedBanner(t *testing.T) {
	d := &dashModel{m: &Meta{ID: "ab12", MainName: "crew-ab12-main", AdvisorName: "crew-ab12-advisor"},
		terms: statusMsg{"main": {found: true, state: "live", agent: "blocked", msg: "Allow Bash: rm -rf vendor?"},
			"advisor": {found: true, state: "live", agent: "idle"}}}
	b := d.blocked(100)
	if !strings.Contains(b, "main needs you") || !strings.Contains(b, "Allow Bash") || strings.Contains(b, "advisor") {
		t.Errorf("banner = %q", b)
	}
	d.terms["main"] = termInfo{found: true, state: "live", agent: "working"}
	if d.blocked(100) != "" {
		t.Error("banner shown while nobody is blocked")
	}
}

func TestCrewLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "projects", "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "11111111-2222-3333-4444-555555555555"
	write := func(lines ...string) {
		if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &Meta{MainSession: id, Task: "start task"}

	write(`{"type":"custom-title","customTitle":"crew-ab12-main"}`, `{"type":"ai-title","aiTitle":"AI name"}`)
	if got := crewLabel(m); got != "start task" {
		t.Errorf("crew- launch name must not win over the task: got %q", got)
	}
	write(`{"type":"custom-title","customTitle":"crew-ab12-main"}`, `{"type":"custom-title","customTitle":"Old"}`, `{"type":"custom-title","customTitle":"Renamed"}`)
	if got := crewLabel(m); got != "Renamed" {
		t.Errorf("last /rename should win: got %q", got)
	}
	write(`{"type":"ai-title","aiTitle":"AI name"}`)
	if got := crewLabel(&Meta{MainSession: id}); got != "AI name" {
		t.Errorf("no task: want AI title, got %q", got)
	}
	if got := crewLabel(&Meta{Task: "mine"}); got != "mine" {
		t.Errorf("empty session id must not match another session: got %q", got)
	}
	if got := crewLabel(&Meta{MainSession: "99999999-0000-0000-0000-000000000000", Task: "t"}); got != "t" {
		t.Errorf("unsaved session: want task, got %q", got)
	}
}
