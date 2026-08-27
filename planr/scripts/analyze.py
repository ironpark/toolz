#!/usr/bin/env python3
"""Summarize a Codex/planr harness run without external Python packages."""

from __future__ import annotations

import collections
import dataclasses
import functools
import json
import pathlib
import re
import shlex
from typing import Any, Iterable

from common import (
    SESSION_EXIT,
    SESSION_LOG,
    SESSION_PROMPT,
    STATE_DIR,
    HarnessError,
    artifact_directory,
    effective_harness_config,
    load_run_harness_config,
    one_line,
    read_metadata as load_metadata,
    tool_name,
)


TOKEN_FIELDS = (
    "input_tokens",
    "cached_input_tokens",
    "output_tokens",
    "reasoning_output_tokens",
    "total_tokens",
)

@dataclasses.dataclass
class CommandExecution:
    """One shell command the agent ran, kept together with its own output.

    Commands and outputs used to be gathered into two independent deduplicated
    lists, so the transcript's two sections drifted out of step and a reader
    would attribute an output to the wrong command. Pairing them by item id
    keeps the association the SDK already provides.
    """

    command: str = ""
    output: str = ""
    exit_code: int | None = None
    status: str = ""


@dataclasses.dataclass
class SessionStats:
    events: int = 0
    invalid_lines: int = 0
    event_types: collections.Counter[str] = dataclasses.field(
        default_factory=collections.Counter
    )
    item_types: collections.Counter[str] = dataclasses.field(
        default_factory=collections.Counter
    )
    commands: list[str] = dataclasses.field(default_factory=list)
    texts: list[str] = dataclasses.field(default_factory=list)
    outputs: list[str] = dataclasses.field(default_factory=list)
    # Keyed by SDK item id, in first-seen order; later notifications for the
    # same id (started -> completed) overwrite the earlier snapshot.
    executions: dict[str, CommandExecution] = dataclasses.field(default_factory=dict)
    usage: collections.Counter[str] = dataclasses.field(default_factory=collections.Counter)
    exit_code: int | None = None
    prompt_chars: int = 0


