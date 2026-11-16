package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Main runs slow commands in their own hc terminals (crew rule 4):
//   hc run --bg --name crew-ab12-test -- php artisan test
//   hc read crew-ab12-test --lines 80
// The hook notes a test terminal when it starts and takes the result from the read.

// hcRunValueFlags are `hc run` options that take a value.
var hcRunValueFlags = map[string]bool{"--name": true, "--cwd": true, "--on": true, "--label": true,
	"--parent": true, "--timeout": true}

// hcRunArgv drops `hc run` options and returns the program's argv.
func hcRunArgv(f []string) []string {
	for i := 0; i < len(f); i++ {
		switch {
		case f[i] == "--":
			return f[i+1:]
		case hcRunValueFlags[f[i]]:
			i++
		case strings.HasPrefix(f[i], "-"):
		default:
			return f[i:]
		}
	}
	return nil
}

// hcRunParts finds an `hc run` in a shell command: its --name, the program
// (joined), and whether it runs in the background.
func hcRunParts(cmd string) (name, prog string, bg bool) {
	for _, seg := range reSegSplit.Split(cmd, -1) {
		f := strings.Fields(seg)
		if len(f) < 2 || f[0] != "hc" || f[1] != "run" {
			continue
		}
		for i := 2; i < len(f); i++ {
			if f[i] == "--" {
				break
			}
			if f[i] == "--bg" {
				bg = true
			}
			if f[i] == "--name" && i+1 < len(f) {
				name = f[i+1]
			} else if strings.HasPrefix(f[i], "--name=") {
				name = strings.TrimPrefix(f[i], "--name=")
			}
		}
		return name, strings.Join(hcRunArgv(f[2:]), " "), bg
	}
	return "", "", false
}

// hcReadName returns the terminal of an `hc read <t>` in a shell command.
func hcReadName(cmd string) string {
	for _, seg := range reSegSplit.Split(cmd, -1) {
		f := strings.Fields(seg)
		if len(f) < 3 || f[0] != "hc" || f[1] != "read" {
			continue
		}
		for i := 2; i < len(f); i++ {
			if f[i] == "--lines" {
				i++
				continue
			}
			if !strings.HasPrefix(f[i], "-") {
				return f[i]
			}
		}
	}
	return ""
}

func hcTestFile(name string) string {
	id := os.Getenv("CREW_ID")
	return filepath.Join(crewDir(id), "hctests", filepath.Base(name))
}

func markHCTest(name, cmd string) {
	p := hcTestFile(name)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(cmd), 0o644)
}

func takeHCTest(name string) (string, bool) {
	p := hcTestFile(name)
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	_ = os.Remove(p)
	return string(b), true
}
