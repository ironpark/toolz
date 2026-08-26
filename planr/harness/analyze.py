#!/usr/bin/env python3
"""Summarize a Codex/planr harness run without external Python packages."""

from __future__ import annotations

import argparse
import collections
import dataclasses
import functools
import json
import pathlib
import re
import shlex
import sys
from typing import Any, Iterable


TOKEN_FIELDS = (
    "input_tokens",
    "cached_input_tokens",
    "output_tokens",
    "reasoning_output_tokens",
    "total_tokens",
)

PLANR_ACTIONS = {"new", "add", "status", "overview", "phase"}
PHASE_ACTIONS = {"add", "set", "update", "start", "done", "reset"}


@dataclasses.dataclass
class TurnStats:
    number: int
    path: pathlib.Path
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


def collect_event(event: dict[str, Any], stats: TurnStats) -> None:
    """Harvest item types, commands, texts, outputs and usage in one walk.

    A single ``turn.completed`` notification embeds every item and tool output
    of the turn, so traversing it once per key class is the dominant cost of
    reading a run.
    """

    seen_usage: set[int] = set()
    item_types: set[str] = set()
    for obj in walk_dicts(event):
        item = obj.get("item")
        if isinstance(item, dict) and isinstance(item.get("type"), str):
            # Counted once per event, however deeply the item is nested.
            item_types.add(item["type"])

        command = obj.get("command")
        if isinstance(command, str):
            stats.commands.append(command)
        elif isinstance(command, list) and all(isinstance(part, str) for part in command):
            stats.commands.append(" ".join(command))

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
                    stats.usage[field] += int(value)
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


def read_turn(path: pathlib.Path, number: int) -> TurnStats:
    events, invalid = read_events(path)
    stats = TurnStats(number=number, path=path, events=len(events), invalid_lines=invalid)
    for event in events:
        kind = event_type(event)
        if kind:
            stats.event_types[kind] += 1
        collect_event(event, stats)
    stats.commands = unique_strings(stats.commands)
    stats.texts = unique_strings(stats.texts)
    stats.outputs = unique_strings(stats.outputs)
    prompt_path = path.with_suffix(".prompt.md")
    if prompt_path.exists():
        stats.prompt_chars = len(prompt_path.read_text(encoding="utf-8", errors="replace"))
    exit_path = path.with_suffix(".exit")
    if exit_path.exists():
        try:
            stats.exit_code = int(exit_path.read_text(encoding="utf-8").strip())
        except ValueError:
            stats.exit_code = None
    return stats


def session_id_from_events(events: Iterable[dict[str, Any]]) -> str:
    preferred_types = {"thread.started", "session.started", "conversation.started"}
    candidates: list[tuple[int, str]] = []
    for event in events:
        kind = event_type(event)
        priority = 0 if kind in preferred_types else 1
        for obj in walk_dicts(event):
            for key in ("thread_id", "session_id", "conversation_id"):
                value = obj.get(key)
                if isinstance(value, str) and value.strip():
                    candidates.append((priority, value.strip()))
    if not candidates:
        return ""
    candidates.sort(key=lambda item: item[0])
    return candidates[0][1]


@functools.lru_cache(maxsize=None)
def read_metadata(run_dir: pathlib.Path) -> dict[str, str]:
    metadata: dict[str, str] = {}
    path = run_dir / "metadata.env"
    if not path.exists():
        return metadata
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        metadata[key.strip()] = value
    return metadata


def turn_files(run_dir: pathlib.Path) -> list[pathlib.Path]:
    paths = list((run_dir / "turns").glob("turn-*.jsonl"))
    return sorted(paths, key=lambda path: path.name)


def read_text(path: pathlib.Path) -> str:
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8", errors="replace").strip()


def command_words(command: str) -> list[str]:
    try:
        return shlex.split(command)
    except ValueError:
        return command.split()


