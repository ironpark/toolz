# Invalid worktree metadata

Tell the user the `REASON` reported by the status command. Ask the user again for both of the following values:

1. The purpose of this worktree.
2. A valid development directory. It must already exist and must be inside the reported `WORKTREE_ROOT`.

Do not invent, infer, or select either value for the user. After receiving both answers, run this command from the directory containing the root `AGENTS.md`:

```sh
./scripts/gitwt-ctx set --purpose "<user-provided purpose>" --dev-dir "<user-provided path>"
```

The `set` command is the sole permitted write while the status is `INVALID`.

Run `./scripts/gitwt-ctx status` once after setting the values. Return to the root `AGENTS.md` and follow the document selected by the new status. Do not modify project files unless the new status is `READY`.
