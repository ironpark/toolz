# ppwk 설계 문서

Git ref namespace를 이용한 멀티 에이전트 작업 조율 시스템

버전 5.2 (스키마 v1) / 2026-09-02

관련 문서: 결정 기록 `ppwk-decisions.md` · 구현 계획 `ppwk-implementation.md` · 기능 명세 `ppwk-features.md` · E2E 테스트 `ppwk-e2e.md`

이 문서는 **현재 상태**만 서술한다. 왜 다른 선택지를 기각했는지는 결정 기록의 `D#` 를 참조한다.

---

## 1. 목적과 범위

### 1.1 해결하려는 문제

하나의 Git 저장소에 여러 linked worktree를 만들고, 각 worktree에서 별도 에이전트가 동시에 작업하는 환경이다. 이때 필요한 것은:

- 모든 에이전트가 동일한 작업 목록을 **즉시** 공유
- 두 에이전트가 같은 작업을 중복으로 가져가지 않음
- 에이전트가 죽었을 때 잡고 있던 작업이 회수됨
- 작업 상태 변경 이력이 남음
- 체크아웃한 브랜치와 무관하게 동작

### 1.2 설계 원칙

1. **canonical 데이터는 Git ref 하나뿐이다.** 캐시, 인덱스, 파생 파일은 전부 재생성 가능해야 한다. 런타임 생존 신호는 데이터가 아니다 (§3.6).
2. **모든 쓰기는 CAS를 통과한다.** 예외 경로를 만들지 않는다.
3. **읽기는 저장소를 변형하지 않는다.**
4. **알림은 부가 기능이다.** 알림이 전부 유실돼도 정합성이 유지되어야 한다.
5. **소스 히스토리와 섞이지 않는다.** 브랜치, 인덱스, 워킹 디렉터리를 건드리지 않는다.
6. **상시 데몬을 요구하지 않는다.** 모든 명령이 단발성으로 완결된다.

### 1.3 범위 밖 (v1)

- 여러 clone / 여러 머신 간 동기화 → 12장에서 v2 방향만 제시
- 웹 UI, 이슈 본문 리치 텍스트, 첨부 파일
- 한 worktree 에서 여러 작업 에이전트 — 잠금으로 차단한다 (§3.6)
- **작업 배정과 메시지 전달** — 오케스트레이터가 담당한다 (아래)
- plan 간 의존성 (plan A가 끝나야 plan B 시작) — task 단위 `depends_on` 으로 우회
- 권한 모델 (저장소 쓰기 권한 = 보드 쓰기 권한)

#### 오케스트레이터와의 경계

이 도구는 **스케줄러가 아니라 공유 상태 저장소**다. 누가 무엇을 언제 할지는 오케스트레이터(사람, 스크립트, 상위 에이전트)가 정하고, 에이전트 간 메시지도 그쪽 도구로 전달된다.

```
A: add "작업1" → T001
   [오케스트레이터 메시지] "T001 하세요" → B
B: show T001 → claim T001 → start → done
```

메시지는 포인터이고 내용은 ref 에 있다. 따라서 `assign` 류의 명령을 두지 않는다 (D14). 이슈를 `open` 으로 두는 것이 "아무도 받지 않았다" 를 정확히 표현한다.

오케스트레이터는 **세션 종료 시 정리**도 맡는다. B 의 대화가 끝난 것을 오케스트레이터는 알지만 ppwk 는 훅 없이는 늦게 안다 (§3.6). `release --mine` 한 줄이면 된다.

### 1.4 전제 조건

- Git 2.28 이상 (`reference-transaction` hook)
- 모든 에이전트가 **동일한 `$GIT_COMMON_DIR`** 을 공유 (같은 파일시스템)
- POSIX 셸 환경
- `flock` 을 지원하는 로컬 파일시스템 (§3.6)
- **worktree 하나당 작업 에이전트 하나** — 첫 상태 변경 명령이 이를 강제한다 (§3.6)
- `git` 실행 바이너리가 `PATH` 에 존재 (쓰기 경로에서 사용, 14장 참조)

---

## 2. 왜 이 구조인가

### 2.1 대안 비교

| 방식 | 문제 |
|---|---|
| tracked `TASKS.json` | 브랜치마다 상태가 갈림. merge conflict. 소스 히스토리 오염 |
| 전용 브랜치 + commit | 매번 checkout/commit 필요. 여전히 conflict |
| `git notes` | 이슈가 특정 commit에 종속. 단일 notes ref에 경쟁 집중 |
| SQLite 파일 | Git 외부 상태. 락 직접 구현. 이력 없음 |
| **custom ref namespace** | **채택** |

### 2.2 Git이 대신 해주는 것

```
object store      → 이슈 데이터 저장소
commit parent     → 변경 이력
custom ref        → 현재 상태 포인터
update-ref CAS    → 원자적 갱신 / 락
author/committer  → 변경 주체 기록
gc reachability   → 자동 수명 관리
```

핵심 근거는 linked worktree가 `HEAD`, index, `refs/bisect/*`, `refs/worktree/*` 만 개별로 갖고, 나머지 `refs/*`와 `objects/`는 `$GIT_COMMON_DIR`에서 **공유**한다는 점이다. 따라서 worktree A의 `update-ref`가 B와 C에 fetch 없이 즉시 반영된다.

---

## 3. 데이터 모델

### 3.1 ref 레이아웃

```
refs/ppwk/
├─ issues/<id>       commit chain — canonical, CAS 대상
├─ plans/<plan-id>   commit chain — 계획 구조. 상태를 갖지 않음
├─ decisions/<id>    commit chain — 불변 결정 기록 (3.9)
├─ archive/<id>      닫힌 이슈. issues/에서 이동
└─ meta/schema       blob 하나. 스키마 버전 문자열
```

`phases/`는 없다. phase는 독립적으로 claim되지 않고 수명이 plan에 종속되므로 별도 ref로 뺄 이유가 없다. plan 문서 안의 배열이다 (3.7).

`agents/`도 없다. 에이전트 생존은 런타임 신호이며 `$GIT_COMMON_DIR/ppwk/locks/` 의 파일로 관리한다 (3.6, D13).

`issues/`와 `archive/`를 나누는 이유: 활성 이슈 조회(`for-each-ref refs/ppwk/issues/`)가 닫힌 이슈 수에 비례해 느려지지 않게 하기 위함. 삭제가 아니라 이동이므로 이력은 보존된다.

### 3.2 이슈 ID

두 가지 전략을 지원하고 `ppwk.idStrategy` 설정으로 고른다.

**sequential (기본)** — `T001`, `T002`. 사람이 읽기 쉽다. 채번 자체를 CAS로 처리한다:

```
n = 현재 최대 번호 + 1
loop:
    update-ref refs/ppwk/issues/T<n> <commit> ""   # create-only
    성공 → n 확정
    실패 → n += 1, 재시도
```

빈 문자열 `<old>`는 "ref가 존재하지 않을 때만 생성"을 의미하므로 별도 카운터 ref가 필요 없다.

**ulid** — `01K3QZ8...`. 채번 경쟁이 없고 여러 머신으로 확장할 때 안전하다. v2를 염두에 둔다면 이쪽.

ID는 ref 경로에 들어가므로 `git check-ref-format`을 통과해야 한다. `[A-Za-z0-9_-]+` 로 제한한다.

### 3.3 이슈 commit 구조

```
tree:
  issue.json          전체 문서 (canonical)
  body.md             긴 설명 (선택)

parent:               직전 상태 commit (최초는 없음)

author:               에이전트 신원
committer:            동일

message:
  claim: SQLite storage 구현          ← subject = 이벤트 한 줄
                                      ← 빈 줄
  Status: working                     ← trailer 블록
  Owner: agent-b
  Priority: high
  Depends-On: T000
  Agent-Session: 8f3a2c1d
```

