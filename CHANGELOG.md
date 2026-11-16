# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[unreleased]: https://github.com/shadowbane/hc-crew/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/shadowbane/hc-crew/releases/tag/v0.1.0
