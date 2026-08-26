"""Shared plumbing for the planr runners.

Both runners build the ``planr`` binary, copy a fixture from ``planr/fixtures``
and drive the CLI against it.  Every invocation gets its own timestamped
directory under ``planr/run`` holding that run's artifacts.  This module holds
the parts they have in common; it deliberately depends only on the standard
library so the plan scenario runs without the Codex SDK installed.
"""

from __future__ import annotations

import os
import pathlib
import shutil
import subprocess
import tempfile
import time


MODULE_DIR = pathlib.Path(__file__).resolve().parents[1]
FIXTURES_DIR = MODULE_DIR / "fixtures"
RUN_ROOT = MODULE_DIR / "run"
METADATA_FILE = "metadata.env"

# One run directory layout, named once: codex.py writes these and analyze.py
# reads them, and a rename that reached only one side would leave the analyzer
# silently reporting an empty run.
SESSION_LOG = "session.jsonl"
SESSION_PROMPT = "session.prompt.md"
SESSION_EXIT = "session.exit"
STATE_DIR = "state"
PLANS_DIR = "plans"


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


def init_git_repository(workspace: pathlib.Path, *, message: str = "harness baseline") -> None:
    """Turn a prepared workspace into a repository with one baseline commit.

    planr refuses to run outside a repository, records completions as git notes
    against HEAD, and blocks `phase done` on uncommitted source changes, so both
    runners need a repository with at least one commit before the first command.
    """

    require_command("git")
    init = run_command(["git", "init", "-q"], cwd=workspace)
    if init.returncode != 0:
        raise HarnessError(f"could not initialize isolated Git repository: {init.stdout.strip()}")
    for key, value in {
        "user.name": "planr harness",
        "user.email": "planr-harness@example.invalid",
    }.items():
        configured = run_command(["git", "config", key, value], cwd=workspace)
        if configured.returncode != 0:
            raise HarnessError(f"could not configure Git {key}: {configured.stdout.strip()}")
    baseline = run_command(["git", "add", "-A"], cwd=workspace)
    if baseline.returncode == 0:
        baseline = run_command(["git", "commit", "-qm", message], cwd=workspace)
    if baseline.returncode != 0:
        raise HarnessError(f"could not create Git baseline: {baseline.stdout.strip()}")


def force_remove_tree(path: pathlib.Path) -> None:
    """Delete a tree, including files Go's module cache marks read-only.

    `go` stores downloaded modules without write permission, so a plain
    `rmtree` leaves most of the cache behind -- silently, when errors are
    ignored -- and a workspace that once held one can never be cleaned up.
    """

    if not path.exists():
        return
    for parent, directories, files in os.walk(path):
        for name in directories + files:
            try:
                os.chmod(os.path.join(parent, name), 0o700)
            except OSError:
                pass
    shutil.rmtree(path, ignore_errors=True)


def remove_runs(label: str) -> int:
    """Delete every run directory for a runner, plus any workspace it recorded."""

    RUN_ROOT.mkdir(parents=True, exist_ok=True)
    removed = 0
    for path in sorted(RUN_ROOT.glob(f"*-{label}")):
        if not path.is_dir():
            continue
        workspace = read_metadata(path).get("workspace", "")
        if workspace:
            force_remove_tree(pathlib.Path(workspace))
        shutil.rmtree(path)
        removed += 1
    return removed