# Cached: the same command strings are re-scanned by planr_commands,
# make_observations and markdown_report, and parsing is regex/shlex heavy.
@functools.lru_cache(maxsize=None)
def planr_actions(command: str) -> list[str]:
    words = command_words(command)
    actions: list[str] = []
    for index, word in enumerate(words):
        if pathlib.PurePosixPath(word).name == "planr":
            # `command -v planr` is a discoverability check, not an
            # invocation of the tool.
            if index > 0 and words[index - 1] in {"-v", "which", "type"}:
                continue
            if index + 1 >= len(words):
                actions.append("help")
                continue
            action = words[index + 1]
            if action.startswith("-"):
                actions.append("help")
            elif action in PLANR_ACTIONS:
                if action == "phase" and index + 2 < len(words):
                    phase_action = words[index + 2]
                    actions.append(
                        f"phase {phase_action}"
                        if phase_action in PHASE_ACTIONS
                        else "phase help"
                    )
                else:
                    actions.append(action)
            # A shell operator or another command's argument immediately
            # after the word `planr` is not a planr subcommand.
            elif action in {"&&", "||", ";", "|"}:
                actions.append("help")
            else:
                continue
            continue
        # Codex command events normally wrap the actual shell command as
        # `/bin/zsh -lc 'planr ...'`. Inspect the -c argument recursively so
        # the report sees the same planr calls the agent saw.
        if word.lstrip("-") in {"c", "lc", "ic", "ilc", "cl"} and index + 1 < len(words):
            actions.extend(planr_actions(words[index + 1]))
    if actions:
        return unique_strings(actions)
    # Keep a fallback for command renderings that are not shell-parseable.
    pattern = re.compile(r"(?:^|[;&|]\s*)(?:\./|[^\s/]+/)?planr(?:\s+([a-z-]+))?")
    for match in pattern.finditer(command):
        prefix = command[: match.start()]
        if re.search(r"(?:command|which|type)\s+-?v\s*$", prefix):
            continue
        action = match.group(1) or "help"
        if action.startswith("--"):
            action = "help"
        elif action not in PLANR_ACTIONS:
            continue
        if action == "phase":
            phase_match = re.match(r"\s*phase\s+([a-z-]+)", command[match.end() :])
            if phase_match and phase_match.group(1) in PHASE_ACTIONS:
                action = f"phase {phase_match.group(1)}"
            else:
                action = "phase help"
        actions.append(action)
    return unique_strings(actions)


def planr_commands(turns: Iterable[TurnStats]) -> list[str]:
    commands: list[str] = []
    for turn in turns:
        for command in turn.commands:
            if planr_actions(command):
                commands.append(command)
    return commands


def parse_overview_statuses(value: str) -> list[dict[str, Any]]:
    pattern = re.compile(
        r"^\s{2}(.+?):\s+(done|in-progress|planned|conditional|unknown)\s+"
        r"\((\d+)/(\d+) phases(?: done)?\)"
    )
    result: list[dict[str, Any]] = []
    for line in value.splitlines():
        match = pattern.match(line)
        if not match:
            continue
        result.append(
            {
                "name": match.group(1),
                "status": match.group(2),
                "done": int(match.group(3)),
                "total": int(match.group(4)),
            }
        )
    return result


def latest_overview(run_dir: pathlib.Path) -> tuple[str, pathlib.Path | None]:
    paths = sorted((run_dir / "state").glob("turn-*-overview.txt"))
    if paths:
        return read_text(paths[-1]), paths[-1]
    final = run_dir / "state" / "final-overview.txt"
    return read_text(final), final if final.exists() else None


def planr_event_lines(run_dir: pathlib.Path) -> list[str]:
    path = run_dir / "repo" / ".harness" / "planr-events.log"
    if not path.exists():
        return []
    return [line for line in path.read_text(encoding="utf-8", errors="replace").splitlines() if line]


