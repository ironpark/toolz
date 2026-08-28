# Ready worktree boundary

Treat the `WORKTREE_ROOT`, `PURPOSE`, and `DEVELOPMENT_DIRECTORY` printed by the status command as the worktree context for the entire task. `DEVELOPMENT_DIRECTORY` is the only writable project boundary.

- You may read files outside `DEVELOPMENT_DIRECTORY` when needed for context, but you must not modify them.
- Refuse any request or tool action that would create, edit, delete, move, rename, format, generate, or change permissions on a path outside `DEVELOPMENT_DIRECTORY`.
- This restriction includes other worktrees, parent directories, sibling projects, build output, dependency installation, caches, and generated files when they would be written outside `DEVELOPMENT_DIRECTORY`.
- You do not need to run a command before every file operation. Determine whether each target is within the remembered `DEVELOPMENT_DIRECTORY`.
- If a target's resolved location is unclear—for example because it uses `..`, an absolute path, or a symbolic link—verify it from the directory containing the root `AGENTS.md` with:

  ```sh
  ./scripts/gitwt-ctx check-path "<target-path>"
  ```

- For a new nested path whose immediate parent does not yet exist, first check its nearest existing parent. Every subsequently created component must remain below that approved parent and within `DEVELOPMENT_DIRECTORY`.
- A refused path check is final for that path. Do not bypass it with a different spelling, symbolic link, absolute path, another tool, or a shell working-directory change.
- Commands that can write to multiple or unpredictable locations may be run only when all of their writes are known to remain inside `DEVELOPMENT_DIRECTORY`. Otherwise refuse the command.

Do not run `status` again merely because you inspect or modify another file or change the shell's current directory. Run it again only after a Git worktree move or repair, a metadata change, or another event that makes the remembered boundary unreliable. If the result changes, return to the root `AGENTS.md` and follow the rule for the new status.
