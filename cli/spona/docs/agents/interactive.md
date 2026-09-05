# 대화형 터미널 실행 (spona의 기본 동작)

spona는 `spona run <preset>` 같은 명령으로 **사용자가 직접 `claude` / `codex`를 터미널에서
실행했을 때와 똑같은 대화형 TUI**를 띄우는 것이 기본 동작입니다. 프리셋은 그 실행에
설정·환경변수·인자를 얹는 역할만 합니다.

이 문서는 그 관점에서 반드시 지켜야 할 제약을 정리합니다.
비대화형(`-p` / `codex exec`) 계약은 [spawn.md](spawn.md)를 참고하세요.

## 1. 핵심 제약 — TTY를 반드시 물려줘야 한다

두 하네스 모두 **stdin/stdout이 TTY인지**로 동작 모드를 바꿉니다. spona가 파이프를
끼워 넣는 순간 사용자가 기대한 대화형 세션이 아니게 됩니다.

> [!WARNING]
> Claude Code는 **stdout이 TTY가 아니면**(파이프·리다이렉트) 워크스페이스 신뢰 다이얼로그를
> 건너뜁니다. 이 상태에서는 신뢰하지 않은 폴더의 훅과 MCP 서버가 확인 없이 실행됩니다.
> 또한 이 모드에서는 검증에 실패한 설정 파일이 **오류 다이얼로그 없이 조용히 무시**됩니다.
> spona가 로그 수집을 위해 stdout을 가로채면 보안 경계가 사라집니다.

Codex도 마찬가지로 TUI가 alternate screen·raw 모드·키 입력 처리를 직접 하므로 TTY가 필요합니다.

### 결론: 두 가지 선택지뿐

| 방식 | 언제 | 특징 |
| --- | --- | --- |
| **TTY 그대로 상속** | 기본. 단순 런처 | spona는 설정만 하고 빠짐. 가장 안전하고 동작이 정확함 |
| **PTY 할당** (`creack/pty` 등) | 출력 기록·다중 세션 관리가 필요할 때 | 창 크기 전파·raw 모드·시그널 중계를 직접 해야 함 |

## 2. 방식 A — 프로세스 치환 (권장)

유닉스에서 런처가 할 수 있는 가장 깨끗한 동작은 **자기 자신을 하네스로 교체**하는 것입니다.
TTY·프로세스 그룹·포그라운드 상태·시그널이 모두 그대로 유지되고, 중계할 것이 없습니다.

```go
// 프리셋 해석이 끝난 뒤
bin, err := exec.LookPath("claude")
if err != nil {
    return err
}
argv := append([]string{bin}, presetArgs...)
env := buildEnv(os.Environ(), preset) // CLAUDE_CONFIG_DIR 등 주입
return syscall.Exec(bin, argv, env)   // 반환되지 않음
```

- spona 프로세스가 남지 않으므로 Ctrl+C·Ctrl+Z·`SIGWINCH`가 하네스에 직접 전달됩니다.
- 종료 코드도 자연스럽게 하네스의 것이 됩니다(Claude Code의 SIGTERM `143` 등).
- **Windows에는 `syscall.Exec`가 없습니다.** 방식 B로 대체해야 합니다.

## 3. 방식 B — 자식 프로세스 + 표준 스트림 상속

세션 종료 후 후처리(정리, 기록 남기기)가 필요하거나 Windows를 지원해야 할 때 씁니다.

```go
cmd := exec.Command(bin, presetArgs...)
cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
cmd.Env = buildEnv(os.Environ(), preset)
cmd.Dir = preset.WorkDir
err := cmd.Run()
```

지켜야 할 것:

- **`SysProcAttr.Setpgid`를 켜지 말 것.** 별도 프로세스 그룹으로 분리하면 자식이 포그라운드
  프로세스 그룹이 아니게 되어 Ctrl+C(`SIGINT`)를 받지 못하고, 터미널 읽기에서 `SIGTTIN`으로
  멈출 수 있습니다.
- **spona가 `SIGINT`를 가로채지 말 것.** `signal.Notify`로 잡아버리면 사용자의 Ctrl+C가
  하네스에 도달하지 않습니다. 굳이 정리 작업이 필요하면 §6의 종료 규약을 따르세요.
- 스트림을 `bytes.Buffer`나 파이프로 바꾸지 말 것 (§1 참고).