def all_done_by_turn(run_dir: pathlib.Path) -> list[bool]:
    result: list[bool] = []
    for path in sorted((run_dir / "state").glob("turn-*-overview.txt")):
        statuses = parse_overview_statuses(read_text(path))
        result.append(bool(statuses) and all(item["status"] == "done" for item in statuses))
    return result


def output_tokens(turn: TurnStats) -> int:
    return turn.usage.get("output_tokens", 0)


def total_tokens(turn: TurnStats) -> int:
    if turn.usage.get("total_tokens"):
        return turn.usage["total_tokens"]
    return turn.usage.get("input_tokens", 0) + turn.usage.get("output_tokens", 0)


def build_transcript(run_dir: pathlib.Path, turns: list[TurnStats]) -> str:
    metadata = read_metadata(run_dir)
    lines = [
        "# Codex planr harness transcript",
        "",
        f"- Model: `{metadata.get('model', 'unknown')}`",
        f"- Reasoning effort: `{metadata.get('reasoning', 'unknown')}`",
        "- Raw events: `turns/turn-*.jsonl`",
        "",
        "이 파일은 JSONL 이벤트에서 추출한 명령과 에이전트 텍스트입니다. 정확한 원문과"
        " 도구 입력은 각 turn의 JSONL 로그를 확인하세요.",
        "",
    ]
    for turn in turns:
        lines.extend(
            [
                f"## Turn {turn.number + 1}",
                "",
                f"- exit code: `{turn.exit_code if turn.exit_code is not None else 'unknown'}`",
                f"- events: `{turn.events}`",
                f"- token total: `{total_tokens(turn) or 'unavailable'}`",
                "",
            ]
        )
        if turn.commands:
            lines.extend(["### Commands", "", "```text", *turn.commands, "```", ""])
        if turn.outputs:
            lines.extend(["### Tool outputs", "", "```text"])
            for output in turn.outputs:
                excerpt = output if len(output) <= 8000 else output[:8000] + "\n[…truncated…]"
                lines.extend([excerpt, "---"])
            lines.extend(["```", ""])
        if turn.texts:
            lines.extend(["### Agent messages", ""])
            for text in turn.texts:
                excerpt = text if len(text) <= 8000 else text[:8000] + "\n[…truncated…]"
                lines.extend([excerpt, "", "---", ""])
        else:
            lines.extend(["(추출 가능한 에이전트 메시지가 없습니다.)", ""])
    return "\n".join(lines).rstrip() + "\n"


def repeated_commands(commands: Iterable[str]) -> list[tuple[str, int]]:
    counts = collections.Counter(" ".join(command.split()) for command in commands)
    return sorted(((command, count) for command, count in counts.items() if count >= 3), key=lambda item: (-item[1], item[0]))


