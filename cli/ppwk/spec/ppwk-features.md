# ppwk 기능 명세

완성 시점에 지원해야 하는 명령어와 기능 / v1 범위

버전 5.2 / 2026-09-02

설계: `ppwk-design.md` v5.2 · 구현 계획: `ppwk-implementation.md` v5.2 · E2E: `ppwk-e2e.md` v5.2 · 결정 기록: `ppwk-decisions.md`

---

## 0. 전역 규약

### 0.1 전역 플래그

모든 명령에 적용된다.

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `--json` | off | 출력을 JSON 으로. 스크립트/에이전트용 |
| `--agent <id>` | 자동 감지 | 에이전트 신원 override |
| `-C <path>` | cwd | 저장소 경로 |
| `-q, --quiet` | off | 오류만 출력 |
| `-v, --verbose` | off | git 명령 실행 로그 포함 |
| `--no-color` | 자동 | TTY 가 아니면 자동으로 켜짐 |
| `--timeout <dur>` | 10s | CAS 재시도 총 상한 |

### 0.2 에이전트 신원 결정 순서

```
1. --agent 플래그
2. PPWK_AGENT 환경변수
3. 도구 감지 (§3.8 층 2)      claude-code:<worktree> / codex:<worktree>
4. git config ppwk.agent
5. <hostname>:<worktree basename>
```

### 0.2.1 세션 ID 결정 순서

```
1. PPWK_SESSION 환경변수
2. CLAUDE_CODE_SESSION_ID 등 도구 세션 ID (§3.8 층 2)
3. SessionStart 훅이 등록한 세션 (§3.8 층 3)
4. 현재 프로세스 (단발 실행)
```

2번이 잡히면 **같은 대화에서 실행된 모든 명령이 같은 세션으로 묶인다.** 이것이 `--mine` 필터의 근거가 된다.

세션 nonce 는 세션 ID 를 얻지 못했을 때만 128비트 랜덤으로 생성한다. OID 충돌 방지(§4.3)에는 어느 쪽이든 충분하다.

### 0.2.2 last_activity 갱신

상태 변경 명령만 갱신한다. 조회 명령(`list`, `show`, `history`, `agents`, `watch`, `export`, `doctor`, `fsck`)은 갱신하지 않는다. **읽기는 쓰지 않는다.**

### 0.3 종료 코드

| 코드 | 의미 | 에이전트 대응 |
|---|---|---|
| 0 | 성공 | 계속 |
| 1 | 일반 오류 | 중단 |
| 2 | 사용법 오류 | 중단 (버그) |
| 3 | 전이 규칙 위반 | 중단 (로직 오류) |
| 4 | CAS 경쟁 실패 | **재시도 가능** |
| 5 | 대상 없음 | 상황에 따라 |
| 6 | 스키마 버전 불일치 | 중단, 업그레이드 필요 |

**3과 4의 구분이 에이전트 동작을 좌우한다.** 4는 남이 먼저 가져간 것이므로 다른 작업을 찾으면 되고, 3은 코드가 잘못된 상태 전이를 시도한 것이다.

### 0.4 출력 형식

기본 출력은 사람이 읽는 표. `--json` 은 다음 형태로 통일한다.

```json
{"ok": true, "data": {...}}
{"ok": false, "error": {"code": 4, "kind": "cas_conflict", "message": "..."}}
```

`list` 계열은 `data` 가 배열, 단건 조회는 객체다. TTY 가 아니면 색상과 진행 표시를 자동으로 끈다.

---

## 1. 저장소 관리

### `ppwk init`

보드를 초기화한다. 저장소당 한 번.

```
ppwk init [--no-agents-md]
```

수행 내용:

1. `meta/schema` ref 생성 (없으면)
2. `git config --add log.excludeDecoration refs/ppwk/`
3. `git config core.filesRefLockTimeout 1000`
4. `git` 버전 확인 (최소 2.28)
5. 에이전트 문서 생성 (§1.1)

### 1.1 생성되는 에이전트 문서

```
AGENTS.md                              항상 로드되는 진입점 (~50줄)
docs/ppwk/
├─ query.md            조회 명령
├─ authoring.md        이슈 생성·수정
├─ states.md           상태와 전이 규칙
├─ plans.md            plan / phase
├─ decisions.md        결정 기록
├─ troubleshooting.md  오류와 복구
├─ git-behavior.md     git 동작과 가시성
└─ project.md          저장소별 지침 (빈 템플릿)
```