## 4. 방식 C — PTY 할당

spona가 여러 세션을 관리하거나 출력을 기록해야 한다면 PTY가 필요합니다. 이때 직접 해야 할 일:

1. **raw 모드 전환** — 부모 터미널을 raw로 두고 종료 시 반드시 복구 (`defer`).
2. **창 크기 초기 설정 + `SIGWINCH` 중계** — 하지 않으면 TUI 레이아웃이 깨집니다.
3. **`TERM` 등 터미널 능력 변수 전달** (§5).
4. **비정상 종료 시 터미널 상태 복구** — alternate screen에 들어간 채 죽으면 사용자의
   터미널이 망가진 상태로 남습니다.

> [!TIP]
> Codex는 `--no-alt-screen` 으로 alternate screen을 끄고 인라인 모드로 실행해 터미널
> 스크롤백을 보존할 수 있습니다. spona가 출력을 기록하거나 세션을 로그로 남기는 모드를
> 제공한다면 이 플래그를 함께 쓰는 편이 사용자 경험이 낫습니다.
> Claude Code 쪽 대응은 `/tui fullscreen` 설정과 `CLAUDE_CODE_NO_FLICKER` 입니다.

## 5. 터미널 관련 환경변수

프리셋 격리를 위해 환경을 새로 구성할 때, **터미널 능력 변수를 빠뜨리면 TUI가 깨집니다.**
`os.Environ()`을 통째로 물려주고 프리셋 값만 덧씌우는 방식이 안전합니다.
화이트리스트 방식을 쓴다면 최소한 다음은 포함해야 합니다.

| 변수 | 역할 |
| --- | --- |
| `TERM` | 터미널 능력 |
| `COLORTERM` | truecolor 지원 여부 |
| `TERM_PROGRAM`, `TERM_PROGRAM_VERSION` | 터미널 종류별 분기(알림 채널, 백스페이스 처리 등) |
| `LANG`, `LC_*` | 문자 인코딩 |
| `HOME`, `PATH`, `SHELL` | 기본 실행 환경 |
| `TMUX`, `TMUX_PANE` | tmux 내부 여부 감지 |
| `NO_COLOR` | 색상 비활성화(Codex가 인식) |

### 하네스별 표시 관련 변수

| 변수 | 하네스 | 효과 |
| --- | --- | --- |
| `CLAUDE_CODE_NO_FLICKER` | Claude Code | 풀스크린 렌더링으로 시작 |
| `CLAUDE_CODE_FORCE_SYNC_OUTPUT` | Claude Code | 동기화 출력 강제(자동 감지 실패 시 깜빡임 해결) |
| `CLAUDE_CODE_TMUX_TRUECOLOR` | Claude Code | tmux에서 truecolor 활성화 |
| `CLAUDE_CODE_BS_AS_CTRL_BACKSPACE` | Claude Code | Windows 백스페이스 `^H` 해석 (`0`/`1`) |
| `CLAUDE_AX_SCREEN_READER` | Claude Code | 스크린리더 친화 출력 (`--ax-screen-reader`와 동일) |
| `FORCE_HYPERLINK` | Claude Code | 하이퍼링크 출력 강제 (숫자, `0`이면 끔) |
| `NO_COLOR` | Codex | 색상 끄기 |

### tmux 안에서 띄울 때

spona가 tmux 세션/윈도를 만들어 그 안에서 하네스를 띄운다면, 사용자의 `~/.tmux.conf`에
다음이 없으면 Shift+Enter와 데스크톱 알림·진행 표시가 동작하지 않습니다.

```tmux
set -g allow-passthrough on
set -s extended-keys on
set -as terminal-features 'xterm*:extkeys'
```

`allow-passthrough`는 알림·진행 업데이트가 바깥 터미널까지 도달하게 하고,
`extended-keys` 두 줄은 tmux가 Shift+Enter와 Enter를 구분하게 합니다.
spona가 tmux 통합을 제공한다면 이 설정을 **점검하고 안내**하는 것이 좋습니다.

> [!NOTE]
> Claude Code에는 `--tmux` 플래그가 이미 있습니다(`--worktree`와 함께 사용, iTerm2에서는
> 네이티브 페인 사용, `--tmux=classic`으로 전통적 tmux 강제). spona가 자체 tmux 통합을
> 만들기 전에 이 기능과의 역할 분담을 정해야 합니다.

