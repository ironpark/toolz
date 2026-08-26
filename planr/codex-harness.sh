#!/usr/bin/env bash

set -euo pipefail

module_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
harness_directory="$module_directory/harness"

if ! command -v uv >/dev/null 2>&1; then
  echo "codex harness: uv is required; install it from https://docs.astral.sh/uv/" >&2
  exit 2
fi

exec uv run --locked --project "$harness_directory" python "$harness_directory/main.py" "$@"