**분리 이유는 context 예산이다.** 에이전트는 매 세션 `AGENTS.md` 를 컨텍스트에 싣는다. 여기에 전체 매뉴얼을 담으면 실제 작업에 쓸 토큰이 줄어든다. 그래서 `AGENTS.md` 에는 **기본 루프를 돌리는 데 필요한 최소한**만 두고, 나머지는 상황별 링크로 뺀다.

배치 기준:

| `AGENTS.md` 에 두는 것 | 별도 파일로 빼는 것 |
|---|---|
| 매 세션 필요 | 특정 상황에만 필요 |
| 모르면 데이터가 깨짐 | 모르면 불편할 뿐 |
| 기본 루프 구성 요소 | 참조용 상세 |

`AGENTS.md` 의 라우팅 표는 **주제명이 아니라 상황**으로 적는다. "plans.md — 계획 문서" 대신 "계획 작업일 때 → plans.md" 로 쓴다. 에이전트는 자기가 처한 상황은 알지만 문서 분류 체계는 모르기 때문이다.

각 하위 문서는 **자족적이다.** 다른 문서를 읽어야 이해되는 구성을 피하고, 각각 상단에 `AGENTS.md` 로 돌아가는 링크를 둔다.

생성 규칙:

- 이미 존재하는 파일은 건드리지 않는다 (개별 파일 단위로 판단)
- `--no-agents-md` 로 전체 생성을 건너뛴다
- `project.md` 는 빈 템플릿이며 사용자가 채운다
- 전부 **저장소에 커밋되는 tracked 파일**이다. 보드 데이터와 달리 `init` 이후 `git status` 에 나타난다

멱등하다. 두 번 실행해도 안전하며, `--force` 는 기존 hook 을 덮어쓸 때만 필요하다.

**경고 출력:**

- `git log --all` 에 이슈 커밋이 섞임 → 별칭 제안
- `git push --mirror` 가 이슈 내용을 원격에 노출함

### `ppwk doctor`

환경을 점검한다. `init` 이후 문제 진단용.

```
ppwk doctor
```

점검 항목:

```
git 바이너리 및 버전
$GIT_COMMON_DIR 접근 가능 여부
linked worktree 목록과 각각의 가시성
schema 버전 일치
도구 훅 설치 상태
현재 에이전트 신원과 lease 상태
stale .lock 파일 존재 여부 (보고만 한다 — 지우지 않는다)
도구 감지 결과와 감지 근거
도구 훅 설치 상태
```

도구 감지는 **근거를 함께 표시한다.** 환경변수 이름이 도구 버전에 따라 바뀔 수 있으므로, 사람이 감지가 맞는지 확인할 수 있어야 한다.

```
tool detection   claude-code       via CLAUDECODE
session id       7f3a2c1d-...      via CLAUDE_CODE_SESSION_ID
agent id         claude-code:repo-a
liveness         hook_pid 48211 alive   즉시 감지
                 (훅 없으면 last_activity 8h — WARN)
worktree         /repo-a  배타 확보
holding          T001, T005
tool hooks       SessionStart ✓  SessionEnd ✓
```

감지에 실패하면 폴백을 표시하며, 이는 FAIL 이 아니라 정보다.

각 항목을 OK / WARN / FAIL 로 보고한다. FAIL 이 하나라도 있으면 exit 1.

### `ppwk version`

CLI 버전, 스키마 버전, git 버전, go-git 버전을 출력한다.

---

## 2. 이슈 생성과 조회

### `ppwk add`

```
ppwk add <title>
    [--priority high|med|low|none] 기본 med. none 은 next 후보 제외 (백로그)
    [--label <label>]              반복 가능
    [--depends-on <id>]            반복 가능
    [--body <text> | --body-file <path> | --body-stdin]
    [--plan <id> --phase <id> [--seq <n>]]
```

생성된 이슈 ID 를 stdout 에 출력한다. `--json` 이면 이슈 전체.

제약:

- 제목 필수, 빈 문자열 거부
- 제목에 개행이 있으면 첫 줄만 subject, 나머지는 body 로
- `--plan` 과 `--phase` 는 함께 지정해야 함
- `--seq` 생략 시 해당 phase 최대값 + 10
- 자기 자신 의존 거부

### `ppwk list`

```
ppwk list
    [--status open|claimed|working|blocked|done|cancelled]   반복 가능
    [--priority high|med|low|none]                           반복 가능
    [--owner <agent>]
    [--label <label>]
    [--plan <id>]
    [--phase <id>]
    [--unassigned]
    [--mine]
    [--archived]           archive 만
    [--all]                issues + archive
    [--sort next|id|updated|priority]
    [--limit <n>]
```

기본은 `issues/` 만, `done`/`cancelled` 제외 없음 (아직 archive 안 된 것 포함).

