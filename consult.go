package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var checkpoints = []string{"plan", "error", "done"}

// cmdConsult is run by the main session at a checkpoint. It sends the question
// to the advisor and returns at once: the advisor's reply arrives in main as a
// message ("Advisor: ..."), so main never sits in a blocked shell.
func cmdConsult(args []string) error {
	m, err := envCrew()
	if err != nil {
		return err
	}
	if len(args) < 1 || !contains(checkpoints, args[0]) {
		return fmt.Errorf("usage: hc-crew consult %s \"<question>\"", strings.Join(checkpoints, "|"))
	}
	cp := args[0]
	q := strings.TrimSpace(strings.Join(args[1:], " "))
	if q == "" {
		b, _ := io.ReadAll(os.Stdin)
		q = strings.TrimSpace(string(b))
	}
	if q == "" {
		return errors.New("empty question")
	}
	_ = appendEvent(m.ID, Event{Kind: "consult", Role: cp, Text: q})

	msg := fmt.Sprintf("[crew checkpoint: %s]\n%s\n\nMain session transcript (JSONL, read the tail if you need context): %s\nReply with: %s advise <<'EOF'\n<your advice>\nEOF",
		cp, q, transcriptPath(m.MainSession), selfPath())
	if out, err := exec.Command("hc", "send", m.AdvisorName, msg).CombinedOutput(); err != nil {
		return fmt.Errorf("could not reach advisor %s: %v: %s", m.AdvisorName, err, strings.TrimSpace(string(out)))
	}
	// hc wait on the advisor returns at once while it is still idle; let it pick the question up first.
	for i := 0; i < 20 && hcGet(m.AdvisorName).agent != "working"; i++ {
		time.Sleep(500 * time.Millisecond)
	}
	wait := fmt.Sprintf("hc wait %s --timeout 100", m.AdvisorName)
	switch cp {
	case "plan":
		fmt.Printf("Asked the advisor. Its reply arrives as a message starting with \"Advisor:\".\nUntil then you may read and prepare, but do not start edits. To wait, run: %s (repeat on timeout; it also returns when the message arrives).\n", wait)
	case "done":
		fmt.Printf("Asked the advisor. Its reply arrives as a message starting with \"Advisor:\".\nDo not call the task done before you have it. To wait, run: %s (repeat on timeout; it also returns when the message arrives).\n", wait)
	default:
		fmt.Println("Asked the advisor. Carry on; its reply arrives as a message starting with \"Advisor:\".")
	}
	return nil
}

// cmdAdvise is run by the advisor: it logs the advice and sends it to main.
func cmdAdvise(args []string) error {
	m, err := envCrew()
	if err != nil {
		return err
	}
	a := strings.TrimSpace(strings.Join(args, " "))
	if a == "" {
		b, _ := io.ReadAll(os.Stdin)
		a = strings.TrimSpace(string(b))
	}
	if a == "" {
		return errors.New("empty advice")
	}
	_ = appendEvent(m.ID, Event{Kind: "advice", Text: a})
	if out, err := exec.Command("hc", "send", m.MainName, "Advisor: "+a).CombinedOutput(); err != nil {
		return fmt.Errorf("could not reach main %s: %v: %s", m.MainName, err, strings.TrimSpace(string(out)))
	}
	fmt.Println("Sent to main.")
	return nil
}

// transcriptPath finds a Claude Code session transcript by session id.
func transcriptPath(session string) string {
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", session+".jsonl"))
	if len(matches) > 0 {
		return matches[0]
	}
	return "(not written yet)"
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "hc-crew"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
