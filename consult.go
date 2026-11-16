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
// to the advisor and waits for the advisor's reply (written by cmdAdvise),
// printing it so the main session gets it as the Bash tool result.
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
	timeout := 540 * time.Second // under the Bash tool's 600s maximum

	dir := crewDir(m.ID)
	replyFile := filepath.Join(dir, "reply.txt")
	waitFile := filepath.Join(dir, "waiting")
	_ = os.Remove(replyFile)
	_ = os.WriteFile(waitFile, []byte(cp), 0o644)
	defer os.Remove(waitFile)

	_ = appendEvent(m.ID, Event{Kind: "consult", Role: cp, Text: q})

	msg := fmt.Sprintf("[crew checkpoint: %s]\n%s\n\nMain session transcript (JSONL, read the tail if you need context): %s\nReply with: %s advise <<'EOF'\n<your advice>\nEOF",
		cp, q, transcriptPath(m.MainSession), selfPath())
	if out, err := exec.Command("hc", "send", m.AdvisorName, msg).CombinedOutput(); err != nil {
		return fmt.Errorf("could not reach advisor %s: %v: %s", m.AdvisorName, err, strings.TrimSpace(string(out)))
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(replyFile); err == nil {
			_ = os.Remove(replyFile)
			fmt.Printf("Advisor (%s checkpoint):\n%s\n", cp, strings.TrimSpace(string(b)))
			return nil
		}
		time.Sleep(time.Second)
	}
	fmt.Println("The advisor is still thinking. Carry on; its reply will arrive as a message.")
	return nil
}

// cmdAdvise is run by the advisor. If main is waiting in consult, the reply is
// handed over through a file; otherwise it is sent to main as a message.
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

	dir := crewDir(m.ID)
	if _, err := os.Stat(filepath.Join(dir, "waiting")); err == nil {
		tmp := filepath.Join(dir, "reply.tmp")
		if err := os.WriteFile(tmp, []byte(a), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(dir, "reply.txt")); err != nil {
			return err
		}
		fmt.Println("Delivered to main.")
		return nil
	}
	if out, err := exec.Command("hc", "send", m.MainName, "Advisor: "+a).CombinedOutput(); err != nil {
		return fmt.Errorf("could not reach main %s: %v: %s", m.MainName, err, strings.TrimSpace(string(out)))
	}
	fmt.Println("Sent to main as a message.")
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