`--priority none` 이 백로그다. 상태는 `open` 이지만 `next` 가 후보로 고르지 않는다. 별도 상태를 두지 않는 이유는 "당분간 안 함" 이 작업의 속성이지 상태가 아니기 때문이다 — 전이 규칙·gate·회수가 전부 그대로 적용된다.

```bash
ppwk add "언젠가 리팩터링" --priority none
ppwk list --priority none              # 백로그 보기
ppwk edit T042 --priority low          # 꺼내기
```

`--mine` 은 현재 대화 세션에서 claim 한 이슈만 보여준다. 세션 ID 가 도구에서 감지된 경우(§0.2.1) 의미 있게 동작한다.

`--sort next` 는 `next` 가 쓰는 것과 동일한 정렬이다. 에이전트가 "다음에 뭐가 올지" 미리 볼 때 쓴다.

출력:

```
ID     STATUS    OWNER      PLAN  PHASE  TITLE
T001   working   agent-a    P01   p2     SQLite storage 구현
T002   open      -          P01   p2     parser cleanup
T005   blocked   agent-b    -     -      migration script
```

`for-each-ref` 한 번으로 처리한다 (§5.1).

### `ppwk show`

```
ppwk show <id> [--json]
```

이슈 전체를 출력한다. `archive` 에 있어도 찾는다.

```
T001  SQLite storage 구현

Status      working
Owner       agent-a (session 8f3a2c1d)
Priority    high
Plan        P01 / p2 구현 (seq 30)
Labels      storage, backend
Depends on  T000 (done)
Blocks      T004
Created     2026-08-30 04:12  by agent-a
Updated     2026-08-30 05:40  by agent-a

<body.md 내용>
```

### `ppwk history`

```
ppwk history <id> [-n <count>] [--json]
```

commit chain 을 이벤트 순서로 출력한다. subject 가 이벤트명이므로 가공이 거의 없다 (§5.3).

```
q8n1  06:03  agent-a  done: SQLite storage 구현
p4k2  05:14  agent-a  start: SQLite storage 구현
n1x9  05:12  agent-a  claim: SQLite storage 구현
c1a2  04:12  agent-a  create: SQLite storage 구현
```

### `ppwk edit`

메타데이터를 수정한다. 상태는 바꾸지 않는다.

```
ppwk edit <id>
    [--title <text>]
    [--priority P]
    [--add-label L] [--remove-label L]
    [--add-depends-on ID] [--remove-depends-on ID]
    [--body-file <path>]
    [--plan <id> --phase <id>] [--seq <n>]
    [--clear-plan]
```

CAS 를 거친다. 다른 에이전트가 동시에 상태를 바꾸면 exit 4.

---

## 3. 상태 전이

전이 규칙은 §3.5 를 따른다. 모든 명령이 CAS 를 거친다.

| 명령 | 전이 | 비고 |
|---|---|---|
| `claim <id>` | open → claimed | 예약만. 시작은 나중에 |
| `start <id>` | open → working, claimed → working | open 이면 claim 을 겸한다 (D16) |
| `done <id>` | working → done | archive 로 이동 |
| `block <id> [--on <id>] [--message T]` | working → blocked | `--on` 은 차단 원인 이슈, `--message` 는 사유 |
| `unblock <id>` | blocked → working | |
| `release <id>` | claimed → open | 소유권 반납. `--force` 면 working 도 (§4.5) |
| `cancel <id>` | any → cancelled | archive 로 이동 |

공통 플래그:

```
--force      소유자가 아니어도 강제 (release, cancel)
--mine       현재 세션이 보유한 이슈 전체에 적용 (release)
--allow-shared-worktree   worktree 배타를 건너뜀
--message    이벤트 subject 에 사유 추가
--retry <n>  CAS 실패 시 재시도 횟수 (기본 0 — 즉시 exit 4)
```

`--retry` 기본이 0 인 이유: **경쟁에서 진 것은 재시도가 아니라 다른 작업을 찾을 신호다.** `next` 가 이 판단을 대신 해준다.

### 거부되는 경우

- 이미 `done` 인 이슈에 어떤 전이도 불가 (`cancel` 포함) → exit 3
- 다른 에이전트 소유 이슈에 `start`/`done` → exit 3
- 이미 `done` 인 이슈를 다시 `done` → **exit 3.** 멱등 성공으로 처리하지 않는다. 실수를 숨기지 않기 위함이다
- 자기 자신을 block → exit 3

---

## 4. 스케줄링

### `ppwk next`

에이전트가 실제로 호출하는 유일한 스케줄링 명령이다.

