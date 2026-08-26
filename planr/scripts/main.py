#!/usr/bin/env python3
"""Entry point for the planr development scripts.

Two runners live here.  ``scenario`` reproduces the checkout release output and
needs nothing but the standard library; ``codex`` drives a Codex evaluation
and needs the ``openai-codex`` SDK, so it is imported only when that
command is actually requested.
"""

from __future__ import annotations

import sys

from common import HarnessError


USAGE = """\
Usage: python3 planr/scripts/main.py <command> [options]

Commands:
  scenario [clean]      reproduce the checkout release status/overview output
  codex [options]       run an isolated Codex evaluation
  codex analyze <dir>   re-analyze a previous Codex run
  codex clean           remove Codex run directories

The codex command needs the openai-codex SDK, so run it through uv:
  uv run --locked --project planr/scripts python planr/scripts/main.py codex --dry-run

Pass --help to a command for its own options.\
"""


def run(argv: list[str]) -> int:
    if not argv:
        print(USAGE, file=sys.stderr)
        return 2
    command, rest = argv[0], argv[1:]
    if command in {"-h", "--help", "help"}:
        print(USAGE)
        return 0
    if command == "scenario":
        import scenario

        return scenario.main(rest)
    if command == "codex":
        # Imported lazily so `scenario` never pays for the Codex SDK.
        import codex

        return codex.main(rest)
    print(f"planr scripts: unknown command {command!r}", file=sys.stderr)
    print(USAGE, file=sys.stderr)
    return 2


def main(argv: list[str] | None = None) -> int:
    """Single place that turns a runner failure into a message and exit code."""

    try:
        return run(list(sys.argv[1:] if argv is None else argv))
    except HarnessError as exc:
        print(f"planr scripts: {exc}", file=sys.stderr)
        return 2
    except ImportError as exc:
        print(
            f"planr scripts: {exc.name} is not installed; run this command through uv:\n"
            "  uv run --locked --project planr/scripts python planr/scripts/main.py codex ...",
            file=sys.stderr,
        )
        return 2
    except KeyboardInterrupt:
        print("planr scripts: interrupted", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
