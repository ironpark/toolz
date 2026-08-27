#!/usr/bin/env python3
"""Run and analyze an isolated planr evaluation.

The agent is given the request once and left to work until it decides the task
is finished; there are no follow-up prompts nudging it along, so the report
measures what it does on its own.  The harness uses the official
``openai-codex`` Python SDK rather than shelling out to ``codex exec`` so the
raw notification stream can be preserved.
"""

from __future__ import annotations

import argparse
import asyncio
import dataclasses
import enum
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
import time
import traceback
from typing import Any

from analyze import TEXT_KEYS, TOKEN_FIELDS, analyze, command_text, usage_total
from common import (
    SESSION_EXIT,
    SESSION_LOG,
    SESSION_PROMPT,
    PLANS_DIR,
    STATE_DIR,
    HarnessError,
    build_planr,
    fixture_dir,
    init_git_repository,
    make_agent_workspace,
    one_line,
    make_run_dir,
    remove_runs,
    require_command,
    run_command,
    write_metadata,
)


def load_sdk():
    """Import the Codex SDK on demand.

    `codex clean` and `codex analyze` only touch local files, so importing the
    SDK at module scope would charge them for a dependency they never use --
    and would make this module unimportable without it.
    """

    import openai_codex

    return openai_codex


DEFAULT_FIXTURE = "codex-harness"
# Run directories are labelled per fixture so runs of different scenarios stay
# distinguishable in `run/` and can be cleaned independently. The default
# fixture keeps the original `codex` label so existing run directories and the
# paths in earlier reports remain valid.
FIXTURE_LABELS = {
    "codex-harness": "codex",
    "codex-greenfield": "codex-greenfield",
}
RUN_LABEL = FIXTURE_LABELS[DEFAULT_FIXTURE]
# `FIXTURE.*` files configure the evaluation and must never reach the agent's
# workspace verbatim: FIXTURE.PROMPT.<lang>.md is only read, FIXTURE.TEST.sh is
# run against the finished workspace from outside it, and
# FIXTURE.AGENTS.<lang>.md is installed under the name the agent expects.
FIXTURE_PREFIX = "FIXTURE."
FIXTURE_TEST_FILE = f"{FIXTURE_PREFIX}TEST.sh"
INSTALLED_AGENTS_FILE = "AGENTS.md"

# Both the request and the instructions exist once per language, so a run never
# mixes them: an English request paired with Korean guidance would measure the
# mixture rather than either language.  planr's own `language` setting decides
# which documents are generated; these decide which ones the agent reads.
SUPPORTED_LANGUAGES = ("en", "ko")
DEFAULT_LANGUAGE = "en"
PLANR_CONFIG_FILE = ".planr.yaml"
# .planr.yaml is read here without a YAML parser: the runners depend only on
# the standard library so the plan scenario runs on a bare interpreter.  Only
# planr's own top-level `language:` key is needed, and planr itself rejects a
# malformed file long before the value would matter.
LANGUAGE_SETTING = re.compile(r"^language:\s*[\"']?([A-Za-z-]+)[\"']?\s*$", re.MULTILINE)


def agents_file_for(language: str) -> str:
    return f"{FIXTURE_PREFIX}AGENTS.{language}.md"


def prompt_file_for(language: str) -> str:
    return f"{FIXTURE_PREFIX}PROMPT.{language}.md"


DEFAULT_MODEL = "gpt-5.6-luna"
DEFAULT_REASONING = "medium"
DEFAULT_TIMEOUT = 3600.0