def walk_dicts(value: Any) -> Iterable[dict[str, Any]]:
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk_dicts(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk_dicts(child)


def event_type(event: dict[str, Any]) -> str:
    value = event.get("type", event.get("event", ""))
    return value if isinstance(value, str) else ""


def unique_strings(values: Iterable[str]) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for value in values:
        value = value.strip()
        if not value or value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


TEXT_KEYS = ("text", "message", "summary", "final_response")

# The SDK serializes Pydantic fields with their wire aliases by default (for
# example ``aggregatedOutput``), while older CLI JSONL used snake case.  Accept
# both representations so transcripts retain command output regardless of the
# producer.
OUTPUT_KEYS = ("aggregated_output", "aggregatedOutput", "output")


def command_text(obj: dict[str, Any]) -> str | None:
    """Render an object's `command` field, which the SDK sends as str or argv."""

    command = obj.get("command")
    if isinstance(command, str):
        return command
    if isinstance(command, list) and all(isinstance(part, str) for part in command):
        return " ".join(command)
    return None


def record_execution(item: dict[str, Any], stats: SessionStats) -> None:
    """Keep a commandExecution item's command and output paired by item id."""

    if item.get("type") != "commandExecution":
        return
    item_id = item.get("id")
    if not isinstance(item_id, str) or not item_id:
        return
    command = command_text(item)
    execution = stats.executions.setdefault(item_id, CommandExecution())
    if command:
        execution.command = command
    for key in OUTPUT_KEYS:
        value = item.get(key)
        if isinstance(value, str) and value.strip():
            execution.output = value
            break
    exit_code = item.get("exitCode", item.get("exit_code"))
    if isinstance(exit_code, int) and not isinstance(exit_code, bool):
        execution.exit_code = exit_code
    status = item.get("status")
    if isinstance(status, str) and status:
        execution.status = status


def collect_event(event: dict[str, Any], stats: SessionStats) -> None:
    """Harvest item types, commands, texts, outputs and usage in one walk.

    A single ``turn.completed`` notification embeds every item and tool output
    of the session, so traversing it once per key class is the dominant cost of
    reading a run.
    """

    seen_usage: set[int] = set()
    item_types: set[str] = set()
    for obj in walk_dicts(event):
        item = obj.get("item")
        if isinstance(item, dict) and isinstance(item.get("type"), str):
            # Counted once per event, however deeply the item is nested.
            item_types.add(item["type"])
            record_execution(item, stats)

        command = command_text(obj)
        if command:
            stats.commands.append(command)

        for key in TEXT_KEYS:
            value = obj.get(key)
            if isinstance(value, str) and value.strip():
                stats.texts.append(value)

        for key in OUTPUT_KEYS:
            value = obj.get(key)
            if isinstance(value, str) and value.strip():
                stats.outputs.append(value)

        usage = obj.get("usage")
        if isinstance(usage, dict) and id(usage) not in seen_usage:
            seen_usage.add(id(usage))
            for field in TOKEN_FIELDS:
                value = usage.get(field)
                if isinstance(value, (int, float)) and not isinstance(value, bool):
                    # Each usage record is a cumulative thread total, not a
                    # delta, so keep the largest one seen. Summing would
                    # multiply the run's token count by however many snapshots
                    # happen to be present.
                    stats.usage[field] = max(stats.usage[field], int(value))
    stats.item_types.update(item_types)


def read_events(path: pathlib.Path) -> tuple[list[dict[str, Any]], int]:
    events: list[dict[str, Any]] = []
    invalid = 0
    if not path.exists():
        return events, invalid
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        if not line.strip():
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            invalid += 1
            continue
        if isinstance(value, dict):
            events.append(value)
        else:
            invalid += 1
    return events, invalid


def read_session(run_dir: pathlib.Path) -> SessionStats:
    events, invalid = read_events(run_dir / SESSION_LOG)
    stats = SessionStats(events=len(events), invalid_lines=invalid)
    for event in events:
        kind = event_type(event)
        if kind:
            stats.event_types[kind] += 1
        collect_event(event, stats)
    stats.commands = unique_strings(stats.commands)
    stats.texts = unique_strings(stats.texts)
    stats.outputs = unique_strings(stats.outputs)
    prompt_path = run_dir / SESSION_PROMPT
    if prompt_path.exists():
        stats.prompt_chars = len(prompt_path.read_text(encoding="utf-8", errors="replace"))
    exit_path = run_dir / SESSION_EXIT
    if exit_path.exists():
        try:
            stats.exit_code = int(exit_path.read_text(encoding="utf-8").strip())
        except ValueError:
            stats.exit_code = None
    return stats


# Cached: three report sections ask for the same run's metadata, and by the
# time the analyzer runs the file is no longer being written.
@functools.lru_cache(maxsize=None)
def read_metadata(run_dir: pathlib.Path) -> dict[str, str]:
    return load_metadata(run_dir)


def read_text(path: pathlib.Path) -> str:
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8", errors="replace").strip()


def optional_text(path: pathlib.Path) -> str | None:
    return None if not path.exists() else read_text(path)


def command_words(command: str) -> list[str]:
    try:
        return shlex.split(command)
    except ValueError:
        return command.split()


def planr_actions(command: str, config: dict[str, Any] | None = None) -> list[str]:
    """Return configured tool actions found in one shell command.

    The shell-wrapper recursion is deliberately fixed here: it describes how
    Codex reports commands, not how the tool under test is named. Everything
    about the executable and its subcommand groups comes from the config.
    """

    config = effective_harness_config(config)
    observe = config["observe"]
    executable = tool_name(config)
    known_actions = set(observe["actions"])
    groups = observe["groups"]
    groups_by_command = {
        group["command"]: (name, set(group["actions"]))
        for name, group in groups.items()
    }
    words = command_words(command)
    actions: list[str] = []
    for index, word in enumerate(words):
        if pathlib.PurePosixPath(word).name == executable:
            # `command -v <tool>` is a discoverability check, not an
            # invocation of the tool.
            if index > 0 and words[index - 1] in {"-v", "which", "type"}:
                continue
            if index + 1 >= len(words):
                actions.append("help")
                continue
            action = words[index + 1]
            if action.startswith("-"):
                actions.append("help")
            elif action in groups_by_command:
                group_name, group_actions = groups_by_command[action]
                if index + 2 < len(words):
                    phase_action = words[index + 2]
                    actions.append(
                        f"{group_name} {phase_action}"
                        if phase_action in group_actions
                        else f"{group_name} help"
                    )
                else:
                    actions.append(action)
            elif action in known_actions:
                actions.append(action)
            # A shell operator or another command's argument immediately
            # after the executable is not a tool subcommand.
            elif action in {"&&", "||", ";", "|"}:
                actions.append("help")
            else:
                continue
            continue
        # Codex command events normally wrap the actual shell command as a
        # shell `-c` argument. Inspect it recursively so the report sees the
        # same tool calls the agent saw.
        if word.lstrip("-") in {"c", "lc", "ic", "ilc", "cl"} and index + 1 < len(words):
            actions.extend(planr_actions(words[index + 1], config))
    if actions:
        return unique_strings(actions)
    # Keep a fallback for command renderings that are not shell-parseable.
    pattern = re.compile(
        rf"(?:^|[;&|]\s*)(?:\./|[^\s/]+/)?{re.escape(executable)}(?:\s+([a-z-]+))?"
    )
    for match in pattern.finditer(command):
        prefix = command[: match.start()]
        if re.search(r"(?:command|which|type)\s+-?v\s*$", prefix):
            continue
        action = match.group(1) or "help"
        if action.startswith("--"):
            action = "help"
        elif action not in known_actions and action not in groups_by_command:
            continue
        if action in groups_by_command:
            group_name, group_actions = groups_by_command[action]
            phase_match = re.match(r"\s+([a-z-]+)", command[match.end() :])
            if phase_match and phase_match.group(1) in group_actions:
                action = f"{group_name} {phase_match.group(1)}"
            else:
                action = f"{group_name} help"
        actions.append(action)
    return unique_strings(actions)


def planr_commands(session: SessionStats, config: dict[str, Any] | None = None) -> list[str]:
    """The planr invocations the agent actually made, counted once each.

    ``session.commands`` holds both the `/bin/zsh -lc '...'` wrapper and the
    inner command it carries, so counting actions over it reported every planr
    call twice. The per-item executions carry one entry per real invocation.
    """

    config = effective_harness_config(config)
    if session.executions:
        commands = [item.command for item in session.executions.values() if item.command]
    else:
        commands = session.commands
    return [command for command in commands if planr_actions(command, config)]


def parse_overview_statuses(
    value: str, config: dict[str, Any] | None = None
) -> list[dict[str, Any]]:
    """Normalize the configured JSON completion payload for the report."""

    config = effective_harness_config(config)
    completion = config["completion"]
    try:
        payload = json.loads(value)
    except json.JSONDecodeError:
        return []
    if not isinstance(payload, dict):
        return []
    plans = payload.get(completion["plans_key"])
    if not isinstance(plans, list):
        return []
    fields = completion["fields"]
    display_name = completion["display_name"]
    result: list[dict[str, Any]] = []
    for plan in plans:
        if not isinstance(plan, dict):
            continue
        name = plan.get(fields["name"])
        display_value = plan.get(fields[display_name["field"]])
        status = plan.get(fields["status"])
        done = plan.get(fields["done"])
        total = plan.get(fields["total"])
        if not isinstance(name, str) or not isinstance(display_value, str) or not isinstance(status, str):
            continue
        if not isinstance(done, int) or isinstance(done, bool):
            continue
        if not isinstance(total, int) or isinstance(total, bool):
            continue
        rendered_name = (
            pathlib.PurePosixPath(display_value).name
            if display_name["basename"]
            else display_value
        )
        result.append(
            {
                "name": rendered_name,
                "status": status,
                "done": done,
                "total": total,
            }
        )
    return result


def completion_is_complete(statuses: list[dict[str, Any]], config: dict[str, Any]) -> bool:
    complete_when = config["completion"]["complete_when"]
    return bool(statuses) and all(
        item.get(complete_when["field"]) == complete_when["equals"] for item in statuses
    )


def planr_event_lines(
    run_dir: pathlib.Path, config: dict[str, Any] | None = None
) -> list[str]:
    config = effective_harness_config(config)
    # The workspace lives outside run_dir so the agent cannot see the run's
    # artifacts; metadata.env records where it was put.
    workspace = read_metadata(run_dir).get("workspace", "")
    if not workspace:
        return []
    path = pathlib.Path(workspace) / config["signals"]["event_log"]
    if not path.exists():
        return []
    return [line for line in path.read_text(encoding="utf-8", errors="replace").splitlines() if line]


def output_tokens(session: SessionStats) -> int:
    return session.usage.get("output_tokens", 0)


def usage_total(usage: dict[str, int]) -> int:
    """Total tokens, falling back to input+output when the SDK omits it."""

    if usage.get("total_tokens"):
        return usage["total_tokens"]
    return usage.get("input_tokens", 0) + usage.get("output_tokens", 0)


def total_tokens(session: SessionStats) -> int:
    return usage_total(session.usage)


def ratio(part: int, whole: int) -> float | None:
    return None if not whole else round(part / whole, 4)


def token_shares(tokens: dict[str, int]) -> dict[str, float | None]:
    """Token ratios worth comparing across runs.

    Each is a share of its parent, matching how the SDK nests the counts:
    cached input inside input, reasoning output inside output.
    """

    input_tokens = tokens.get("input_tokens", 0)
    output_tokens = tokens.get("output_tokens", 0)
    total = tokens.get("total_tokens") or (input_tokens + output_tokens)
    return {
        "input_of_total": ratio(input_tokens, total),
        "output_of_total": ratio(output_tokens, total),
        "cached_of_input": ratio(tokens.get("cached_input_tokens", 0), input_tokens),
        "reasoning_of_output": ratio(tokens.get("reasoning_output_tokens", 0), output_tokens),
    }


def excerpt_text(value: str, limit: int = 8000) -> str:
    return value if len(value) <= limit else value[:limit] + "\n[…truncated…]"


def build_transcript(
    run_dir: pathlib.Path,
    session: SessionStats,
    config: dict[str, Any] | None = None,
) -> str:
    config = effective_harness_config(config)
    executable = tool_name(config)
    metadata = read_metadata(run_dir)
    lines = [
        f"# Codex {executable} harness transcript",
        "",
        f"- Model: `{metadata.get('model', 'unknown')}`",
        f"- Reasoning effort: `{metadata.get('reasoning', 'unknown')}`",
        "- Raw events: `session.jsonl`",
        f"- exit code: `{session.exit_code if session.exit_code is not None else 'unknown'}`",
        f"- events: `{session.events}`",
        f"- token total: `{total_tokens(session) or 'unavailable'}`",
        "",
        "이 파일은 JSONL 이벤트에서 추출한 명령과 에이전트 텍스트입니다. 정확한 원문과"
        " 도구 입력은 `session.jsonl`을 확인하세요.",
        "",
    ]
    executions = list(session.executions.values())
    if executions:
        lines.extend(["## Command executions", ""])
        for number, execution in enumerate(executions, start=1):
            heading = f"### {number}. exit {execution.exit_code}" if execution.exit_code is not None else f"### {number}. {execution.status or 'no exit code'}"
            lines.extend([heading, "", "```text", execution.command or "(command unavailable)", "```", ""])
            if execution.output.strip():
                lines.extend(["```text", excerpt_text(execution.output), "```", ""])
            else:
                lines.extend(["(출력 없음)", ""])
    elif session.commands:
        # Older JSONL without per-item ids: fall back to the unpaired listing
        # rather than dropping the commands from the transcript.
        lines.extend(["## Commands", "", "```text", *session.commands, "```", ""])
        if session.outputs:
            lines.extend(["## Tool outputs (명령과 짝지어지지 않음)", "", "```text"])
            for output in session.outputs:
                lines.extend([excerpt_text(output), "---"])
            lines.extend(["```", ""])
    if session.texts:
        lines.extend(["## Agent messages", ""])
        for text in session.texts:
            lines.extend([excerpt_text(text), "", "---", ""])
    else:
        lines.extend(["(추출 가능한 에이전트 메시지가 없습니다.)", ""])
    return "\n".join(lines).rstrip() + "\n"


# The fixed patterns catch common shell and Go failure shapes. The configured
# patterns add errors whose format belongs to the tool under evaluation.
# `--- FAIL:` names the failing test, which is what a reader needs -- the bare
# `FAIL` summary lines `go test` ends with say nothing, so they are deliberately
# not matched.
ERROR_LINE = re.compile(
    # `\s*` on the file:line form because `go test` indents its failure detail.
    r"^(fatal|error|panic)\b|^\s*--- FAIL:|^\s*\S+:\d+: |\berror\b",
    re.IGNORECASE,
)

def error_excerpt(
    output: str, limit: int = 3, config: dict[str, Any] | None = None
) -> str:
    """The lines of a failed command's output that explain the failure.

    Agents chain steps with `;`, so the tail of the output is often the
    downstream fallout ("plan not found") rather than the root cause ("NEXT
    description must not be empty") that scrolled past above it. Prefer lines
    that look like error messages, oldest first, and fall back to the tail when
    nothing matches.
    """

    config = effective_harness_config(config)
    configured_patterns = [
        re.compile(item["pattern"], re.IGNORECASE)
        for item in config["signals"]["error_patterns"]
    ]
    lines = [line.rstrip() for line in output.splitlines() if line.strip()]
    errors = [
        line
        for line in lines
        if ERROR_LINE.search(line) or any(pattern.search(line) for pattern in configured_patterns)
    ]
    return "\n".join(errors[:limit] if errors else lines[-limit:])


def failed_executions(
    session: SessionStats, config: dict[str, Any] | None = None
) -> list[dict[str, Any]]:
    """Commands that ended non-zero, numbered as in the transcript.

    A failed command is the clearest evidence that the tool or its
    documentation misled the agent, so the report names each one instead of
    leaving it buried in the transcript.
    """

    config = effective_harness_config(config)
    failures: list[dict[str, Any]] = []
    for number, execution in enumerate(session.executions.values(), start=1):
        if execution.exit_code in (None, 0):
            continue
        failures.append(
            {
                "index": number,
                "command": execution.command,
                "exit_code": execution.exit_code,
                "planr_actions": planr_actions(execution.command, config) if execution.command else [],
                "error": error_excerpt(execution.output, config=config),
            }
        )
    return failures


# A path is absolute only when the slash does not continue a word or a
# relative prefix.  Excluding `.` and `*` is what keeps `./...`, `../x` and the
# glob `**/.git/**` from being read as absolute paths -- they appear in almost
# every Go or ripgrep command, so treating them as absolute made the
# "outside the workspace" signal fire on every single run.
ABSOLUTE_PATH = re.compile(r"(?<![A-Za-z0-9_.*])/(?:[^\s'\";&|]|\\ )+")

# Interpreters, system binaries and scratch space are not workspace escapes.
ALLOWED_PATH_PREFIXES = ("/bin/", "/usr/", "/sbin/", "/private/tmp/", "/tmp/", "/dev/")


def find_paths_outside(commands: Iterable[str], workspace: str) -> list[str]:
    """Return absolute paths a command referenced outside the isolated workspace."""

    outside = []
    for command in commands:
        for path in ABSOLUTE_PATH.findall(command):
            if path.startswith(ALLOWED_PATH_PREFIXES):
                continue
            if not path.startswith(workspace) and "/.codex/" not in path:
                outside.append(path)
    return outside


def repeated_commands(commands: Iterable[str]) -> list[tuple[str, int]]:
    counts = collections.Counter(" ".join(command.split()) for command in commands)
    return sorted(((command, count) for command, count in counts.items() if count >= 3), key=lambda item: (-item[1], item[0]))


def make_observations(
    run_dir: pathlib.Path,
    session: SessionStats,
    commands: list[str],
    statuses: list[dict[str, Any]],
    final_test_exit: str,
    failures: list[dict[str, Any]] | None = None,
    checks: list[dict[str, str]] | None = None,
    config: dict[str, Any] | None = None,
) -> dict[str, list[str]]:
    config = effective_harness_config(config)
    executable = tool_name(config)
    actions = collections.Counter(
        action for command in commands for action in planr_actions(command, config)
    )
    workflow: list[str] = []
    documentation: list[str] = []
    efficiency: list[str] = []

    if not commands:
        no_commands = config["observe"]["no_commands"]
        workflow.append(no_commands["workflow"].format(tool=executable))
        documentation.append(no_commands["documentation"].format(tool=executable))
    else:
        for expectation in config["observe"]["expectations"]:
            expected_action = expectation["action"]
            if any(
                key == expected_action or key.startswith(expected_action + " ")
                for key in actions
            ):
                continue
            message = expectation["message"].format(
                tool=executable,
                action=expected_action,
                hint=expectation["hint"],
            )
            if expectation["category"] == "workflow":
                workflow.append(message)
            else:
                documentation.append(message)

    outputs = session.outputs
    for warning in config["signals"]["warnings"]:
        values = outputs if warning["source"] == "output" else commands
        pattern = re.compile(warning["pattern"], re.IGNORECASE)
        if not any(pattern.search(value) for value in values):
            continue
        message = warning["message"].format(tool=executable)
        if warning["category"] == "workflow":
            workflow.append(message)
        else:
            documentation.append(message)

    workspace = read_metadata(run_dir).get("workspace", "")
    if workspace:
        outside_paths = find_paths_outside(commands, workspace)
        if outside_paths:
            documentation.append("격리 저장소 밖의 절대 경로를 참조한 명령이 관찰되었습니다. AGENTS.md의 격리 경계를 더 강하게 하거나 허용 범위를 명시하세요.")

    if not statuses:
        workflow.append(config["completion"]["messages"]["no_status"])
    elif completion_is_complete(statuses, config):
        workflow.append(config["completion"]["messages"]["all_complete"])
    else:
        complete_when = config["completion"]["complete_when"]
        pending = ", ".join(
            f"{item['name']}={item['status']}"
            for item in statuses
            if item.get(complete_when["field"]) != complete_when["equals"]
        )
        workflow.append(config["completion"]["messages"]["incomplete"].format(pending=pending))

    # A failed tool call is a documentation/UX signal; a failed shell command
    # around it is a workflow signal. Both matter, but they point at different
    # fixes, so they are reported separately.
    for failure in failures or []:
        location = f"실행 #{failure['index']} (exit {failure['exit_code']})"
        # The first error line is the root cause; later ones are its fallout.
        first_error = one_line(failure["error"].splitlines()[0], limit=160) if failure["error"] else "출력 없음"
        if failure["planr_actions"]:
            actions = ", ".join(f"`{executable} {action}`" for action in failure["planr_actions"])
            documentation.append(
                f"{location}에서 {actions} 호출이 실패했습니다 — “{first_error}”."
                " 명령 형식이나 오류 메시지가 다음 행동을 바로 알려 주는지 확인하세요."
            )
        else:
            workflow.append(f"{location} 명령이 실패했습니다 — “{first_error}”.")

    # The acceptance script judges the requested behaviour, which `go test` does
    # not: the agent writes those tests itself and they can pass while the
    # request goes unmet.
    failed_checks = [check for check in checks or [] if check["result"] == "FAIL" and check["name"] != "summary"]
    if failed_checks:
        listed = ", ".join(f"`{check['name']}`" for check in failed_checks)
        workflow.append(
            f"인수 검사 {len(failed_checks)}건이 실패했습니다: {listed}."
            " 요청한 동작이 실제로 구현되지 않았습니다."
        )
    elif checks:
        workflow.append("인수 검사를 모두 통과했습니다.")

    if final_test_exit and final_test_exit != "0":
        workflow.append(f"하네스 종료 검증 `go test ./...`가 exit {final_test_exit}로 끝났습니다.")
    elif final_test_exit == "0":
        workflow.append("하네스 종료 검증 `go test ./...`가 통과했습니다.")

    for command, count in repeated_commands(commands):
        efficiency.append(f"동일한 명령이 {count}회 반복되었습니다: `{command}`. 상태 확인을 캐시하거나 한 번에 묶을 수 있는지 검토하세요.")

    if not total_tokens(session):
        efficiency.append("Codex 이벤트에서 token usage를 찾지 못했습니다. 원본 JSONL을 보존했으므로 CLI 이벤트 형식에 맞춰 분석기를 확장할 수 있습니다.")
    if session.invalid_lines:
        efficiency.append(f"해석할 수 없는 JSONL 줄이 {session.invalid_lines}개 있습니다.")
    if session.exit_code == 124:
        workflow.append("세션이 타임아웃으로 중단되었습니다. 에이전트가 스스로 완료하지 못했거나 `--timeout`이 과제 대비 짧습니다.")
    elif session.exit_code not in (None, 0):
        workflow.append(f"Codex 세션이 exit {session.exit_code}로 실패했습니다.")

    return {"workflow": workflow, "documentation": documentation, "efficiency": efficiency}


def result_data(
    run_dir: pathlib.Path,
    session: SessionStats,
    config: dict[str, Any] | None = None,
) -> dict[str, Any]:
    config = load_run_harness_config(run_dir) if config is None else config
    metadata = read_metadata(run_dir)
    commands = planr_commands(session, config)
    overview_file = run_dir / config["completion"]["state_file"]
    statuses = parse_overview_statuses(read_text(overview_file), config)
    final_test_exit = read_text(run_dir / STATE_DIR / "final-go-test.exit")
    failures = failed_executions(session, config)
    checks = parse_fixture_checks(read_text(run_dir / STATE_DIR / "final-fixture-test.txt"))
    observations = make_observations(
        run_dir, session, commands, statuses, final_test_exit, failures, checks, config
    )
    result = {
        "metadata": metadata,
        "session": {
            "events": session.events,
            "invalid_lines": session.invalid_lines,
            "exit_code": session.exit_code,
            "tokens": dict(session.usage),
            # Derived here so runs of different sizes can be compared directly.
            "token_shares": token_shares(dict(session.usage)),
            "total_tokens": total_tokens(session),
            "output_tokens": output_tokens(session),
            "prompt_chars": session.prompt_chars,
            "prompt_estimated_tokens": (session.prompt_chars + 3) // 4 if session.prompt_chars else 0,
            "event_types": dict(session.event_types),
            "item_types": dict(session.item_types),
            "commands": session.commands,
            "commands_executed": len(session.executions),
        },
        "planr_commands": commands,
        "failed_commands": failures,
        "planr_events": planr_event_lines(run_dir, config),
        "overview": statuses,
        "final_test_exit": final_test_exit,
        "fixture_test_exit": read_text(run_dir / STATE_DIR / "final-fixture-test.exit"),
        "fixture_checks": checks,
        "plan_documents": plan_documents(run_dir, config),
        "final_git_status": read_text(run_dir / STATE_DIR / "final-git-status.txt"),
        # None (not "") when the capture is absent, so a run recorded before
        # this state file existed is not mistaken for a clean worktree.
        "final_git_status_tracked": optional_text(run_dir / STATE_DIR / "final-git-status-tracked.txt"),
        "final_git_log": read_text(run_dir / STATE_DIR / "final-git-log.txt"),
        "observations": observations,
        "overview_file": config["completion"]["state_file"],
    }
    return result


def plan_documents(
    run_dir: pathlib.Path, config: dict[str, Any] | None = None
) -> list[str]:
    """The plan documents copied out of the workspace, newest layout first."""

    config = effective_harness_config(config)
    plans = run_dir / artifact_directory(config)
    if not plans.is_dir():
        return []
    return sorted(str(path.relative_to(plans)) for path in plans.rglob("*") if path.is_file())


def parse_fixture_checks(output: str) -> list[dict[str, str]]:
    """Read `CHECK<TAB>name<TAB>PASS|FAIL<TAB>detail` lines from a fixture script.

    Anything the script prints that is not a CHECK line is ignored here; the
    full output is kept in state/final-fixture-test.txt.
    """

    checks: list[dict[str, str]] = []
    for line in output.splitlines():
        parts = line.split("\t")
        if len(parts) < 3 or parts[0] != "CHECK":
            continue
        result = parts[2].strip().upper()
        if result not in ("PASS", "FAIL"):
            continue
        checks.append(
            {
                "name": parts[1].strip(),
                "result": result,
                "detail": parts[3].strip() if len(parts) > 3 else "",
            }
        )
    return checks


def fixture_check_rows(checks: list[dict[str, str]], exit_code: str) -> list[str]:
    if not checks:
        if exit_code:
            return [
                f"검사 스크립트가 exit {exit_code}로 끝났지만 CHECK 줄을 찾지 못했습니다."
                " `state/final-fixture-test.txt`를 확인하세요."
            ]
        return ["이 픽스처에는 인수 검사 스크립트가 없습니다."]
    # `summary` is the script's own tally; it is shown as the headline instead
    # of a table row so the per-check rows stay one-to-one with real checks.
    summary = next((check for check in checks if check["name"] == "summary"), None)
    rows = [check for check in checks if check["name"] != "summary"]
    passed = sum(1 for check in rows if check["result"] == "PASS")
    headline = f"{passed}/{len(rows)} 통과"
    if summary and summary["detail"]:
        headline += f" (스크립트 보고: {summary['detail']})"
    lines = [headline + f" · 종료 코드 `{exit_code or 'unknown'}`", "", "| 검사 | 결과 | 비고 |", "| --- | --- | --- |"]
    for check in rows:
        mark = "PASS" if check["result"] == "PASS" else "**FAIL**"
        lines.append(f"| `{check['name']}` | {mark} | {check['detail'] or '—'} |")
    return lines


def worktree_state(data: dict[str, Any]) -> str:
    """Judge the end state by tracked files only.

    An untracked draft is planr's normal output, not work the agent forgot to
    commit; older runs without the tracked-only capture fall back to the full
    status.
    """

    tracked = data.get("final_git_status_tracked")
    if tracked is None:
        tracked = data.get("final_git_status")
    if tracked:
        return "dirty"
    untracked = len([line for line in (data.get("final_git_status") or "").splitlines() if line.strip()])
    if untracked:
        return f"clean (untracked만 {untracked}건)"
    return "clean"


def share(part: int, whole: int) -> str:
    return "—" if not whole else f"{part / whole * 100:.1f}%"


def token_breakdown_rows(tokens: dict[str, int]) -> list[str]:
    """Per-kind token table.

    `cached_input_tokens` and `reasoning_output_tokens` are *subsets* of input
    and output respectively, not sibling buckets, so each is shown as a share of
    its parent. Adding all four together would double-count the run.
    """

    if not tokens:
        return ["토큰 사용량을 읽지 못했습니다."]
    input_tokens = tokens.get("input_tokens", 0)
    output_tokens = tokens.get("output_tokens", 0)
    cached = tokens.get("cached_input_tokens", 0)
    reasoning = tokens.get("reasoning_output_tokens", 0)
    total = tokens.get("total_tokens") or (input_tokens + output_tokens)

    rows = [
        "| 종류 | 토큰 | 비율 |",
        "| --- | ---: | ---: |",
        f"| input | {input_tokens:,} | {share(input_tokens, total)} (전체 대비) |",
        f"| └ cached input | {cached:,} | {share(cached, input_tokens)} (input 대비) |",
        f"| └ uncached input | {input_tokens - cached:,} | {share(input_tokens - cached, input_tokens)} (input 대비) |",
        f"| output | {output_tokens:,} | {share(output_tokens, total)} (전체 대비) |",
        f"| └ reasoning output | {reasoning:,} | {share(reasoning, output_tokens)} (output 대비) |",
        f"| └ 그 외 output | {output_tokens - reasoning:,} | {share(output_tokens - reasoning, output_tokens)} (output 대비) |",
        f"| **합계** | **{total:,}** | 100% |",
    ]
    if total and input_tokens + output_tokens != total:
        rows.extend(
            [
                "",
                f"주의: input+output({input_tokens + output_tokens:,})가 보고된 total({total:,})과"
                " 일치하지 않습니다. SDK가 집계하지 않은 항목이 있을 수 있습니다.",
            ]
        )
    return rows


def markdown_report(
    data: dict[str, Any], run_dir: pathlib.Path, config: dict[str, Any] | None = None
) -> str:
    config = effective_harness_config(config)
    executable = tool_name(config)
    metadata = data.get("metadata", {})
    session = data.get("session", {})
    statuses = data.get("overview", [])
    observations = data.get("observations", {})
    total = session.get("total_tokens", 0)
    output = session.get("output_tokens", 0)
    done = completion_is_complete(statuses, config)
    complete_when = config["completion"]["complete_when"]

    lines = [
        f"# Codex {executable} harness report",
        "",
        "## 실행 요약",
        "",
        f"- Run directory: `{run_dir}`",
        f"- Model: `{metadata.get('model', 'unknown')}`",
        f"- Reasoning effort: `{metadata.get('reasoning', 'unknown')}`",
        f"- Session exit: `{session.get('exit_code', 'unknown')}`",
        f"- Plan completion: **{complete_when['equals'] if done else 'incomplete/unknown'}**",
        f"- Fixture: `{metadata.get('fixture', 'unknown')}`",
        # Document language changes what the agent reads and writes, so runs
        # are only comparable to each other when this line matches.
        f"- Document language: `{metadata.get('language', 'unknown')}`",
        # The two variables an A/B run changes: the request the agent was sent
        # and the AGENTS.md it worked under.
        f"- Prompt variant: `{metadata.get('prompt_variant', 'unknown')}`",
        f"- Instructions variant: `{metadata.get('agents_variant', 'unknown')}`",
        f"- Final `go test ./...`: `{data.get('final_test_exit') or 'not recorded'}`",
        f"- Final Git worktree (tracked files): **{worktree_state(data)}**",
        "",
        "## Plan 상태",
        "",
    ]
    if statuses:
        lines.extend(["| plan | status | phases |", "| --- | --- | --- |"])
        lines.extend(
            f"| {item['name']} | `{item['status']}` | {item['done']}/{item['total']} |"
            for item in statuses
        )
    else:
        lines.append("최종 overview에서 읽을 수 있는 plan 상태가 없습니다.")

    session_tokens = session.get("tokens", {})
    lines.extend(
        [
            "",
            "## 세션·토큰 요약",
            "",
            "| events | prompt≈ | input | output | total |",
            "| ---: | ---: | ---: | ---: | ---: |",
            f"| {session.get('events', 0)} | {session.get('prompt_estimated_tokens') or '—'} | "
            f"{session_tokens.get('input_tokens', '—')} | {session_tokens.get('output_tokens', '—')} | "
            f"{session.get('total_tokens') or '—'} |",
            "",
            f"누적 token: `{total or 'unavailable'}` (output `{output or 'unavailable'}`).",
            "",
            "### 토큰 종류별",
            "",
        ]
    )
    lines.extend(token_breakdown_rows(session_tokens))
    lines.extend(["", "## 도구 사용 관찰", ""])
    if data.get("planr_commands"):
        action_counts = collections.Counter(
            action
            for command in data["planr_commands"]
            for action in planr_actions(command, config)
        )
        lines.append(f"관찰된 {executable} 단계: " + ", ".join(f"`{key}`×{value}" for key, value in action_counts.items()) + ".")
        lines.append("")
        lines.append(f"전체 {executable} 명령:")
        lines.extend(f"- `{command}`" for command in data["planr_commands"])
    else:
        lines.append(f"관찰된 `{executable}` 명령이 없습니다.")

    item_counts = collections.Counter[str](session.get("item_types", {}))
    if item_counts:
        lines.extend(["", "Codex 도구 이벤트: " + ", ".join(f"`{key}`×{value}" for key, value in item_counts.items()) + "."])
    if data.get("planr_events"):
        event_counts = collections.Counter(line.split("|", 1)[0] for line in data["planr_events"])
        lines.extend(["", f"{executable} hook 이벤트: " + ", ".join(f"`{key}`×{value}" for key, value in event_counts.items()) + "."])

    documents = data.get("plan_documents") or []
    lines.extend(["", f"## {executable} 산출 문서", ""])
    if documents:
        artifact_dir = artifact_directory(config)
        lines.append(f"에이전트가 만든 계획 문서 {len(documents)}개를 `{artifact_dir}/`에 보관했습니다.")
        lines.append("")
        lines.extend(f"- `{artifact_dir}/{name}`" for name in documents)
    else:
        lines.append(f"보관된 계획 문서가 없습니다. 에이전트가 {executable} plan을 등록하지 않았을 수 있습니다.")

    checks = data.get("fixture_checks") or []
    if checks or data.get("fixture_test_exit"):
        lines.extend(["", "## 인수 검사", ""])
        lines.extend(fixture_check_rows(checks, data.get("fixture_test_exit", "")))

    failures = data.get("failed_commands") or []
    executed = session.get("commands_executed") or 0
    lines.extend(["", "## 실패한 명령", ""])
    if failures:
        # Tool failures are evidence about the tool under evaluation; the rest
        # are the agent's own shell mistakes. Keeping them in one list makes the
        # tool signal easy to miss, so each group gets its own heading.
        tool_failures = [failure for failure in failures if failure["planr_actions"]]
        other_failures = [failure for failure in failures if not failure["planr_actions"]]
        summary = f"{len(failures)}건 실패"
        if executed:
            summary += f" / 전체 {executed}건"
        summary += f" (`{executable}` {len(tool_failures)}건, 기타 {len(other_failures)}건)"
        lines.extend([summary + ".", ""])
        for heading, group, empty in (
            (f"### `{executable}` 명령 실패", tool_failures, "없음."),
            ("### 기타 명령 실패", other_failures, "없음."),
        ):
            lines.extend([heading, ""])
            if not group:
                lines.extend([empty, ""])
                continue
            for failure in group:
                label = ", ".join(f"{executable} {action}" for action in failure["planr_actions"]) or "shell"
                lines.extend(
                    [
                        f"- **실행 #{failure['index']} · exit {failure['exit_code']} · {label}**",
                        "",
                        "  ```text",
                        *[f"  {line}" for line in (failure["command"] or "(command unavailable)").splitlines()],
                        "  ```",
                        "",
                        "  ```text",
                        *[f"  {line}" for line in (failure["error"] or "(출력 없음)").splitlines()],
                        "  ```",
                        "",
                    ]
                )
        lines.append("전체 출력은 `transcript.md`의 같은 번호 항목에 있습니다.")
    else:
        lines.append("0건. 실행된 모든 명령이 exit 0으로 끝났습니다.")

    for title, key in (
        ("도구/워크플로", "workflow"),
        ("설명/지침", "documentation"),
        ("토큰 효율", "efficiency"),
    ):
        lines.extend(["", f"## {title} 개선 신호", ""])
        values = observations.get(key, [])
        if values:
            lines.extend(f"- {value}" for value in values)
        else:
            lines.append("- 현재 실행에서 뚜렷한 신호가 없습니다.")

    lines.extend(
        [
            "",
            "## 재현·원본 자료",
            "",
            "```sh",
            "python3 planr/scripts/main.py codex analyze <run-directory>",
            "```",
            "",
            "- `transcript.md`: 대화 텍스트·명령 추출본",
            "- `session.jsonl`: Codex 원본 JSONL 이벤트",
            "- `session.prompt.md`: 에이전트에게 전달한 요청",
            "- `state/`: 종료 시점의 overview/status/Git 상태와 종료 검증",
            f"- `{artifact_directory(config)}/`: 에이전트가 만든 {executable} 계획 문서 사본 (워크스페이스는 clean으로 사라짐)",
            "- `metrics.json`: 후속 실행과 비교할 수 있는 구조화 통계",
        ]
    )
    return "\n".join(lines).rstrip() + "\n"


def analyze(run_dir: pathlib.Path, output: pathlib.Path | None = None) -> int:
    run_dir = run_dir.resolve()
    if not run_dir.is_dir():
        raise HarnessError(f"run directory not found: {run_dir}")
    config = load_run_harness_config(run_dir)
    session = read_session(run_dir)
    data = result_data(run_dir, session, config)
    if output is None:
        output = run_dir / "REPORT.md"
    output = output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(markdown_report(data, run_dir, config), encoding="utf-8")
    (run_dir / "transcript.md").write_text(
        build_transcript(run_dir, session, config), encoding="utf-8"
    )
    (run_dir / "metrics.json").write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(output)
    return 0