## 6. 시그널과 종료 규약

| 상황 | 기대 동작 |
| --- | --- |
| 사용자 Ctrl+C | 하네스가 직접 받아 현재 턴을 취소. spona는 개입하지 않음 |
| 사용자 Ctrl+Z | 하네스가 정지. 방식 A/B에서는 자동으로 동작 |
| 터미널 리사이즈 | `SIGWINCH`. 방식 A/B는 자동, 방식 C는 직접 중계 |
| spona가 세션을 끝내야 할 때 | **`SIGINT` 먼저** → 유예 → `SIGTERM` |

Claude Code는 SIGTERM을 받으면 종료 코드 `143`으로 끝나며 **진행 중이던 턴을 결과 없이
미완료 상태로 남깁니다**(재개하면 그 턴부터 이어짐). 곧바로 SIGTERM을 보내면 사용자의 마지막
작업 결과가 유실되므로, SIGINT로 턴을 마무리시킨 뒤 종료하는 순서를 지켜야 합니다.

## 7. 대화형 전용 플래그

비대화형 문서에는 없지만 대화형 런처가 다뤄야 할 것들입니다.

### Claude Code

| 플래그 | 설명 |
| --- | --- |
| `-n, --name <name>` | 세션 표시 이름. 프롬프트 박스·`/resume` picker·터미널 타이틀에 표시 |
| `--ide` | 유효한 IDE가 정확히 하나면 시작 시 자동 연결 |
| `-w, --worktree [name]` | 세션용 git worktree 생성 |
| `--tmux` | worktree용 tmux 세션 생성(`--worktree` 필요) |
| `--remote-control`, `--rc [name]` | Remote Control 활성화 상태로 대화형 세션 시작 |
| `--chrome` / `--no-chrome` | Chrome 통합 |
| `-c, --continue` / `-r, --resume [term]` | 최근 대화 이어가기 / picker 열기 |
| `--teleport [session]` | 웹 세션을 로컬 터미널에서 재개 |
| `--ax-screen-reader` | 스크린리더 친화 출력(장식 테두리·애니메이션 제거) |
| `--allow-dangerously-skip-permissions` | bypassPermissions를 Shift+Tab 모드 순환에 **추가만** 함 |

`--name`은 spona가 프리셋 이름을 세션에 새겨 넣기에 적합합니다.
`--allow-dangerously-skip-permissions`는 "그 모드로 시작"이 아니라 "사용자가 선택할 수 있게
열어둠"이므로, 위험 옵션을 프리셋에서 표현할 때 `--dangerously-skip-permissions`와 구분해야 합니다.

### Codex

| 플래그 | 설명 |
| --- | --- |
| `--search` | 네이티브 `web_search` 도구 활성화(호출별 승인 없음) |
| `--no-alt-screen` | 인라인 모드, 터미널 스크롤백 보존 |
| `--remote <ADDR>` | 원격 app server에 TUI 연결 (`ws://`, `wss://`, `unix://PATH`) |
| `--remote-auth-token-env <ENV_VAR>` | 원격 연결 베어러 토큰이 담긴 환경변수 이름 |
| `codex resume [SESSION_ID]` / `--last` | 세션 재개 (기본은 picker) |
| `codex fork` | 세션 포크 |

서브커맨드 없이 `codex [OPTIONS] [PROMPT]` 로 호출하면 옵션이 그대로 TUI로 전달됩니다.
즉 spona는 **`codex exec`를 쓰지 않는 것만으로** 대화형이 됩니다.

## 8. 대화형에서만 나타나는 상호작용

프리셋을 적용해도 아래는 여전히 사용자에게 물어봅니다. spona가 이를 미리 처리하려 하거나
반대로 예상하지 못하면 UX가 어긋납니다.

| 상호작용 | 하네스 | 비고 |
| --- | --- | --- |
| 워크스페이스 신뢰 다이얼로그 | Claude Code | stdout이 TTY일 때만 표시 |
| MCP 서버별 승인 프롬프트 | Claude Code | `-p`에서는 표시되지 않음 |
| 권한 프롬프트 | 둘 다 | `--permission-mode` / `--ask-for-approval`로 조절 |
| 플러그인 신뢰 경고 | Claude Code | `pluginTrustMessage`로 문구 추가 가능 |
| 프로젝트 신뢰 여부 | Codex | untrusted면 `.codex/` 레이어 전체를 건너뜀 |
| 훅 신뢰 등록 | Codex | `config.toml`의 `[hooks.state]`에 기록 |
| 첫 실행 터미널 설정 안내 | Claude Code | `/terminal-setup` 실행 여부를 묻는 프롬프트 |
| 모델 마이그레이션 안내 | Codex | `[notice.model_migrations]`에 확인 기록 |

