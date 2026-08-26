Could you build me a to-do CLI? This directory is empty right now, so you'll
have to set the Go project up yourself. Use `example.com/tasks` as the module
name, and building it should produce a `tasks` executable.

Here's how I want to use it:

```
tasks add "buy milk" --due 2026-09-01 --tag home --tag errand
tasks list
tasks list --status all --tag home
tasks list --json
tasks done 1
tasks rm 2
```

Some specifics:

- Store the to-dos in a JSON file. Default it to `tasks.json` in the current
  directory, and it would be nice if `--file` could point somewhere else. If the
  file doesn't exist yet, just create it.
- `id` starts at 1 and counts up, and once a number has been used don't reuse it
  even after that entry is deleted. It would be a problem if I removed one, added
  a new one, and the numbers collided.
- `add` requires a title; `--due` (YYYY-MM-DD) and `--tag` are optional. Let
  `--tag` be given more than once. If the date format is off, don't quietly move
  on — report it as an error.
- Options have to work **after the title**, like in the examples above. If
  `tasks add "buy milk" --tag home` doesn't work it won't feel natural and I
  won't use it. Same for `--file` — it has to be allowed after the subcommand.
- `list` shows only unfinished ones by default. `--status open|done|all` picks
  which, and `--tag` narrows it to entries carrying that tag. If there's nothing
  to show, print a readable message rather than an empty screen. That isn't an
  error, so the exit code should be 0.
- `list --json` gets consumed by other scripts, so it has to be one JSON array
  and nothing else. Each entry needs `id` (a number), `title`, `status` (`open`
  or `done`), `tags` (an array) and `due` (a string, empty when unset). Don't mix
  in any text meant for a human to read.
- `done` and `rm` take an id. If the id doesn't exist, don't silently succeed —
  say what went wrong on stderr and exit with a non-zero code. Same for a
  subcommand it doesn't recognize.
- I'd like tests covering each command and the error cases too. Leave it with
  `go test ./...` passing.
- Put installation and usage examples for the commands above in the README.

I'd rather not pull in outside libraries. The standard library should be plenty.

Please work through this on your own without stopping to ask me. Don't stop at
a plan or at "here's how you'd do it" — actually write the code and leave it in
a state where the tests pass. Keep going until you've confirmed for yourself
that everything above is handled, and when you're done give me a short summary
of what you did.