def positive_seconds(value: str) -> float:
    try:
        parsed = float(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError("must be a positive number of seconds") from exc
    if parsed <= 0:
        raise argparse.ArgumentTypeError("must be a positive number of seconds")
    return parsed


def load_initial_prompt(fixture: str = DEFAULT_FIXTURE, language: str = DEFAULT_LANGUAGE) -> str:
    """Load the first user message from the fixture, in the run's language.

    It stays out of the workspace on purpose: the agent has to work from the
    conversation, not from a task file it can re-read on disk.
    """

    path = fixture_dir(fixture) / prompt_file_for(language)
    if not path.is_file():
        raise HarnessError(f"missing initial prompt: {path}")
    prompt = path.read_text(encoding="utf-8").strip()
    if not prompt:
        raise HarnessError(f"empty initial prompt: {path}")
    return prompt


ITEM_LABELS = {
    "commandExecution": "cmd",
    "agentMessage": "say",
    "reasoning": "think",
    "fileChange": "edit",
    "mcpToolCall": "tool",
    "webSearch": "search",
    "todoList": "todo",
    "error": "error",
}


def elapsed(seconds: float) -> str:
    minutes, second = divmod(int(seconds), 60)
    hour, minute = divmod(minutes, 60)
    return f"{hour}:{minute:02d}:{second:02d}" if hour else f"{minute}:{second:02d}"


class Progress:
    """Monitoring log on stderr, so stdout stays machine-readable."""

    def __init__(self) -> None:
        self.enabled = True
        self.started = time.monotonic()

    def start(self, *, enabled: bool) -> None:
        self.enabled = enabled
        self.started = time.monotonic()

    def __call__(self, message: str) -> None:
        if not self.enabled:
            return
        print(f"[{elapsed(time.monotonic() - self.started):>7}] {message}", file=sys.stderr, flush=True)


progress = Progress()


def describe_item(item: dict[str, Any]) -> str | None:
    """Summarize one completed thread item as a single monitoring line."""

    kind = str(item.get("type", ""))
    if not kind:
        return None
    label = ITEM_LABELS.get(kind, kind)
    command = command_text(item)
    if command:
        exit_code = item.get("exitCode", item.get("exit_code"))
        suffix = "" if exit_code in (0, None) else f" (exit {exit_code})"
        return f"{label}  {one_line(command)}{suffix}"
    for key in (*TEXT_KEYS, "query", "path"):
        value = item.get(key)
        if isinstance(value, str) and value.strip():
            return f"{label}  {one_line(value)}"
    changes = item.get("changes")
    if isinstance(changes, list) and changes:
        paths = [str(change.get("path", "?")) for change in changes if isinstance(change, dict)]
        return f"{label}  {one_line(', '.join(paths) or len(changes))}"
    return label


def format_usage(usage: dict[str, int] | None) -> str:
    if not usage:
        return "tokens n/a"
    total = usage_total(usage)
    return (
        f"{total / 1000:.1f}k tokens"
        f" (in {usage.get('input_tokens', 0) / 1000:.1f}k /"
        f" out {usage.get('output_tokens', 0) / 1000:.1f}k)"
    )


def write_output(path: pathlib.Path, result: subprocess.CompletedProcess[str]) -> None:
    path.write_text(result.stdout or "", encoding="utf-8")


def append_event(path: pathlib.Path, event: dict[str, Any]) -> None:
    with path.open("a", encoding="utf-8") as stream:
        stream.write(json.dumps(event, ensure_ascii=False, default=str) + "\n")


def append_error(path: pathlib.Path, kind: str, exc: BaseException, **extra: Any) -> None:
    append_event(
        path,
        {
            "type": kind,
            "error": {"class": type(exc).__name__, "message": str(exc)},
            **extra,
        },
    )


async def cancel(task: "asyncio.Task[Any]") -> None:
    task.cancel()
    try:
        await task
    except BaseException:
        pass


def jsonable(value: Any) -> Any:
    """Convert SDK Pydantic models/dataclasses/enums to JSON-safe values."""

    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    if isinstance(value, enum.Enum):
        return value.value
    if hasattr(value, "model_dump"):
        try:
            return value.model_dump(mode="json", by_alias=True)
        except TypeError:
            return value.model_dump()
    if dataclasses.is_dataclass(value):
        return {key: jsonable(item) for key, item in dataclasses.asdict(value).items()}
    if isinstance(value, pathlib.Path):
        return str(value)
    if isinstance(value, dict):
        return {str(key): jsonable(item) for key, item in value.items()}
    if isinstance(value, (list, tuple, set)):
        return [jsonable(item) for item in value]
    return str(value)


def camel_to_snake(value: str) -> str:
    result: list[str] = []
    for character in value:
        if character.isupper():
            result.append("_")
            result.append(character.lower())
        else:
            result.append(character)
    return "".join(result).lstrip("_")


def token_usage_summary(value: Any) -> dict[str, int] | None:
    """Flatten the SDK's token breakdown for the analyzer.

    ``ThreadTokenUsage`` exposes both ``last`` and ``total``.  ``last`` covers
    only the most recent model request, and an agentic session issues one per
    tool call, so it undercounts a run by an order of magnitude -- reading it
    reported 173 output tokens for a session that actually spent 6223.
    ``total`` is the cumulative thread figure and is the one worth comparing
    across runs.
    """

    raw = jsonable(value)
    if not isinstance(raw, dict):
        return None
    total = raw.get("total") or raw.get("last") or raw
    result: dict[str, int] = {}
    for key, item in total.items():
        field = camel_to_snake(str(key))
        if field not in TOKEN_FIELDS:
            continue
        if isinstance(item, (int, float)) and not isinstance(item, bool):
            result[field] = int(item)
    return result or None


def final_response_from_items(items: list[dict[str, Any]]) -> str | None:
    for item in reversed(items):
        if item.get("type") == "agentMessage" and isinstance(item.get("text"), str):
            return item["text"]
    return None


def write_run_metadata(
    run_dir: pathlib.Path,
    *,
    model: str,
    reasoning: str,
    timeout: float,
    started_at: str,
) -> None:
    write_metadata(
        run_dir,
        {
            "run_directory": str(run_dir),
            "model": model,
            "reasoning": reasoning,
            "timeout_seconds": str(timeout),
            "sdk": "openai-codex",
            "sdk_version": str(getattr(load_sdk(), "__version__", "unknown")),
            "runtime": "codex app-server via official Python SDK",
            "started_at": started_at,
        },
    )


def is_plan_draft(path: pathlib.Path) -> bool:
    """Whether a Markdown file is a draft produced by `planr new`.

    Matches planr's own rule: a `plan_name` key in the document frontmatter.
    """

    if path.suffix.lower() != ".md":
        return False
    try:
        head = path.read_text(encoding="utf-8", errors="replace")[:2000]
    except OSError:
        return False
    if not head.startswith("---\n"):
        return False
    front, _, _ = head[4:].partition("\n---\n")
    return any(line.startswith("plan_name:") and line[10:].strip() for line in front.splitlines())


def copy_plan_artifacts(workspace: pathlib.Path, run_dir: pathlib.Path) -> list[str]:
    """Copy the plan documents the agent produced into the run directory.

    The workspace is a temporary directory that `clean` deletes, so without
    this the actual plans -- the thing the evaluation is about -- are gone as
    soon as the run is tidied up, leaving only the one-line summaries in
    state/final-overview.txt.

    Plan directories are found by their on-disk signature (a child directory
    holding PLAN.md) rather than by reading plans_dirs out of .planr.yaml, so
    this keeps working whatever a fixture names them.
    """

    destination = run_dir / PLANS_DIR
    copied: list[str] = []
    for entry in sorted(workspace.iterdir()):
        if entry.name in {".git", ".harness", "bin"}:
            continue
        if entry.is_dir():
            if not any(child.joinpath("PLAN.md").is_file() for child in entry.iterdir() if child.is_dir()):
                continue
            shutil.copytree(entry, destination / entry.name, dirs_exist_ok=True)
            copied.extend(
                str(path.relative_to(destination))
                for path in sorted((destination / entry.name).rglob("*"))
                if path.is_file()
            )
        elif entry.is_file() and is_plan_draft(entry):
            destination.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(entry, destination / entry.name)
            copied.append(entry.name)
    return copied


def run_fixture_test(fixture_name: str, workspace: pathlib.Path, run_dir: pathlib.Path) -> int | None:
    """Grade the finished workspace with the fixture's acceptance script.

    The script is never copied into the workspace, so the agent works from the
    request alone and cannot tailor its code to the checks. It runs in a scratch
    directory of its own -- not in the workspace -- so grading cannot leave
    files behind that a later `git status` would report as the agent's work.

    Returns the script's exit code, or None when the fixture has no script.
    """

    script = fixture_dir(fixture_name) / FIXTURE_TEST_FILE
    if not script.is_file():
        return None
    state_dir = run_dir / STATE_DIR
    scratch = pathlib.Path(tempfile.mkdtemp(prefix="planr-fixture-test-"))
    env = os.environ.copy()
    env["PLANR_EVAL_WORKSPACE"] = str(workspace)
    try:
        result = run_command(["bash", str(script)], cwd=scratch, env=env)
    finally:
        shutil.rmtree(scratch, ignore_errors=True)
    write_output(state_dir / "final-fixture-test.txt", result)
    state_dir.joinpath("final-fixture-test.exit").write_text(f"{result.returncode}\n", encoding="utf-8")
    return result.returncode


def final_state(
    workspace: pathlib.Path, run_dir: pathlib.Path, fixture_name: str = DEFAULT_FIXTURE
) -> tuple[int, int | None]:
    """Record how the workspace ended up, and whether it passes verification.

    Returns the `go test` exit code and the fixture acceptance script's exit
    code (None when the fixture has no script).
    """

    state_dir = run_dir / STATE_DIR
    planr = str(workspace / "bin" / "planr")
    test = run_command(["go", "test", "./..."], cwd=workspace)
    write_output(state_dir / "final-go-test.txt", test)
    state_dir.joinpath("final-go-test.exit").write_text(f"{test.returncode}\n", encoding="utf-8")
    for name, args in {
        "overview": [planr, "overview"],
        "status": [planr, "status"],
        "git-status": ["git", "status", "--short"],
        # planr deliberately leaves its own draft and plan files untracked, so
        # judging the run by the full status would call every successful run
        # dirty. Tracked-file changes are the ones the agent failed to commit.
        "git-status-tracked": ["git", "status", "--short", "--untracked-files=no"],
        "git-log": ["git", "log", "--oneline", "--decorate", "-20"],
    }.items():
        write_output(state_dir / f"final-{name}.txt", run_command(args, cwd=workspace))
    plans = copy_plan_artifacts(workspace, run_dir)
    progress(f"captured {len(plans)} planr document(s) into {PLANS_DIR}/")
    return test.returncode, run_fixture_test(fixture_name, workspace, run_dir)


async def collect_sdk_session(
    thread: Any,
    prompt: str,
    *,
    thread_id: str,
    workspace: pathlib.Path,
    reasoning: str,
    log_path: pathlib.Path,
    timeout: float,
) -> int:
    """Run the agent to completion, preserving every SDK notification as JSONL."""

    progress(f"session started ({len(prompt)} prompt chars)")

    append_event(
        log_path,
        {
            "type": "session.started",
            "thread_id": thread_id,
            "prompt_chars": len(prompt),
        },
    )
    from openai_codex import ApprovalMode, Sandbox

    started = time.monotonic()
    try:
        handle = await asyncio.wait_for(
            thread.turn(
                prompt,
                approval_mode=ApprovalMode.deny_all,
                cwd=str(workspace),
                effort=reasoning,
                sandbox=Sandbox.workspace_write,
            ),
            timeout=timeout,
        )
    except Exception as exc:
        append_error(log_path, "session.error", exc, thread_id=thread_id)
        progress(f"session failed to start: {type(exc).__name__}: {one_line(exc)}")
        return 1

    items: list[dict[str, Any]] = []
    usage: dict[str, int] | None = None
    completed_turn: dict[str, Any] | None = None

    async def consume() -> None:
        nonlocal usage, completed_turn
        async for notification in handle.stream():
            raw = jsonable(notification)
            method = getattr(notification, "method", "sdk/notification")
            event_type = str(method).replace("/", ".")
            append_event(
                log_path,
                {
                    "type": event_type,
                    "thread_id": thread_id,
                    "notification": raw,
                },
            )
            if isinstance(raw, dict):
                payload = raw.get("payload")
                if isinstance(payload, dict):
                    item = payload.get("item")
                    if isinstance(item, dict):
                        items.append(item)
                        if progress.enabled and event_type.endswith(".completed"):
                            line = describe_item(item)
                            if line:
                                progress(f"  {line}")
                    token_value = payload.get("tokenUsage") or payload.get("token_usage")
                    if token_value:
                        usage = token_usage_summary(token_value) or usage
                    turn_value = payload.get("turn")
                    if event_type == "turn.completed" and isinstance(turn_value, dict):
                        completed_turn = turn_value

    task = asyncio.create_task(consume())
    timed_out = False
    try:
        await asyncio.wait_for(asyncio.shield(task), timeout=max(0.1, timeout - (time.monotonic() - started)))
    except asyncio.TimeoutError:
        timed_out = True
        progress(f"session timed out after {timeout:.0f}s; interrupting")
        append_event(
            log_path,
            {
                "type": "session.timeout",
                "thread_id": thread_id,
                "timeout_seconds": timeout,
            },
        )
        try:
            await asyncio.wait_for(handle.interrupt(), timeout=5)
        except Exception as exc:
            append_error(log_path, "session.interrupt_error", exc, thread_id=thread_id)
        try:
            await asyncio.wait_for(asyncio.shield(task), timeout=10)
        except BaseException:
            await cancel(task)
    except Exception as exc:
        append_error(log_path, "session.error", exc, thread_id=thread_id)
        await cancel(task)
        progress(f"session failed: {type(exc).__name__}: {one_line(exc)}")
        return 1

    final_response = final_response_from_items(items)
    status = completed_turn.get("status") if completed_turn else None
    outcome = status or ("interrupted" if timed_out else "unknown")
    append_event(
        log_path,
        {
            "type": "session.completed",
            "thread_id": thread_id,
            # `usage` and `final_response` are written exactly once: the
            # analyzer sums every `usage` dict it finds, and a JSON round-trip
            # turns a repeated reference into two distinct objects, so a second
            # copy here would double every token count in the report.
            "result": {
                "id": completed_turn.get("id") if completed_turn else None,
                "status": outcome,
                "error": completed_turn.get("error") if completed_turn else None,
                "item_count": len(items),
            },
            "final_response": final_response,
            "usage": usage,
        },
    )
    progress(
        f"session {outcome} in {elapsed(time.monotonic() - started)}"
        f" · {len(items)} items · {format_usage(usage)}"
    )
    if timed_out:
        return 124
    return 0 if status == "completed" else 1


async def run_sdk_session(
    workspace: pathlib.Path,
    run_dir: pathlib.Path,
    prompt: str,
    *,
    model: str,
    reasoning: str,
    timeout: float,
) -> int:
    from openai_codex import ApprovalMode, AsyncCodex, CodexConfig, Sandbox

    sdk_env = os.environ.copy()
    sdk_path = str(workspace / "bin")
    sdk_env["PATH"] = sdk_path + os.pathsep + sdk_env.get("PATH", "")
    config = CodexConfig(
        cwd=str(workspace),
        env=sdk_env,
        config_overrides=(
            'approval_policy="never"',
            'sandbox_mode="workspace-write"',
            # workspace-write keeps `.git` read-only by default, which makes
            # `git commit` fail on `.git/index.lock`. The scenario requires the
            # agent to commit, so hand it back write access to this workspace's
            # own Git directory.
            f'sandbox_workspace_write.writable_roots=["{workspace.resolve() / ".git"}"]',
            f'model_reasoning_effort="{reasoning}"',
        ),
    )
    log_path = run_dir / SESSION_LOG
    try:
        async with AsyncCodex(config=config) as codex:
            thread = await codex.thread_start(
                approval_mode=ApprovalMode.deny_all,
                cwd=str(workspace),
                config={"model_reasoning_effort": reasoning},
                # Keep the conversation in this process only; do not persist it
                # in the user's Codex thread history.
                ephemeral=True,
                model=model,
                sandbox=Sandbox.workspace_write,
            )
            thread_id = thread.id
            write_metadata(run_dir, {"thread_id": thread_id})
            progress(f"thread {thread_id} started on {model} (reasoning {reasoning})")
            return await collect_sdk_session(
                thread,
                prompt,
                thread_id=thread_id,
                workspace=workspace,
                reasoning=reasoning,
                log_path=log_path,
                timeout=timeout,
            )
    except Exception as exc:
        append_error(log_path, "sdk.error", exc)
        (run_dir / "session.stderr.log").write_text(traceback.format_exc(), encoding="utf-8")
        progress(f"SDK failed: {type(exc).__name__}: {one_line(exc)}")
        return 1


def fixture_language(fixture: pathlib.Path) -> str | None:
    """Return the language a fixture's .planr.yaml configures, if any."""

    path = fixture / PLANR_CONFIG_FILE
    if not path.is_file():
        return None
    found = LANGUAGE_SETTING.search(path.read_text(encoding="utf-8"))
    return found.group(1).strip().lower() if found else None


def resolve_language(fixture_name: str, override: str | None = None) -> str:
    """Decide which language a run uses.

    An explicit `--language` wins, so one fixture can be evaluated in either
    language without editing it.  Otherwise the fixture's own planr setting
    decides, and a fixture that configures nothing gets planr's default.
    """

    language = (override or fixture_language(fixture_dir(fixture_name)) or DEFAULT_LANGUAGE).strip().lower()
    if language not in SUPPORTED_LANGUAGES:
        raise HarnessError(
            f"unsupported language {language!r}; use one of: {', '.join(SUPPORTED_LANGUAGES)}"
        )
    return language


def set_workspace_language(workspace: pathlib.Path, language: str) -> None:
    """Pin planr's `language` setting in the workspace to the resolved value.

    Without this a `--language` override would change only the instructions the
    agent reads while planr kept generating documents in the fixture's own
    language -- the run would measure a contradiction rather than a language.
    """

    path = workspace / PLANR_CONFIG_FILE
    setting = f"language: {language}"
    if not path.is_file():
        path.write_text(setting + "\n", encoding="utf-8")
        return
    contents = path.read_text(encoding="utf-8")
    if LANGUAGE_SETTING.search(contents):
        contents = LANGUAGE_SETTING.sub(setting, contents, count=1)
    else:
        contents = setting + "\n" + contents
    path.write_text(contents, encoding="utf-8")


def install_fixture(
    fixture: pathlib.Path, workspace: pathlib.Path, language: str = DEFAULT_LANGUAGE
) -> None:
    """Copy the fixture into the agent's workspace.

    `FIXTURE.*` files are configuration for the evaluation, not repository
    content, so they are skipped wholesale; the instructions for the run's
    language are then written back under the name the agent is expected to
    find, and planr's setting is pinned to match.
    """

    shutil.copytree(
        fixture,
        workspace,
        dirs_exist_ok=True,
        ignore=shutil.ignore_patterns(f"{FIXTURE_PREFIX}*"),
    )
    agents = fixture / agents_file_for(language)
    if not agents.is_file():
        raise HarnessError(f"missing fixture instructions: {agents}")
    shutil.copyfile(agents, workspace / INSTALLED_AGENTS_FILE)
    set_workspace_language(workspace, language)


def prepare_workspace(
    fixture_name: str = DEFAULT_FIXTURE, language: str = DEFAULT_LANGUAGE
) -> tuple[pathlib.Path, pathlib.Path]:
    fixture = fixture_dir(fixture_name)
    required = fixture / agents_file_for(language)
    if not required.is_file():
        raise HarnessError(f"missing fixture: {required}")
    run_dir = make_run_dir(FIXTURE_LABELS.get(fixture_name, fixture_name))
    write_metadata(run_dir, {"fixture": fixture_name, "language": language})
    # Outside run_dir on purpose: the agent must not be able to read this run's
    # transcripts and reports by walking up from its own working directory.
    workspace = make_agent_workspace(run_dir, FIXTURE_LABELS.get(fixture_name, fixture_name))
    (workspace / "bin").mkdir()
    (workspace / ".harness").mkdir()
    (run_dir / STATE_DIR).mkdir()
    install_fixture(fixture, workspace, language)
    build_planr(workspace / "bin" / "planr")
    # The agent's sandbox only grants writes inside the workspace, so Go's
    # user-level caches are unreachable: GOCACHE stops the first `go test`
    # before it compiles anything, and GOMODCACHE makes every later `go`
    # command print a "writing stat cache: operation not permitted" warning.
    # Point both at gitignored directories in the workspace. The fixtures are
    # standard-library only, so an empty cache costs the agent nothing.
    #
    # Set after build_planr on purpose: the harness itself is not sandboxed, so
    # building planr belongs in the user's shared caches. Doing it earlier
    # copied planr's own dependencies into every run's workspace -- ~360MB of
    # throwaway cache per run.
    for variable, directory in (("GOCACHE", "go-cache"), ("GOMODCACHE", "go-mod-cache")):
        os.environ[variable] = str(workspace / ".harness" / directory)
    init_git_repository(workspace)
    return run_dir, workspace


def run_harness(args: argparse.Namespace) -> int:
    progress.start(enabled=not args.quiet)
    require_command("git")
    if not args.model:
        raise HarnessError("--model must not be empty")
    if not args.reasoning:
        raise HarnessError("--reasoning must not be empty")
    language = resolve_language(args.fixture, args.language)
    initial_prompt = load_initial_prompt(args.fixture, language)
    progress(
        f"preparing isolated repository from fixture {args.fixture}"
        f" in {language} (build planr, git init)"
    )
    run_dir, workspace = prepare_workspace(args.fixture, language)
    progress(f"run directory {run_dir.name}; workspace {workspace}")
    started_at = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    write_run_metadata(
        run_dir,
        model=args.model,
        reasoning=args.reasoning,
        timeout=args.timeout,
        started_at=started_at,
    )
    (run_dir / SESSION_PROMPT).write_text(initial_prompt, encoding="utf-8")

    if args.dry_run:
        append_event(
            run_dir / SESSION_LOG,
            {"type": "harness.dry_run", "message": "Codex SDK was not invoked"},
        )
        session_exit = 0
        progress("session skipped (dry run)")
    else:
        session_exit = asyncio.run(
            run_sdk_session(
                workspace,
                run_dir,
                initial_prompt,
                model=args.model,
                reasoning=args.reasoning,
                timeout=args.timeout,
            )
        )
    (run_dir / SESSION_EXIT).write_text(f"{session_exit}\n", encoding="utf-8")
    overall_exit = 1 if session_exit != 0 else 0

    progress("running final verification (go test, fixture checks) and capturing end state")
    test_exit, fixture_exit = final_state(workspace, run_dir, args.fixture)
    if test_exit != 0 or fixture_exit not in (None, 0):
        overall_exit = 1
    write_metadata(
        run_dir,
        {
            "finished_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "final_test_exit": str(test_exit),
        },
    )
    fixture_note = "" if fixture_exit is None else f"; fixture checks exit {fixture_exit}"
    progress(f"final go test exit {test_exit}{fixture_note}; writing report")
    analyze(run_dir, run_dir / "REPORT.md")
    print(f"Run directory: {run_dir}")
    print(f"Isolated repository: {workspace}")
    print(f"Report: {run_dir / 'REPORT.md'}")
    print(f"Transcript: {run_dir / 'transcript.md'}")
    if args.dry_run:
        print("Dry run: Codex SDK was not invoked")
    return overall_exit


def clean_runs() -> int:
    removed = sum(remove_runs(label) for label in sorted(set(FIXTURE_LABELS.values())))
    print(f"Removed {removed} Codex harness run(s)")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="main.py codex", description="run an isolated Codex planr evaluation"
    )
    parser.add_argument(
        "--fixture",
        choices=sorted(FIXTURE_LABELS),
        default=os.environ.get("PLANR_HARNESS_FIXTURE") or DEFAULT_FIXTURE,
        help=f"evaluation fixture (default: {DEFAULT_FIXTURE})",
    )
    parser.add_argument(
        "--language",
        choices=SUPPORTED_LANGUAGES,
        default=os.environ.get("PLANR_HARNESS_LANGUAGE") or None,
        help="document language; overrides the fixture's own planr setting "
        f"(default: the fixture's setting, otherwise {DEFAULT_LANGUAGE})",
    )
    parser.add_argument(
        "--model",
        default=os.environ.get("PLANR_HARNESS_MODEL") or DEFAULT_MODEL,
        help=f"Codex model (default: {DEFAULT_MODEL})",
    )
    parser.add_argument(
        "--reasoning",
        default=os.environ.get("PLANR_HARNESS_REASONING") or DEFAULT_REASONING,
        help=f"reasoning effort (default: {DEFAULT_REASONING})",
    )
    parser.add_argument(
        "--timeout",
        type=positive_seconds,
        default=positive_seconds(os.environ.get("PLANR_HARNESS_TIMEOUT") or str(DEFAULT_TIMEOUT)),
        help="maximum seconds for the whole session (default: 3600)",
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="suppress the progress log on stderr",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="create the isolated repository but do not invoke the Codex SDK",
    )
    return parser


def main(argv: list[str]) -> int:
    """Run a Codex evaluation. Errors are reported by the caller in main.py."""

    if argv and argv[0] == "clean":
        if len(argv) != 1:
            raise HarnessError("clean accepts no options")
        return clean_runs()
    if argv and argv[0] == "analyze":
        if len(argv) != 2:
            raise HarnessError("analyze requires exactly one run directory")
        return analyze(pathlib.Path(argv[1]))
    return run_harness(build_parser().parse_args(argv))
