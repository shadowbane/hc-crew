package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Main tends to explore the repo itself instead of handing reading to the
// Haiku swarm. The hook counts main's own reading calls (Read, Grep, Glob and
// read-only Bash such as cat/sed/grep) since its last subagent or edit, and
// every nudgeEvery of them injects a reminder into main's context.

const nudgeEvery = 4

const nudgeText = `hc-crew: you have made %d reading calls yourself in a row. Crew rule 3: hand exploration to the Haiku swarm. Spawn 2-4 Agent subagents in parallel (subagent_type "Explore", model "haiku"), each with one narrow question, and work from their answers. Read files yourself only right before you edit them.`

var reQuoted = regexp.MustCompile(`'[^']*'|"(?:[^"\\]|\\.)*"`)

var readOnlyBins = map[string]bool{"cat": true, "sed": true, "head": true, "tail": true, "grep": true, "rg": true,
	"find": true, "fd": true, "ls": true, "tree": true, "wc": true, "awk": true, "jq": true, "less": true,
	"cd": true, "echo": true, "file": true, "stat": true, "nl": true, "sort": true, "uniq": true, "cut": true}

// isReadCommand reports whether every segment of a shell command only reads.
func isReadCommand(cmd string) bool {
	if reCrewCmd.MatchString(cmd) {
		return false
	}
	seen := false
	cmd = reQuoted.ReplaceAllString(cmd, "Q") // |, ; and && inside quotes are not separators
	for _, seg := range reSegSplit.Split(cmd, -1) {
		f := strings.Fields(seg)
		for len(f) > 0 && (reEnvAssign.MatchString(f[0]) || f[0] == "rtk" || f[0] == "(") {
			f = f[1:]
		}
		if len(f) == 0 {
			continue
		}
		bin := f[0][strings.LastIndex(f[0], "/")+1:]
		if bin == "git" && len(f) > 1 && (f[1] == "log" || f[1] == "show" || f[1] == "status" || f[1] == "diff" || f[1] == "remote") {
			seen = true
			continue
		}
		if !readOnlyBins[bin] || (bin == "sed" && strings.Contains(seg, " -i")) {
			return false
		}
		seen = true
	}
	return seen
}

// nudge updates main's reading streak and returns the hook's stdout, if any.
func nudge(id string, in *hookInput) string {
	if in.AgentID != "" {
		return "" // a subagent's own calls: reading is its job
	}
	p := filepath.Join(crewDir(id), "reads")
	switch {
	case in.ToolName == "Agent" || in.ToolName == "Task" || in.ToolName == "Edit" || in.ToolName == "Write" || in.ToolName == "MultiEdit":
		_ = os.WriteFile(p, []byte("0"), 0o644)
		return ""
	case in.Event != "PostToolUse":
		return ""
	case in.ToolName == "Read" || in.ToolName == "Grep" || in.ToolName == "Glob":
	case in.ToolName == "Bash":
		var ti struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		if !isReadCommand(ti.Command) {
			return ""
		}
	default:
		return ""
	}
	b, _ := os.ReadFile(p)
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	n++
	_ = os.WriteFile(p, []byte(strconv.Itoa(n)), 0o644)
	if n%nudgeEvery != 0 {
		return ""
	}
	_ = appendEvent(id, Event{Kind: "nudge", Text: fmt.Sprintf("main read %d times itself; told it to use the swarm", n)})
	out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":     "PostToolUse",
		"additionalContext": fmt.Sprintf(nudgeText, n),
	}})
	return string(out)
}
