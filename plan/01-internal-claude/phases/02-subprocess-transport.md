---
completed_at: "2026-08-27T20:20:32Z"
depends_on:
- "01-internal-claude#0"
perf_phase: false
status: done
---
> DONE-WHEN: Unit tests (no real CLI) cover: command-line construction per option, CLI
> NEXT: none

# Transport interface and subprocess CLI transport

## Planned Work

- `transport.go`: `Transport` interface — `Connect(ctx) error`,
  `Write(ctx, data []byte) error`, `ReadMessages() iter.Seq2[json.RawMessage,
  error]` (or a channel-returning equivalent), `EndInput() error`,
  `Close() error`, `Ready() bool` — mirroring `_internal/transport/__init__.py`
  so tests and callers can substitute fakes.
- `subprocess.go`: `newSubprocessTransport(prompt promptSpec, opts *Options)`
  porting `subprocess_cli.py`: CLI discovery (opts.CLIPath, `exec.LookPath
  ("claude")`, common install paths: `~/.claude/local/claude`,
  npm-global bin, etc.), returning `CLINotFoundError` with install hint;
  command construction covering the in-scope flags (`--output-format
  stream-json --input-format stream-json --verbose`, `--print` + prompt for
  one-shot string mode vs streaming stdin mode, `--system-prompt` /
  append variant, `--allowedTools`/`--disallowedTools`, `--mcp-config` (with
  sdk servers listed by name only), `--permission-mode`,
  `--permission-prompt-tool`, `--continue`/`--resume`/`--fork-session`,
  `--session-id`, `--model`/`--fallback-model`, `--settings`,
  `--setting-sources`, `--add-dir`, `--max-turns`, `--agents` JSON,
  `--include-partial-messages`, extra_args passthrough); env merge with
  `CLAUDE_CODE_ENTRYPOINT=sdk-go` and `CLAUDE_AGENT_SDK_VERSION`; cwd.
- Process management with `os/exec` + context: newline-delimited JSON reader
  on stdout with configurable max buffer (default 1MB) returning
  `JSONDecodeError` on overflow/bad JSON; stderr pumped to opts.Stderr
  callback; `Close` kills the process group and reaps; exit-code errors
  surfaced as `ProcessError` including captured stderr tail.

## Done When

- Unit tests (no real CLI) cover: command-line construction per option, CLI
  discovery fallback and `CLINotFoundError`, JSON line splitting including
  oversized-line and partial-line accumulation, and clean shutdown via a stub
  executable (e.g. `cat`-like helper built with `go test` TestMain or a shell
  script); `go test ./internal/claude` passes.