**subject를 이벤트로 쓴다.** `git log refs/ppwk/issues/T001` 이 그대로 `ppwk history T001` 이 된다. 별도 이력 자료구조가 필요 없다.

**trailer에 상태를 복제한다.** 목록 조회를 `for-each-ref` 한 번으로 끝내기 위한 비정규화다. `issue.json`이 진실이고 trailer는 인덱스다. 둘이 어긋나면 `issue.json`을 신뢰하고, `ppwk fsck`가 검출한다.

### 3.4 issue.json 스키마

```json
{
  "schema": 1,
  "id": "T001",
  "title": "SQLite storage 구현",
  "status": "working",
  "priority": "high",           // high | med | low | none
  "labels": ["storage", "backend"],
  "plan": "P01",
  "phase": "p2",
  "seq": 30,
  "owner": "agent-b",
  "session": "8f3a2c1d",
  "depends_on": ["T000"],
  "created_at": "2026-08-30T04:12:00Z",
  "updated_at": "2026-08-30T05:40:11Z",
  "updated_by": "agent-b"
}
```

생존 정보는 여기 두지 않는다. 3.6 참조.

`plan`/`phase`/`seq`는 선택 필드다. 셋 다 없으면 계획에 속하지 않는 독립 이슈이며, 기존 동작과 완전히 동일하다. 셋은 함께 존재하거나 함께 없어야 한다 — `plan`만 있고 `phase`가 없으면 fsck가 검출한다.

### 3.5 상태 전이

```
     open ──start──────────────────► working ──done──► done
        │                              ▲   │
        ├──claim──► claimed ──start────┘   ├──block──► blocked
        ▲              │                   │              │
        └──release─────┘                   └──────────────┘
        ▲                                        unblock
        └──────────── reap (소유자 사망) ─────────┘

     any ──cancel──► cancelled
```

전이 규칙은 CLI에서 강제한다. `done`과 `cancelled`는 종료 상태이며 `archive/`로 이동 대상이다.

`start` 는 `open` 에서도 허용된다 — claim 과 start 를 한 CAS 로 수행한다 (D16). 배정 모델에서 에이전트는 `show → start → done` 세 단계로 끝난다. `claim` 은 "예약만 하고 시작은 나중에" 가 필요할 때 쓴다.

### 3.6 에이전트 생존

세션은 관리 대상이 아니라 **부수적으로 발생하는 사실**이다. 사용자 대면에 세션 명령이 없다 (D12). 데몬도 없다 (D8).

```
첫 상태 변경 명령    → 세션 등록 (잠금 파일 생성)
이후 상태 변경 명령  → last_activity 갱신
프로세스 종료        → 아무것도 하지 않음
다음 확인 시점       → 생존 판정 후 필요하면 회수 (§4.5)
```

조회 명령은 `last_activity` 를 갱신하지 않는다. 읽기는 쓰지 않는다.

#### 잠금 파일

```
$GIT_COMMON_DIR/ppwk/locks/
├─ agent-<name>.lock        에이전트 신원 및 생존 기록
└─ worktree-<hash>.lock     worktree 배타
```

```json
{
  "agent": "claude-code:repo-a",
  "session": "7f3a2c1d-...",
  "worktree": "/repo-a",
  "since": "2026-09-02T04:12:00Z",
  "last_activity": "2026-09-02T05:41:02Z",
  "hook_pid": null,
  "hook_starttime": null
}
```

`$GIT_COMMON_DIR` 아래에 있으므로 모든 worktree 가 읽는다. **별도 ref 를 두지 않는다** (D13). `agents` 명령이 이 파일들을 직접 읽는다.

`hook_pid` 는 `SessionStart` 훅이 설치된 경우에만 채워진다 (§3.8). `hook_starttime` 은 pid 재사용을 걸러낸다.

`flock` 은 이 파일을 갱신하는 **순간에만** 잡는다 (D9). 파일이 잠긴 상태로 남지 않는다. 프로세스 트리를 탐색하지 않는다 (D10).

#### 쓰기 프로토콜

```
1. flock(LOCK_EX)          블로킹, 짧은 타임아웃
2. 기존 기록 읽기
3. 소유자가 살아있고 다른 세션이면 → 충돌 (아래 "worktree 배타")
4. 자기 기록으로 덮어씀
5. flock 해제
```

read-modify-write 가 원자적이므로 두 에이전트가 동시에 같은 worktree 를 잡는 경쟁이 없다. **잠금의 유일한 용도가 이것이다.**

#### 생존 판정

강한 신호부터 순서대로 적용하고, 결론이 나면 멈춘다 (D11).

| # | 조건 | 판정 | 가용 |
|---|---|---|---|
| 1 | `hook_pid` 가 같은 starttime 으로 존재 | 생존 | 훅 설치 시 |
| 2 | `hook_pid` 가 존재하지 않음 | 사망 | 훅 설치 시 |
| 3 | `last_activity` 가 임계값 이내 | 생존 | 항상 |
| 4 | `last_activity` 가 임계값 초과 | 사망 | 항상 |
| 5 | 기록 없음 / 손상 | 사망 | 항상 |

**훅이 있으면 즉시, 없으면 임계값.**

기본 임계값은 **8시간** 이며 `PPWK_ACTIVITY_TTL` 로 조정한다. 배정 모델(§1.3)에서는 `start` 후 몇 시간 동안 CLI 호출이 없는 것이 정상이므로, 짧게 잡으면 산 작업을 회수한다. "죽은 작업 방치" 보다 "산 작업 회수" 가 훨씬 나쁜 오류다.

따라서 **훅 없는 환경에서 자동 회수는 사실상 하루 단위**다. 그 사이의 정리는 오케스트레이터와 사람의 몫이다.

```bash
ppwk release --mine       # 오케스트레이터가 세션 종료 시
ppwk release T009 --force # 사람이 판단해 회수
```

`doctor` 는 훅이 없으면 WARN 한다.

```
liveness   last_activity (8h threshold)               WARN
hint       자동 회수가 느립니다. 훅을 설치하거나
           오케스트레이터가 세션 종료 시 release --mine 을 호출하세요.
```

#### worktree 배타

worktree 는 `HEAD` 와 index 를 하나만 가지므로, 두 에이전트가 같은 워킹 디렉터리에서 작업하면 보드는 정확한 채로 **작업 결과가 조용히 손상된다.** 살아있는 다른 세션이 같은 worktree 를 점유하고 있으면 상태 변경 명령을 **거부한다.**

```
$ ppwk start T001
error: worktree /repo-a is in use by claude-code:repo-a (session 7f3a..., pid 48211)
hint:  git worktree add 로 새 worktree 를 만드세요.
       의도한 구성이라면 --allow-shared-worktree 또는
       git config ppwk.allowSharedWorktree true 를 쓰세요.
```

조회 명령(`list`, `show`, `history`, `agents`, `watch`, `export`)은 잠금을 요구하지 않는다.

#### session 비교

이슈는 `owner` + `session` 을 참조한다. 같은 이름의 에이전트가 재시작하면 잠금 기록은 새 세션으로 갱신되지만, 이슈에 기록된 session 은 죽은 쪽이다. 이름만 비교하면 새 세션이 죽은 세션의 claim 을 자기 것으로 착각한다.

```
claim 이 유효한 조건:
    owner 의 잠금 기록이 생존으로 판정되고
    그 기록의 session 이 이슈의 session 과 일치
```

#### 원칙

> canonical **데이터**는 ref 하나뿐이다. 런타임 생존 신호는 데이터가 아니다.

생존 신호는 머신 로컬이고 재생성 가능하며 보존 가치가 없으므로 ref 밖에 둔다.

---

### 3.7 plan과 phase