```
ppwk next
    [--claim]              후보를 claim 까지 수행
    [--plan <id>]          특정 plan 으로 제한
    [--label <label>]      capability 필터
    [--dry-run]            후보 목록만 표시
    [--max-attempts <n>]   claim 시도 상한 (기본 5)
```

동작 (§7.2):

```
1. reap 실행
2. open 수집 (priority none 제외)
3. depends_on 검사 (archive 포함)
4. phase gate 검사
5. 정렬: plan priority → seq → priority → created_at
6. --claim 이면 상위부터 CAS 시도, 실패 시 다음 후보
```

후보가 없으면 **exit 0 에 빈 결과**다. 오류가 아니다. 에이전트는 이걸 "할 일 없음" 으로 해석하고 대기하면 된다.

`--claim` 없이 부르면 reap 은 수행하되 배정은 하지 않는다. `--dry-run` 은 reap 도 건너뛰어 **저장소를 전혀 변형하지 않는다.**

### `ppwk internal` (비공개)

```
ppwk internal session-event
```

도구 훅에서만 호출된다. stdin 의 JSON(`session_id`, `cwd`, `hook_event_name`)을 읽어 `SessionStart` 는 세션을 등록하고 `hook_pid` 를 기록하며, `SessionEnd` 는 **`claimed` 만** `open` 으로 되돌린다. `working` 은 미커밋 작업이 있을 수 있어 건드리지 않는다 (D15).

`hook install` 이 이 명령을 훅 설정에 써넣으므로 **사용자는 존재를 알 필요가 없다.** `--help` 에 노출하지 않는다.

훅에서 실행되므로 **빠르게 끝내고 세션을 막지 않는다.** 알 수 없는 입력이나 오류를 만나면 조용히 exit 0 한다.

### `ppwk reap`

```
ppwk reap [--dry-run] [--json]
```

죽은 소유자가 붙잡고 있던 이슈를 `open` 으로 되돌린다. 소유자 생존은 잠금으로 판정한다 (§4.5).

**평소에는 `next` 가 자동으로 수행하므로 직접 부를 일이 드물다.** 진단이나 수동 복구용이다.

`--dry-run` 은 회수 대상만 보여주고 변경하지 않는다.

### `ppwk agents`

```
ppwk agents [--json]
```

`$GIT_COMMON_DIR/ppwk/locks/` 의 잠금 파일을 읽어 출력한다. ref 가 아니다 (D13).

```
AGENT      SESSION   WORKTREE   PID     STATUS   HOLDING   FOR
agent-a    8f3a2c1d  /repo-a    48211   alive    T001      12m
agent-b    9c04ea11  /repo-b    48377   DEAD     T003      —
agent-c    2ea8b730  /repo-c    48512   alive    T009      3h12m
```

`STATUS` 는 잠금 확인 결과다. `DEAD` 는 다음 `next` 또는 `reap` 에서 회수될 대상이다.

`FOR` 는 해당 이슈를 보유한 시간이다. **멈춘 프로세스를 사람이 판단하는 근거가 된다.** 잠금 방식은 살아있지만 진전이 없는 프로세스를 자동 회수하지 않으므로, 비정상적으로 긴 보유는 여기서 드러난다.

```bash
ppwk release T009 --force    # 사람이 판단해 강제 회수
```

---

## 5. plan 과 phase

### `ppwk plan new`

```
ppwk plan new <title> [--priority P] [--id <id>]
```

### `ppwk plan phase add`

```
ppwk plan phase add <plan> <title>
    [--gate all_done|any_done|manual]    기본 all_done
    [--id <phase-id>]
    [--before <phase-id> | --after <phase-id>]
```

### `ppwk plan show`

```
ppwk plan show <plan> [--json]
```

진행률과 현재 phase 를 **파생 계산**해서 보여준다. 저장된 값이 아니다 (§3.7.1).

```
P01  storage 레이어 재작성          [active]

  p1  스키마 설계                    3/3  done
      T001  done     agent-a   테이블 정의
      ...

  p2  구현                          1/4  working      ← 현재 phase
      T004  done     agent-a   SQLite storage 구현
      T005  working  agent-b   parser cleanup
      T006  open     -         에러 처리

  p3  마이그레이션                   0/2  blocked (gate: manual)
      T008  open     -         migration script
```

p3 의 task 는 `status` 가 `open` 이다. `blocked (gate)` 는 **표시상의 파생값**이며 저장된 상태가 아니다.

### 나머지 plan 명령

```
ppwk plan list [--status active|closed|cancelled]
ppwk plan advance <plan> <phase>     manual gate 개방
ppwk plan close <plan>
ppwk plan cancel <plan>
ppwk plan edit <plan> [--title T] [--priority P]
ppwk plan phase edit <plan> <phase> [--title T] [--gate G]
ppwk plan phase remove <plan> <phase>    소속 task 있으면 거부
```

