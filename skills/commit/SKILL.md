---
name: commit
description: Review, stage, and create a precise Git commit when the user explicitly asks to commit changes.
---

# Commit

Create a Git commit that accurately represents the intended change while
preserving the user's existing staging decisions.

## Scope and staging

- Inspect `git status` before staging. If there are staged changes, commit only
  that staged scope unless the user explicitly asks to change it.
- If nothing is staged, stage only working-tree changes that belong to the
  requested commit. Do not include unrelated files, generated output, caches,
  logs, editor files, or potential secrets.
- Before staging untracked files, check whether any should instead be ignored.
  Do not edit `.gitignore` without a request; leave suspicious files unstaged
  and identify the paths and suggested ignore pattern to the user.
- Review the final staged scope with `git status` and `git diff --cached`.

## Commit message

- Follow the repository's established commit-message convention when one is
  evident from recent non-merge commits.
- Otherwise use a concise, imperative, capitalized subject without a trailing
  period; target 50 characters and avoid exceeding 72 where practical.
- Base every claim on the staged diff and relevant user-provided context. Do not
  invent issues, motivations, behavior changes, performance claims, or ticket
  numbers.
- Prefer a subject-only message. Add a body only for durable context that the
  diff cannot convey, such as an established constraint, trade-off, or
  compatibility implication. Wrap body prose near 72 characters.

## Completion

Create the commit only after verifying the staged diff. Report the resulting
commit subject and any intentionally uncommitted or suspicious files requiring
user attention.