#### 3.7.1 가장 중요한 제약: plan은 상태를 갖지 않는다

직관적으로는 plan ref 하나에 phase와 task 목록, 진행 상태를 전부 담고 싶어진다. 그러면 이렇게 된다.

```
task 상태가 바뀔 때마다 → plan ref 갱신
에이전트 N개가 동시 작업 → 전부 같은 plan ref에서 CAS 경쟁
```

이슈별로 ref를 나눠 경쟁을 분산시킨 이득이 통째로 사라진다 (D6).

**따라서 plan은 구조만 갖고 진행 상태는 절대 갖지 않는다.** 진행률, 현재 phase, 완료 개수는 전부 task를 읽어서 파생한다. 어디에도 저장하지 않는다.

결과적으로:

| 동작 | plan ref 쓰기 |
|---|---|
| task 추가 | 없음 |
| task 상태 변경 | 없음 |
| task 완료 | 없음 |
| phase 추가/재정렬 | 1회 |
| manual gate 개방 | 1회 |

plan 쓰기는 사실상 계획을 세울 때와 사람이 게이트를 열 때만 발생하므로 경쟁이 없다.

#### 3.7.2 엣지는 한 방향만

양방향 링크는 두 ref를 원자적으로 갱신해야 하고, 어긋나면 복구가 어렵다. **task가 위로 가리키는 방향 하나만** 둔다. plan에는 task 목록이 없다.

```
plan.json
{
  "schema": 1,
  "id": "P01",
  "title": "storage 레이어 재작성",
  "status": "active",
  "priority": "high",
  "phases": [
    {"id": "p1", "title": "스키마 설계",   "gate": "all_done"},
    {"id": "p2", "title": "구현",         "gate": "all_done"},
    {"id": "p3", "title": "마이그레이션",  "gate": "manual"}
  ],
  "advanced_phases": [],
  "created_at": "2026-08-30T04:00:00Z",
  "updated_at": "2026-08-30T04:00:00Z"
}
```

phase의 `gate`는 **그 phase가 열리기 위한 조건**이며 직전 phase에 적용된다. 첫 phase의 gate는 무시된다.

#### 3.7.3 순서는 seq가 갖는다

plan이 task 목록을 갖지 않으므로 순서도 task 쪽에 있다. `seq`를 10, 20, 30으로 띄워 두면 중간 삽입 시 재번호가 필요 없다. 생략하면 해당 phase의 최대값 + 10.

같은 `seq`가 중복돼도 동작에 문제는 없다 (created_at으로 tie-break). fsck가 경고만 한다.

#### 3.7.4 plan commit 구조

이슈와 동일한 규칙을 따른다. subject가 이벤트명, trailer에 인덱스용 필드를 복제한다.

```
tree:     plan.json
message:
  phase-add: 마이그레이션

  Status: active
  Phases: 3
  Agent-Session: 8f3a2c1d
```

`Agent-Session` trailer는 이슈와 같은 이유로 필수다 (4.3 OID 충돌 방지).

#### 3.7.5 phase gate

phase 경계를 task의 `depends_on`에 박아두지 않는다. 그렇게 하면 phase에 task를 추가할 때마다 이전 phase의 모든 task를 나열해야 하고, 이전 phase가 바뀌면 전부 고쳐야 한다. **gate는 조회 시점에 계산한다.**

```
phase 가 열려 있음 =
    첫 phase 이거나, 직전 phase 가 gate 조건을 만족

  all_done : 직전 phase 의 모든 task 가 done 또는 cancelled
  any_done : 직전 phase 의 task 중 하나 이상이 done
  manual   : plan.advanced_phases 에 이 phase id 가 있을 때만
```

task가 하나도 없는 phase는 `all_done`이 공허참으로 즉시 통과한다. 의도와 다를 수 있으므로 fsck가 경고한다.

`manual` gate는 `ppwk plan advance`가 `advanced_phases`에 추가하며, 이것이 plan ref를 쓰는 몇 안 되는 경우다.

#### 3.7.6 plan 상태

```
active ──close──► closed
   │
   └──cancel──► cancelled
```

plan에는 `working` 같은 중간 상태가 없다. 진행 정도는 task에서 파생되므로 저장할 것이 없다. `closed`인 plan에 속한 open task는 후보에서 제외되며 fsck가 보고한다.

---

### 3.8 코딩 에이전트 도구 통합

Claude Code, Codex 같은 도구 안에서 동작할 때, 신원과 세션 경계를 도구로부터 직접 얻을 수 있다. 이를 **세 개의 독립된 층**으로 구성한다. 위층이 없어도 아래층으로 완결된다.

```
층 3  훅            정상 종료 시 즉시 정리          선택
층 2  환경변수 감지  신원·세션 ID 자동 결정          선택, 훅 불필요
층 1  잠금          생존 판정의 진실                항상
```

#### 층 1 — 암묵 세션 (항상)

§3.6 그대로다. 첫 상태 변경 명령이 세션을 등록하므로 **에이전트가 아무것도 호출하지 않아도 동작한다.** 도구도 훅도 환경변수도 없는 환경에서 이것만으로 정합성이 유지된다. 다만 이 층만으로는 자동 회수가 느리다 (8시간). **아래 두 층은 정확도와 속도를 높이는 최적화다.**

#### 층 2 — 환경변수 감지 (훅 불필요)

`ppwk` 실행 시점에 환경을 읽는다. 설정 없이 얻어진다.

| 신호 | 얻는 것 |
|---|---|
| `CLAUDE_CODE_SESSION_ID` | 대화 세션 UUID (Bash 도구 하위 프로세스에 설정됨) |
| `CLAUDECODE` | 도구 = claude-code |
| `CODEX_*` | 도구 = codex |
| `CI` | 자동화 환경 |

이것만으로 신원 결정이 자동화된다.

```
agent-id = claude-code:repo-a          도구 + worktree
session  = 7f3a...                     실제 대화 세션 UUID
```

사람이 `PPWK_AGENT` 를 정할 필요가 없어지고, **같은 대화에서 실행된 모든 명령이 같은 세션으로 묶인다.** §3.6 의 session 비교가 이제 진짜 대화 경계를 반영한다.

감지에 실패하면 조용히 넘어가지 않고 폴백을 쓴다: `<hostname>:<worktree basename>` + 현재 프로세스. `doctor` 가 감지 결과를 표시해 사람이 확인할 수 있게 한다.

#### 층 3 — 훅 (선택)

두 도구의 훅 표면이 대칭이므로 **스크립트 하나를 양쪽에 등록**한다.

| 이벤트 | 두 도구 | 사용 |
|---|---|---|
| `SessionStart` | 있음 | 세션 등록 |
| `SessionEnd` | 있음 (서브에이전트 제외) | `claimed` 반납 |
| `SubagentStart` / `Stop` | 있음 | **사용하지 않음** |

stdin 으로 `session_id`, `cwd`, `hook_event_name` 등이 JSON 으로 전달된다.

```
SessionStart → ppwk internal session-event   세션 등록, hook_pid 기록
SessionEnd   → ppwk internal session-event   claimed 만 open 으로
```

`SessionEnd` 가 `working` 을 건드리지 않는 이유 (D15): 사용자가 도구를 닫았다 다시 열어 같은 작업을 잇는 것은 흔하고, `working` 에는 worktree 의 미커밋 변경이 있다. 반납하면 다른 에이전트가 다른 worktree 에서 처음부터 한다. `working` 은 §3.6 의 생존 판정에 맡긴다.

서브에이전트 이벤트를 쓰지 않는 이유: 서브에이전트는 별도 세션이 아니라 부모 세션의 일부다. `SessionEnd` 가 서브에이전트에 실행되지 않는 설계라, 훅을 걸지 않으면 자연히 부모 세션으로 묶인다. 걸면 서브에이전트마다 유령 에이전트가 생긴다.