---

## 5.5 결정 기록

불변 ADR 을 ref 에 저장한다 (§3.9). 상태 머신과 동시성 모델에 영향이 없다.

### `ppwk decide`

```
ppwk decide <title>
    --context <text>              배경
    --option <text>               검토한 선택지. 반복 가능
    --decision <text>             택한 것
    [--consequences <text>]       결과·재검토 조건
    [--issue <id>]                관련 이슈. 반복 가능
    [--plan <id>]
    [--supersedes <id>]           대체하는 이전 결정
    [--body-file <path>]          긴 근거
```

생성된 ID(`D007`)를 출력한다. **수정 명령이 없다.** 바꾸려면 `--supersedes` 로 새 결정을 만든다.

### `ppwk decisions`

```
ppwk decisions                     유효한 것만 (superseded 제외)
ppwk decisions --all
ppwk decisions --issue <id>        이슈와 연결된 결정
ppwk decisions --plan <id>
ppwk decisions --search <text>     제목·본문 검색
ppwk decisions show <id>
ppwk decisions history <id>        supersedes 체인
```

`show` 출력:

```
D007  저장소는 SQLite                          2026-09-02  claude-code:repo-a

Context      단일 머신, 동시 쓰기 적음, 배포 단순화 필요
Options      SQLite · PostgreSQL · 파일 기반 JSON
Decision     SQLite
Consequences 동시 쓰기 확장 시 재검토

Issues       T001, T004
Plan         P01
Supersedes   D003 (파일 기반 JSON)
Superseded by —
```

`Superseded by` 는 저장된 값이 아니라 조회 시 계산된다.

### 이슈 연동

`show T001` 이 연결된 결정을 함께 표시한다.

```
T001  SQLite storage 구현
...
Decisions    D007 저장소는 SQLite
```

### export

```
ppwk export --decisions [-o docs/decisions/]
```

결정 하나당 ADR 마크다운 파일 하나를 만든다. 헤더에 생성 시각과 "파생물" 경고가 들어간다. 이 파일들은 평범하게 커밋한다.

---

## 6. 변경 감지

### `ppwk watch`

```
ppwk watch
    [--interval <dur>]     polling 주기, 기본 2s
    [--filter <prefix>]    특정 ref prefix 만
    [--json]               줄당 JSON (기본)
```

이벤트 형식:

```json
{"ref":"refs/ppwk/issues/T001","old":"abc...","new":"def...","kind":"updated","id":"T001","status":"working"}
```

`kind` 는 `created` / `updated` / `deleted`.

**첫 실행 시 기존 ref 를 전부 created 로 쏟지 않는다.** 베이스라인만 잡고 이후 변경부터 보고한다.

polling 만 쓴다. git 의 `reference-transaction` 훅 경로는 채택하지 않았다 — 아래 참조.

### `ppwk hook install / uninstall / status`

```
ppwk hook install [--agent-tools] [--claude-code] [--codex] [--force]
ppwk hook uninstall [--agent-tools] [--claude-code] [--codex]
ppwk hook status [--json]
```

도구의 대화 세션 훅만 다룬다 (§3.8 층 3).

| 플래그 | 대상 | 설치 위치 | 목적 |
|---|---|---|---|
| `--claude-code` | `SessionStart`/`SessionEnd` | `.claude/settings.json` | 세션 신원·정리 |
| `--codex` | `SessionStart`/`SessionEnd` | `.codex/hooks.json` | 동일 |
| `--agent-tools` | 위 둘 | | |

두 도구의 훅 표면이 대칭이므로 **같은 명령이 양쪽에 등록된다.** 서브에이전트 이벤트에는 등록하지 않는다 (§3.8).

기존 설정은 **병합**한다. 남의 훅과 나란히 서고, 우리가 모르는 설정 키는 원본 그대로 보존한다 — 사람이 손대는 파일이기 때문이다. 충돌은 "우리 것으로 보이는데 내용이 다른 항목"(예: 옛 경로의 `ppwk`)이며 그때만 중단한다. `--force` 는 그 경우에만 필요하다.

`status` 출력:

```
claude-code  SessionStart ✓  SessionEnd ✓  .claude/settings.json
codex        not configured                .codex/hooks.json
```

**Codex 훅 주의사항** — 실험적 기능이라 기본 비활성이며 Windows 를 지원하지 않는다. 프로젝트 로컬 훅은 `/hooks` 에서 신뢰 검토를 거쳐야 실행된다. `install --codex` 가 이를 안내한다.

