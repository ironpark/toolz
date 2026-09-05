# Claude Code (`claude`) — 훅

> [README](README.md) · [../](../README.md)

출처: <https://code.claude.com/docs/en/hooks>

## 정의 위치

| 위치 | 스코프 | 공유 |
| --- | --- | --- |
| `~/.claude/settings.json` | 모든 프로젝트 | 불가 |
| `<project>/.claude/settings.json` | 프로젝트 | 가능(커밋) |
| `<project>/.claude/settings.local.json` | 프로젝트, 나만 | 불가 |
| managed settings | 조직 | 가능(관리자) |
| 플러그인 `hooks/hooks.json` | 플러그인 활성 시 | 가능(번들) |
| 스킬 frontmatter | 호출 후 세션 끝까지 | 가능 |
| 서브에이전트 frontmatter | 해당 서브에이전트 실행 중 | 가능 |

## 구조

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "if": "Bash(rm *)",
            "command": "${CLAUDE_PROJECT_DIR}/.claude/hooks/block-rm.sh",
            "args": [],
            "timeout": 600
          }
        ]
      }
    ]
  }
}
```

핸들러 타입: `command`, `http`, `mcp_tool`, `prompt`, `agent`.
공통 필드: `type`, `if`(도구 이벤트 한정 권한 규칙), `timeout`(기본 command/http/mcp_tool 600초,
prompt 30초, agent 60초), `statusMessage`, `once`.

- `command`는 `args`가 있으면 **exec 형식**(셸 없이 직접 spawn), 없으면 **셸 형식**(토큰화·확장 수행).
- 경로 플레이스홀더: `${CLAUDE_PROJECT_DIR}`, `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}`.
- `http` 훅은 `allowedEnvVars`에 나열된 변수만 헤더에 보간합니다.
- `async: true`는 블로킹하지 않고(타임아웃 미적용), `asyncRewake: true`는 종료 코드 2로
  Claude를 깨웁니다.

## matcher 문법

| 형태 | 해석 |
| --- | --- |
| `"*"`, `""`, 생략 | 전체 매칭 |
| 문자·숫자·`_`·`-`·공백·`,`·`\|` | 정확한 문자열 또는 목록 (`Edit\|Write`) |
| 그 외 문자 포함 | 정규식(비앵커) — `^Notebook`, `mcp__.*` |

MCP 도구는 `mcp__<server>__<tool>`, 플러그인 번들 서버는
`mcp__plugin_<plugin>_<server>__<tool>` 형태로 매칭합니다.

## 주요 이벤트

- 세션당 1회: `SessionStart`(matcher: `startup`/`resume`/`clear`/`compact`/`fork`),
  `SessionEnd`, `Setup`(matcher: `init`/`maintenance`)
- 턴당 1회: `UserPromptSubmit`, `Stop`, `StopFailure`
- 도구 호출마다: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`,
  `PermissionDenied`, `PostToolBatch`
- 그 외: `SubagentStart`, `SubagentStop`, `PreCompact`/`PostCompact`,
  `PreModelSwitch`/`PostModelSwitch`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`,
  `DirectoryAdded`, `FileChanged`, `WorktreeCreate`/`WorktreeRemove`,
  `Elicitation`/`ElicitationResult`, `Notification`, `MessageDisplay`,
  `TaskCreated`/`TaskCompleted`, `TeammateIdle`, `UserPromptExpansion`

## 입출력

훅은 stdin(command) 또는 POST 본문(http)으로 공통 필드를 받습니다: `session_id`,
`prompt_id`, `transcript_path`, `cwd`, `permission_mode`, `effort`, `hook_event_name`,
`agent_id`, `agent_type`. 도구 이벤트에는 `tool_name`, `tool_input`, `tool_use_id`가 추가됩니다.

결정은 stdout(또는 응답 본문) JSON으로 돌려줍니다.

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow|deny|needsPermission",
    "permissionDecisionReason": "...",
    "additionalContext": "...",
    "updatedInput": { },
    "retry": true
  },
  "systemMessage": "..."
}
```

## 종료 코드

| 코드 | 의미 |
| --- | --- |
| `0` | 성공. JSON 출력을 읽고 정상 진행. `UserPromptSubmit`·`SessionStart`·`PostModelSwitch`에서는 stdout이 컨텍스트로 추가됨 |
| `2` | **블로킹 오류**. 동작을 차단하고 stderr가 차단 사유가 됨. JSON의 `permissionDecision`보다 우선 |
| 그 외 | 비블로킹 오류. 유효한 JSON이면 존중, 아니면 그대로 진행 |

종료 코드 2가 차단하는 이벤트: `PreToolUse`, `UserPromptSubmit`, `UserPromptExpansion`,
`Stop`, `SubagentStop`, `TeammateIdle`, `TaskCreated`, `TaskCompleted`, `PostToolBatch`,
`ConfigChange`, `PreModelSwitch`.
차단하지 못하는 이벤트(이미 발생한 것): `PermissionRequest`, `PostToolUse`,
`PostToolUseFailure`, `StopFailure`, `Elicitation`, `ElicitationResult`.

전체 비활성화는 `"disableAllHooks": true` (관리형 훅은 이것으로 끌 수 없음).
