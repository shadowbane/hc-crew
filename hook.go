package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// hookInput is the subset of the Claude Code hook payload we use.
// Shapes verified against Claude Code 2.1.325.
type hookInput struct {
	Event        string          `json:"hook_event_name"`
	ToolName     string          `json:"tool_name"`
	ToolUseID    string          `json:"tool_use_id"`
	AgentID      string          `json:"agent_id"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
}

// cmdHook logs crew-relevant tool calls. It is silent and always exits 0.
func cmdHook() {
	defer func() { _ = recover() }()
	id := os.Getenv("CREW_ID")
	if id == "" {
		return
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}
	var in hookInput
	if json.Unmarshal(raw, &in) != nil {
		return
	}
	if ev, ok := hookEvent(&in); ok {
		_ = appendEvent(id, ev)
	}
	if out := nudge(id, &in); out != "" {
		fmt.Print(out)
	}
}

var (
	reFailed  = regexp.MustCompile(`(?i)(\d+)\s+(failed|failing|failures?|errors?)\b`)
	rePassed  = regexp.MustCompile(`(?i)(\d+)\s+(passed|passing|tests? passed|ok)\b`)
	reCrewCmd = regexp.MustCompile(`hc-crew\s+(consult|advise)`)
)

func hookEvent(in *hookInput) (Event, bool) {
	switch {
	case in.Event == "SubagentStop" && in.AgentID != "":
		// Completion of a subagent; the only completion signal for background ones.
		return Event{Kind: "sub_stop", Agent: in.AgentID}, true

	case in.ToolName == "Agent" || in.ToolName == "Task":
		var ti struct {
			Description  string `json:"description"`
			SubagentType string `json:"subagent_type"`
			Model        string `json:"model"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		model := ti.Model
		if model == "" {
			model = ti.SubagentType
		}
		if in.Event == "PreToolUse" {
			return Event{Kind: "sub_start", Key: in.ToolUseID, Role: roleFor(model), Model: model, Text: ti.Description}, true
		}
		if in.Event == "PostToolUse" {
			var tr struct {
				Status        string `json:"status"`
				AgentID       string `json:"agentId"`
				ResolvedModel string `json:"resolvedModel"`
				TotalTokens   int    `json:"totalTokens"`
			}
			_ = json.Unmarshal(in.ToolResponse, &tr)
			if tr.ResolvedModel != "" {
				model = tr.ResolvedModel
			}
			return Event{Kind: "sub_done", Key: in.ToolUseID, Agent: tr.AgentID, Role: roleFor(model), Model: model,
				Text: ti.Description, Detail: tr.Status}, true
		}

	case in.ToolName == "Bash" && in.Event == "PostToolUse":
		var ti struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		if reCrewCmd.MatchString(ti.Command) || !isTestCommand(ti.Command) {
			return Event{}, false // consult/advise log themselves; only tests are worth a feed line
		}
		var tr struct {
			Stdout string `json:"stdout"`
			Stderr string `json:"stderr"`
		}
		_ = json.Unmarshal(in.ToolResponse, &tr)
		return Event{Kind: "bash", Agent: in.AgentID, Text: firstLine(ti.Command), Detail: testSummary(tr.Stdout + "\n" + tr.Stderr)}, true

	case (in.ToolName == "Edit" || in.ToolName == "Write" || in.ToolName == "MultiEdit") && in.Event == "PostToolUse":
		var ti struct {
			FilePath  string `json:"file_path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
			Content   string `json:"content"`
			Edits     []struct {
				OldString string `json:"old_string"`
				NewString string `json:"new_string"`
			} `json:"edits"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		add, del := lines(ti.NewString)+lines(ti.Content), lines(ti.OldString)
		for _, e := range ti.Edits {
			add += lines(e.NewString)
			del += lines(e.OldString)
		}
		return Event{Kind: "edit", Agent: in.AgentID, Text: shortPath(ti.FilePath), Detail: fmt.Sprintf("+%d -%d", add, del)}, true
	}
	return Event{}, false
}

// roleFor maps a model name to its crew role.
func roleFor(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "haiku"):
		return "swarm"
	case strings.Contains(m, "sonnet"):
		return "checker"
	default:
		return "helper"
	}
}

// testSummary pulls "N failed" / "N passed" out of test runner output.
func testSummary(out string) string {
	f := lastMatch(reFailed, out)
	p := lastMatch(rePassed, out)
	switch {
	case f != "" && f != "0":
		return f + " failed"
	case p != "":
		return p + " passed"
	}
	return "ran"
}

func lastMatch(re *regexp.Regexp, s string) string {
	all := re.FindAllStringSubmatch(s, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1][1]
}

func lines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimRight(s, "\n"), "\n") + 1
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " …"
	}
	return s
}

func shortPath(p string) string {
	if wd, err := os.Getwd(); err == nil && strings.HasPrefix(p, wd+"/") {
		return strings.TrimPrefix(p, wd+"/")
	}
	return p
}

var (
	reSegSplit  = regexp.MustCompile(`&&|\|\||[;|\n]`)
	reEnvAssign = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=\S*$`)
	testRunners = map[string]bool{"pest": true, "phpunit": true, "paratest": true, "pytest": true, "jest": true,
		"vitest": true, "rspec": true, "ctest": true, "mocha": true, "playwright": true}
	testVerbs = map[string]bool{"go": true, "cargo": true, "npm": true, "pnpm": true, "yarn": true, "bun": true,
		"composer": true, "make": true, "mix": true, "dotnet": true, "deno": true}
)

// isTestCommand reports whether any segment of a shell command runs a test
// runner (pest, go test, npm run test, php artisan test, ...). A command that
// merely mentions a tests/ path, such as sed or grep on a test file, is not one.
func isTestCommand(cmd string) bool {
	for _, seg := range reSegSplit.Split(cmd, -1) {
		f := strings.Fields(seg)
		for len(f) > 0 && (reEnvAssign.MatchString(f[0]) || f[0] == "rtk" || f[0] == "npx" || f[0] == "env" ||
			f[0] == "time" || f[0] == "herd" || f[0] == "(") {
			f = f[1:]
		}
		if len(f) == 0 {
			continue
		}
		bin := f[0][strings.LastIndex(f[0], "/")+1:]
		switch {
		case testRunners[bin]:
			return true
		case bin == "php" && len(f) > 2 && f[1] == "artisan" && f[2] == "test":
			return true
		case testVerbs[bin] && len(f) > 1 && (f[1] == "test" || (f[1] == "run" && len(f) > 2 && strings.HasPrefix(f[2], "test"))):
			return true
		}
	}
	return false
}
