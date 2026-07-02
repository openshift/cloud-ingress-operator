#!/usr/bin/env bash
set -euo pipefail

# Ensure we're running from the repository root
if ! REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null); then
  echo "Error: Not in a git repository" >&2
  exit 1
fi
cd "$REPO_ROOT" || exit 1

if ! command -v prek &>/dev/null; then
  echo "Error: prek is not installed. Install with: uv tool install prek" >&2
  exit 1
fi

prek run --config hack/prek.ci.toml --all-files