### git `reference-transaction` 훅은 두지 않는다

`watch` 는 polling 만으로 동작한다 (§6.2). git 훅 경로가 사 오는 것은 알림 지연 1~2초를 없애는 것뿐인데 (§6.1 의 표), 함께 사 와야 하는 것이 크다.

- `socat` 외부 의존. 훅과 listener 사이를 이어 줄 unix socket 이 필요하다
- 공용 hooks 디렉터리에 설치되므로 **그 저장소의 모든 git 작업**이 훅을 거친다
- socket 수명 관리 — stale 파일 정리, listener 하나만 허용
- SHA-1 / SHA-256 zero OID 양쪽 처리. 틀리면 created/deleted 판정이 전부 뒤집힌다
- 훅이 git 프로세스 안에서 동기 실행되므로, 잘못 만들면 그 저장소의 모든 ref 쓰기가 멈춘다

에이전트는 대화 턴마다 `next --claim` 을 한 번 부르고 (§8.2), `watch` 를 소비하는 쪽은 사람이 보는 대시보드나 오케스트레이터다. 거기서 2초는 보이지 않는다.

---

## 7. 운영

### `ppwk export`

```
ppwk export
    [--format json|md|csv]     기본 json
    [--all]                    archive 포함
    [--decisions]              ADR 마크다운 (§5.5)
    [--plan <id>]
    [-o <path>]                기본 stdout
```

**단방향 파생물이다.** 생성된 파일을 편집해도 반영되지 않는다. md/csv 헤더에 생성 시각과 경고를 넣는다.

### import 는 두지 않는다

`export --format json` 은 **현재 문서만** 담고 commit chain 을 담지 않는다. 그런데 §3.3 은 그 parent 체인 자체가 이력이라고 정의한다. 되돌려 넣으면 반드시 import commit 하나짜리 가짜 이력이 생기고, 감사 추적이 사라진다. 단방향 파생물의 역방향은 손실이 있을 수밖에 없다.

용도별로 더 나은 답이 이미 있다.

| 하려던 것 | 대신 쓸 것 |
|---|---|
| 백업·복원 | `git bundle create ppwk.bundle refs/ppwk/*` — 이력까지 그대로 보존한다 |
| 다른 저장소로 옮기기 | `git push --mirror` 또는 명시적 refspec push (§12) |
| 외부 트래커에서 초기 이관 | `ppwk add` 셸 루프 — 이슈마다 제대로 된 create commit 이 남는다 |

추가로, import 는 `owner`/`session` 을 임의로 써넣을 수 있어 잠금 모델(§3.6)을 우회한다.

### `ppwk fsck`

```
ppwk fsck [--fix] [--json]
```

검사 항목 (§9.3):

```
trailer 와 issue.json 의 status 불일치
depends_on 이 존재하지 않는 ID 를 가리킴
의존성 순환
owner 가 있는데 대응 잠금 파일 없음
종료 상태인데 issues/ 에 남아있음
schema 버전 불일치
task 의 plan/phase 가 존재하지 않음
plan/phase/seq 중 일부만 존재
plan 이 closed 인데 미완 task 존재
task 가 없는 phase (gate 공허참 통과)
같은 phase 안 seq 중복 (경고)
advanced_phases 가 존재하지 않는 phase 를 가리킴
depends_on 대상이 cancelled (경고)
활성 이슈가 priority none 이슈에 의존 (경고)
결정의 issues/plan/supersedes 가 존재하지 않는 ID 를 가리킴
supersedes 순환
stale .lock 파일 (경고, 자동 삭제 안 함)
```

`--fix` 는 **trailer 재생성과 archive 이동만** 자동 처리한다. 나머지는 보고만 한다. 판단이 필요한 수정을 도구가 임의로 하지 않는다.

### gc 는 두지 않는다

정리는 git 이 이미 한다. `git gc` 가 `pack-refs` 와 dangling commit 정리(CAS 에서 밀려 버려진 commit)를 함께 처리하므로, 얇게 감싸 봐야 우리가 더하는 것이 없다.

우리가 아는 것은 **지금 얼마나 쌓였는지** 뿐이고 그것은 `doctor` 의 `refs` 항목이 보고한다.

```
refs   OK   issues 42, archive 318   via loose 12
```

loose ref 가 임계값을 넘으면 WARN 하고 `git gc` 를 안내한다. ppwk 는 ref 를 `update-ref` 로만 쓰는데 그것은 auto-gc 를 유발하지 않으므로, 사람이 커밋을 하지 않는 저장소에서는 저절로 정리되지 않는다 — 그래서 안내가 필요하다 (§9.2).

