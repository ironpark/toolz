# 에이전트 하네스 조사 자료

`spona`가 프리셋으로 관리·재현해야 할 대상 하네스의 CLI 인터페이스를 정리한 문서입니다.

| 문서 | 대상 | 조사 버전 |
| --- | --- | --- |
```
docs/agents/
├── README.md          이 문서 — 하네스 비교와 설계 시사점
├── interactive.md     대화형 터미널 실행 (spona의 기본 동작)
├── spawn.md           비대화형 실행 계약 (보조 경로)
├── claude-code/       Claude Code 2.1.257
│   ├── README.md      개요·기본 사용법·주의점
│   ├── cli.md         CLI 플래그와 서브커맨드
│   ├── settings.md    settings.json 스코프·우선순위·키
│   ├── env.md         환경변수
│   ├── agents.md      서브에이전트 정의 포맷
│   └── hooks.md       훅
└── codex/             codex-cli 0.153.0
    ├── README.md      개요·기본 사용법·주의점
    ├── cli.md         CLI 옵션과 서브커맨드
    ├── config.md      설정 레이어와 config.toml
    ├── env.md         환경변수
    ├── rules.md       execpolicy .rules
    └── hooks.md       훅
```

**읽는 순서**: [interactive.md](interactive.md) → 대상 하네스 폴더의 `README.md` →
필요한 세부 문서.

> [!IMPORTANT]
> spona의 기본 동작은 **사용자가 터미널에서 직접 `claude` / `codex`를 실행했을 때와 똑같은
> 대화형 TUI를 띄우는 것**입니다. 프리셋은 그 실행에 설정·환경변수·인자를 얹는 역할만 합니다.
> 구현 시 먼저 읽어야 할 문서는 [interactive.md](interactive.md)입니다.
>
> [spawn.md](spawn.md)는 스크립트·CI처럼 결과만 회수하는 보조 경로를 다룹니다. 두 경로는
> 요구사항이 상당히 다르고, 일부는 정면으로 충돌합니다(예: `--bare`는 CI에서는 권장되지만
> 대화형 기본값으로는 부적절).

## 출처

각 문서는 **공식 문서를 1차 출처**로 하고, 로컬 설치본의 `--help` 출력으로 교차 확인했습니다.
문서화되지 않은 항목은 문서 안에서 별도로 표시합니다. 하네스별 공식 문서 링크는 각 폴더의
`README.md`에 정리돼 있습니다.

갱신 시 참고할 로컬 명령:

```sh
claude --help
codex --help && codex exec --help
```

## 대화형 실행이 거는 제약 (요약)

| 항목 | 요구사항 |
| --- | --- |
| 표준 스트림 | TTY를 그대로 상속. 파이프·버퍼로 감싸면 안 됨 |
| 프로세스 모델 | 유닉스는 `syscall.Exec`로 치환이 가장 깨끗함 |
| 프로세스 그룹 | `Setpgid`를 켜지 않음 (Ctrl+C가 도달하지 못함) |
| 시그널 | `SIGINT`를 가로채지 않음. 종료 시 SIGINT → 유예 → SIGTERM |
| 환경 | `TERM`, `COLORTERM`, `TERM_PROGRAM`, `LANG` 등 터미널 능력 변수 필수 |
| 비대화형 플래그 | `-p`, `--output-format`, `codex exec` 를 쓰지 않음 |
| 인증 | 대화형은 OAuth·키체인·`auth.json` 사용 가능. `--bare`는 이를 차단 |

`claude` / `codex`가 대화형으로 동작하려면 **stdout이 TTY여야 합니다.** Claude Code는
stdout이 TTY가 아니면 워크스페이스 신뢰 다이얼로그를 건너뛰고 설정 파일 검증 오류도
조용히 무시합니다. 자세한 내용은 [interactive.md](interactive.md) 참고.

## spona 관점의 공통 축

두 하네스는 이름이 다를 뿐 프리셋으로 다뤄야 할 축은 거의 같습니다.

| 축 | Claude Code | Codex |
| --- | --- | --- |
| 모델 지정 | `--model` / `model` / `ANTHROPIC_MODEL` | `-m/--model` / `model` |
| 추론 강도 | `--effort` / `effortLevel` | `model_reasoning_effort` |
| 작업 디렉터리 | (cwd) + `--add-dir` | `-C/--cd` + `--add-dir` |
| 권한·샌드박스 | `--permission-mode`, `permissions.*`, `sandbox.*` | `-s/--sandbox`, `-a/--ask-for-approval`, `permissions.<name>`, `sandbox_workspace_write.*` |
| **대화형 실행(기본)** | `claude [flags] [prompt]` | `codex [flags] [prompt]` (서브커맨드 없음) |
| 비대화형 실행 | `-p/--print` | `codex exec` |
| 세션 이름 | `-n/--name` | `codex resume <name>` 로 참조 가능 |
| 터미널 렌더링 | `/tui fullscreen`, `CLAUDE_CODE_NO_FLICKER` | `--no-alt-screen` |
| worktree·멀티플렉서 | `-w/--worktree`, `--tmux` | (없음) |
| 구조화 출력 | `--output-format json\|stream-json`, `--json-schema` | `--json`, `--output-schema` |
| 도구 제어 | `--tools`, `--allowedTools`, `--disallowedTools` | `tools.*`, `features.shell_tool`, execpolicy `.rules` |
| MCP | `--mcp-config`, `--strict-mcp-config`, `allowedMcpServers` | `mcp_servers.<id>`, `codex mcp` |
| 에이전트 지침 | `CLAUDE.md`, `--system-prompt[-file]`, `--append-system-prompt[-file]` | `AGENTS.md`, `model_instructions_file`, `developer_instructions` |
| 설정 형식 | `settings.json` (JSON) | `config.toml` (TOML) |
| 설정 홈 | `~/.claude`, `CLAUDE_CONFIG_DIR` | `~/.codex`, `CODEX_HOME` |
| 세션 한정 오버라이드 | `--settings <file\|json>`, `--setting-sources` | `-c key=value`, `-p/--profile` |
| 세션 재개 | `--continue`, `--resume`, `--fork-session` | `codex resume`, `codex fork` |
| 격리 실행 | `--bare`, `--safe-mode`, `--restricted` | `--ephemeral`, `--ignore-user-config`, `--ignore-rules` |
| 서브에이전트 | `--agents <json>`, `agent` 설정 키 | `agents.<name>.config_file`, `agents.default_subagent_model` |
| 훅 | `hooks` 설정 키 (5가지 핸들러 타입, 30+ 이벤트) | `hooks.json` / `[hooks]`, `features.hooks`(기본 off) |
| 명령 실행 정책 | 권한 규칙 문법 (`Bash(git diff *)`) | execpolicy `.rules` (Starlark `prefix_rule`) |
| 무인 실행 권한 | `--permission-mode dontAsk` + `--permission-prompts none` | `--sandbox` + `--ask-for-approval never` |
| CI 인증 | `ANTHROPIC_API_KEY`, `claude setup-token` | `CODEX_API_KEY` |
| 조직 강제 | managed settings / MDM / 콘솔 | `requirements.toml`, `/etc/codex/config.toml` |

