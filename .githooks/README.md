# Git Hooks

This directory contains git hooks for the project.

## Setup

After cloning the repository, run the install script:

```bash
./.githooks/install.sh
```

Or set up manually:

```bash
git config core.hooksPath .githooks
```

## Pre-push Hook

The `pre-push` hook runs `make validate-all` before push (frontend typecheck and tests). Fix any reported errors before pushing, or bypass with `git push --no-verify` (not recommended).