#### SessionEnd 는 최적화다

크래시·SIGKILL 에는 실행되지 않고, 종료를 막을 수 없으며, Codex 에서는 실험적이고 신뢰 검토를 거친다. 따라서 생존 판정의 근거가 아니다.

```
정상 종료 (다수)    → SessionEnd 훅 → claimed 즉시 반납
비정상 종료 (소수)  → hook_pid 확인 → 다음 next 호출 시 회수
```

**정합성은 층 1이 단독으로 보장한다.**

#### 훅이 생존 판정을 정확하게 만든다

훅이 없으면 §3.6 의 판정이 `last_activity` 임계값에 의존해 최대 8시간 지연된다. `SessionStart` 훅은 **도구가 직접 spawn** 하므로 그 시점의 부모가 도구 프로세스다. 프로세스 트리를 뒤질 필요 없이 `$PPID` 하나면 된다.

```
SessionStart 훅이 잠금 파일에 기록:
    hook_pid       = 부모 pid
    hook_starttime = 그 프로세스의 시작 시각 (pid 재사용 방지)
```

이 값이 있으면 판정이 우선순위 2·3번에서 끝나고 **감지가 즉시** 이루어진다. 훅은 더 정확한 입력을 제공할 뿐, 판정 절차 자체는 §3.6 과 동일하다.

`hook_pid` 가 신뢰할 만한 이유는 **훅의 부모가 도구라는 것이 구조적으로 보장**되기 때문이다. 임의 위치에서 트리를 거슬러 올라가는 것과는 다르다.

#### 도구 훅 vs reference-transaction 훅

이름이 겹치므로 구분한다.

| | `reference-transaction` (§6.3) | 도구 훅 (여기) |
|---|---|---|
| 설치 위치 | `$GIT_COMMON_DIR/hooks/` | `.claude/settings.json`, `.codex/hooks.json` |
| 트리거 | git ref 변경 | 대화 세션 시작·종료 |
| 목적 | 변경 알림 | 세션 신원·정리 |

둘은 독립적이며 각각 선택 사항이다.

#### 스펙 안정성

훅 이벤트 이름과 환경변수는 **문서화된 안정 API 가 아니다.** 도구 버전에 따라 바뀔 수 있으므로:

- 감지 실패는 폴백으로 흡수하고 오류로 만들지 않는다
- `doctor` 가 감지 결과를 명시적으로 표시한다
- 훅 스크립트는 알 수 없는 입력을 만나면 조용히 exit 0 한다 (세션을 막지 않는다)
- `SessionStart` 훅은 빠르게 끝낸다. ref 쓰기 한 번이 상한이다

---

### 3.9 결정 기록

에이전트 A 가 `feature/a` 에서 "저장소는 SQLite" 로 결정하면, `feature/b` 의 B 는 merge 전까지 그것을 모른다. tracked 파일로 기록하면 브랜치마다 갈린다 — `TASKS.json` 을 기각한 이유(D1)와 같은 문제이고, ref namespace 가 같은 해법이다.

결정은 이슈보다 ref 모델에 더 잘 맞는다. 이슈는 상태가 전이하지만 **결정은 불변**이다.

#### 두 단계 수명

결정은 영속적 지식이라 사람이 코드 리뷰와 PR 에서 읽고 싶어 한다. ref 에만 있으면 일반 git UX 에서 보이지 않는다.

```
만드는 동안   → ref        모든 worktree 즉시 공유, merge 불필요
정착시킬 때   → export     docs/decisions/D007.md 로 tracked 파일 생성
```

ref 가 진실이고 파일은 파생물이다 (§5.4 와 같은 규칙).

#### 문서 형식

```
refs/ppwk/decisions/<id> → commit (사실상 1개)
tree: decision.json
```

```json
{
  "schema": 1,
  "id": "D007",
  "title": "저장소는 SQLite",
  "context": "단일 머신, 동시 쓰기 적음, 배포 단순화 필요",
  "options": ["SQLite", "PostgreSQL", "파일 기반 JSON"],
  "decision": "SQLite",
  "consequences": "동시 쓰기 확장 시 재검토",
  "issues": ["T001", "T004"],
  "plan": "P01",
  "supersedes": "D003",
  "decided_by": "claude-code:repo-a",
  "decided_at": "2026-09-02T06:10:00Z"
}
```

trailer 에 `Title`, `Supersedes`, `Issues` 를 복제해 목록을 `for-each-ref` 한 번으로 만든다 (D5).

#### 규칙

**불변.** `edit` 이 없다. 바꾸려면 새 결정을 `--supersedes` 로 만든다. 이력이 곧 논거의 변천이다.

**엣지는 한 방향.** 결정 → 이슈 / plan / 이전 결정. 역방향(`superseded_by`, 이슈에서 결정 목록)은 조회 시 계산한다 (D6 과 같은 이유).

**상태 없음.** `proposed → accepted` 전이를 두지 않는다. 만들어지면 결정이다. 논의 단계가 필요하면 이슈로 하고, 결론이 나면 결정을 만든다. 승인 워크플로우는 오케스트레이터와 사람의 영역이다 (D14 와 같은 경계).

#### 이슈와의 연결

`show T001` 이 연결된 결정을 함께 표시한다. 에이전트가 작업 시작 전에 관련 결정을 자연스럽게 보게 되어, 세션을 넘기며 같은 논의를 반복하지 않는다. 이것이 이 기능의 실질적 효과다.

#### ID

`D` 접두 + 순번. 이슈와 같은 create-only CAS 채번(§3.2).

---

## 4. 동시성


### 4.1 CAS 프로토콜

모든 상태 변경은 예외 없이 이 경로를 탄다.

```
1. old = rev-parse <ref>              (없으면 "")
2. 현재 issue.json 파싱
3. 전이 규칙 검사 → 위반이면 사용자 오류로 종료
4. 새 issue.json 생성
   blob = hash-object -w
   tree = mktree
   new  = commit-tree <tree> -p <old> -m "<subject>\n\n<trailers>"
5. update-ref <ref> <new> <old>
6. 실패 분기:
     lock 실패  → backoff 후 5 재시도 (최대 N회)
     CAS 실패   → 1로 복귀 (claim이면 다른 이슈 시도)
```

### 4.2 두 종류의 실패를 반드시 구분한다

files backend에서 `update-ref`는 `<ref>.lock` 파일을 잡는다. 동시 접근 시 나오는 실패는 두 가지이고 대응이 다르다.

| 실패 | stderr 패턴 | 의미 | 대응 |
|---|---|---|---|
| lock 실패 | `cannot lock ref`, `unable to create ... .lock` | 다른 프로세스가 같은 ref를 쓰는 중 | 그대로 재시도. 상태는 안 변했음 |
| CAS 실패 | `is at <x> but expected <y>` | 다른 에이전트가 이미 상태를 바꿈 | 다시 읽고 재판단 |

이걸 뭉뚱그리면 lock 실패에도 불필요하게 전체 재계산을 하거나, 반대로 CAS 실패를 재시도로 오인해 무한 루프에 빠진다.

`core.filesRefLockTimeout`(기본 100ms)을 늘리면 lock 실패 자체가 줄어든다. `init`에서 1000ms 정도로 설정한다.

### 4.3 OID 충돌 방지

Git object는 content-addressed다. 두 에이전트가 같은 초에, 같은 parent, 같은 tree, 같은 author 정보로 commit을 만들면 **OID가 동일해져서 양쪽 CAS가 모두 "성공"** 한다. 그러면 둘 다 자기가 claim했다고 믿는다.

따라서 commit content에 반드시 고유값을 넣는다:

- `Agent-Session:` trailer (세션마다 랜덤)
- `issue.json`의 `session` 필드
- committer email에 세션 포함 (`agent-b+8f3a2c1d@local`)

이 중 하나만 있어도 충분하지만, trailer는 검증에도 쓰이므로 기본으로 넣는다.

### 4.4 다중 ref 트랜잭션

한 번에 여러 ref를 원자적으로 바꿔야 하는 경우가 있다:

- 종료된 이슈를 `issues/`에서 `archive/`로 이동 (삭제 + 생성)
- 이전 owner 해제와 새 claim을 동시에

`git update-ref --stdin` 을 쓴다.

```
start
update refs/ppwk/archive/T001 <oid> <zero>
delete refs/ppwk/issues/T001 <oid>
prepare
commit
```

전부 성공하거나 전부 실패한다. 개별 `update-ref`를 순차 호출하면 중간에 죽었을 때 이슈가 사라지거나 중복된다.

### 4.5 회수 (reap) — 게으른 확인

#### 유휴 사각지대가 존재하지 않는 이유

TTL 방식에서는 사망 판정에 시간이 필요했고, 그 시간을 흘려보낼 주체가 필요했다. 그래서 주기적 reap 이 필요했고, 아무도 `next` 를 부르지 않는 구간이 사각지대가 되었다.

잠금 기반에서는 **판정이 즉각적**이다. 그러면 아무도 그 이슈를 원하지 않는 동안 방치되어도 손해가 없다. 누군가 `next` 를 부르는 순간이 곧 그 이슈가 필요해진 순간이고, 바로 그때 확인하면 된다.

**게으른 확인으로 충분하다.** 이것이 데몬을 제거할 수 있는 근거다.

#### 알고리즘

```
next 호출 시 (또는 reap 명령):
    non-open 이슈들을 owner 기준으로 묶는다
    for each owner:
        §3.6 의 생존 판정 절차를 한 번 수행
        생존  → 해당 owner 의 이슈 전부 그대로 둔다
        사망  → 해당 owner 의 이슈를 순회하며 CAS 회수
```

**owner 기준으로 묶는 것이 중요하다.** 이슈마다 잠금을 확인하면 활성 에이전트 수가 아니라 이슈 수에 비례한다. 묶으면 보통 한 자릿수 번의 확인으로 끝난다.

회수도 CAS 를 거치므로 여러 에이전트가 동시에 같은 이슈를 회수하려 하면 한쪽만 성공한다.

#### 실행 시점

```
next 호출 시     — 배정 직전. 이것이 유일한 자동 실행 지점
SessionEnd 훅    — claimed 만 반납 (§3.8, D15, 선택)
release --mine   — 오케스트레이터가 세션 종료 시 (§1.3)
reap 명령        — 수동 진단·복구용
```

**주기적 실행 지점이 없다.** 데몬도 없다.

#### 상태별 처리

| 상태 | 소유자 사망 시 |
|---|---|
| `claimed` | `open` 으로 복귀 |
| `working` | `open` 으로 복귀 |
| `blocked` | `blocked` 유지, owner 만 해제 |

`blocked` 는 사람의 판단이 필요한 상태이므로 소유자가 죽었다고 자동으로 후보에 넣지 않는다.

#### 잃는 것: 멈춘 프로세스

살아있지만 진전이 없는 에이전트는 생존으로 판정되므로 회수되지 않는다. TTL 이라면 회수됐을 상황이다.

이것을 **더 나은 동작**으로 본다. TTL 은 정상적으로 오래 걸리는 작업도 오탐으로 뺏었다. 진짜로 멈춘 프로세스는 사람이 판단할 문제이며, `agents` 명령이 보유 시간을 보여주면 개입할 수 있다.

```bash
ppwk agents
# claude-code:repo-c  alive  T009  holding 3h12m     ← 사람이 판단
ppwk release T009 --force
```

#### 환경 제약

`flock` 은 로컬 파일시스템을 전제한다. NFS 등에서는 신뢰할 수 없으므로 `doctor` 가 파일시스템 종류를 확인해 경고한다. 여러 worktree 를 네트워크 파일시스템에 두는 구성은 이 설계의 전제(§1.4)와도 맞지 않는다.

여러 머신으로 확장하는 v2(§12)에서는 잠금 파일이 머신 경계를 넘지 못하므로, 그때 비로소 `agents/` ref 를 추가해 머신 로컬 잠금과 조합한다.

---

## 5. 읽기 경로

### 5.1 목록 — 명령 한 번

```bash
git for-each-ref \
  --format='%(refname:lstrip=3)%09%(trailers:key=Status,valueonly,unfold)%09%(trailers:key=Owner,valueonly,unfold)%09%(trailers:key=Plan,valueonly,unfold)%09%(trailers:key=Phase,valueonly,unfold)%09%(trailers:key=Seq,valueonly,unfold)%09%(subject)' \
  refs/ppwk/issues/
```

trailer 비정규화 덕분에 이슈 수와 무관하게 프로세스 하나로 끝난다. `issue.json`을 열지 않는다.

`Plan`/`Phase`/`Seq`까지 trailer에 있으므로 **계획 뷰도 같은 호출 하나로 만들어진다.** CLI가 결과를 메모리에서 plan/phase로 그룹핑하고 진행률을 계산한다. plan ref는 phase 정의를 얻기 위해 plan 개수만큼만 읽는다 (보통 한 자릿수).

### 5.2 상세

```bash
git show refs/ppwk/issues/T001:issue.json
```

여러 개를 한 번에 볼 때는 `cat-file --batch`에 `<ref>:issue.json` 을 줄 단위로 밀어넣는다.

### 5.3 이력

```bash
git log --format='%h %ad %an %s' --date=iso refs/ppwk/issues/T001
```

subject가 이벤트명이므로 추가 가공이 거의 필요 없다.

### 5.4 export

파생물이며 단방향이다. 편집해도 반영되지 않는다.

```bash
ppwk export --format=json > board.json
ppwk export --format=md   > BOARD.md
```

생성 파일은 `.gitignore`에 넣는다. 사람이 직접 고치지 못하게 헤더에 생성 시각과 경고를 넣는다.

---

## 6. 알림

### 6.1 두 경로

| | 기본: polling | 선택: hook |
|---|---|---|
| 지연 | 1~2초 | 즉시 |
| 의존성 | 없음 | socat, hook 설치 권한 |
| 실패 모드 | 없음 | 조용히 유실 |

**polling이 기본이고 hook은 최적화다.** hook이 없거나 죽어도 시스템은 정상 동작한다.

### 6.2 polling

```bash
git for-each-ref --format='%(refname) %(objectname)' refs/ppwk/
```

결과를 이전 스냅샷과 ref 단위로 비교한다. 파일 mtime이나 inotify를 쓰면 안 된다 — `pack-refs`가 loose 파일을 없애고, reftable backend에는 애초에 ref별 파일이 없다.

### 6.3 reference-transaction hook

`$GIT_COMMON_DIR/hooks/reference-transaction` 에 설치한다. hook은 공용이므로 한 번 설치하면 모든 worktree에 적용된다.

제약:

- **`committed` 단계만 처리한다.** `preparing`/`prepared`는 아직 확정 전이고, `prepared`에서 non-zero exit은 트랜잭션을 abort시킨다.
- **prefix 필터를 가장 먼저 한다.** 이 hook은 일반 commit, fetch, rebase, checkout 등 모든 ref 갱신마다 호출된다. 우리 namespace가 아니면 즉시 종료한다.
- **절대 ref를 쓰지 않는다.** 재귀가 된다.
- **절대 블로킹하지 않는다.** git 프로세스 안에서 동기 실행되므로 listener가 죽으면 `ppwk done`이 영영 안 끝난다. `timeout`으로 감싸고 실패는 무시한다.
- **SHA-1과 SHA-256 zero OID를 모두 비교한다.** 40자리만 보면 SHA-256 저장소에서 created/deleted 판정이 전부 틀린다.

