# Claude Code (`claude`) — CLI 플래그

> [README](README.md) · [../](../README.md)

## 모델 / 추론

| 플래그 | 설명 |
| --- | --- |
| `--model <model>` | 세션 모델. 별칭(`opus`, `sonnet`, `haiku`, `fable`) 또는 전체 ID |
| `--fallback-model <models>` | 주 모델 과부하 시 순서대로 시도(콤마 구분) |
| `--effort <level>` | `low`, `medium`, `high`, `xhigh`, `max` |
| `--advisor <model>` | 서버측 advisor 도구를 지정 모델로 활성화 |
| `--betas <betas...>` | 추가 `anthropic-beta` 헤더 (API 키 사용자 한정) |
| `--autocompact <auto\|tokens>` | 자동 컴팩트 윈도(예: `500k`) |
| `--max-turns <n>` | 에이전트 턴 수 제한 |
| `--max-budget-usd <amount>` | 지출 상한 도달 시 중단 (print 모드) |

## 지침 / 시스템 프롬프트

| 플래그 | 설명 |
| --- | --- |
| `--system-prompt <text>` | 시스템 프롬프트 **전체 교체** |
| `--system-prompt-file <path>` | 파일에서 읽어 전체 교체 |
| `--append-system-prompt <text>` | 기본 프롬프트 뒤에 추가 |
| `--append-system-prompt-file <path>` | 파일 내용을 뒤에 추가 |
| `--append-subagent-system-prompt <text>` | 모든 서브에이전트 프롬프트 뒤에 추가 |
| `--exclude-dynamic-system-prompt-sections` | 머신별 섹션(cwd, env, git status)을 첫 user 메시지로 이동 → 프롬프트 캐시 재사용률 향상 |
| `--add-dir <dirs...>` | 읽기/편집 허용 디렉터리 추가 (`CLAUDE.md` 탐색 대상에도 포함) |

프로젝트 지침은 `CLAUDE.md`로 자동 탐색되며, `claudeMd` / `claudeMdExcludes` 설정 키로
조직 차원 주입·제외가 가능합니다.

## 도구 / 권한

| 플래그 | 설명 |
| --- | --- |
| `--allowedTools`, `--allowed-tools <tools...>` | 프롬프트 없이 실행할 도구. `"Bash(git log *)" "Read"` 형태 패턴 |
| `--disallowedTools`, `--disallowed-tools <tools...>` | 거부 규칙 |
| `--tools <tools...>` | 내장 도구 집합 지정. `""`=전체 비활성, `default`=전체, 또는 `Bash,Edit,Read` |
| `--permission-mode <mode>` | `acceptEdits`, `auto`, `bypassPermissions`, `manual`, `dontAsk`, `plan` |
| `--permission-prompt-tool <tool>` | 비대화형에서 권한 프롬프트를 처리할 MCP 도구 |
| `--permission-prompts <mode>` | print 모드에서 권한 프롬프트에 누가 답할지 (`none` 등) |
| `--dangerously-skip-permissions` | `--permission-mode bypassPermissions`와 동등 (**위험**) |
| `--allow-dangerously-skip-permissions` | bypassPermissions를 모드 순환에 추가만 하고 그 상태로 시작하지 않음 |
| `--restricted` | 평가 하네스/공용 머신용 제한 모드 |

> [!NOTE]
> `permissions.defaultMode`의 `auto`·`bypassPermissions` 값은 project/local 설정 파일에서는
> 적용되지 않습니다(user 또는 managed 설정, 혹은 `--permission-mode` 사용). v2.1.257 이전에는
> `bypassPermissions`가 어느 파일에서든 적용됐습니다.

## 설정 소스 / 확장

| 플래그 | 설명 |
| --- | --- |
| `--settings <file-or-json>` | 설정 JSON 파일 경로 **또는 인라인 JSON 문자열** |
| `--setting-sources <sources>` | 로드할 소스 콤마 목록: `user`, `project`, `local` |
| `--mcp-config <configs...>` | JSON 파일/문자열에서 MCP 서버 로드 |
| `--strict-mcp-config` | `--mcp-config`의 서버만 사용 |
| `--agent <name>` | 이 세션에 사용할 에이전트 |
| `--agents <json>` | 커스텀 서브에이전트를 JSON으로 동적 정의 |
| `--plugin-dir <path>` / `--plugin-url <url>` | 세션 한정 플러그인 로드 |
| `--disable-slash-commands` | 이 세션의 모든 스킬/커맨드 비활성화 |
| `--channels <spec>` | (리서치 프리뷰) 채널 알림을 수신할 MCP 서버 |