def make_observations(
    run_dir: pathlib.Path,
    turns: list[TurnStats],
    commands: list[str],
    statuses: list[dict[str, Any]],
    final_test_exit: str,
) -> dict[str, list[str]]:
    actions = collections.Counter(
        action for command in commands for action in planr_actions(command)
    )
    workflow: list[str] = []
    documentation: list[str] = []
    efficiency: list[str] = []

    if not commands:
        workflow.append("대화에서 `planr` 명령 호출이 관찰되지 않았습니다. 도구가 PATH에 없거나 AGENTS.md의 사용 지침이 실행 흐름에서 충분히 드러나지 않았을 수 있습니다.")
        documentation.append("AGENTS.md의 planr 사용 지침이 관찰된 도구 호출로 이어지지 않았습니다. 지침의 위치·표현·필수성 또는 CLI 발견성을 개선할 수 있습니다.")
    else:
        for action, hint in (
            ("new", "초안 생성"),
            ("add", "등록"),
            ("status", "상세 상태 확인"),
            ("overview", "요약 상태 확인"),
        ):
            if not any(key == action or key.startswith(action + " ") for key in actions):
                documentation.append(f"`planr {action}` ({hint}) 호출이 없습니다. 해당 단계의 설명이나 명령 발견성이 약한지 확인하세요.")
        if not any(key.startswith("phase start") for key in actions):
            documentation.append("phase 시작 명령이 관찰되지 않았습니다. phase 라이프사이클 안내가 충분히 구체적인지 확인하세요.")
        if not any(key.startswith("phase done") for key in actions):
            workflow.append("phase 완료 명령이 관찰되지 않았습니다. 계획을 실제 완료 상태로 연결하지 못했을 가능성이 있습니다.")

    outputs = [output for turn in turns for output in turn.outputs]
    if any(
        "cannot mark phase done while source changes are uncommitted" in output.lower()
        or "cannot check uncommitted source changes" in output.lower()
        for output in outputs
    ):
        documentation.append("`phase done`의 소스 커밋 전제에서 오류/경고가 발생했습니다. 커밋 순서와 `--force` 사용 경계가 실제 작업 흐름에 맞는지 검토하세요.")
    if any(
        re.search(r"\bplanr\b.*\bphase\s+done\b.*--force", command, re.IGNORECASE)
        for command in commands
    ):
        workflow.append("phase 완료 과정에서 `--force`가 언급되었습니다. 소스 검증을 우회하지 않고 완료할 수 있었는지 확인하세요.")

    workspace = read_metadata(run_dir).get("workspace", "")
    if workspace:
        outside_paths = []
        for command in commands:
            for path in re.findall(r"(?<![A-Za-z0-9_])/(?:[^\s'\";&|]|\\ )+", command):
                if path.startswith(("/bin/", "/usr/", "/sbin/", "/private/tmp/", "/tmp/", "/dev/")):
                    continue
                if not path.startswith(workspace) and "/.codex/" not in path:
                    outside_paths.append(path)
        if outside_paths:
            documentation.append("격리 저장소 밖의 절대 경로를 참조한 명령이 관찰되었습니다. AGENTS.md의 격리 경계를 더 강하게 하거나 허용 범위를 명시하세요.")

    if not statuses:
        workflow.append("최종 overview에서 plan 상태를 읽지 못했습니다. 계획을 만들지 못했거나 출력 형식이 분석기와 맞지 않습니다.")
    elif all(item["status"] == "done" for item in statuses):
        workflow.append("최종 overview의 모든 plan이 done입니다.")
    else:
        pending = ", ".join(f"{item['name']}={item['status']}" for item in statuses if item["status"] != "done")
        workflow.append(f"완료되지 않은 plan이 남아 있습니다: {pending}.")

    if final_test_exit and final_test_exit != "0":
        workflow.append(f"하네스 종료 검증 `go test ./...`가 exit {final_test_exit}로 끝났습니다.")
    elif final_test_exit == "0":
        workflow.append("하네스 종료 검증 `go test ./...`가 통과했습니다.")

    for command, count in repeated_commands(commands):
        efficiency.append(f"동일한 명령이 {count}회 반복되었습니다: `{command}`. 상태 확인을 캐시하거나 한 번에 묶을 수 있는지 검토하세요.")

    completion_by_turn = all_done_by_turn(run_dir)
    first_complete = next((index for index, complete in enumerate(completion_by_turn) if complete), None)
    if first_complete is not None and any(completion_by_turn[first_complete + 1 :]):
        extra = len(completion_by_turn) - first_complete - 1
        efficiency.append(f"Turn {first_complete + 1}에서 이미 모든 plan이 done으로 보인 뒤 {extra}개 turn이 더 실행되었습니다. 종료 조건을 추가하면 토큰을 절약할 수 있습니다.")

    if len(turns) >= 2:
        idle_turns = 0
        for previous, current in zip(turns, turns[1:]):
            if total_tokens(current) and not current.commands and not current.texts:
                idle_turns += 1
            elif total_tokens(current) and output_tokens(current) < 20 and not current.commands:
                idle_turns += 1
        if idle_turns:
            efficiency.append(f"실질적인 명령/메시지가 거의 없는 turn이 {idle_turns}개 있습니다. 멀티턴 횟수 또는 후속 프롬프트를 줄일 수 있습니다.")

    usage_available = any(total_tokens(turn) for turn in turns)
    if not usage_available:
        efficiency.append("Codex 이벤트에서 token usage를 찾지 못했습니다. 원본 JSONL을 보존했으므로 CLI 이벤트 형식에 맞춰 분석기를 확장할 수 있습니다.")

    for turn in turns:
        if turn.invalid_lines:
            efficiency.append(f"Turn {turn.number + 1}에 해석할 수 없는 JSONL 줄이 {turn.invalid_lines}개 있습니다.")
        if turn.exit_code not in (None, 0):
            workflow.append(f"Turn {turn.number + 1}의 Codex 실행이 exit {turn.exit_code}로 실패했습니다.")

    return {"workflow": workflow, "documentation": documentation, "efficiency": efficiency}