출력은 줄당 JSON 하나:

```json
{"ref":"refs/ppwk/issues/T001","old":"abc...","new":"def...","kind":"updated"}
```

`core.hooksPath`가 전역으로 설정되어 있으면 `$GIT_COMMON_DIR/hooks`의 hook이 무시된다. `init`이 이를 감지해 경고한다.

---

## 7. CLI

### 7.1 명령

```
ppwk init [--hooks]
ppwk add <title> [--priority P] [--label L] [--depends-on ID]
                        [--plan P01 --phase p2 [--seq N]]
ppwk list [--status S] [--owner A] [--plan P] [--json]
ppwk show <id> [--json]
ppwk history <id>

ppwk claim <id>          open      → claimed
ppwk start <id>          claimed   → working
ppwk done <id>           working   → done      (+ archive)
ppwk block <id> <on>     working   → blocked
ppwk unblock <id>        blocked   → working
ppwk release <id>        claimed   → open
ppwk cancel <id>         any       → cancelled (+ archive)

ppwk next [--claim]      다음 작업 선택
ppwk reap                  죽은 소유자의 이슈 회수 (수동·진단용)
ppwk agents                에이전트 생존 및 보유 현황 (잠금 파일 기반)
ppwk watch               변경 스트림
ppwk export [--format F]
ppwk fsck [--fix]

ppwk plan new <title> [--priority P]
ppwk plan phase add <plan> <title> [--gate all_done|any_done|manual]
ppwk plan show <plan>
ppwk plan list
ppwk plan advance <plan> <phase>     manual gate 개방
ppwk plan close <plan>
ppwk plan cancel <plan>
```

#### plan show 출력

```
P01  storage 레이어 재작성          [active]

  p1  스키마 설계                    3/3  done
      T001  done     agent-a   테이블 정의
      T002  done     agent-a   인덱스 설계
      T003  done     agent-b   리뷰 반영

  p2  구현                          1/4  working      ← 현재 phase
      T004  done     agent-a   SQLite storage 구현
      T005  working  agent-b   parser cleanup
      T006  open     -         에러 처리
      T007  open     -         테스트

  p3  마이그레이션                   0/2  blocked (gate: manual)
      T008  open     -         migration script
      T009  open     -         롤백 절차
```

여기서 p3의 task는 `status`가 `open`이다. `blocked`가 아니다. **gate로 막힌 것은 이슈 상태가 아니라 후보 선정 시점의 판단**이므로 저장된 상태를 바꾸지 않는다. 표시상의 `blocked (gate)`는 파생값이다.

### 7.2 `next` 알고리즘

에이전트가 실제로 호출하는 유일한 스케줄링 명령이다.

```
1. reap 한 번 실행 (만료 lease 회수)
2. status == open 이고 priority != none 인 이슈 수집
       priority none 은 백로그다. 상태가 아니라 속성이므로
       전이·gate·회수 규칙에 예외가 생기지 않는다.
3. depends_on 이 전부 done 인 것만 남김
       의존 대상을 issues/ 에서 못 찾으면 archive/ 도 조회한다.
       완료된 이슈는 archive/ 로 옮겨지므로, issues/ 만 보면
       "의존 대상이 사라짐"으로 오판해 영원히 후보에서 빠진다.
4. phase gate 검사 (3.7.5)
       plan 에 속한 task 는 소속 phase 가 열려 있어야 후보가 된다.
       closed / cancelled plan 에 속한 task 는 제외한다.
5. 정렬: plan priority DESC, seq ASC, priority DESC, created_at ASC
6. --claim 이면:
       상위부터 순회하며 CAS claim 시도
       CAS 실패 → 다음 후보로 (다른 에이전트가 가져감)
       lock 실패 → 같은 후보 재시도
       전부 실패 → 빈 결과
7. 결과 출력
```

**정렬에서 `seq`가 `priority`보다 앞선다.** plan 안에서는 저자가 의도한 순서를 따르고, `priority`는 같은 seq 구간이나 plan에 속하지 않은 이슈를 정렬할 때만 쓴다. 반대로 두면 high priority task가 계획 순서를 뛰어넘어 먼저 실행된다.

plan에 속하지 않은 이슈는 `plan priority`를 `med`로 간주해 계획된 작업들 사이에 자연스럽게 섞이게 한다.

CAS 실패 시 재시도가 아니라 **다음 후보로 넘어가는 것**이 핵심이다. 같은 이슈를 두고 경쟁하지 않고 자연스럽게 분산된다.

### 7.3 종료 코드

| 코드 | 의미 |
|---|---|
| 0 | 성공 |
| 1 | 일반 오류 |
| 2 | 사용법 오류 |
| 3 | 전이 규칙 위반 (예: open이 아닌 이슈에 claim) |
| 4 | CAS 경쟁 실패 (재시도 소진) |
| 5 | 해당 이슈 없음 |

에이전트가 3과 4를 구분해야 한다. 3은 로직 오류이고 4는 재시도 대상이다.

### 7.4 에이전트 신원

`user.name`을 바꾸면 소스 commit까지 오염된다. 이슈 commit을 만들 때만 환경변수로 주입한다:

```
GIT_AUTHOR_NAME=agent-b
GIT_AUTHOR_EMAIL=agent-b+8f3a2c1d@ppwk.local
GIT_COMMITTER_NAME / GIT_COMMITTER_EMAIL 동일
```

`commit.gpgsign`이 켜져 있으면 `commit-tree`가 서명을 시도해 느려지거나 실패한다. 호출 시 `git -c commit.gpgsign=false` 로 끈다.

에이전트 ID는 `PPWK_AGENT` 환경변수 → `ppwk.agent` 설정 → `hostname:worktree basename` 순으로 결정한다.

---

## 8. 초기화 (`init`)

```
1. meta/schema ref 생성 (없으면)
2. git config --add log.excludeDecoration refs/ppwk/
3. git config core.filesRefLockTimeout 1000
4. --hooks 이면:
     core.hooksPath 확인 → 설정돼 있으면 경고하고 그쪽에 설치
     기존 reference-transaction 있으면 중단 (덮어쓰지 않음)
     hook 복사 + chmod +x
5. 안내 출력:
     git log --all 에 이슈 커밋이 섞임 → 별칭 제안
     git push --mirror 하면 원격에 노출됨
```

`log.excludeDecoration`은 **데코레이션 표시만** 지운다. 커밋 자체를 빼려면 사용자가 `--exclude`를 써야 하므로 별칭을 제안한다:

```bash
git config alias.la "log --exclude=refs/ppwk/* --all"
```

---

## 9. 운영

### 9.1 가시성

| 범위 | 노출 |
|---|---|
| 같은 저장소의 모든 worktree | 즉시 — 의도된 동작 |
| `git log`, `git status`, checkout | 안 보임 — 브랜치와 직교 |
| `git log --all`, `gitk --all` | 보임 — `--exclude`로 완화 |
| 다른 clone / 원격 | 안 보임 — `--mirror`나 명시 push 시에만 |

`git push --mirror`는 이슈 제목, 본문, 에이전트 신원을 전부 원격에 올린다. 공개 저장소라면 작업 계획이 노출된다. `init` 안내에 반드시 포함한다.

### 9.2 GC와 packing

- 활성 ref가 가리키는 객체는 reachable하므로 안전하다
- CAS 실패로 버려진 commit은 dangling이 되어 gc가 정리한다 — 무시해도 된다
- 이슈가 수천 개면 loose ref 파일이 쌓인다. 주기적으로 `git pack-refs --all` 권장
- `git gc --prune=now`를 CAS 진행 중에 돌리면 이론적으로 경쟁 가능하나, `gc.pruneExpire` 기본값(2주)이면 문제없다

