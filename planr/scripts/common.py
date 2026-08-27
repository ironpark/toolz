"""Shared plumbing for the planr runners.

Both runners build the ``planr`` binary, copy a fixture from ``planr/fixtures``
and drive the CLI against it.  Every invocation gets its own timestamped
directory under ``planr/run`` holding that run's artifacts.  This module holds
the parts they have in common; it deliberately depends only on the standard
library so the plan scenario runs without the Codex SDK installed.
"""

from __future__ import annotations

import json
import os
import pathlib
import re
import shutil
import subprocess
import tempfile
import time
from typing import Any


MODULE_DIR = pathlib.Path(__file__).resolve().parents[1]
FIXTURES_DIR = MODULE_DIR / "fixtures"
RUN_ROOT = MODULE_DIR / "run"
METADATA_FILE = "metadata.env"
HARNESS_CONFIG_FILE = "HARNESS.json"
RUN_CONFIG_FILE = "harness.json"
HARNESS_CONFIG_PATH = FIXTURES_DIR / HARNESS_CONFIG_FILE

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


def _config_error(path: pathlib.Path, message: str) -> HarnessError:
    return HarnessError(f"harness config {path}: {message}")


def _mapping(value: Any, path: pathlib.Path, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise _config_error(path, f"{label} must be an object")
    return value


def _required(mapping: dict[str, Any], key: str, path: pathlib.Path, label: str) -> Any:
    if key not in mapping:
        raise _config_error(path, f"{label}.{key} is required")
    return mapping[key]


def _string(value: Any, path: pathlib.Path, label: str, *, nonempty: bool = True) -> str:
    if not isinstance(value, str) or (nonempty and not value.strip()):
        suffix = " must be a non-empty string" if nonempty else " must be a string"
        raise _config_error(path, f"{label}{suffix}")
    return value


def _string_list(value: Any, path: pathlib.Path, label: str, *, nonempty: bool = True) -> list[str]:
    if not isinstance(value, list) or (nonempty and not value):
        suffix = " must be a non-empty array of strings" if nonempty else " must be an array of strings"
        raise _config_error(path, f"{label}{suffix}")
    if not all(isinstance(item, str) and item.strip() for item in value):
        raise _config_error(path, f"{label} must contain only non-empty strings")
    return value


def _relative_path(value: str, path: pathlib.Path, label: str, *, allow_parent: bool = False) -> None:
    candidate = pathlib.PurePosixPath(value)
    if candidate.is_absolute() or (not allow_parent and ".." in candidate.parts):
        raise _config_error(path, f"{label} must be a workspace-relative path")


def _unique(values: list[str], path: pathlib.Path, label: str) -> None:
    if len(set(values)) != len(values):
        raise _config_error(path, f"{label} must not contain duplicates")


def validate_harness_config(value: Any, path: pathlib.Path) -> dict[str, Any]:
    """Validate the small JSON contract shared by the runners.

    Keeping validation here means every entry point rejects the same malformed
    configuration, including bare-stdlib commands such as ``analyze`` and
    ``clean``.  The returned object is the original JSON mapping, unchanged,
    so the exact resolved configuration can be written into a run directory.
    """

    root = _mapping(value, path, "root")
    for section in ("tool", "observe", "signals", "completion"):
        _required(root, section, path, "root")

    tool = _mapping(root["tool"], path, "tool")
    name = _string(_required(tool, "name", path, "tool"), path, "tool.name")
    if any(character.isspace() for character in name) or "/" in name:
        raise _config_error(path, "tool.name must be one executable name")
    binary = _string(_required(tool, "binary", path, "tool"), path, "tool.binary")
    _relative_path(binary, path, "tool.binary")
    build = _mapping(_required(tool, "build", path, "tool"), path, "tool.build")
    directory = _string(
        _required(build, "directory", path, "tool.build"), path, "tool.build.directory"
    )
    build_command = _string_list(
        _required(build, "command", path, "tool.build"), path, "tool.build.command"
    )
    if sum(part.count("{destination}") for part in build_command) != 1:
        raise _config_error(
            path,
            "tool.build.command must contain {destination} exactly once",
        )
    if not pathlib.PurePath(directory).parts:
        raise _config_error(path, "tool.build.directory must not be empty")

    observe = _mapping(root["observe"], path, "observe")
    actions = _string_list(
        _required(observe, "actions", path, "observe"), path, "observe.actions"
    )
    _unique(actions, path, "observe.actions")
    groups = _mapping(_required(observe, "groups", path, "observe"), path, "observe.groups")
    group_commands: set[str] = set()
    for group_name, raw_group in groups.items():
        _string(group_name, path, "observe.groups key")
        group = _mapping(raw_group, path, f"observe.groups.{group_name}")
        group_command = _string(
            _required(group, "command", path, f"observe.groups.{group_name}"),
            path,
            f"observe.groups.{group_name}.command",
        )
        if group_command in group_commands:
            raise _config_error(path, f"observe.groups has duplicate command {group_command!r}")
        group_commands.add(group_command)
        group_actions = _string_list(
            _required(group, "actions", path, f"observe.groups.{group_name}"),
            path,
            f"observe.groups.{group_name}.actions",
        )
        _unique(group_actions, path, f"observe.groups.{group_name}.actions")

    expectations = _required(observe, "expectations", path, "observe")
    if not isinstance(expectations, list) or not expectations:
        raise _config_error(path, "observe.expectations must be a non-empty array")
    expectation_actions: set[str] = set()
    for index, raw_expectation in enumerate(expectations):
        label = f"observe.expectations[{index}]"
        expectation = _mapping(raw_expectation, path, label)
        action = _string(_required(expectation, "action", path, label), path, f"{label}.action")
        if action in expectation_actions:
            raise _config_error(path, f"{label}.action {action!r} is duplicated")
        expectation_actions.add(action)
        # An expectation names an action the analyzer counts, so it must be one
        # the observer actually watches. Without this check a command that is
        # renamed or removed keeps producing "it was never called" advice about
        # a command that no longer exists, and the report reads as a finding
        # about the agent rather than a stale configuration.
        head, _, tail = action.partition(" ")
        known = head in actions if not tail else any(
            group.get("command") == head and tail in group.get("actions", [])
            for group in groups.values()
            if isinstance(group, dict)
        )
        if not known:
            raise _config_error(
                path,
                f"{label}.action {action!r} is not in observe.actions or observe.groups",
            )
        _string(_required(expectation, "hint", path, label), path, f"{label}.hint", nonempty=False)
        category = _string(
            _required(expectation, "category", path, label), path, f"{label}.category"
        )
        if category not in {"workflow", "documentation"}:
            raise _config_error(path, f"{label}.category must be workflow or documentation")
        _string(_required(expectation, "message", path, label), path, f"{label}.message")
    no_commands = _mapping(
        _required(observe, "no_commands", path, "observe"), path, "observe.no_commands"
    )
    for category in ("workflow", "documentation"):
        _string(
            _required(no_commands, category, path, "observe.no_commands"),
            path,
            f"observe.no_commands.{category}",
        )

    signals = _mapping(root["signals"], path, "signals")
    event_log = _string(
        _required(signals, "event_log", path, "signals"), path, "signals.event_log"
    )
    _relative_path(event_log, path, "signals.event_log")
    error_patterns = _required(signals, "error_patterns", path, "signals")
    if not isinstance(error_patterns, list):
        raise _config_error(path, "signals.error_patterns must be an array")
    error_names: set[str] = set()
    for index, raw_pattern in enumerate(error_patterns):
        label = f"signals.error_patterns[{index}]"
        pattern = _mapping(raw_pattern, path, label)
        pattern_name = _string(_required(pattern, "name", path, label), path, f"{label}.name")
        if pattern_name in error_names:
            raise _config_error(path, f"{label}.name {pattern_name!r} is duplicated")
        error_names.add(pattern_name)
        expression = _string(_required(pattern, "pattern", path, label), path, f"{label}.pattern")
        try:
            re.compile(expression, re.IGNORECASE)
        except re.error as exc:
            raise _config_error(path, f"{label}.pattern is not a valid regex: {exc}") from exc

    warnings = _required(signals, "warnings", path, "signals")
    if not isinstance(warnings, list):
        raise _config_error(path, "signals.warnings must be an array")
    warning_names: set[str] = set()
    for index, raw_warning in enumerate(warnings):
        label = f"signals.warnings[{index}]"
        warning = _mapping(raw_warning, path, label)
        warning_name = _string(_required(warning, "name", path, label), path, f"{label}.name")
        if warning_name in warning_names:
            raise _config_error(path, f"{label}.name {warning_name!r} is duplicated")
        warning_names.add(warning_name)
        source = _string(_required(warning, "source", path, label), path, f"{label}.source")
        if source not in {"command", "output"}:
            raise _config_error(path, f"{label}.source must be command or output")
        expression = _string(_required(warning, "pattern", path, label), path, f"{label}.pattern")
        try:
            re.compile(expression, re.IGNORECASE)
        except re.error as exc:
            raise _config_error(path, f"{label}.pattern is not a valid regex: {exc}") from exc
        category = _string(_required(warning, "category", path, label), path, f"{label}.category")
        if category not in {"workflow", "documentation"}:
            raise _config_error(path, f"{label}.category must be workflow or documentation")
        _string(_required(warning, "message", path, label), path, f"{label}.message")

    completion = _mapping(root["completion"], path, "completion")
    _string_list(
        _required(completion, "command", path, "completion"), path, "completion.command"
    )
    state_file = _string(
        _required(completion, "state_file", path, "completion"), path, "completion.state_file"
    )
    _relative_path(state_file, path, "completion.state_file")
    _string_list(
        _required(completion, "status_command", path, "completion"),
        path,
        "completion.status_command",
    )
    status_file = _string(
        _required(completion, "status_file", path, "completion"), path, "completion.status_file"
    )
    _relative_path(status_file, path, "completion.status_file")
    output_format = _string(
        _required(completion, "format", path, "completion"), path, "completion.format"
    )
    if output_format != "json":
        raise _config_error(path, "completion.format must be json")
    _string(
        _required(completion, "plans_key", path, "completion"), path, "completion.plans_key"
    )
    fields = _mapping(_required(completion, "fields", path, "completion"), path, "completion.fields")
    for field in ("name", "directory", "status", "done", "total"):
        _string(
            _required(fields, field, path, "completion.fields"),
            path,
            f"completion.fields.{field}",
        )
    display_name = _mapping(
        _required(completion, "display_name", path, "completion"),
        path,
        "completion.display_name",
    )
    display_field = _string(
        _required(display_name, "field", path, "completion.display_name"),
        path,
        "completion.display_name.field",
    )
    if display_field not in fields:
        raise _config_error(
            path,
            "completion.display_name.field must name a completion field",
        )
    if not isinstance(
        _required(display_name, "basename", path, "completion.display_name"), bool
    ):
        raise _config_error(path, "completion.display_name.basename must be boolean")
    complete_when = _mapping(
        _required(completion, "complete_when", path, "completion"),
        path,
        "completion.complete_when",
    )
    complete_field = _string(
        _required(complete_when, "field", path, "completion.complete_when"),
        path,
        "completion.complete_when.field",
    )
    if complete_field not in {"name", "status", "done", "total"}:
        raise _config_error(
            path,
            "completion.complete_when.field must be name, status, done, or total",
        )
    _string(
        _required(complete_when, "equals", path, "completion.complete_when"),
        path,
        "completion.complete_when.equals",
    )
    messages = _mapping(
        _required(completion, "messages", path, "completion"), path, "completion.messages"
    )
    for message_name in ("no_status", "all_complete", "incomplete"):
        _string(
            _required(messages, message_name, path, "completion.messages"),
            path,
            f"completion.messages.{message_name}",
        )
    artifacts = _mapping(
        _required(completion, "artifacts", path, "completion"), path, "completion.artifacts"
    )
    _string(
        _required(artifacts, "directory", path, "completion.artifacts"),
        path,
        "completion.artifacts.directory",
    )
    _relative_path(
        artifacts["directory"], path, "completion.artifacts.directory"
    )
    _string(
        _required(artifacts, "document_marker", path, "completion.artifacts"),
        path,
        "completion.artifacts.document_marker",
    )
    _string(
        _required(artifacts, "draft_frontmatter_key", path, "completion.artifacts"),
        path,
        "completion.artifacts.draft_frontmatter_key",
    )
    return root


def load_harness_config(path: pathlib.Path | None = None) -> dict[str, Any]:
    """Load and validate the machine-readable harness configuration."""

    path = (path or HARNESS_CONFIG_PATH).resolve()
    if not path.is_file():
        raise _config_error(path, "file not found")
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise _config_error(path, f"invalid JSON at line {exc.lineno}, column {exc.colno}: {exc.msg}") from exc
    except (OSError, UnicodeError) as exc:
        raise _config_error(path, f"could not read file: {exc}") from exc
    return validate_harness_config(value, path)


def load_run_harness_config(run_dir: pathlib.Path) -> dict[str, Any]:
    """Load the immutable configuration captured alongside one run."""

    return load_harness_config(run_dir / RUN_CONFIG_FILE)


def write_run_harness_config(run_dir: pathlib.Path, config: dict[str, Any]) -> pathlib.Path:
    path = run_dir / RUN_CONFIG_FILE
    validated = validate_harness_config(config, path)
    path.write_text(json.dumps(validated, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return path


def effective_harness_config(config: dict[str, Any] | None) -> dict[str, Any]:
    """Use the source config only for standalone helper calls.

    Full run analysis calls ``load_run_harness_config`` first.  This helper is
    retained for small unit-level helpers that predate the run-directory
    configuration boundary; it still validates and reads the declarative file.
    """

    return load_harness_config() if config is None else config


def tool_name(config: dict[str, Any]) -> str:
    return config["tool"]["name"]


def tool_binary(config: dict[str, Any], workspace: pathlib.Path) -> pathlib.Path:
    return workspace / config["tool"]["binary"]


def artifact_directory(config: dict[str, Any]) -> str:
    return config["completion"]["artifacts"]["directory"]


def one_line(value: Any, limit: int = 110) -> str:
    """Collapse a value to a single readable line at most `limit` characters."""

    # Slice before normalizing: an agent message or tool output can be tens of
    # kilobytes, and only the first `limit` characters survive.
    text = " ".join(str(value)[: limit * 4].split())
    return text if len(text) <= limit else text[: limit - 1] + "…"


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


def make_agent_workspace(run_dir: pathlib.Path, label: str, *, prefix: str) -> pathlib.Path:
    """Create a workspace for an agent to work in, outside of run/.

    The artifacts of a run -- transcripts, reports, per-turn state -- must stay
    invisible to the agent being evaluated, so its workspace cannot live inside
    or beside the run directory: reading `..` would expose them.  It goes in the
    system temp directory instead, and the run directory records where, so
    `clean` can still find it.
    """

    workspace = pathlib.Path(tempfile.mkdtemp(prefix=f"{prefix}-{label}-"))
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


def build_tool(
    config: dict[str, Any],
    destination: pathlib.Path,
    *,
    config_path: pathlib.Path = HARNESS_CONFIG_PATH,
) -> None:
    """Build the configured tool under test into ``destination``."""

    build_config = config["tool"]["build"]
    command = [
        part.replace("{destination}", str(destination.resolve()))
        for part in build_config["command"]
    ]
    require_command(command[0])
    destination.parent.mkdir(parents=True, exist_ok=True)
    cwd = (config_path.resolve().parent / build_config["directory"]).resolve()
    if not cwd.is_dir():
        raise _config_error(config_path.resolve(), f"tool.build.directory does not exist: {cwd}")
    build = run_command(command, cwd=cwd)
    if build.returncode != 0:
        raise HarnessError(
            f"could not build {tool_name(config)} (exit {build.returncode}):\n{build.stdout}"
        )


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
