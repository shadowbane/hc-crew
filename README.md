# hc-crew

An opt-in "crew run" on top of `hc` (herdr-core): **Fable advises, Opus builds, Sonnet checks, Haiku swarms**, with a live dashboard. Plain `hc` is untouched — nothing here runs unless you call `hc-crew`.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/shadowbane/hc-crew/main/install.sh | sh
```

or with wget: `wget -qO- https://raw.githubusercontent.com/shadowbane/hc-crew/main/install.sh | sh`.

It puts the binary in `~/.local/bin` (set `HC_CREW_BIN_DIR` to change that; `HC_CREW_VERSION=v0.1.0` pins a version). macOS and Linux, arm64 and amd64. Update later with `hc-crew update` (or run the same line again).

Needs:

- [Claude Code](https://claude.com/claude-code) (`claude` on your PATH), logged in
- `hc` (herdr-core) for the crew's terminals; the Herdr app is optional (without it the dashboard runs in your terminal)
- Access to the advisor model. The default is `claude-fable-5-2`; without Fable use `--advisor-model opus`

From source: `go install github.com/shadowbane/hc-crew@latest`.

## Use

```sh
hc-crew start "Move the API to Rust. Done means: every test passes"
```

Inside Herdr this gives the main session a tab and opens the dashboard to its right. Outside Herdr the dashboard runs in the current terminal (`hc attach crew-<id>-main` to drive main).

## What runs

| Role | Where | How |
|---|---|---|
| Advisor (Fable 5.2) | hc terminal `crew-<id>-advisor` | read-only; answers at 3 checkpoints |
| Main (Opus) | hc terminal `crew-<id>-main` | does the work; crew rules appended to its system prompt |
| Checker (Sonnet) | subagent of main | runs tests, tries to break the change |
| Swarm (Haiku) | subagents of main | bulk reading in parallel |

Checkpoints: main runs `hc-crew consult plan|error|done "<question>"`, which sends the question to the advisor (`hc send`) and returns at once. The advisor answers with `hc-crew advise`, which arrives in main as a message ("Advisor: ..."). At plan and done, main waits for it with `hc wait <advisor>`, which also returns as soon as the message lands.

Slow commands (test suites, builds) run in their own hc terminals (`hc run --bg --name crew-<id>-<x> -- <cmd>`, then `hc wait` and `hc read`), so main never sits in a blocked shell. The dashboard picks up test results from the `hc read`.

When main or the advisor is blocked on a permission prompt or a question, the dashboard shows a red "needs you" banner.

## Commands

```
hc-crew start [--cwd DIR] [--no-ui] [--main-model M] [--advisor-model M] "<task>"
hc-crew dash [ID]      hc-crew ls      hc-crew stop ID      hc-crew resume ID
hc-crew start --from SESSION ["<message>"]   # continue a plain Claude session as the crew's main
hc-crew update         hc-crew version
```

## How the dashboard knows things

- Main/advisor status: `hc get <name> --json` every 1.5s (Claude Code reports status as `{app, state, msg}`; `msg` is shown in the main box).
- Subagents, test runs, edits: Claude Code hooks in a settings file passed only to the crew's main session (`--settings`), written to `~/.local/state/hc-crew/<id>/events.jsonl`.

`stop` keeps the crew's state; `resume` restarts both terminals with `claude --resume <saved session id>` and the same hooks/rules (no task re-sent).

## Releases

Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html), and changes are recorded in [CHANGELOG.md](CHANGELOG.md) ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)). To release, move the `[Unreleased]` entries under a new `## [X.Y.Z] - YYYY-MM-DD` heading, update the links at the bottom, commit, and push a tag (`git tag vX.Y.Z && git push origin vX.Y.Z`). The `release` workflow fails if CHANGELOG.md has no section for the tag. Otherwise it runs the tests, builds the four binaries and attaches them, with `checksums.txt`, to a GitHub release. The installer always takes the latest release.

## Tests

```sh
go test ./...                                   # unit + render tests
HC_CREW_RENDER=1 go test -run TestRender -v .   # look at the dashboard
```

Integration tests must use their own hc core: `HC_SESSION=<short>` plus XDG dirs under a short path (macOS socket paths max 103 bytes), and `HC_CREW_CLAUDE=<fake claude script>` to avoid spending tokens.
