# Shellman

[![CI Status](https://github.com/thecybersailor/shellman/actions/workflows/pr-ci.yml/badge.svg)](https://github.com/thecybersailor/shellman/actions/workflows/pr-ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/thecybersailor/shellman)](https://goreportcard.com/report/github.com/thecybersailor/shellman)
[![codecov](https://codecov.io/gh/thecybersailor/shellman/branch/main/graph/badge.svg)](https://codecov.io/gh/thecybersailor/shellman)
[![Go Reference](https://pkg.go.dev/badge/github.com/thecybersailor/shellman.svg)](https://pkg.go.dev/github.com/thecybersailor/shellman)
[![GitHub release](https://img.shields.io/github/release/thecybersailor/shellman.svg)](https://github.com/thecybersailor/shellman/releases)
[![GitHub issues](https://img.shields.io/github/issues/thecybersailor/shellman.svg)](https://github.com/thecybersailor/shellman/issues)

Your AI coding sidecar.

Organizing every agent thread into a task tree, summarizing progress, and guiding (or automating) the next steps.

## Install

```bash
npm install -g shellman
```

## What is Shellman?

Shellman is an AI coding sidecar that turns scattered agent conversations into a structured execution system.

Build order from chaos with a `TODO Tree + Sidecar Modes`.

## Core Capabilities

### TODO Tree

Parallel agent work fails first at visibility. TODO Tree restores control with one operational view: parent-child structure, runtime state, flags, and priority in a single scan.

It turns the left sidebar from a passive list into an execution surface, so your next action is always obvious.

### Sidecar Modes

- `Advisor`: use it during planning or risky edits when you want ideas and review, but keep every terminal action under your explicit control.
- `Observer`: use it when many threads run in parallel and you need Task Tree to stay trustworthy without constantly checking each terminal.
- `Autopilot`: use it for execution-heavy delivery windows where routine steps should run continuously until done, with automatic progress reporting back to the team.

### Browser Access

Access your workspace from any browser, not just a fixed local setup.

Continue task coordination from anywhere on PC or mobile, with the same operational context.

### Best Practices

Align team behavior with `AGENTS-SIDECAR.md`.

Use the file as your operational contract for sidecar collaboration: role boundaries, control-plane discipline, and coordination rules across Codex processes.

### Skill Index Context

- Task-agent prompts now separate stable and runtime context:
  - `system_context_json`: static rules, sidecar context docs, `skills_index`
  - `event_context_json`: current turn signal and timeline snapshot
- Skill discovery uses two base paths with project override:
  - `~/.config/shellman/skills`
  - `<repo>/.shellman/skills`
- Prompt injection is metadata-only (`name/description/path/source`); `SKILL.md` body is loaded lazily through `readfile`.