### `ppwk archive`

```
ppwk archive <id>            수동 이동
ppwk archive --sweep         종료 상태인데 issues/ 에 남은 것 일괄 이동
ppwk unarchive <id>          v1 미지원, 명시적 오류
```

평소에는 `done`/`cancel` 이 자동으로 이동하므로 `--sweep` 은 복구용이다.

---

## 8. 에이전트 통합

### 8.0 ppwk 가 하지 않는 것

**작업 배정과 메시지 전달은 오케스트레이터의 몫이다.** ppwk 는 그 메시지가 가리키는 **공유 상태**만 담당한다.

| | 오케스트레이터 | ppwk |
|---|---|---|
| 누가 무엇을 할지 결정 | 담당 | 관여 안 함 |
| 에이전트에게 메시지 전달 | 담당 | 관여 안 함 |
| 에이전트 프로세스 수명 | 담당 | 관여 안 함 |
| 세션 종료 시 정리 | 담당 (`release --mine`) | 훅 있으면 보조 |
| 작업 목록과 상태 | | 담당 |
| 중복 방지 (CAS) | | 담당 |
| 의존성과 순서 | | 담당 |
| 죽은 작업 회수 | | 담당 |

따라서 `assign` 류의 명령을 두지 않는다. **배정은 메시지가 이미 하고 있으며**, ref 에 중복 기록하면 상태가 두 곳에 생겨 어긋난다. 배정 대상을 바꿨을 때 ref 를 정리하는 책임이 생기고, 잊으면 거짓 정보가 남는다.

이슈는 `open` 으로 두면 된다. 아무도 받지 않았다는 사실을 정확히 표현한다.

### 8.1 배정 흐름

```
사람 → A: "작업1 등록하고 B에게 넘겨"

A:  ppwk add "작업1" --priority high
    → T001
    [오케스트레이터 메시지 도구] "T001 작업하세요" → B

B:  ppwk show T001        내용 확인
    ppwk start T001       claim 을 겸한다. 여기서 처음 소유자가 생긴다
    ... 작업 ...
    ppwk done T001
```

메시지는 **포인터**다. 내용은 ref 에 있다. 메시지에 작업 내용을 담으면 상태가 두 곳에 생긴다.

이 모델이 자연스럽게 처리하는 것들:

- B 가 응답하지 않으면 T001 은 `open` 으로 남는다. 정확한 상태다
- A 가 마음을 바꿔 C 에게 다시 보내면 C 가 `claim` 한다. 정리할 것이 없다
- B 와 C 가 동시에 받으면 CAS 가 하나만 통과시킨다 (§4.1)
- B 가 작업 중 죽으면 §4.5 가 회수한다

라벨로 힌트를 남길 수 있으나 선택이다. 상태 모델을 건드리지 않는다.

```bash
ppwk add "작업1" --label for:B
ppwk list --label for:B
```

### 8.2 스스로 가져가는 경우

오케스트레이터가 배정하지 않고 에이전트가 직접 고르게 할 수도 있다.

```bash
ppwk next --claim     # 의존성·gate·우선순위를 반영해 하나 선택
```

대화 중 한 번 호출하는 형태가 일반적이다.

```
사람: 다음 작업 진행해줘
에이전트: [next --claim] → T005 획득 → start → 작업 → done
```

**폴링 루프는 기본 사용법이 아니다.** 상시 실행되는 워커를 두는 구성에서만 쓴다.

```bash
# 상시 워커가 필요한 특수한 경우에만
while true; do
    id=$(ppwk next --claim --json | jq -r '.data.id // empty')
    [ -z "$id" ] && sleep 10 && continue
    ppwk start "$id"
    do_work "$id" && ppwk done "$id" || ppwk block "$id" --message "$(reason)"
done
```

### 8.3 세션은 자동이다

첫 상태 변경 명령이 세션을 암묵 등록한다. 초기화 명령이 없다. 신원과 세션 ID 는 환경에서 감지된다 (§0.2, §0.2.1).

**훅 없는 환경에서 자동 회수는 8시간 후다.** 배정 모델에서는 `start` 후 몇 시간 동안 CLI 호출이 없는 것이 정상이라, 짧게 잡으면 산 작업을 회수한다 (D11).

빠른 회수가 필요하면:

```bash
ppwk hook install --agent-tools     # 즉시 감지
ppwk release --mine                 # 오케스트레이터가 세션 종료 시
```

`doctor` 가 훅이 없으면 WARN 한다.

### 8.4 잠금과 배타성

| 잠금 파일 | 역할 |
|---|---|
| `agent-<name>.lock` | 에이전트 신원 및 생존 기록 |
| `worktree-<hash>.lock` | worktree 배타 |