### 9.3 fsck

`ppwk fsck`가 검사할 것:

- trailer의 `Status`와 `issue.json`의 `status` 불일치
- `depends_on`이 존재하지 않는 ID를 가리킴
- 의존성 순환
- `owner`가 있는데 대응하는 잠금 파일이 없음
- 종료 상태인데 `issues/`에 남아있음
- `schema` 버전 불일치
- task의 `plan` 또는 `phase`가 존재하지 않는 것을 가리킴
- `plan`/`phase`/`seq` 중 일부만 존재 (셋은 함께 있거나 함께 없어야 함)
- plan이 closed인데 미완 task가 남음
- task가 하나도 없는 phase (gate가 공허참으로 통과됨)
- 같은 phase 안에 `seq` 중복 (경고)
- `advanced_phases`가 존재하지 않는 phase id를 가리킴

`--fix`는 trailer 재생성과 archive 이동만 자동 처리한다. 나머지는 보고만 한다.

### 9.4 스키마 마이그레이션

`meta/schema`의 값이 CLI가 아는 버전보다 높으면 **읽기만 허용하고 쓰기를 거부**한다. 낮으면 마이그레이션을 안내한다. 여러 에이전트가 섞인 버전으로 도는 상황에서 데이터를 깨뜨리지 않기 위함이다.

---

## 10. 성능

명령당 git 프로세스 호출 횟수:

| 명령 | 호출 수 |
|---|---|
| `list` | 1 |
| `show` | 1 |
| `claim` | 5 (rev-parse, cat-file, hash-object, commit-tree, update-ref) |
| `next --claim` | 2 + 5×후보수 |
| `plan show` | 2 (issues 목록 + plan 1개) |

위 표는 셸 구현 기준이며, Go 하이브리드(14장)에서는 읽기가 fork 0회, 쓰기가 fork 1회다. `list`가 1회, `plan show`가 2회인 것이 trailer 비정규화의 이득이다. plan이 task 목록을 갖지 않으므로 계획 뷰도 이슈 목록 한 번에서 파생된다. 쓰기 5회는 셸 구현에서 수십 ms 수준이며, 에이전트 작업 주기(수 초~수 분)에 비하면 무시할 만하다.

성능이 문제가 되면 `git cat-file --batch`를 장기 실행 프로세스로 띄워 재사용하거나, gitoxide/libgit2로 내려간다. **v1은 `git` CLI를 감싸는 얇은 wrapper로 시작한다.**

---

## 11. 테스트 계획

### 11.1 동시성 (필수)

```
- N개 프로세스가 동시에 같은 이슈 claim → 정확히 1개만 성공
- N개 프로세스가 동시에 next --claim → 중복 배정 0건
- 동일 tree/parent/시각으로 두 commit 생성 → OID가 달라야 함 (4.3 회귀 테스트)
- lock 실패 주입 → 재시도로 최종 성공
- archive 이동 중 SIGKILL → 이슈가 사라지거나 중복되지 않음
```

### 11.2 수명주기 (필수)

```
- 에이전트 SIGKILL → 다음 next 호출 시 즉시 이슈가 open 으로 복귀
- 죽은 세션과 같은 이름으로 재시작 → 이전 세션의 claim이 정상 회수됨
  (owner 이름만 비교하면 실패하는 회귀 테스트)
- 모든 에이전트가 작업 중이고 아무도 next 를 호출하지 않는 상태에서
  하나가 죽음 → 이후 next 호출 시 정확히 회수 (게으른 확인 검증)
- 같은 worktree 에서 두 번째 에이전트의 상태 변경 → 거부
- 의존 대상이 done 되어 archive 로 이동 → 의존 이슈가 후보에 등장
- 여러 에이전트가 동시에 같은 만료 lease 를 reap → 정확히 1회만 회수
```

### 11.3 plan / phase

```
- phase 의 마지막 task 가 done → 다음 phase 의 task 가 후보에 등장
- all_done gate: 직전 phase 에 open task 가 하나라도 있으면 막힘
- any_done gate: 하나만 done 되어도 열림
- manual gate: plan advance 전까지 막히고, 이후 열림
- task 가 없는 phase → gate 통과 (공허참) + fsck 경고
- N개 에이전트가 같은 plan 의 task 들을 동시에 claim
      → 중복 배정 0건, plan ref 쓰기 0회 (경쟁 분산 회귀 테스트)
- plan close 후 소속 open task 가 후보에서 제외됨
- seq 가 priority 를 앞서는지: high priority 인 seq 30 이
      med priority 인 seq 10 보다 나중에 배정됨
```

### 11.4 환경

```
- SHA-1 / SHA-256 저장소 양쪽
- files backend / reftable backend 양쪽
- pack-refs 실행 후에도 정상 동작
- worktree 3개에서 교차 가시성
- hook 미설치 상태에서 polling만으로 동작
- core.hooksPath 설정된 상태에서 init 경고
```

### 11.5 오염 검사

```
- claim 후 git status 가 clean
- claim 후 git log <branch> 에 변화 없음
- 소스 commit의 author가 에이전트 ID로 오염되지 않음
```

---

## 12. v2: 여러 머신으로 확장

같은 데이터 모델이 그대로 올라간다. 이슈 chain은 항상 fast-forward이므로 **원격의 non-fast-forward reject = 원격 CAS 실패** 로 해석하면 된다.

```bash
git config --add remote.origin.push  'refs/ppwk/issues/*:refs/ppwk/issues/*'
git config --add remote.origin.fetch '+refs/ppwk/issues/*:refs/ppwk/issues/*'
```

추가로 필요한 것:

- ID를 ULID로 (채번 경쟁 회피)
- 오프라인에서 chain이 갈라진 경우의 병합 규칙
  - claim: 서버에 먼저 도달한 쪽 승
  - 그 외 필드: `updated_at` 최신 승
  - 병합 결과는 두 부모를 갖는 merge commit
- 잠금 파일은 머신 경계를 넘지 못하므로 이 단계에서 비로소 `refs/ppwk/agents/` 를 추가한다 (D13). TTL 이 다시 필요해진다

**v1에서는 이 전부를 구현하지 않는다.** 단일 `$GIT_COMMON_DIR` 전제를 명시하고, 다른 clone에서 실행하면 경고한다.

---

## 13. 구현 순서

단계별 상세 계획, 각 단계의 통과 조건과 엣지 케이스는 별도 문서 `ppwk-implementation.md` 를 따른다. 아래는 순서 요약이다.

0. `RefStore` 인터페이스 + `ExecRefStore` + `classifyRefError` (14.5, 14.6)
1. `init`, `add`, `list`, `show` — 읽기/쓰기 기본 (CAS 없이 단일 에이전트)
2. CAS 프로토콜 + lock/CAS 실패 구분 — 여기서 11.1 테스트를 붙인다
3. 상태 전이 명령 (`claim`/`start`/`done`/...)
4. 잠금(`session`) + 게으른 `reap`
5. `next --claim`
6. `archive` 이동 (다중 ref 트랜잭션)
7. polling `watch`
8. `export`, `fsck`, `history`
9. plan / phase (`plan` 서브커맨드, gate 검사, 정렬 변경)
10. hook (선택 기능)

0번과 2번을 끝내기 전에 3번 이후로 넘어가지 않는다. 동시성이 이 시스템의 전부이고, 나머지는 그 위의 편의 기능이다.

---

## 14. 구현 스택

### 14.1 결정: go-git 하이브리드