## 설정 우선순위 비교

**Claude Code** (높은 순): managed → 커맨드라인(`--settings`) → project local
(`.claude/settings.local.json`) → shared project (`.claude/settings.json`) →
user (`~/.claude/settings.json`). 리스트형 키는 덮어쓰지 않고 병합됩니다.

**Codex** (높은 순): CLI 플래그·`-c` → 프로젝트 `.codex/config.toml`(신뢰된 경우만) →
프로필 `$CODEX_HOME/<name>.config.toml` → 사용자 `$CODEX_HOME/config.toml` →
시스템 `/etc/codex/config.toml` → 기본값. 조직은 `requirements.toml`로 제약을 강제합니다.

주요 차이: Claude Code는 **커맨드라인이 모든 파일보다 위**에 있고, Codex는 **프로젝트 설정이
프로필보다 위**에 있습니다. 또 Codex는 프로젝트가 untrusted면 프로젝트 스코프 레이어를
통째로 건너뜁니다.

## 설계 시사점

1. **설정 홈 격리가 가장 확실한 주입 경로.** 두 하네스 모두 홈 디렉터리를 환경변수로
   교체할 수 있습니다(`CLAUDE_CONFIG_DIR`, `CODEX_HOME`). 프리셋별 홈을 만들어 주입하면
   사용자의 기존 설정과 간섭하지 않고, 우선순위 규칙에 휘둘리지도 않습니다.
2. **전달 스타일은 하네스마다 반대로 잡는 게 자연스럽다.** Claude Code는
   `--settings '{...}'` 로 프리셋 전체를 **한 덩어리 JSON**으로 넘기는 게 맞고, Codex는
   `-c key=value` 를 **펼쳐서** 넘기거나 프로필 파일로 내보내는 게 맞습니다.
3. **환경변수 주입은 하네스 자체 정책을 통과해야 한다.** Claude Code는 설정 파일의 `env`
   값이 셸 변수를 이기고, Codex는 `shell_environment_policy`가 기본적으로
   `KEY`/`SECRET`/`TOKEN` 이름을 가진 변수를 걸러냅니다. 단순히 `os/exec`의 `Env`에
   넣는 것만으로는 에이전트가 실행하는 셸까지 도달하지 않을 수 있습니다.
4. **위험 플래그는 스키마에서 분리한다.** `--dangerously-skip-permissions`,
   `--dangerously-bypass-approvals-and-sandbox`, `--dangerously-bypass-hook-trust`는
   명시적 opt-in 필드로 두고 기본값은 항상 비활성이어야 합니다.
5. **재현 실행 프로파일을 하네스별로 고정해 둔다.** Claude Code는 `--bare`, Codex는
   `--ephemeral --ignore-user-config --ignore-rules` 가 "프리셋에 적힌 것만" 적용하는
   기준점입니다.
6. **프리셋이 적용됐는지 검증할 수단이 각각 있다.** Claude Code는 `system/init` 이벤트의
   `plugin_errors` / `mcp_server_errors` 배열(오류 없으면 키 자체가 없음)로 CI 게이트를 만들 수
   있고, Codex는 `codex execpolicy check`로 규칙 적용 결과를 미리 확인할 수 있습니다.
   두 하네스 모두 **잘못된 설정을 조용히 건너뛰고 정상 종료**하므로, 검증 없이는 프리셋이
   반영되지 않은 채 실행됩니다.
7. **신뢰 경계가 서로 반대 방향으로 위험하다.** `--bare` 없는 `claude -p`는 신뢰한 적 없는
   폴더의 훅과 MCP 서버를 그대로 실행합니다(신뢰 다이얼로그 없음). 반대로 Codex는 프로젝트가
   untrusted면 `.codex/` 레이어를 통째로 무시해 프리셋이 조용히 사라집니다.
8. **겹치는 기능을 확인할 것.** `claude import codex` 가 이미 존재하고, Codex는
   `agents.<name>.config_file` 로 역할별 설정 레이어를 지원합니다. spona는 이들을
   대체하기보다 상위에서 통합하는 위치를 잡는 편이 낫습니다.
