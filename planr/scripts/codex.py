#!/usr/bin/env python3
"""Run and analyze an isolated, multi-turn planr evaluation.

The harness intentionally uses the official ``openai-codex`` Python SDK rather
than shelling out to ``codex exec``.  A single SDK thread is reused for every
turn so the model receives the same conversation context while the workspace
is reset for each run.
"""

from __future__ import annotations

import argparse
import asyncio
import dataclasses
import enum
import json
import os
import pathlib
import shutil
import subprocess
import time
import traceback
from typing import Any, Iterable

from analyze import TOKEN_FIELDS, analyze
from common import (
    HarnessError,
    build_planr,
    fixture_dir,
    make_agent_workspace,
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


FIXTURE_NAME = "codex-harness"
RUN_LABEL = "codex"
GOAL_FILE = pathlib.Path(__file__).resolve().parent / "goal.md"
DEFAULT_MODEL = "gpt-5.6-luna"
DEFAULT_REASONING = "medium"
DEFAULT_TURNS = 4
DEFAULT_TIMEOUT = 600.0

INITIAL_PROMPT = """\
You are the implementation agent in this repository. Read AGENT.md and AGENTS.md first, then inspect the existing source and tests. The task specification is included in this initial prompt. Work autonomously and do not ask for clarification.

Use the planr CLI as a real part of the workflow: create a short described draft with `planr new`, edit it into an accurate multi-phase plan, and register it with `planr add`. Inspect it with both `planr status` and `planr overview`. Then implement the goal phase by phase. Keep phase state current, run verification, commit source changes before `planr phase done`, and do not use `--force`. Do not stop at a proposal: make the changes and finish the goal. Leave the repository clean with all acceptance criteria and all plan phases complete. At the end of this turn, report concise evidence and the next concrete action for the following turn.
"""

CONTINUATION_PROMPT = """\
Continue autonomously from the current repository state. Use the task specification from the initial prompt, run `planr overview` and `planr status`, inspect git diff/log, and perform the next unfinished phase. Implement and test changes rather than merely describing them. Commit source changes before `planr phase done`; do not use `--force`. If the goal is already complete, verify every acceptance criterion, all plan phases, and a clean worktree, then give a concise evidence-based summary.
"""

FINAL_PROMPT = """\
Final verification turn: independently check the task specification from the initial prompt, the implementation tests, `planr overview`, `planr status`, and git status. Fix any remaining issue now, update phase state, commit source before marking a phase done, and do not stop with an incomplete plan. If everything is complete, report only the key evidence and remaining risks.
"""


def positive(convert, description: str):
    """Build an argparse type that rejects non-positive values."""

    def parse(value: str):
        try:
            parsed = convert(value)
        except ValueError as exc:
            raise argparse.ArgumentTypeError(f"must be {description}") from exc
        if parsed <= 0:
            raise argparse.ArgumentTypeError(f"must be {description}")
        return parsed

    return parse


positive_int = positive(int, "a positive integer")
positive_seconds = positive(float, "a positive number of seconds")


def load_goal() -> str:
    """Load the task source that is injected into the first prompt only."""

    if not GOAL_FILE.is_file():
        raise HarnessError(f"missing task specification: {GOAL_FILE}")
    goal = GOAL_FILE.read_text(encoding="utf-8").strip()
    if not goal:
        raise HarnessError(f"empty task specification: {GOAL_FILE}")
    return goal


def write_output(path: pathlib.Path, result: subprocess.CompletedProcess[str]) -> None:
    path.write_text(result.stdout or "", encoding="utf-8")


def append_event(path: pathlib.Path, event: dict[str, Any]) -> None:
    with path.open("a", encoding="utf-8") as stream:
        stream.write(json.dumps(event, ensure_ascii=False, default=str) + "\n")


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
    """Flatten the SDK's per-turn token breakdown for the analyzer.

    ``ThreadTokenUsage`` exposes both ``last`` (the current turn) and
    ``total`` (the cumulative thread).  The analyzer adds turn values, so use
    ``last`` to avoid counting the cumulative total repeatedly in multi-turn
    runs.
    """

    raw = jsonable(value)
    if not isinstance(raw, dict):
        return None
    total = raw.get("last") or raw.get("total") or raw
    result: dict[str, int] = {}
    for key, item in total.items():
        field = camel_to_snake(str(key))
        if field not in TOKEN_FIELDS:
            continue
        if isinstance(item, (int, float)) and not isinstance(item, bool):
            result[field] = int(item)
    return result or None


def final_response_from_items(items: Iterable[dict[str, Any]]) -> str | None:
    for item in reversed(list(items)):
        if item.get("type") == "agentMessage" and isinstance(item.get("text"), str):
            return item["text"]
    return None


def write_run_metadata(
    run_dir: pathlib.Path,
    *,
    model: str,
    reasoning: str,
    turns: int,
    timeout: float,
    started_at: str,
) -> None:
    write_metadata(
        run_dir,
        {
            "run_directory": str(run_dir),
            "model": model,
            "reasoning": reasoning,
            "turns_requested": str(turns),
            "timeout_seconds": str(timeout),
            "sdk": "openai-codex",
            "sdk_version": str(getattr(load_sdk(), "__version__", "unknown")),
            "runtime": "codex app-server via official Python SDK",
            "started_at": started_at,
        },
    )


def state_probes(workspace: pathlib.Path) -> dict[str, list[str]]:
    """Commands captured to describe the workspace at a point in time."""

    planr = str(workspace / "bin" / "planr")
    return {
        "overview": [planr, "overview"],
        "status": [planr, "status"],
        "git-status": ["git", "status", "--short"],
        "git-log": ["git", "log", "--oneline", "--decorate", "-20"],
    }


def capture_state(workspace: pathlib.Path, run_dir: pathlib.Path, prefix: str) -> None:
    state_dir = run_dir / "state"
    for name, args in state_probes(workspace).items():
        write_output(state_dir / f"{prefix}{name}.txt", run_command(args, cwd=workspace))


def final_state(workspace: pathlib.Path, run_dir: pathlib.Path) -> int:
    state_dir = run_dir / "state"
    test = run_command(["go", "test", "./..."], cwd=workspace)
    write_output(state_dir / "final-go-test.txt", test)
    state_dir.joinpath("final-go-test.exit").write_text(f"{test.returncode}\n", encoding="utf-8")
    capture_state(workspace, run_dir, "final-")
    return test.returncode


def make_prompts(turns: int, goal: str = "") -> list[str]:
    prompts: list[str] = []
    for number in range(turns):
        if number == 0:
            task = f"\n\n## Task specification\n\n{goal.strip()}\n" if goal.strip() else ""
            prompts.append(INITIAL_PROMPT + task)
        elif number == turns - 1:
            prompts.append(FINAL_PROMPT)
        else:
            prompts.append(CONTINUATION_PROMPT)
    return prompts


async def collect_sdk_turn(
    thread: Any,
    prompt: str,
    *,
    turn_number: int,
    thread_id: str,
    workspace: pathlib.Path,
    reasoning: str,
    log_path: pathlib.Path,
    timeout: float,
) -> int:
    """Run one turn, preserving every SDK notification as JSONL."""

    append_event(
        log_path,
        {
            "type": "turn.started",
            "turn": turn_number + 1,
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
        append_event(
            log_path,
            {
                "type": "turn.error",
                "turn": turn_number + 1,
                "thread_id": thread_id,
                "error": {"class": type(exc).__name__, "message": str(exc)},
            },
        )
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
                    "turn": turn_number + 1,
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
                    token_value = payload.get("tokenUsage") or payload.get("token_usage")
                    summary = token_usage_summary(token_value)
                    if summary:
                        usage = summary
                    turn_value = payload.get("turn")
                    if event_type == "turn.completed" and isinstance(turn_value, dict):
                        completed_turn = turn_value

    task = asyncio.create_task(consume())
    timed_out = False
    try:
        await asyncio.wait_for(asyncio.shield(task), timeout=max(0.1, timeout - (time.monotonic() - started)))
    except asyncio.TimeoutError:
        timed_out = True
        append_event(
            log_path,
            {
                "type": "turn.timeout",
                "turn": turn_number + 1,
                "thread_id": thread_id,
                "timeout_seconds": timeout,
            },
        )
        try:
            await asyncio.wait_for(handle.interrupt(), timeout=5)
        except Exception as exc:
            append_event(
                log_path,
                {
                    "type": "turn.interrupt_error",
                    "turn": turn_number + 1,
                    "thread_id": thread_id,
                    "error": {"class": type(exc).__name__, "message": str(exc)},
                },
            )
        try:
            await asyncio.wait_for(asyncio.shield(task), timeout=10)
        except BaseException:
            task.cancel()
            try:
                await task
            except BaseException:
                pass
    except Exception as exc:
        append_event(
            log_path,
            {
                "type": "turn.error",
                "turn": turn_number + 1,
                "thread_id": thread_id,
                "error": {"class": type(exc).__name__, "message": str(exc)},
            },
        )
        task.cancel()
        try:
            await task
        except BaseException:
            pass
        return 1

    final_response = final_response_from_items(items)
    status = completed_turn.get("status") if completed_turn else None
    append_event(
        log_path,
        {
            "type": "turn.completed",
            "turn": turn_number + 1,
            "thread_id": thread_id,
            "result": {
                "id": completed_turn.get("id") if completed_turn else None,
                "status": status or ("interrupted" if timed_out else "unknown"),
                "error": completed_turn.get("error") if completed_turn else None,
                "final_response": final_response,
                "items": items,
                "usage_per_turn": usage,
            },
            "final_response": final_response,
            "usage": usage,
        },
    )
    if timed_out:
        return 124
    return 0 if status == "completed" else 1


async def run_sdk_turns(
    workspace: pathlib.Path,
    run_dir: pathlib.Path,
    prompts: list[str],
    *,
    model: str,
    reasoning: str,
    timeout: float,
) -> list[int]:
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
            f'model_reasoning_effort="{reasoning}"',
        ),
    )
    exit_codes: list[int] = []
    try:
        async with AsyncCodex(config=config) as codex:
            thread = await codex.thread_start(
                approval_mode=ApprovalMode.deny_all,
                cwd=str(workspace),
                config={"model_reasoning_effort": reasoning},
                # Keep the conversation available to this process for
                # multi-turn evaluation, but do not persist it in the user's
                # Codex thread history.
                ephemeral=True,
                model=model,
                sandbox=Sandbox.workspace_write,
            )
            thread_id = thread.id
            write_metadata(run_dir, {"thread_id": thread_id})
            for number, prompt in enumerate(prompts):
                log_path = run_dir / "turns" / f"turn-{number:02d}.jsonl"
                exit_code = await collect_sdk_turn(
                    thread,
                    prompt,
                    turn_number=number,
                    thread_id=thread_id,
                    workspace=workspace,
                    reasoning=reasoning,
                    log_path=log_path,
                    timeout=timeout,
                )
                exit_codes.append(exit_code)
                # Capture the workspace as it stood at the end of this turn;
                # snapshotting after the whole run would record the final
                # state under every turn's label.
                capture_state(workspace, run_dir, f"turn-{number:02d}-")
                # An interrupted turn leaves the SDK thread usable.  Keep the
                # conversation going so the next continuation prompt can
                # recover unfinished work; hard SDK failures still stop the
                # run because the thread may no longer be consistent.
                if exit_code not in (0, 124):
                    break
    except Exception as exc:
        log_path = run_dir / "turns" / f"turn-{len(exit_codes):02d}.jsonl"
        append_event(
            log_path,
            {
                "type": "sdk.error",
                "error": {"class": type(exc).__name__, "message": str(exc)},
            },
        )
        run_dir.joinpath("turns", f"turn-{len(exit_codes):02d}.stderr.log").write_text(
            traceback.format_exc(), encoding="utf-8"
        )
        exit_codes.append(1)
    return exit_codes


def prepare_workspace() -> tuple[pathlib.Path, pathlib.Path]:
    fixture = fixture_dir(FIXTURE_NAME)
    for required in ("AGENT.md", "AGENTS.md"):
        if not (fixture / required).is_file():
            raise HarnessError(f"missing fixture: {fixture / required}")
    run_dir = make_run_dir(RUN_LABEL)
    # Outside run_dir on purpose: the agent must not be able to read this run's
    # transcripts and reports by walking up from its own working directory.
    workspace = make_agent_workspace(run_dir, RUN_LABEL)
    (workspace / "bin").mkdir()
    (workspace / ".harness").mkdir()
    (run_dir / "turns").mkdir()
    (run_dir / "state").mkdir()
    shutil.copytree(fixture, workspace, dirs_exist_ok=True)
    build_planr(workspace / "bin" / "planr")
    init = run_command(["git", "init", "-q"], cwd=workspace)
    if init.returncode != 0:
        raise HarnessError(f"could not initialize isolated Git repository: {init.stdout.strip()}")
    for key, value in {
        "user.name": "planr codex harness",
        "user.email": "planr-harness@example.invalid",
    }.items():
        configured = run_command(["git", "config", key, value], cwd=workspace)
        if configured.returncode != 0:
            raise HarnessError(f"could not configure Git {key}: {configured.stdout.strip()}")
    baseline = run_command(["git", "add", "-A"], cwd=workspace)
    if baseline.returncode == 0:
        baseline = run_command(["git", "commit", "-qm", "harness baseline"], cwd=workspace)
    if baseline.returncode != 0:
        raise HarnessError(f"could not create Git baseline: {baseline.stdout.strip()}")
    return run_dir, workspace


def run_harness(args: argparse.Namespace) -> int:
    require_command("git")
    if not args.model:
        raise HarnessError("--model must not be empty")
    if not args.reasoning:
        raise HarnessError("--reasoning must not be empty")
    goal = load_goal()
    run_dir, workspace = prepare_workspace()
    started_at = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    write_run_metadata(
        run_dir,
        model=args.model,
        reasoning=args.reasoning,
        turns=args.turns,
        timeout=args.timeout,
        started_at=started_at,
    )
    prompts = make_prompts(args.turns, goal)
    for number, prompt in enumerate(prompts):
        (run_dir / "turns" / f"turn-{number:02d}.prompt.md").write_text(prompt, encoding="utf-8")

    overall_exit = 0
    if args.dry_run:
        for number in range(args.turns):
            log_path = run_dir / "turns" / f"turn-{number:02d}.jsonl"
            append_event(
                log_path,
                {
                    "type": "harness.dry_run",
                    "turn": number + 1,
                    "message": "Codex SDK was not invoked",
                },
            )
            (run_dir / "turns" / f"turn-{number:02d}.exit").write_text("0\n", encoding="utf-8")
            capture_state(workspace, run_dir, f"turn-{number:02d}-")
    else:
        exit_codes = asyncio.run(
            run_sdk_turns(
                workspace,
                run_dir,
                prompts,
                model=args.model,
                reasoning=args.reasoning,
                timeout=args.timeout,
            )
        )
        for number, exit_code in enumerate(exit_codes):
            (run_dir / "turns" / f"turn-{number:02d}.exit").write_text(
                f"{exit_code}\n", encoding="utf-8"
            )
            if exit_code != 0:
                overall_exit = 1

    test_exit = final_state(workspace, run_dir)
    if test_exit != 0:
        overall_exit = 1
    write_metadata(
        run_dir,
        {
            "finished_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "final_test_exit": str(test_exit),
        },
    )
    analyze(run_dir, run_dir / "REPORT.md")
    print(f"Run directory: {run_dir}")
    print(f"Isolated repository: {workspace}")
    print(f"Report: {run_dir / 'REPORT.md'}")
    print(f"Transcript: {run_dir / 'transcript.md'}")
    if args.dry_run:
        print("Dry run: Codex SDK was not invoked")
    return overall_exit


def clean_runs() -> int:
    print(f"Removed {remove_runs(RUN_LABEL)} Codex harness run(s)")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="main.py codex", description="run an isolated Codex planr evaluation"
    )
    parser.add_argument(
        "--turns",
        type=positive_int,
        default=positive_int(os.environ.get("PLANR_HARNESS_TURNS") or str(DEFAULT_TURNS)),
        help="number of SDK turns, including the first turn (default: 4)",
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
        help="maximum seconds per SDK turn (default: 600)",
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