`flock` 은 이 파일들을 갱신하는 순간에만 잡는다. 세션 수명 동안 쥐지 않는다 (§3.6).

두 번째가 중요하다. worktree 는 `HEAD` 와 index 를 하나만 가지므로, 두 에이전트가 같은 워킹 디렉터리에서 작업하면 보드는 정확한 채로 작업 결과가 손상된다.

**충돌은 첫 상태 변경 명령에서 거부된다.** 별도 초기화 명령이 없어도 검출된다.

```
$ ppwk claim T001
error: worktree /repo-a is in use by claude-code:repo-a (session 7f3a..., pid 48211)
hint:  git worktree add 로 새 worktree 를 만드세요.
       의도한 구성이라면 --allow-shared-worktree 또는
       git config ppwk.allowSharedWorktree true 를 쓰세요.
```

조회 명령(`list`, `show`, `history`, `agents`, `watch`, `export`)은 잠금을 요구하지 않는다. 몇 개든 동시에 실행할 수 있다.

### 8.5 종료 코드 처리

```
claim 이 exit 3                   → 이미 남이 가져감. 사람에게 보고
claim/start/done 이 exit 4        → 경쟁 패배. 다시 조회 후 판단
claim 이 exit 5                   → 이슈 ID 오류. 메시지 확인
next --claim 이 exit 0 + 빈 결과  → 할 일 없음
어느 명령이든 exit 6              → 스키마 불일치, 업그레이드 필요
```

### 8.6 환경변수

| 변수 | 용도 |
|---|---|
| `PPWK_AGENT` | 에이전트 ID |
| `PPWK_POLL_INTERVAL` | watch 기본 주기 |
| `PPWK_LOCK_DIR` | 잠금 디렉터리 override |
| `PPWK_ACTIVITY_TTL` | `last_activity` 임계값 (기본 8h) |
| `PPWK_SESSION` | 세션 ID 명시 지정 |
| `NO_COLOR` | 색상 비활성화 (표준 관례) |

---

## 9. 명세 밖 (v1 에서 지원하지 않음)

명시적으로 하지 않는 것들이다. 요청이 오면 명확한 오류로 거부한다.

| 기능 | 이유 |
|---|---|
| 여러 clone / 원격 동기화 | §12 v2 |
| `unarchive` | 이력 정합성 판단 필요 |
| 이슈 삭제 | archive/cancel 로 대체. 이력 보존 우선 |
| 코멘트 스레드 | body.md 로 충분. 필요해지면 tree 확장 |
| 첨부 파일 | blob 으로 가능하나 GC/크기 정책 필요 |
| 웹 UI | export 로 대체 |
| 권한 모델 | 저장소 쓰기 권한 = 보드 쓰기 권한 |
| plan 간 의존성 | gate 계산이 재귀가 됨. task 단위로 우회 |
| 이슈 템플릿 | 외부 스크립트로 충분 |
| `assign` / `inbox` / 배정 상태 | 배정은 오케스트레이터의 메시지가 담당 (§8.0) |
| 메시지 전달 | 오케스트레이터의 도구를 사용 |
| 결정 승인 워크플로우 | 만들어지면 결정. 논의는 이슈로 (§3.9) |
| 결정 수정 | 불변. `--supersedes` 로 대체 |
| 알림 라우팅 (슬랙 등) | `watch --json` 을 파이프로 |

---

## 10. 완성 판정 기준

다음이 전부 참일 때 v1 완성으로 본다.

```
[ ] 1~7 절(5.5 포함)의 모든 명령이 동작하고 --json 을 지원
[ ] 구현 문서의 단계 0~10 Exit criteria 전부 통과
[ ] 공통 회귀 스위트 R1~R10 통과
[ ] SHA-1 / SHA-256 × files / reftable 4개 조합 CI 통과
[ ] 8.1 의 배정 흐름이 3개 worktree 에서 동작
[ ] 8.2 의 상시 워커가 3개 worktree 에서 24시간 무중단 동작
[ ] 에이전트 프로세스 외에 상시 실행 프로세스가 없음
[ ] 사용자 대면 명령에 세션 관리 명령이 없음
[ ] doctor 가 정상 저장소에서 FAIL 0
[ ] fsck 가 정상 저장소에서 무결점
[ ] hook 을 삭제해도 전체 통합 테스트 통과
[ ] 모든 명령 실행 후 git status clean, 소스 브랜치 무변경
```

마지막 항목이 이 도구의 존재 이유다. 작업 관리 데이터가 소스 히스토리를 오염시키지 않는 것.
