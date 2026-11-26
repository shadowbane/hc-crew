package main

import (
	"testing"
	"time"
)

func TestFormatLsRow(t *testing.T) {
	created := time.Date(2026, 11, 26, 10, 27, 0, 0, time.Local)
	head := "80d3  11-26 10:27  main:working   advisor:done      "
	cases := []struct {
		name, cwd, label, auto, want string
	}{
		{"folder and title", "/Users/bob/projects/leapcount", "invoice", "Fix CN/DN rounding on invoice totals",
			head + "invoice\n      ~/projects/leapcount  ·  Fix CN/DN rounding on invoice totals\n"},
		{"title repeats task, cut short", "/Users/bob/x", "fix the login bug", "fix the login bug…",
			head + "fix the login bug\n      ~/x\n"},
		{"task longer than first prompt cut", "/Users/bob/x", "fix  the login\nbug in auth", "fix the login…",
			head + "fix the login bug in auth\n      ~/x\n"},
		{"home itself", "/Users/bob", "a", "b", head + "a\n      ~  ·  b\n"},
		{"sibling of home is not ~", "/Users/bob2/x", "a", "", head + "a\n      /Users/bob2/x\n"},
		{"no folder, no title", "", "a", "", head + "a\n"},
		{"no folder, title only", "", "a", "b", head + "a\n      b\n"},
		{"no label, title shown", "/Users/bob/x", "", "Claude title", head + "\n      ~/x  ·  Claude title\n"},
	}
	for _, c := range cases {
		m := &Meta{ID: "80d3", Created: created, Cwd: c.cwd}
		got := formatLsRow(m, "working", "done", c.label, c.auto, "/Users/bob")
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestSameTitle(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"fix the login bug", "fix the login bug…", true},
		{"fix the login…", "fix the login bug", true},
		{"fix the login bug", "fix the login", false},
		{"fix", "fix the deploy script", false},
		{"deploy", "deployment of X…", false},
		{"…", "anything", false},
		{"invoice", "Fix invoice rounding", false},
		{"", "", true},
		{"a", "", false},
	}
	for _, c := range cases {
		if got := sameTitle(c.a, c.b); got != c.want {
			t.Errorf("sameTitle(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestInjectedOrigin(t *testing.T) {
	for o, want := range map[string]bool{
		"": false, "human": false, "sdk": false, "something-new": false,
		"peer": true, "task_notification": true, "system": true,
	} {
		if got := injectedOrigin(o); got != want {
			t.Errorf("injectedOrigin(%q) = %v, want %v", o, got, want)
		}
	}
}
