# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.8] - 2026-11-18

### Fixed

- Crew terminals start through your login shell (`$SHELL -l`), so they get the PATH from `~/.zprofile` or `~/.bash_profile`. hc ran them with the environment its server inherited from the Herdr app, which could miss tools like `npx`; then Claude hooks and MCP servers that need them failed ("SessionStart:startup hook failed"), and restarting the hc server did not help. Tools set up only in `~/.zshrc` are still not seen; put their PATH in a login file.

## [0.1.7] - 2026-11-18

### Added

- `hc-crew start --main-effort LEVEL` sets main's effort (low, medium, high, xhigh or max). An unknown level is rejected before anything starts.

### Changed

- Main always runs at high effort unless you pass `--main-effort`, instead of following the effort level in your own Claude settings. If you set xhigh or max globally, pass it here. Existing crews switch to high the next time they are resumed. The advisor is unchanged.

## [0.1.6] - 2026-11-17

### Changed

- `hc-crew ls` and the dashboard header show the name you give main with `/rename`. The order is: your latest `/rename`, then the task you started the crew with, then Claude's own title for the session. A crew started with `--from` keeps the source session's name until you rename it. The dashboard picks up a rename within about 10 seconds, and it reads the transcript in the background, so a long session doesn't freeze it.

## [0.1.5] - 2026-11-17

### Changed

- The README and `hc-crew help` show the task as optional and quote it with single quotes. With double quotes, the shell runs anything in backticks before hc-crew sees it, so a task such as "look at `routes/api.php`" reached main as "look at".

## [0.1.4] - 2026-11-17

### Changed

- The advisor starts with Remote Control off, even when `remoteControlAtStartup` is on in your Claude settings, so it no longer clutters the session list on your phone. Main keeps your setting.

## [0.1.3] - 2026-11-16

### Changed

- `hc-crew resume` sends main a short first message saying the crew rules were updated and replace any it remembers. A resumed conversation kept quoting the old rules from memory.

## [0.1.2] - 2026-11-16

### Fixed

- `hc-crew resume` no longer fails for an advisor that was never consulted. Claude Code had not saved that session, so `claude --resume` exited with "No conversation found"; the advisor now starts a fresh session with the same id.

## [0.1.1] - 2026-11-16

### Added

- Main runs slow commands (test suites, builds, installs) in their own hc terminals with `hc run --bg`, then `hc wait` and `hc read`, so its shell is never blocked. The dashboard counts test runs started this way and shows their pass/fail result.
- The dashboard shows a red "needs you" banner when main or the advisor is blocked on a permission prompt or a question, or has stopped on an error.
- Main's crew settings allow `hc wait`, `hc read`, and `hc kill`/`hc rm` on the crew's own terminals without a prompt.

### Changed

- `hc-crew consult` returns at once instead of waiting up to 9 minutes for the advisor. The advisor's answer always arrives in main as an "Advisor: ..." message. At the plan and done checkpoints, main waits for it with `hc wait`, which also returns as soon as the message lands.

### Removed

- The `waiting` and `reply.txt` hand-over files in the crew's state folder.

### Fixed

- The render test no longer writes a `demo` crew into the real state folder when run inside a crew session.

## [0.1.0] - 2026-11-16

### Added

- `hc-crew start` launches a crew. It has an Opus main session and a read-only Fable advisor, each in its own hc terminal labelled `crew=<id>`, plus a live dashboard. Inside Herdr, main gets a tab with the dashboard to its right.
- `hc-crew start --from SESSION` continues an existing Claude session as the crew's main. The task name comes from that session's title, and the goal is optional.
- `hc-crew stop` and `hc-crew resume` end a crew's terminals and restart them on the saved sessions.
- `hc-crew dash` and `hc-crew ls` show one crew's dashboard and list all crews.
- Advisor checkpoints: `hc-crew consult plan|error|done` and `hc-crew advise`.
- Claude Code hooks feed the dashboard with subagents (Sonnet checker, Haiku swarm), test runs and edits. Subagent state is read from Claude Code's own subagent files.
- A nudge reminds main to hand file reading to the Haiku swarm after 4 reading calls in a row.
- Herdr steps are logged to `herdr.log` in the crew's state folder, and the dashboard split is retried.
- `hc-crew version` and `hc-crew update`.
- A one-line installer and updater (`install.sh`) for curl or wget, which verifies checksums. A tag-triggered release workflow builds macOS and Linux binaries for arm64 and amd64.

[unreleased]: https://github.com/shadowbane/hc-crew/compare/v0.1.8...HEAD
[0.1.8]: https://github.com/shadowbane/hc-crew/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/shadowbane/hc-crew/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/shadowbane/hc-crew/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/shadowbane/hc-crew/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/shadowbane/hc-crew/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/shadowbane/hc-crew/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/shadowbane/hc-crew/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/shadowbane/hc-crew/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/shadowbane/hc-crew/releases/tag/v0.1.0