def result_data(run_dir: pathlib.Path, turns: list[TurnStats]) -> dict[str, Any]:
    metadata = read_metadata(run_dir)
    commands = planr_commands(turns)
    overview, overview_path = latest_overview(run_dir)
    statuses = parse_overview_statuses(overview)
    final_test_exit = read_text(run_dir / "state" / "final-go-test.exit")
    observations = make_observations(run_dir, turns, commands, statuses, final_test_exit)
    usage = collections.Counter[str]()
    for turn in turns:
        usage.update(turn.usage)
    result = {
        "metadata": metadata,
        "turns": [
            {
                "number": turn.number + 1,
                "events": turn.events,
                "invalid_lines": turn.invalid_lines,
                "exit_code": turn.exit_code,
                "tokens": dict(turn.usage),
                "total_tokens": total_tokens(turn),
                "output_tokens": output_tokens(turn),
                "prompt_chars": turn.prompt_chars,
                "prompt_estimated_tokens": (turn.prompt_chars + 3) // 4 if turn.prompt_chars else 0,
                "event_types": dict(turn.event_types),
                "item_types": dict(turn.item_types),
                "commands": turn.commands,
            }
            for turn in turns
        ],
        "tokens": dict(usage),
        "planr_commands": commands,
        "planr_events": planr_event_lines(run_dir),
        "overview": statuses,
        "final_test_exit": final_test_exit,
        "final_git_status": read_text(run_dir / "state" / "final-git-status.txt"),
        "final_git_log": read_text(run_dir / "state" / "final-git-log.txt"),
        "observations": observations,
        "overview_file": str(overview_path.relative_to(run_dir)) if overview_path else None,
    }
    return result


