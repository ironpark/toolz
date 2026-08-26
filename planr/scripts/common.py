"""Shared plumbing for the planr test runners.

Both runners build the ``planr`` binary, copy a fixture from ``planr/fixtures``
into a throwaway directory under ``planr/test`` and drive the CLI there.  This
module holds the parts they have in common; it deliberately depends only on the
standard library so the plan scenario runs without the Codex SDK installed.
"""

from __future__ import annotations

import pathlib
import shutil
import subprocess
import tempfile


MODULE_DIR = pathlib.Path(__file__).resolve().parents[1]
FIXTURES_DIR = MODULE_DIR / "fixtures"
RUN_ROOT = MODULE_DIR / "test"


class HarnessError(RuntimeError):
    """A user-facing harness configuration or setup error."""


def require_command(name: str) -> None:
    if shutil.which(name) is None:
        raise HarnessError(f"command not found: {name}")


def run_command(
    args: list[str], *, cwd: pathlib.Path | None = None, env: dict[str, str] | None = None
) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            args,
            cwd=str(cwd) if cwd else None,
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
    except OSError as exc:
        return subprocess.CompletedProcess(args, 127, str(exc))


def fixture_dir(name: str) -> pathlib.Path:
    """Resolve a fixture directory under fixtures/.

    fixtures/MANIFEST.yaml documents what each one is for, but it is not read
    here: the plan scenario must run on a bare interpreter and there is no YAML
    parser in the standard library.
    """

    path = FIXTURES_DIR / name
    if not path.is_dir():
        raise HarnessError(f"missing fixture: {path}")
    return path


def make_run_dir(prefix: str) -> pathlib.Path:
    """Create a throwaway run directory under test/ for one runner invocation."""

    RUN_ROOT.mkdir(parents=True, exist_ok=True)
    return pathlib.Path(tempfile.mkdtemp(prefix=prefix, dir=RUN_ROOT))


def build_planr(destination: pathlib.Path) -> None:
    """Build the planr binary under test into destination."""

    require_command("go")
    build = run_command(["go", "build", "-o", str(destination), "."], cwd=MODULE_DIR)
    if build.returncode != 0:
        raise HarnessError(f"could not build planr (exit {build.returncode}):\n{build.stdout}")


def remove_runs(prefix: str) -> int:
    """Delete every throwaway run directory with the given name prefix."""

    RUN_ROOT.mkdir(parents=True, exist_ok=True)
    removed = 0
    for path in RUN_ROOT.glob(f"{prefix}*"):
        if path.is_dir():
            shutil.rmtree(path)
            removed += 1
    return removed
