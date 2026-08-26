#!/usr/bin/env bash

set -euo pipefail

module_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fixture_directory="$module_directory/test/fixtures"

if [[ "${1:-}" == "clean" ]]; then
  find "$module_directory/test" -maxdepth 1 -type d -name 'work.*' -print0 |
    while IFS= read -r -d '' work_directory; do
      rm -rf "$work_directory"
    done
  echo "Removed test workspaces"
  exit 0
fi

if [[ -n "${1:-}" ]]; then
  echo "Usage: $0 [clean]" >&2
  exit 2
fi

work_directory="$(mktemp -d "$module_directory/test/work.XXX")"

cp "$fixture_directory/.planr.yaml" "$work_directory/.planr.yaml"
cp "$fixture_directory/checkout-v2.md" "$work_directory/checkout-v2.md"

(
  cd "$module_directory"
  go build -o "$work_directory/planr" .
)

(
  cd "$work_directory"
  ./planr add --name auth-foundation checkout-v2.md
  ./planr add checkout-v2.md
  ./planr add --name payment-adapter checkout-v2.md
  ./planr add --name legacy-report checkout-v2.md
  ./planr add --name partial-rollout checkout-v2.md
)

perl -0pi -e 's/plan_status: in-progress/plan_status: done/' "$work_directory/plans-active/00-auth-foundation/PLAN.md" "$work_directory/plans-active/03-legacy-report/PLAN.md"
perl -0pi -e 's/status: (planned|conditional)/status: done/' "$work_directory/plans-active/00-auth-foundation/phases/"*.md "$work_directory/plans-active/03-legacy-report/phases/"*.md
perl -0pi -e 's/status: planned/status: done/' "$work_directory/plans-active/04-partial-rollout/phases/00-api-contract.md"
perl -0pi -e 's/depends_on: \[\]/depends_on:\n- "00-auth-foundation#2"/' "$work_directory/plans-active/01-checkout-v2/phases/00-api-contract.md"
perl -0pi -e 's/depends_on: \[\]/depends_on:\n- "01-checkout-v2#1"/' "$work_directory/plans-active/02-payment-adapter/phases/00-api-contract.md"

printf 'Test workspace: %s\n\n' "$work_directory"
(
  cd "$work_directory"
  printf '%s\n' 'Detailed status:'
  ./planr status
  printf '\n%s\n' 'Overview:'
  ./planr overview
)
