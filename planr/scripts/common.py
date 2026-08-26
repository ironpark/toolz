"""Shared plumbing for the planr runners.

Both runners build the ``planr`` binary, copy a fixture from ``planr/fixtures``
and drive the CLI against it.  Every invocation gets its own timestamped
directory under ``planr/run`` holding that run's artifacts.  This module holds
the parts they have in common; it deliberately depends only on the standard
library so the plan scenario runs without the Codex SDK installed.
"""

from __future__ import annotations

import pathlib
import shutil
import subprocess
import tempfile
import time


MODULE_DIR = pathlib.Path(__file__).resolve().parents[1]
FIXTURES_DIR = MODULE_DIR / "fixtures"
RUN_ROOT = MODULE_DIR / "run"
METADATA_FILE = "metadata.env"


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


def make_run_dir(label: str) -> pathlib.Path:
    """Create run/<UTC timestamp>-<label>/ to hold one invocation's artifacts.

    The timestamp leads the name so a plain directory listing is in
    chronological order.  A counter is appended only when two runs of the same
    kind start within the same second.
    """

    RUN_ROOT.mkdir(parents=True, exist_ok=True)
    stamp = time.strftime("%Y%m%d-%H%M%S", time.gmtime())
    for attempt in range(1, 100):
        suffix = "" if attempt == 1 else f".{attempt}"
        path = RUN_ROOT / f"{stamp}{suffix}-{label}"
        try:
            path.mkdir()
        except FileExistsError:
            continue
        return path
    raise HarnessError(f"too many {label} runs started at {stamp}")


def make_agent_workspace(run_dir: pathlib.Path, label: str) -> pathlib.Path:
    """Create a workspace for an agent to work in, outside of run/.

    The artifacts of a run -- transcripts, reports, per-turn state -- must stay
    invisible to the agent being evaluated, so its workspace cannot live inside
    or beside the run directory: reading `..` would expose them.  It goes in the
    system temp directory instead, and the run directory records where, so
    `clean` can still find it.
    """

    workspace = pathlib.Path(tempfile.mkdtemp(prefix=f"planr-{label}-"))
    write_metadata(run_dir, {"workspace": str(workspace)})
    return workspace


def write_metadata(run_dir: pathlib.Path, values: dict[str, str]) -> None:
    """Append key=value lines to the run's metadata.env."""

    with (run_dir / METADATA_FILE).open("a", encoding="utf-8") as stream:
        for key, value in values.items():
            stream.write(f"{key}={value}\n")


def read_metadata(run_dir: pathlib.Path) -> dict[str, str]:
    path = run_dir / METADATA_FILE
    if not path.is_file():
        return {}
    metadata: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        key, separator, value = line.partition("=")
        if separator:
            metadata[key.strip()] = value
    return metadata


def build_planr(destination: pathlib.Path) -> None:
    """Build the planr binary under test into destination."""

    require_command("go")
    build = run_command(["go", "build", "-o", str(destination), "."], cwd=MODULE_DIR)
    if build.returncode != 0:
        raise HarnessError(f"could not build planr (exit {build.returncode}):\n{build.stdout}")


def remove_runs(label: str) -> int:
    """Delete every run directory for a runner, plus any workspace it recorded."""

    RUN_ROOT.mkdir(parents=True, exist_ok=True)
    removed = 0
    for path in sorted(RUN_ROOT.glob(f"*-{label}")):
        if not path.is_dir():
            continue
        workspace = read_metadata(path).get("workspace", "")
        if workspace:
            shutil.rmtree(workspace, ignore_errors=True)
        shutil.rmtree(path)
        removed += 1
    return removed
