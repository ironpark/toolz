#!/usr/bin/env bash
# Runs after the agent stops, from a scratch directory outside the workspace.
# $MOHAE_WORKSPACE points at the finished workspace.
#
# The exit status is the verdict: zero passes, anything else fails. Print
# whatever helps a human read the result — mohae records the output verbatim
# and imposes no format on it.

set -uo pipefail

workspace="${MOHAE_WORKSPACE:?MOHAE_WORKSPACE is not set}"

if [ ! -f "$workspace/README.md" ]; then
  echo "README.md is missing" >&2
  exit 1
fi
