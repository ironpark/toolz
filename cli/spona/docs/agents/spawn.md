# 비대화형 실행 계약 비교

> [!NOTE]
> spona의 **기본 동작은 대화형 TUI**이며, 그 경로는 [interactive.md](interactive.md)에서
> 다룹니다. 이 문서는 스크립트·CI·후처리처럼 결과만 회수하는 **보조 경로**를 다룹니다.
> 여기의 권장 사항 중 일부(특히 `--bare`)는 대화형 기본값으로는 적절하지 않습니다.

결과 회수용으로 프로세스를 띄울 때 필요한 계약(스트림 분리, 이벤트 형식,
종료 코드, 재개, CI 인증)을 하네스별로 정리합니다. 옵션 목록은
[claude-code/](claude-code/) · [codex/](codex/) 참고.

출처: [Claude Code — 프로그래매틱 실행](https://code.claude.com/docs/en/headless) ·
[Codex — 비대화형 모드](https://learn.chatgpt.com/docs/non-interactive-mode)

## 1. 스트림 분리

| | Claude Code (`claude -p`) | Codex (`codex exec`) |
| --- | --- | --- |
| stdout | 응답 본문 (형식은 `--output-format`) | **최종 에이전트 메시지만** |
| stderr | 경고·오류. 시작 전 오류는 여기로 | **진행 상황 전체** |
| stdin | 프롬프트 입력 가능 (파이프 상한 **10MB**) | 프롬프트 입력 가능. `codex exec -` 로 전체를 stdin에서 |

Codex는 "stdout = 결과, stderr = 진행"이라는 유닉스 관례를 따르므로 파이프가 그대로
동작합니다. Claude Code는 `--output-format`에 따라 stdout 내용이 통째로 바뀝니다.

> [!IMPORTANT]
> Claude Code는 stdin 파이프가 10MB를 넘으면 명확한 에러와 함께 0이 아닌 코드로 종료합니다.
> spona가 큰 컨텍스트를 넘길 때는 파이프 대신 파일 경로를 프롬프트에 넣어야 합니다.

## 2. 출력 형식

### Claude Code

| 형식 | 내용 |
| --- | --- |
| `text` (기본) | 평문 |
| `json` | `result`(본문), `session_id`, `total_cost_usd`, 모델별 비용 내역, 사용량 등 |
| `stream-json` | 줄 단위 JSON 이벤트. **마지막 줄이 `result` 메시지** |

- `--json-schema`와 `--output-format json`을 함께 쓰면 검증된 구조화 출력이
  **`structured_output` 필드**에 담깁니다. 스키마가 유효하지 않으면
  `Error: --json-schema is not a valid JSON Schema`로 종료합니다(v2.1.205+).
  `format` 키워드는 받아들이되 강제하지 않습니다.
- 실시간 토큰 스트리밍은 `--output-format stream-json --verbose --include-partial-messages`.
- 소비자가 느리게 읽으면 큐가 빠질 때까지 최대 30초 대기 후 종료합니다.

주요 이벤트:

| 이벤트 | 용도 |
| --- | --- |
| `system/init` | 세션 메타데이터. `model`, `tools`, `mcp_servers`, `plugins`, `capabilities` |
| `system/api_retry` | 재시도 진행. `attempt`, `max_retries`, `retry_delay_ms`, `error_status`, `error` |
| `system/plugin_install` | 플러그인 설치 진행 (`CLAUDE_CODE_SYNC_PLUGIN_INSTALL` 설정 시) |
| `stream_event` | 부분 토큰 델타 |
| `result` | 최종 결과·비용·`permission_denials` |

**CI 게이트로 쓸 수 있는 필드** (`system/init`):

- `plugin_errors` — 플러그인 로드 실패. 각 항목 `plugin`, `type`, `message`. 오류 없으면 키 자체가 없음
- `mcp_server_errors` — 검증 실패로 건너뛴 `--mcp-config` 항목. `name`, `type`
  (`unknown_type`, `url_missing_type`, `invalid_config`, `reserved_name` 등), `message`

> [!NOTE]
> `--mcp-config`의 잘못된 항목은 **조용히 건너뛰고 실행은 정상 종료**합니다.
> spona가 "프리셋이 실제로 적용됐는지" 검증하려면 `system/init`의 위 두 배열을 확인해야 합니다.
> stderr 경고는 리다이렉트되거나 CI 러너가 캡처하면 출력되지 않습니다.

서브에이전트 메시지는 `parent_tool_use_id`로 구분합니다(메인 대화는 `null`).
기본적으로 `tool_use`/`tool_result`만 방출되며, 텍스트·사고 블록까지 받으려면
`--forward-subagent-text` 또는 `CLAUDE_CODE_FORWARD_SUBAGENT_TEXT`가 필요합니다(v2.1.211+).
중첩 서브에이전트도 같은 방식으로 트리 복원이 가능합니다(v2.1.219+).

### Codex

`--json`을 주면 stdout이 JSONL 스트림이 됩니다. 이벤트 타입: `thread.started`,
`turn.started`, `item.*`, `turn.completed`, `error`.

- `-o, --output-last-message <FILE>` — 최종 메시지를 파일로
- `--output-schema <FILE>` — 최종 응답이 JSON Schema를 따르도록 강제

## 3. 종료 코드와 신호

### Claude Code

| 상황 | 동작 |
| --- | --- |
| 성공 | `0` |
| 실행 실패 | 0이 아닌 코드 |
| 잘못된 플래그 | 실행 전 stderr로 오류 보고 |
| 실행 중 실패(예: 인증 누락) | **stdout에 result로 출력** |
| SIGTERM | 코드 `143`. 진행 중이던 턴은 **미완료로 남고 결과가 기록되지 않음** |
| SIGINT | 턴을 종료 처리 |

SIGTERM 시 Claude Code는 실행 중인 Bash 프로세스 트리를 종료하고, `SessionEnd` 훅만
실행한 뒤 빠져나갑니다. 새 도구 호출·모델 요청·다른 훅은 시작하지 않습니다.
세션을 재개하면 SIGTERM이 남긴 미완료 턴부터 이어집니다.

> [!TIP]
> spona가 에이전트를 정상적으로 멈출 때는 **SIGINT를 먼저** 보내 턴을 마무리시키고,
> 그 다음에 SIGTERM으로 프로세스를 정리해야 결과가 유실되지 않습니다.

### 백그라운드 작업의 종료 대기 (Claude Code)

- 백그라운드 **Bash** 작업: 최종 결과 반환 후 약 5초 뒤 종료됩니다.
- 백그라운드 **서브에이전트·워크플로**: 결과가 최종 출력의 일부이므로 완료까지 대기합니다.
- 연속 유휴 대기 상한은 기본 10분. `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS`로 변경,
  `0`이면 무제한.
- Monitor watch는 자체 타임아웃(기본 5분)과 10분 상한 중 먼저 오는 쪽까지 대기합니다.

## 4. 세션 재개

| | Claude Code | Codex |
| --- | --- | --- |
| 최근 세션 | `claude -p --continue` (백그라운드 세션은 제외) | `codex exec resume --last` |
| ID 지정 | `claude -p --resume <session_id>` | `codex exec resume <SESSION_ID>` |
| ID 획득 | `--output-format json` 결과의 `session_id` | `thread.started` 이벤트 |
| 포크 | `--fork-session` | `codex exec fork` |

Claude Code는 v2.1.223부터 **다른 디렉터리에서도 ID로 세션을 찾습니다**(이전에는 같은
프로젝트 디렉터리에서만 가능). spona가 워크트리를 옮겨 다니며 세션을 이어갈 때 중요합니다.

## 5. 권한 — 무인 실행

### Claude Code

`-p`의 기본 시작 권한 모드는 모든 플랜에서 **Manual**입니다. 즉 아무것도 지정하지 않으면
프롬프트가 필요한 작업은 진행되지 않습니다. 무인 실행에서 골라야 할 조합:

| 모드 | 성격 |
| --- | --- |
| `--permission-mode auto` | 분류기가 대부분의 동작을 대신 검토 |
| `--permission-mode dontAsk` | `permissions.allow`와 읽기 전용 명령 집합 외에는 **거부**. 잠긴 CI에 적합 |
| `--permission-mode acceptEdits` | 파일 쓰기와 `mkdir`/`touch`/`mv`/`cp` 등 자동 승인. 그 외 셸·네트워크는 여전히 allow 규칙 필요 |

추가로 **`--permission-prompts none`** (v2.1.259+): 권한 호스트(SDK `canUseTool` 콜백,
`--permission-prompt-tool`)를 조회하거나 기다리지 않고, 아무것도 해결하지 못한 요청은
거부하며 Claude에게 재시도하지 말라고 알립니다. `AskUserQuestion` 같은 사람 응답이 필요한
도구는 아예 제거됩니다. `stream-json`에서는 `permission_denied` 시스템 메시지로 나타나고
최종 `result`의 `permission_denials`에 목록이 담깁니다.

`--allowedTools`는 권한 규칙 문법을 씁니다. 접두 매칭의 **공백이 중요**합니다:
`Bash(git diff *)`는 `git diff`로 시작하는 명령을 허용하지만, 공백 없는
`Bash(git diff*)`는 `git diff-index`까지 매칭합니다.

### Codex

`--sandbox`는 기본이 `read-only`입니다. 무인 실행은 보통
`--sandbox workspace-write --ask-for-approval never` 조합을 씁니다.
`never`는 승인을 묻지 않고 진행하되 **실행 실패를 모델에게 그대로 반환**합니다.

## 6. 신뢰(trust) 경계 — spona가 반드시 알아야 할 부분

> [!WARNING]
> `--bare` 없이 실행하는 `claude -p` 세션은 **한 번도 신뢰한 적 없는 폴더에서도**
> 그 프로젝트의 `.claude/settings.json` 훅을 실행하고 `.mcp.json` 서버에 연결합니다.
> `-p` 세션에는 워크스페이스 신뢰 다이얼로그도, 서버별 승인 프롬프트도 없습니다.

Codex도 대칭적인 문제가 있습니다. 프로젝트를 untrusted로 표시하면 `.codex/` 레이어를
통째로 건너뛰므로, 반대로 **프리셋이 조용히 무시**됩니다.

spona가 신뢰할 수 없는 저장소를 대상으로 에이전트를 띄운다면:

- Claude Code: `--bare`를 붙여 프로젝트 훅·MCP·`CLAUDE.md` 자동 탐색을 차단
- Codex: `--ignore-user-config --ignore-rules`로 레이어를 끄고 필요한 값만 `-c`로 주입

## 7. `--bare` 모드의 정확한 경계 (Claude Code)

공식 문서는 `--bare`를 **스크립트·SDK 호출의 권장 모드**로 명시하고 있으며, 향후 `-p`의
기본값이 될 예정입니다. spona의 기본 프로파일로 삼기에 적합합니다.

건너뛰는 것: 훅, 스킬, 커스텀 커맨드, 서브에이전트, 플러그인, MCP 서버, auto memory, `CLAUDE.md`.

**부분 예외**: `--add-dir`로 지정한 디렉터리의 `.claude/skills/` 는 로드하지만,
같은 디렉터리의 `.claude/commands/` 와 `.claude/agents/` 는 여전히 건너뜁니다.

인증: OAuth·시스템 키체인을 **읽지 않습니다**. `ANTHROPIC_API_KEY` 또는 `--settings` JSON의
`apiKeyHelper`가 필요합니다. Bedrock·Vertex·Foundry는 각자의 자격증명을 평소대로 사용합니다.

기본 도구: Bash, 파일 읽기, 파일 편집. 나머지 컨텍스트는 플래그로 명시 주입합니다.

| 주입 대상 | 플래그 |
| --- | --- |
| 시스템 프롬프트 추가 | `--append-system-prompt`, `--append-system-prompt-file` |
| 설정 | `--settings <file-or-json>` |
| MCP 서버 | `--mcp-config <file-or-json>` |
| 커스텀 에이전트 | `--agents <json>` |
| 플러그인 | `--plugin-dir <path>`, `--plugin-url <url>` |

## 8. CI 인증

| 하네스 | 방법 |
| --- | --- |
| Claude Code | `ANTHROPIC_API_KEY`, 또는 `claude setup-token`으로 만든 장수명 OAuth 토큰. `--bare`에서는 API 키/`apiKeyHelper`만 유효 |
| Codex | **`CODEX_API_KEY`** 환경변수. GitHub 워크플로에서는 자격증명 노출을 줄이는 Codex GitHub Action 권장 |

두 하네스 모두 "같은 프로세스 환경에 있는 신뢰할 수 없는 코드에 API 키를 노출하지 말 것"을
공통 권고로 두고 있습니다. spona가 프리셋의 자격증명을 주입할 때 반드시 고려해야 합니다.

## 9. spona 기준 레시피

```sh
# Claude Code — 재현 가능한 무인 실행
claude --bare -p "$PROMPT" \
  --model "$MODEL" \
  --permission-mode dontAsk \
  --allowedTools "Read,Grep,Bash(git diff *)" \
  --max-turns 20 \
  --settings "$PRESET_JSON" \
  --mcp-config "$MCP_JSON" \
  --agents "$AGENTS_JSON" \
  --output-format stream-json --verbose
# 환경: CLAUDE_CONFIG_DIR=<preset-home> ANTHROPIC_API_KEY=...
# 검증: system/init 의 plugin_errors / mcp_server_errors 가 비어 있는지 확인
# 종료: SIGINT → (유예) → SIGTERM
```

```sh
# Codex — 재현 가능한 무인 실행
codex exec "$PROMPT" \
  --model "$MODEL" \
  --sandbox workspace-write \
  --ask-for-approval never \
  --cd "$WORKDIR" \
  --ephemeral --ignore-user-config --ignore-rules \
  -c model_reasoning_effort=high \
  -c 'shell_environment_policy.inherit="core"' \
  --json --output-last-message "$LAST_MSG_FILE"
# 환경: CODEX_HOME=<preset-home> CODEX_API_KEY=...
# stdout=JSONL 이벤트, stderr=진행 로그
```
