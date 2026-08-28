# Missing worktree metadata

Ask the user for both of the following values:

1. The purpose of this worktree.
2. The development directory. It must already exist and must be inside the reported `WORKTREE_ROOT`.

Do not invent, infer, or select either value for the user. After receiving both answers, run this command from the directory containing the root `AGENTS.md`:

```sh
./scripts/gitwt-ctx set --purpose "<user-provided purpose>" --dev-dir "<user-provided path>"
```

The `set` command is the sole permitted write while the status is `MISSING`. It automatically enables Git's per-worktree configuration when necessary and stores only worktree metadata.

Run `./scripts/gitwt-ctx status` once after setting the values. Return to the root `AGENTS.md` and follow the document selected by the new status. Do not modify project files unless the new status is `READY`.