def markdown_report(data: dict[str, Any], run_dir: pathlib.Path) -> str:
    metadata = data.get("metadata", {})
    turns = data.get("turns", [])
    statuses = data.get("overview", [])
    tokens = data.get("tokens", {})
    observations = data.get("observations", {})
    total = tokens.get("total_tokens") or sum(turn.get("total_tokens", 0) for turn in turns)
    output = tokens.get("output_tokens") or sum(turn.get("output_tokens", 0) for turn in turns)
    done = bool(statuses) and all(item.get("status") == "done" for item in statuses)

    lines = [
        "# Codex planr harness report",
        "",
        "## 실행 요약",
        "",
        f"- Run directory: `{run_dir}`",
        f"- Model: `{metadata.get('model', 'unknown')}`",
        f"- Reasoning effort: `{metadata.get('reasoning', 'unknown')}`",
        f"- Requested turns: `{metadata.get('turns_requested', 'unknown')}`",
        f"- Recorded turns: `{len(turns)}`",
        f"- Plan completion: **{'done' if done else 'incomplete/unknown'}**",
        f"- Final `go test ./...`: `{data.get('final_test_exit') or 'not recorded'}`",
        f"- Final Git worktree: **{'clean' if not data.get('final_git_status') else 'dirty'}**",
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

    lines.extend(
        [
            "",
            "## Turn·토큰 요약",
            "",
            "| turn | exit | events | prompt≈ | input | output | total |",
            "| ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
        ]
    )
    for turn in turns:
        turn_tokens = turn.get("tokens", {})
        lines.append(
            f"| {turn['number']} | {turn.get('exit_code', 'unknown')} | {turn['events']} | "
            f"{turn.get('prompt_estimated_tokens') or '—'} | "
            f"{turn_tokens.get('input_tokens', '—')} | {turn_tokens.get('output_tokens', '—')} | "
            f"{turn.get('total_tokens') or '—'} |"
        )
    lines.extend(
        [
            "",
            f"누적 token: `{total or 'unavailable'}` (output `{output or 'unavailable'}`).",
            "",
            "## 도구 사용 관찰",
            "",
        ]
    )
    if data.get("planr_commands"):
        action_counts = collections.Counter(
            action
            for command in data["planr_commands"]
            for action in planr_actions(command)
        )
        lines.append("관찰된 planr 단계: " + ", ".join(f"`{key}`×{value}" for key, value in action_counts.items()) + ".")
        lines.append("")
        lines.append("전체 planr 명령:")
        lines.extend(f"- `{command}`" for command in data["planr_commands"])
    else:
        lines.append("관찰된 `planr` 명령이 없습니다.")

    item_counts = collections.Counter[str]()
    for turn in turns:
        item_counts.update(turn.get("item_types", {}))
    if item_counts:
        lines.extend(["", "Codex 도구 이벤트: " + ", ".join(f"`{key}`×{value}" for key, value in item_counts.items()) + "."])
    if data.get("planr_events"):
        event_counts = collections.Counter(line.split("|", 1)[0] for line in data["planr_events"])
        lines.extend(["", "planr hook 이벤트: " + ", ".join(f"`{key}`×{value}" for key, value in event_counts.items()) + "."])

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
            "./planr/codex-harness.sh analyze <run-directory>",
            "```",
            "",
            "- `transcript.md`: 대화 텍스트·명령 추출본",
            "- `turns/turn-*.jsonl`: Codex 원본 JSONL 이벤트",
            "- `turns/turn-*.stderr.log`: Codex stderr",
            "- `state/`: 매 turn의 overview/status/Git 상태와 종료 검증",
            "- `metrics.json`: 후속 실행과 비교할 수 있는 구조화 통계",
        ]
    )
    return "\n".join(lines).rstrip() + "\n"


def analyze(run_dir: pathlib.Path, output: pathlib.Path | None = None) -> int:
    run_dir = run_dir.resolve()
    if not run_dir.is_dir():
        print(f"run directory not found: {run_dir}", file=sys.stderr)
        return 2
    paths = turn_files(run_dir)
    turns = [read_turn(path, index) for index, path in enumerate(paths)]
    data = result_data(run_dir, turns)
    if output is None:
        output = run_dir / "REPORT.md"
    output = output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(markdown_report(data, run_dir), encoding="utf-8")
    (run_dir / "transcript.md").write_text(build_transcript(run_dir, turns), encoding="utf-8")
    (run_dir / "metrics.json").write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(output)
    return 0


def main(argv: list[str]) -> int:
    if argv and argv[0] == "session-id":
        if len(argv) != 2:
            print("usage: analyze.py session-id <jsonl>", file=sys.stderr)
            return 2
        events, _ = read_events(pathlib.Path(argv[1]))
        value = session_id_from_events(events)
        if value:
            print(value)
        return 0

    parser = argparse.ArgumentParser(description="analyze a Codex planr harness run")
    parser.add_argument("run_dir", type=pathlib.Path)
    parser.add_argument("--output", type=pathlib.Path)
    args = parser.parse_args(argv)
    return analyze(args.run_dir, args.output)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
