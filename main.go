// hc-crew: an opt-in "crew run" on top of hc (herdr-core).
//
// A crew is an Opus main session plus a read-only Fable advisor, each in its
// own hc terminal labelled crew=<id>, and a live dashboard. Plain hc usage is
// untouched: nothing here runs unless hc-crew is invoked.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// version is set at release time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

const installURL = "https://raw.githubusercontent.com/shadowbane/hc-crew/main/install.sh"

const usage = `hc-crew - run a crew (Fable advises, Opus builds, Sonnet checks, Haiku swarms)

Usage:
  hc-crew start [--cwd DIR] [--no-ui] ['<task>'] start a crew (task optional) and open its dashboard
  hc-crew start --from SESSION ['<message>']     continue a Claude session as the crew's main
  hc-crew dash [ID]                              dashboard for a crew (default: newest)
  hc-crew ls                                     list crews with their folder and title
  hc-crew stop ID                                end a crew's terminals (state is kept)
  hc-crew resume [--no-ui] ID                    restart a stopped crew on its saved sessions
  hc-crew update                                 install the latest release over this binary
  hc-crew version                                print the version

Used by the crew itself:
  hc-crew consult plan|error|done "<question>"   main asks the advisor, waits for the reply
  hc-crew advise "<answer>"                      advisor replies (or pipe the answer on stdin)
  hc-crew hook                                   Claude Code hook handler (reads stdin)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	args := os.Args[2:]
	switch os.Args[1] {
	case "start":
		err = cmdStart(args)
	case "dash":
		err = cmdDash(args)
	case "ls":
		err = cmdLs()
	case "stop":
		err = cmdStop(args)
	case "resume":
		err = cmdResume(args)
	case "consult":
		err = cmdConsult(args)
	case "advise":
		err = cmdAdvise(args)
	case "hook":
		cmdHook() // never fails: a broken hook must not disturb Claude Code
	case "version", "--version", "-v":
		fmt.Println("hc-crew", version)
	case "update":
		err = cmdUpdate()
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "hc-crew: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hc-crew:", err)
		os.Exit(1)
	}
}

// cmdUpdate re-runs the installer, which replaces this binary with the latest release.
func cmdUpdate() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(self)
	c := exec.Command("sh", "-c", "curl -fsSL "+installURL+" | sh")
	c.Env = append(os.Environ(), "HC_CREW_BIN_DIR="+dir)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}
