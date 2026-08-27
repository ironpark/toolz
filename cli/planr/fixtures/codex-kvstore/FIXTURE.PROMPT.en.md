I need a small key-value store I can drive from shell scripts. This directory
is empty, so set the Go project up yourself. Use `example.com/kv` as the module
name, and building it should produce a `kv` executable.

The commands are just these:

```
kv set <key> <value>
kv get <key>
kv del <key>
kv list
kv compact
```

Everything is stored in one append-only log file, `kv.log` in the current
directory by default, with `--file` to point somewhere else. `set` and `del`
append a record; nothing rewrites the file in place except `compact`.

What I actually need it to survive:

- **Restarts.** Every command is a separate process. What was set stays set.
- **A half-written record.** These are scripts, and they get killed. If the
  last record in the log is truncated or garbage, I want the store to load
  everything before it and carry on, not refuse to start. Losing the interrupted
  write is fine; losing the rest is not.
- **Two scripts writing at once.** If twenty `kv set` calls run in parallel,
  I want twenty keys afterwards. Not nineteen, and not a corrupt file.
- **`compact`.** After a lot of overwrites the log gets big. `compact` should
  rewrite it with only the values that are still live, and if it dies halfway
  through I need the old log intact rather than a half-written one.

The rest of the behaviour:

- `get` on a missing or deleted key isn't a crash — say so on stderr and exit
  with a non-zero code, so `if kv get x; then` works.
- `del` on a missing key is the same: non-zero, message on stderr.
- `list` prints `key<TAB>value`, one per line, sorted by key, and deleted keys
  don't appear. An empty store prints nothing and exits 0.
- Keys and values are plain text on one line. Values may contain spaces; a value
  with a tab or newline in it can be rejected rather than escaped.
- A missing subcommand, an unknown one, or the wrong number of arguments is an
  error with a message, not a silent no-op.

Please write tests for the behaviours above — including the truncated log and
the parallel writes, since those are the ones I'm actually worried about — and
leave it with `go test ./...` passing. Put usage examples in the README, and
say in one or two sentences how the log format and recovery work, so the next
person doesn't have to reverse-engineer it.

I'd rather not pull in outside libraries; the standard library should be plenty.

Work through this on your own without stopping to ask me. Don't stop at a plan
or at a sketch — write the code and leave it in a state where the tests pass.
Keep going until you've checked for yourself that everything above holds, then
give me a short summary of what you did.