이 상태들은 대부분 **설정 홈 디렉터리에 기록**됩니다. 즉 spona가 프리셋마다
`CLAUDE_CONFIG_DIR` / `CODEX_HOME`을 갈아끼우면 **프리셋을 처음 쓸 때마다 온보딩·신뢰
프롬프트가 다시 뜹니다.**

> [!IMPORTANT]
> 격리(프리셋별 독립 홈)와 매끄러운 UX(신뢰·온보딩 상태 공유)는 정면으로 충돌합니다.
> spona는 둘 중 하나를 고르거나, 프리셋 홈을 만들 때 사용자 홈에서 신뢰·온보딩 관련
> 상태만 골라 시드하는 절충안을 설계해야 합니다.
>
> - Claude Code: `~/.claude.json`(로그인 세션, 프로젝트별 신뢰 결정), `~/.claude/settings.json`
> - Codex: `$CODEX_HOME/auth.json`, `config.toml`의 `[projects]`·`[hooks.state]`·`[notice]`

## 9. 인증 — 대화형에서 더 유리한 점

비대화형과 달리 대화형 세션은 **기존 로그인 자격증명을 그대로 쓸 수 있습니다.**

- Claude Code: OAuth 로그인과 시스템 키체인을 사용합니다. 단 `--bare`는 이를 **읽지 않으므로**,
  대화형 런처의 기본값으로 `--bare`를 쓰면 구독 로그인이 무력화됩니다.
- Codex: `$CODEX_HOME/auth.json`을 사용합니다. `CODEX_HOME`을 갈아끼우면 그 프리셋 홈에서
  다시 `codex login`을 해야 합니다.

> [!WARNING]
> `--bare`는 [spawn.md](spawn.md)에서 스크립트·CI의 권장 모드로 소개했지만,
> **대화형 런처의 기본값으로는 부적절합니다.** 훅·스킬·커스텀 커맨드·서브에이전트·플러그인·
> MCP·`CLAUDE.md`가 전부 빠지고 인증도 API 키만 허용되므로, 사용자가 터미널에서
> `claude`를 친 것과 전혀 다른 결과가 됩니다.

## 10. spona 런처 기준 동작

```sh
# Claude Code — 프리셋을 얹은 대화형 세션
CLAUDE_CONFIG_DIR=<preset-home> \
  claude \
    --model "$MODEL" \
    --permission-mode "$MODE" \
    --name "$PRESET_NAME" \
    --settings "$PRESET_JSON" \
    --mcp-config "$MCP_JSON" \
    --agents "$AGENTS_JSON" \
    --add-dir "$EXTRA_DIRS"
# TTY 상속. --print / --output-format 을 절대 붙이지 않음
```

```sh
# Codex — 프리셋을 얹은 대화형 세션
CODEX_HOME=<preset-home> \
  codex \
    --model "$MODEL" \
    --sandbox "$SANDBOX" \
    --ask-for-approval "$APPROVAL" \
    --cd "$WORKDIR" \
    -c model_reasoning_effort=high
# 서브커맨드 없음 = TUI
```

체크리스트:

- [ ] stdin/stdout/stderr를 그대로 물려준다 (파이프·버퍼 금지)
- [ ] 유닉스에서는 `syscall.Exec`로 프로세스를 치환한다
- [ ] `Setpgid`를 켜지 않는다
- [ ] `SIGINT`를 가로채지 않는다
- [ ] `TERM`/`COLORTERM`/`TERM_PROGRAM`/`LANG`을 포함한 환경을 넘긴다
- [ ] 대화형 경로에서 `-p`, `--output-format`, `codex exec`를 쓰지 않는다
- [ ] 대화형 기본값으로 `--bare`를 쓰지 않는다
- [ ] 프리셋 홈 격리 시 신뢰·인증 상태 시드 정책을 정한다