> [!IMPORTANT]
> 조직은 `disableSideloadFlags` 설정으로 플러그인·서브에이전트·MCP를 사이드로드하는
> CLI 플래그 자체를 거부할 수 있습니다. spona가 관리형 환경에서 동작할 때 고려해야 합니다.

## 격리 모드 (프리셋 재현성의 핵심)

| 플래그 | 설명 |
| --- | --- |
| `--bare` | 훅·스킬·커스텀 커맨드·서브에이전트·플러그인·MCP 서버·auto memory·`CLAUDE.md` 자동 탐색을 모두 건너뜀. `CLAUDE_CODE_SIMPLE=1` 설정. 인증은 `ANTHROPIC_API_KEY` 또는 `--settings`의 `apiKeyHelper`만 사용(OAuth·키체인 미조회) |
| `--safe-mode` | 모든 커스터마이즈 비활성화(문제 진단용). 관리형 정책은 그대로 적용 |
| `--restricted` | 명령/코드 실행 도구와 WebFetch 제거, user/project/local 설정 무시, 파일 도구를 작업 디렉터리로 한정, bypassPermissions 거부 |

`--bare`가 "프리셋에 적힌 것만 정확히 주입"하는 재현 실행에 가장 적합합니다.

## 입출력 (프로그래매틱 spawn)

| 플래그 | 설명 |
| --- | --- |
| `-p, --print` | 응답 출력 후 종료 |
| `--input-format <text\|stream-json>` | 입력 형식 |
| `--output-format <text\|json\|stream-json>` | 출력 형식 |
| `--include-partial-messages` | 부분 스트리밍 이벤트 포함 |
| `--include-hook-events` | 훅 라이프사이클 이벤트 포함 |
| `--forward-subagent-text` | 서브에이전트 텍스트/사고 블록 방출 |
| `--replay-user-messages` | stdin의 user 메시지를 stdout으로 재방출 |
| `--prompt-suggestions` | 다음 프롬프트 예측을 `prompt_suggestion` 메시지로 방출 |
| `--json-schema <schema>` | 완료 후 JSON Schema 검증된 출력 |
| `--no-session-persistence` | 세션을 디스크에 저장하지 않음 (print 전용) |

`stream-json` 계열 플래그 다수는 공식 예제에서 `--verbose`와 함께 사용합니다.

## 세션 / 실행 수명주기

| 플래그·명령 | 설명 |
| --- | --- |
| `-c, --continue` | 현재 디렉터리의 최근 대화 이어가기 |
| `-r, --resume <session>` | 세션 ID **또는 이름**으로 재개, 생략 시 picker |
| `--fork-session` | 재개 시 새 세션 ID 생성 |
| `--session-id <uuid>` | 세션 ID 지정 |
| `-n, --name <name>` | 세션 표시 이름 |
| `--bg, --background` | 백그라운드 에이전트로 시작하고 즉시 반환 |
| `--exec '<cmd>'` | PTY 기반 백그라운드 잡으로 셸 명령 실행 |
| `--init` / `--maintenance` | 세션 전에 해당 matcher의 Setup 훅 실행 |
| `--init-only` | Setup·SessionStart 훅만 실행하고 대화 없이 종료 |
| `-w, --worktree [name]` | 세션용 git worktree 생성 |
| `--from-pr <n>` | 특정 PR에 연결된 세션으로 picker 필터링 |
| `--cloud` / `--environment <id>` / `--ref <branch>` / `--teleport` | 클라우드 세션 생성·연결 |

`--init-only`는 spona가 프리셋 적용 결과를 **실제 대화 없이 검증**하는 dry-run 훅으로 쓸 수 있습니다.

## 서브커맨드

`agents`, `attach <id>`, `logs <id>`, `stop <id>`, `rm <id>`, `respawn <id>`,
`daemon status|stop`, `auth login|logout|status`, `setup-token`, `mcp`(+`login`/`logout`),
`plugin`, `project purge`, `doctor`, `import [codex|gemini]`, `install`, `update`,
`gateway`, `remote-control`, `self-hosted-runner`, `ultrareview`, `auto-mode defaults|reset`.

- 백그라운드 오케스트레이션: `claude --bg` → `claude agents` → `attach`/`logs`/`stop`/`rm`
- `claude auth status`는 JSON을 출력하므로 프리셋 적용 전 인증 확인에 쓸 수 있습니다.
- `claude import codex`가 이미 존재합니다 — spona의 설정 변환 기능과 겹치는지 검토 필요.
