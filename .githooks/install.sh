#!/bin/bash

# Install git hooks for this repository
# This script sets up the git hooks path to use .githooks directory

HOOKS_DIR=".githooks"
GIT_HOOKS_PATH=$(git config core.hooksPath)

if [ "$GIT_HOOKS_PATH" = "$HOOKS_DIR" ]; then
    echo "✓ Git hooks are already configured"
    exit 0
fi

echo "Setting up git hooks..."
git config core.hooksPath "$HOOKS_DIR"

if [ $? -eq 0 ]; then
    echo "✓ Git hooks configured successfully"
    echo "  Hooks path: $HOOKS_DIR"
else
    echo "✗ Failed to configure git hooks"
    exit 1
fi
