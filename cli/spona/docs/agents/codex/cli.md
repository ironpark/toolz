# OpenAI Codex CLI (`codex`) — CLI 옵션과 서브커맨드

> [README](README.md) · [../](../README.md)

## 공통 옵션 (대부분의 서브커맨드에 존재)

| 옵션 | 설명 |
| --- | --- |
| `-c, --config <key=value>` | `config.toml` 값을 덮어씀. 점 표기(`foo.bar.baz`) 지원. 값은 TOML로 파싱, 실패 시 문자열 리터럴 |
| `--enable <FEATURE>` / `--disable <FEATURE>` | `-c features.<name>=true/false` 와 동등(반복 가능) |
| `--strict-config` | 이 버전이 모르는 config 필드가 있으면 에러 |
| `-p, --profile <NAME>` | `$CODEX_HOME/<name>.config.toml` 을 기본 설정 위에 레이어링 |
| `-m, --model <MODEL>` | 사용할 모델 |
| `--oss` | 오픈소스 프로바이더 사용 |
| `--local-provider <lmstudio\|ollama>` | 로컬 프로바이더 선택 |
| `-i, --image <FILE>...` | 초기 프롬프트에 이미지 첨부 |
| `-C, --cd <DIR>` | 에이전트 작업 루트 |
| `--add-dir <DIR>` | 주 워크스페이스와 함께 쓰기 가능한 추가 디렉터리 |
| `-s, --sandbox <MODE>` | `read-only`, `workspace-write`, `danger-full-access` |
| `-a, --ask-for-approval <POLICY>` | `on-request`(모델이 판단해 요청), `never`(승인 없이 진행, 실패는 모델에 반환) |
| `--approve-for-me` | 승인 요청을 workspace-write 샌드박스 기반 자동 검토로 라우팅 |
| `--dangerously-bypass-approvals-and-sandbox` | 모든 확인·샌드박스 생략 (**매우 위험**, 외부 샌드박스 환경 전용) |
| `--dangerously-bypass-hook-trust` | 훅 신뢰 등록 없이 훅 실행 (**위험**) |

CLI의 `--sandbox` / `--ask-for-approval` 은 config의 `sandbox_mode` /
`approval_policy` 를 덮어쓰는 세션 한정 오버라이드입니다.

### 대화형(TUI) 전용

| 옵션 | 설명 |
| --- | --- |
| `--search` | 네이티브 `web_search` 도구 활성화(호출별 승인 없음) |
| `--no-alt-screen` | 인라인 모드로 실행해 터미널 스크롤백 보존 |
| `--remote <ADDR>` | 원격 app server에 TUI 연결 (`ws://`, `wss://`, `unix://PATH`) |
| `--remote-auth-token-env <ENV_VAR>` | 원격 app server 베어러 토큰이 담긴 환경변수 이름 |

## `codex exec` 추가 옵션 (프로그래매틱 spawn 핵심)

프롬프트를 생략하거나 `-` 를 주면 stdin에서 읽습니다. stdin이 파이프이면서 프롬프트도
있으면 stdin이 `<stdin>` 블록으로 덧붙습니다.

| 옵션 | 설명 |
| --- | --- |
| `--json` | 이벤트를 JSONL로 stdout 출력 |
| `-o, --output-last-message <FILE>` | 마지막 메시지를 파일로 기록 |
| `--output-schema <FILE>` | 최종 응답 형태를 규정하는 JSON Schema 파일 |
| `--color <always\|never\|auto>` | 색상 (기본 `auto`) |
| `--ephemeral` | 세션 파일을 디스크에 저장하지 않음 |
| `--ignore-user-config` | `$CODEX_HOME/config.toml` 미로드 (인증은 여전히 `CODEX_HOME` 사용) |
| `--ignore-rules` | user/project execpolicy `.rules` 파일 미로드 |
| `--skip-git-repo-check` | Git 저장소 밖에서도 실행 허용 |
| `--thread-source <SOURCE>` | 새로 생성/포크되는 스레드의 소스 분류 |

`--ephemeral --ignore-user-config --ignore-rules` 조합 + 명시적 `-c` 오버라이드가
spona의 재현 실행에 가장 적합합니다.

하위 서브커맨드: `codex exec resume`, `codex exec fork`, `codex exec review`.

## 서브커맨드

| 명령 | 설명 |
| --- | --- |
| `exec` (`e`) | 비대화형 실행 |
| `review` | 비대화형 코드 리뷰. `--uncommitted`, `--base <BRANCH>` |
| `resume` / `fork` | 세션 재개 / 포크. `[SESSION_ID]`는 UUID 또는 세션 이름, `--last`로 최근 세션 |
| `queue` | 기존 세션에 메시지 큐잉 |
| `archive` / `unarchive` / `delete` | 세션 관리(id 또는 이름) |
| `agents` | 공유 로컬 app-server 데몬의 모든 에이전트 세션 브라우징 |
| `login` / `logout` | 인증 관리 |
| `mcp` | 외부 MCP 서버 관리: `list`, `get`, `add`, `remove`, `login`, `logout` |
| `plugin` | 플러그인 관리 |
| `mcp-server` | Codex를 stdio MCP 서버로 실행 |
| `app-server` / `exec-server` / `remote-control` | (실험) 서버·원격 제어 |
| `sandbox` | Codex 제공 샌드박스 안에서 명령 실행 |
| `doctor` | 설치·설정·인증·런타임 진단 |
| `cloud` | (실험) Codex Cloud 태스크 조회 및 로컬 적용 |
| `apply` (`a`) | 마지막 diff를 `git apply` |
| `features` | 피처 플래그 확인 |
| `completion` | 셸 자동완성 스크립트 생성 |
| `update` | 최신 버전으로 업데이트 |