Go로 구현하며 내부적으로 [go-git](https://github.com/go-git/go-git)을 쓴다. 단, **읽기와 쓰기의 백엔드를 분리한다.**

| 경로 | 구현 | 이유 |
|---|---|---|
| 읽기 (ref 조회, 객체 읽기) | go-git | in-process, fork 없음, 가장 잦은 호출 |
| 객체 생성 (blob/tree/commit) | go-git | content-addressed 라 경쟁 자체가 없음 |
| **ref 갱신 (CAS, 트랜잭션)** | **`git` CLI exec** | **원자성, 훅, 다중 ref 트랜잭션** |

### 14.2 왜 ref 갱신만 CLI 인가

go-git의 `CheckAndSetReference(new, old)` 는 CAS 처럼 보이지만 실제 구현은 read-then-write 다. 검사와 쓰기 사이에 다른 프로세스가 끼어들 수 있다. go-git 자체가 thread-safe 하지 않다고 명시하고 있으며, 과거 코드 주석에도 "한 번에 하나의 프로세스만 접근한다고 가정" 이라는 전제가 적혀 있다.

이 설계는 정확히 그 전제가 깨지는 환경이다. 에이전트 N개가 **별도 프로세스**로 같은 ref를 노린다. 4장 전체가 이 CAS의 원자성 위에 서 있으므로, 여기서 타협하면 설계의 근간이 무너진다.

추가로 go-git에 없는 것:

| 필요한 것 | 설계 위치 | go-git |
|---|---|---|
| 프로세스 간 원자적 CAS | 4.1 | 없음 |
| lock 실패 / CAS 실패 구분 | 4.2 | 구분 불가 |
| 다중 ref 원자적 트랜잭션 | 4.4 | 없음 |
| `reference-transaction` hook 실행 | 6.3 | 훅을 실행하지 않음 |
| git 호환 `.lock` 프로토콜 | — | 미보장 |

훅 미실행이 특히 치명적이다. go-git으로 ref를 쓰면 훅이 돌지 않아 알림이 조용히 사라진다. 그리고 사람이 같은 저장소를 진짜 `git`으로 만지는 순간, 서로를 모르는 두 락 프로토콜이 공존하게 된다.

### 14.3 얻는 것

- 4.1~4.5(CAS, 실패 구분, OID 충돌 방지, 다중 ref 트랜잭션, reap)가 **수정 없이 그대로 유효**
- 6.3 훅 알림 경로가 정상 동작
- 사람이 `git` CLI로 직접 조작해도 안전
- `list` 는 fork 0회 — 가장 잦은 호출이 가장 빠르다
- 쓰기 명령당 fork 1회. 셸 구현의 5회보다 오히려 적다

### 14.4 commondir

go-git은 `PlainOpenOptions.EnableDotGitCommonDir` 로 linked worktree의 공유 ref/객체에 접근한다. **파일시스템 스토리지에서만 동작하므로** 이 옵션을 끄거나 다른 스토리지를 쓰면 3.1의 전제가 깨진다.

```go
repo, err := git.PlainOpenWithOptions(cwd, &git.PlainOpenOptions{
    DetectDotGit:          true,
    EnableDotGitCommonDir: true,
})
```

`git` CLI 호출 시 `cmd.Dir` 은 common dir 로 고정한다. worktree 마다 `GIT_DIR` 이 다르지만 우리가 다루는 ref는 전부 공유 영역이다.

### 14.5 RefStore 인터페이스

CLI 의존을 한 경계 뒤에 가둔다. v2에서 순수 Go 구현으로 교체할 여지를 남기기 위함이다.

```go
type RefStore interface {
    Get(ref string) (plumbing.Hash, error)          // ErrRefNotFound
    CAS(ref string, new, old plumbing.Hash) error   // ErrCASConflict | ErrLockBusy
    Transaction(ops []RefOp) error                   // 전부 성공 or 전부 실패
    List(prefix string) ([]RefEntry, error)
}

type RefOp struct {
    Kind RefOpKind // Update | Create | Delete
    Ref  string
    New  plumbing.Hash
    Old  plumbing.Hash
}
```

- `ExecRefStore` — v1. `git update-ref` 및 `git update-ref --stdin`
- `NativeRefStore` — v2 후보. 직접 `.lock` 프로토콜 구현

`List` 만 go-git 으로 구현하고 나머지는 exec 이다. 인터페이스를 나누지 않는 이유는 호출부가 백엔드를 알 필요가 없기 때문이다.

### 14.6 오류 분류

4.2의 두 실패를 구분하는 지점이다. `git update-ref` 의 stderr 를 파싱한다.

```go
var (
    ErrLockBusy    = errors.New("ref lock busy")     // 재시도
    ErrCASConflict = errors.New("ref changed")       // 재판단
)

func classifyRefError(stderr []byte, exitCode int) error {
    s := string(stderr)
    switch {
    case strings.Contains(s, "cannot lock ref"),
         strings.Contains(s, "unable to create") && strings.Contains(s, ".lock"),
         strings.Contains(s, "Unable to create") && strings.Contains(s, ".lock"):
        return ErrLockBusy
    case strings.Contains(s, "but expected"),
         strings.Contains(s, "reference already exists"):
        return ErrCASConflict
    }
    return fmt.Errorf("update-ref: %s", strings.TrimSpace(s))
}
```

문자열 매칭은 git 버전에 따라 문구가 바뀔 수 있으므로 **취약하다.** 완화책:

- 분류 실패 시 기본값은 `ErrLockBusy` 가 아니라 일반 오류로 둔다. 잘못된 재시도보다 명시적 실패가 낫다
- `git --version` 을 `init` 에서 확인하고 최소 버전(2.28)을 강제한다
- 11.1 동시성 테스트가 두 오류를 실제로 유발하도록 작성해 회귀를 잡는다

### 14.7 쓰기 경로 (go-git + exec)

```
issue.json  → go-git  storer.SetEncodedObject (blob)
tree        → go-git  (blob)
commit      → go-git  Agent-Session trailer 포함 (4.3)
──────────────────────────────────────────────────
ref update  → exec    git update-ref <ref> <new> <old>
```

객체 생성이 go-git 인 것이 안전한 이유: 같은 content는 같은 OID 로 수렴하므로 두 프로세스가 동시에 같은 객체를 써도 결과가 같다. **경쟁은 ref 갱신 지점에만 존재한다.**

에이전트 신원(7.4)은 go-git `object.Signature` 로 직접 설정하므로 `GIT_AUTHOR_*` 환경변수 주입이 필요 없다. `commit.gpgsign` 도 go-git 이 자동 적용하지 않으므로 무시할 수 있다.

### 14.8 패키지 구조

```
cmd/ppwk/       CLI 진입점 (cobra)
internal/board/      도메인 로직: 상태 전이, gate, next 알고리즘
internal/refstore/   RefStore 인터페이스 + ExecRefStore
internal/model/      Issue, Plan, Decision 스키마 + JSON
internal/gitobj/     go-git 객체 생성/읽기 래퍼
internal/session/    잠금 파일, 생존 판정, 도구 감지
internal/watch/      polling + hook socket 수신
```

`internal/board` 는 `RefStore` 인터페이스에만 의존한다. 동시성 테스트(11.1)를 메모리 구현으로 빠르게 돌리고, 실제 경쟁은 `ExecRefStore` 로 별도 검증한다.

### 14.9 테스트 보강

11장에 다음을 추가한다.

```
- ExecRefStore: 실제 프로세스 N개를 띄워 동시 CAS (goroutine 아님)
      go-git 의 CheckAndSetReference 로는 통과하지 못하는 테스트여야 한다
- classifyRefError: lock 실패와 CAS 실패를 각각 실제로 유발해 분류 검증
- go-git 으로 만든 commit 을 git CLI 가 정상으로 읽는지 (trailer 포함)
- EnableDotGitCommonDir 없이 열면 linked worktree 에서 ref 가 안 보이는지
      (옵션 누락 회귀 방지)
```
