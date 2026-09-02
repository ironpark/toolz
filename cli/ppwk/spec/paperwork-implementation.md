# paperwork 구현 계획

설계 문서 `paperwork-design.md` v5.2 기준 / Go + go-git 하이브리드

완성 시점의 명령어와 기능 범위는 `paperwork-features.md`, 바이너리 수준 검증은 `paperwork-e2e.md` 참조

버전 5.2 / 2026-09-02

---

## 이 문서의 사용법

각 단계는 **Exit criteria** 를 전부 통과해야 다음 단계로 넘어간다. 통과하지 못한 채 다음 단계를 시작하면, 뒤에서 발견되는 버그가 앞 단계의 설계 결함인지 새 코드의 실수인지 구분할 수 없게 된다.

각 단계는 다음 구조를 갖는다.

- **목표** — 이 단계가 끝났을 때 존재하는 것
- **범위 밖** — 이 단계에서 하지 않을 것 (범위 확장 방지)
- **테스트** — 작성해야 할 테스트
- **엣지 케이스** — 반드시 명시적으로 다뤄야 할 상황
- **Exit criteria** — 다음 단계로 넘어가는 조건

설계 문서의 절 번호를 `(§4.2)` 형태로 참조한다.

### 단계 개요

| 단계 | 내용 | 이 단계 없이는 |
|---|---|---|
| 0 | RefStore + 오류 분류 | 동시성이 전부 무의미 |
| 1 | 모델 + 객체 read/write | 저장할 것이 없음 |
| 2 | CAS 프로토콜 | 설계의 근간이 없음 |
| 3 | 상태 전이 | 이슈가 움직이지 않음 |
| 4 | 암묵 세션 + 게으른 reap | 죽은 에이전트가 작업을 붙잡음 |
| 5 | next 스케줄러 | 에이전트가 스스로 일을 못 가져감 |
| 6 | archive + 트랜잭션 | 보드가 무한히 커짐 |
| 7 | watch | 변경을 모름 |
| 8 | export / fsck / history | 운영이 불가능 |
| 9 | plan / phase | 계획 없이 평면 목록만 |
| 10 | git hook | polling 지연이 남음 |
| 11 | 도구 훅 | 정상 종료 시 정리가 지연됨 |
| 12 | 결정 기록 | 세션 간 같은 논의 반복 |

---

## 단계 0 — RefStore 와 오류 분류

가장 먼저 하는 이유: 이 위에 나머지 전부가 올라간다. 나중에 만들면 도메인 로직이 go-git 타입에 직접 결합되어 분리가 불가능해진다.

### 목표

```go
type RefStore interface {
    Get(ref string) (plumbing.Hash, error)
    CAS(ref string, new, old plumbing.Hash) error
    Transaction(ops []RefOp) error
    List(prefix string) ([]RefEntry, error)
}
```

- `ExecRefStore` — `git update-ref` / `--stdin` 래핑, `List` 는 go-git
- `MemRefStore` — 테스트용. 진짜 mutex 로 원자성 보장
- `classifyRefError` (§14.6)
- repo 열기: `EnableDotGitCommonDir: true` (§14.4)

### 범위 밖

이슈, 상태, JSON, CLI. 이 단계는 순수하게 "ref 를 원자적으로 바꾸는 방법" 만 다룬다.

### 테스트

```
[T0.1] 빈 ref 에 CAS(new, ZeroHash) → 생성 성공
[T0.2] 같은 CAS 를 두 번 → 두 번째는 ErrCASConflict
[T0.3] old 가 현재 값과 다름 → ErrCASConflict
[T0.4] 존재하지 않는 ref 를 Get → ErrRefNotFound
[T0.5] Transaction 3개 op 전부 성공
[T0.6] Transaction 중 하나가 CAS 위반 → 전부 롤백, 나머지 ref 도 안 바뀜
[T0.7] 프로세스 N=16 개를 실제로 spawn 해 같은 ref 에 동시 CAS
           → 정확히 1개 성공, 15개 ErrCASConflict 또는 ErrLockBusy
[T0.8] classifyRefError: lock 실패 문자열 → ErrLockBusy
[T0.9] classifyRefError: CAS 실패 문자열 → ErrCASConflict
[T0.10] classifyRefError: 알 수 없는 문자열 → 일반 오류 (ErrLockBusy 아님)
[T0.11] linked worktree 3개에서 각각 List → 동일한 결과
[T0.12] EnableDotGitCommonDir 없이 열면 linked worktree 에서 ref 안 보임
            (옵션 누락 회귀 방지)
```

#### fuzz (단계 0)

```
[F0.1] FuzzRefName — 우리 검증기와 git check-ref-format 의 차등 테스트
           우리가 통과시킨 ID 는 git 도 반드시 통과시켜야 한다.
           `..`, `@{`, 후행 `.lock`, 제어문자, 빈 컴포넌트 등을
           자체 구현으로 다 막았다고 착각하기 쉽다.

[F0.2] FuzzClassifyRefError — 안전 방향 검증
           임의 문자열이 ErrLockBusy 로 분류되면 실패.
           정확성이 아니라 "위험한 방향으로 틀리지 않는지" 를 본다.
           잘못된 ErrLockBusy 는 무한 재시도나 중복 배정으로 이어진다.
           모르는 입력은 반드시 일반 오류로 떨어져야 한다.
```

**T0.7 은 goroutine 이 아니라 실제 프로세스여야 한다.** goroutine 으로 하면 go-git 의 `CheckAndSetReference` 로도 통과해버려 테스트가 의미를 잃는다 (§14.2). 테스트 바이너리를 `exec.Command` 로 재실행하거나 별도 헬퍼 바이너리를 쓴다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| `git` 바이너리가 `PATH` 에 없음 | 시작 시 명확한 오류. 런타임에 발견되면 안 됨 |
| `git` 버전 < 2.28 | 거부 (§6.3 hook 의존) |
| ref 이름에 `..`, 공백, 제어문자 | `git check-ref-format` 통과 여부를 자체 검증 |
| `List` 중 다른 프로세스가 ref 삭제 | 부분 결과 허용, panic 금지 |
| `packed-refs` 로 packing 된 상태 | `List`/`Get` 정상 동작 |
| CAS 대상이 loose 가 아니라 packed | `update-ref` 가 알아서 처리 — 검증만 |
| stderr 가 비어있는데 exit≠0 | 일반 오류 |
| `cmd.Dir` 이 잘못된 경로 | 시작 시 검증 |
| 동시 CAS 로 `.lock` 이 남은 채 프로세스 죽음 | 다음 시도가 stale lock 을 만남 → 오류 전파, 자동 삭제 금지 |

마지막 항목이 중요하다. **stale `.lock` 을 도구가 임의로 지우면 안 된다.** 진짜로 다른 프로세스가 작업 중일 수 있다. 사용자에게 보고하고 수동 조치를 안내한다.

### Exit criteria

- T0.1~T0.12 전부 통과
- `MemRefStore` 와 `ExecRefStore` 가 **동일한 테스트 스위트**를 통과 (table-driven)
- `internal/board` 가 아직 존재하지 않음 (결합 방지 확인)

---

## 단계 1 — 모델과 객체 I/O

### 목표

- `Issue`, `Plan`, `Lease` 구조체 + JSON 마샬링 (§3.4, §3.7.2, §3.6)
- go-git 으로 blob/tree/commit 생성 (§14.7)
- commit message 조립: subject + trailer 블록 (§3.3)
- trailer 파싱
- `paperwork init` (§8)
- 읽기 전용 `list`, `show` — CAS 없이, 단일 프로세스 가정

### 범위 밖

상태 전이, 동시성, claim. 지금은 이슈를 만들고 읽기만 한다.

### 테스트

```
[T1.1] Issue → JSON → Issue 왕복이 손실 없음
[T1.2] 알 수 없는 필드가 있는 JSON 을 읽어도 보존됨 (forward compat)
[T1.3] commit message 조립 → 파싱 왕복
[T1.4] trailer 값에 콜론/개행 포함 → 정상 파싱 또는 명시적 거부
[T1.5] go-git 으로 만든 commit 을 git CLI 가 정상으로 읽음 (trailer 포함)
[T1.6] git CLI 로 만든 commit 을 go-git 이 정상으로 읽음
[T1.7] init 이 log.excludeDecoration, core.filesRefLockTimeout 설정
[T1.8] init 두 번 실행해도 안전 (멱등)
[T1.9] core.hooksPath 설정된 상태에서 init → 경고 출력
[T1.12] init 이 AGENTS.md + docs/paperwork/*.md 전부 생성
[T1.13] 기존 파일은 덮어쓰지 않음 (파일 단위 판단)
            일부만 존재하면 없는 것만 생성
[T1.14] --no-agents-md 로 전체 생성 건너뜀
[T1.15] AGENTS.md 의 모든 상대 링크가 실제 파일을 가리킴
            (링크 깨짐 회귀 — 파일명을 바꾸면 실패해야 한다)
[T1.16] AGENTS.md 가 크기 예산 이내 (기본 80줄)
            매 세션 로드되므로 증가를 테스트로 막는다
[T1.10] plan/phase/seq 없는 이슈가 정상 처리 (선택 필드)
[T1.11] plan 만 있고 phase 없음 → 검증 오류
```

#### fuzz (단계 1)

이 단계가 fuzz 가 가장 값을 하는 지점이다. 순수 함수이고 입력 공간이 넓다.

```
[F1.1] FuzzMessageRoundTrip — 만들어낸 메시지는 반드시 파싱된다
           BuildMessage 가 성공했다면 ParseMessage 가 반드시 성공하고
           값이 왕복해야 한다. 제목에 "\nStatus: done" 이 들어가
           trailer 를 오염시키는 케이스를 fuzz 가 금방 찾는다.

[F1.2] FuzzIssueJSONRoundTrip — 우리 출력은 우리가 다시 읽는다
           + 미지 필드가 유실되지 않는다 (§9.4 마이그레이션 안전장치).
           손으로는 필드 조합을 다 만들 수 없다.

[F1.3] FuzzTrailerParse — 임의 바이트를 파싱해도 panic 하지 않는다
```

F1.1 의 불변식은 "정확한 파싱" 이 아니라 **"자기가 만든 것을 자기가 읽는다"** 이다. 이 쪽이 훨씬 강한 성질이고 반례를 찾기도 쉽다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| 이슈 제목에 개행 | subject 는 첫 줄만, 나머지는 body.md 로 |
| 제목이 매우 김 (>200자) | 잘리지 않고 보존. subject 만 축약 표시 |
| 제목이 빈 문자열 | 거부 |
| 제목에 트레일러처럼 보이는 문자열 (`Status: x`) | trailer 파싱이 오염되지 않아야 함 |
| 비 ASCII 제목 (한글, 이모지) | 정상 처리 |
| `schema` 가 미래 버전 | 읽기만 허용, 쓰기 거부 (§9.4) |
| `schema` 필드 없음 | 1 로 간주 |
| 손상된 JSON blob | 해당 이슈만 오류, `list` 전체가 죽지 않음 |
| tree 에 예상 밖 파일 | 무시하고 보존 |

**T1.2 와 마지막 항목이 짝을 이룬다.** 미래 버전이 추가한 필드를 구버전 CLI 가 지우면 데이터가 소실된다. 읽은 원본을 보존하고 아는 필드만 갱신한다.

### Exit criteria

- T1.1~T1.11 통과
- `paperwork init && paperwork add "x" && paperwork list` 가 동작
- `git status` 가 clean, `git log <branch>` 에 변화 없음 (§11.5)

---

## 단계 2 — CAS 프로토콜

**이 단계가 시스템의 전부다.** 여기서 타협하면 나머지가 전부 무의미하다.

### 목표

§4.1 의 6단계 루프 구현:

```
1. old 읽기
2. 현재 상태 파싱
3. 전이 규칙 검사
4. 새 객체 생성 (Agent-Session trailer 포함)
5. CAS
6. lock 실패 → 재시도 / CAS 실패 → 처음부터
```

- 세션 nonce 생성 (§4.3)
- backoff 정책 (지수 + jitter)
- 재시도 상한

### 범위 밖

구체적 전이 명령(claim/start/done). 지금은 프로토콜 자체와 테스트용 더미 전이 하나만.

### 테스트

```
[T2.1] 단일 프로세스 CAS 성공
[T2.2] 프로세스 N=16 이 동시에 같은 이슈 변경 → 정확히 1개 성공
[T2.3] 동일 tree/parent/시각으로 두 commit 생성 → OID 가 달라야 함
           (§4.3 회귀. Agent-Session 제거하면 실패해야 정상)
[T2.4] lock 실패 주입 → backoff 후 최종 성공
[T2.5] CAS 실패 → 재시도가 아니라 상태 재읽기 경로로 감
[T2.6] 재시도 상한 초과 → exit code 4 (§7.3)
[T2.7] 전이 규칙 위반 → exit code 3, 재시도하지 않음
[T2.8] 4단계와 5단계 사이에 외부에서 ref 변경 → CAS 실패로 검출
[T2.9] backoff 에 jitter 가 있어 N개가 동시에 재시도하지 않음
```

#### fuzz 는 여기에 맞지 않는다

**CAS 동시성은 fuzz 로 잡히지 않는다.** fuzz 는 입력을 흔드는 도구지 스케줄링을 흔드는 도구가 아니다. 프로세스 인터리빙은 사정거리 밖이다. 대신 두 가지를 쓴다.

```
[D2.1] FaultyRefStore — RefStore 인터페이스(§14.5) 뒤에 지연/실패 주입
           seed 로 결정적 재현이 가능한 동시성 테스트.
           - CAS 직전 지연 주입 → 경쟁 창 확대
           - ErrLockBusy 를 확률적으로 반환
           - 객체 생성 후 CAS 전에 프로세스 중단 시뮬레이션
           실패한 seed 를 기록해 회귀 테스트로 고정한다.

[D2.2] 스트레스 반복 — T2.2 를 100회 (R8)
```

`RefStore` 를 단계 0 에서 인터페이스로 뽑아둔 것이 여기서 값을 한다. 구현체를 결함 주입 버전으로 바꾸는 것만으로 결정적 동시성 테스트가 된다. **fuzz 보다 이쪽이 훨씬 값이 크다.**

**T2.3 이 이 단계의 핵심 회귀 테스트다.** `Agent-Session` trailer 를 코드에서 빼면 이 테스트가 반드시 실패해야 한다. 실패하지 않으면 테스트가 잘못 작성된 것이다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| ref 가 CAS 직전에 삭제됨 | CAS 실패 → 재읽기 → "이슈 없음" (exit 5) |
| 재시도 중 이슈가 다른 상태로 변함 | 전이 규칙 재검사. 이제 위반이면 exit 3 |
| 시스템 시계가 뒤로 감 | `updated_at` 이 역행해도 동작에 영향 없어야 함 |
| 세션 nonce 충돌 | 128비트 이상 사용. 확률적으로 무시 |
| 무한 CAS 경쟁 (라이브락) | 상한 + jitter 로 완화. 상한 도달 시 명시적 실패 |
| 디스크 가득 참 | 객체 생성 실패 → ref 안 바뀜. 부분 상태 없음 |
| 중간에 SIGKILL | dangling commit 만 남고 ref 는 안 바뀜 |

마지막 두 개가 이 설계의 강점이다. **객체를 먼저 만들고 ref 를 나중에 바꾸므로, 어느 시점에 죽어도 부분 상태가 생기지 않는다.** 테스트로 명시적으로 확인한다.

### Exit criteria

- T2.1~T2.9 통과
- **T2.2 를 100회 반복 실행해 flake 없음**
- `Agent-Session` 제거 시 T2.3 이 실패함을 확인 (테스트의 테스트)
- 이 단계 이전으로 돌아갈 일이 없다는 확신

---

## 단계 3 — 상태 전이

### 목표

`claim` / `start` / `done` / `block` / `unblock` / `release` / `cancel` (§3.5)

### 테스트

```
[T3.1] 각 유효 전이가 성공
[T3.1b] start 가 open 에서 working 으로 한 CAS 에 전이 (claim 겸함, D16)
[T3.1c] block 이 --on 과 --message 를 각각·함께 받음
[T3.2] 각 무효 전이가 exit 3 으로 거부 (전체 조합 table-driven)
[T3.3] done 상태에서 어떤 전이도 불가 (cancel 포함)
[T3.4] block 시 대상 이슈 ID 기록
[T3.5] 다른 에이전트가 소유한 이슈를 start → 거부
[T3.6] 소유자가 아닌 에이전트가 release → 거부 또는 강제 옵션 요구
[T3.7] history 가 이벤트 순서대로 나옴
[T3.8] 배정 관련 명령이 존재하지 않음
           assign / unassign / inbox / accept / reject 부재
           (재도입 방지 회귀 — 배정은 오케스트레이터 담당, §8.0)
```

#### fuzz 는 여기에 맞지 않는다

상태 6개, 전이 7개라 조합이 작다. **exhaustive table-driven 이 fuzz 보다 낫다** — 전부 덮으면서 결정적이다. T3.2 가 그 역할을 한다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| 자기 자신에게 block | 거부 |
| 순환 block (A→B→A) | 거부 또는 fsck 검출 |
| 존재하지 않는 이슈를 block 대상으로 | 거부 |
| claim 된 이슈를 다른 에이전트가 claim | exit 3 (전이 위반), 4 아님 |
| 아무도 claim 하지 않은 이슈 | open 유지. 방치 자체가 정상 상태 |
| 이미 done 인 이슈를 done | 멱등으로 성공? → **거부한다.** 실수를 숨기지 않음 |
| cancel 후 남은 owner 필드 | 정리 |

`claim` 경쟁이 exit 3 인지 4 인지 구분이 중요하다. **CAS 에서 진 것은 4, 이미 남이 갖고 있는 걸 뒤늦게 안 것은 3이다.** 에이전트가 재시도 여부를 여기서 판단한다.

### Exit criteria

- T3.1~T3.7 통과
- 전이 매트릭스 전체 조합이 테스트로 덮임

---

## 단계 4 — 암묵 세션과 게으른 reap

### 목표

- **암묵 세션 등록** — 첫 상태 변경 명령이 자동 수행 (§3.6)
- `flock` 기반 잠금 파일 read-modify-write
- 생존 판정 우선순위 5단계 (§3.6)
- `last_activity` 갱신 — 명령 실행 자체가 갱신
- worktree 배타 — 첫 상태 변경 명령에서 거부
- 게으른 reap: `next` 진입 시 owner 기준 확인 (§4.5)
- `reap` 수동 명령

### 범위 밖

heartbeat, 데몬, 세션 관리 명령. **상시 프로세스와 사용자 대면 세션 명령을 만들지 않는다.**

### 테스트

```
[T4.1]  초기화 명령 없이 claim → 세션이 암묵 등록됨
[T4.1b] 사용자 대면 CLI 에 session 계열 명령이 존재하지 않음
            (--help 출력 검증. 재도입 방지 회귀)
[T4.1c] 잠금 파일 read-modify-write 가 원자적
            (N개 프로세스 동시 등록 → 정확히 1개만 worktree 확보)
[T4.2]  flock 이 파일 갱신 순간에만 잡힘
            (명령 종료 후 파일이 잠긴 상태로 남지 않음)
[T4.3] 죽은 소유자의 claimed 이슈가 open 으로 회수
[T4.4] 죽은 소유자의 working 이슈가 open 으로 회수
[T4.5] 살아있는 소유자의 이슈는 회수되지 않음
[T4.6] blocked 이슈는 owner 만 해제, 상태는 blocked 유지
[T4.7] 같은 이름으로 재시작 → 이전 session 의 claim 이 회수됨
           (owner 이름만 비교하면 실패하는 회귀 테스트)
[T4.8] 같은 worktree 에서 두 번째 에이전트의 상태 변경 → 거부
[T4.9] --allow-shared-worktree 로 우회 가능
[T4.10] 조회 명령(list/show/watch)은 잠금 없이 병행 가능
[T4.11] N개 프로세스가 동시 reap → 정확히 1회만 회수
[T4.12] 회수 대상이 없으면 ref 쓰기 0회
[T4.13] 임계값 조정(PAPERWORK_ACTIVITY_TTL)이 반영됨
[T4.14] refs/paperwork/agents/ 가 존재하지 않음
           (D13 재도입 방지. agents 명령은 잠금 파일만 읽는다)
[T4.15] 생존 판정 5단계가 각각 정확히 동작 (§3.6 표, 케이스당 1개)
[T4.16] last_activity 가 임계값 이내면 생존, 초과면 사망
[T4.16b] 상태 변경 명령이 last_activity 를 갱신
[T4.16d] 조회 명령(list/show/agents 등)은 last_activity 를 갱신하지 않음
[T4.16e] 기본 임계값이 8h (start 후 7시간 CLI 무호출 → 생존 유지)
            30분 등으로 줄이는 변경은 반드시 실패해야 한다 (D11)
[T4.16c] 프로세스 이름 조회 코드가 존재하지 않음
            /proc/<pid>/comm, sysctl, exec name 매칭 전부 없어야 함
            (트리 탐색 재도입 방지 회귀)
[T4.17] 잠금 기록이 손상된 JSON → 사망 판정, panic 없음
[T4.18] reap 이 owner 기준으로 묶여 확인 횟수가 이슈 수에 비례하지 않음
```

#### 도구 감지 (§3.8 층 2)

훅 없이 환경변수만으로 동작하는 부분이라 이 단계에 포함한다.

```
[T4.20] CLAUDE_CODE_SESSION_ID 설정 시 세션 ID 로 채택
[T4.21] CLAUDECODE 설정 시 agent-id 가 claude-code:<worktree>
[T4.22] CODEX_* 설정 시 agent-id 가 codex:<worktree>
[T4.23] 감지 신호가 전무하면 <hostname>:<worktree> 폴백
[T4.24] PAPERWORK_AGENT 가 감지보다 우선
[T4.25] PAPERWORK_SESSION 이 도구 세션 ID 보다 우선
[T4.26] 같은 세션 ID 로 실행된 여러 명령이 같은 세션으로 묶임
[T4.27] doctor 가 감지 근거(환경변수 이름)를 함께 표시
```

**T4.23 이 중요하다.** 환경변수 이름은 도구 버전에 따라 바뀔 수 있으므로, 감지 실패가 오류가 아니라 폴백으로 흡수되어야 한다.

**T4.1 과 T4.1c 가 이 설계의 핵심이다.** 에이전트가 아무것도 호출하지 않아도 세션이 등록되는 것, 그리고 그 등록이 원자적이라 worktree 경쟁이 없는 것.

**T4.1b 는 재도입 방지다.** `session begin` 류의 명령은 "명시적인 편이 안전하지 않나" 라는 이유로 다시 들어오기 쉽다. 세션 수명 동안 잠금을 쥐면 멈춘 프로세스를 영구히 붙잡게 된다 (§3.6).

**T4.16c 도 중요하다.** 프로세스 이름으로 도구를 찾는 접근은 조용히 틀리고 이식성 비용이 크다 (§3.6). 코드에 다시 들어오는 것을 테스트로 막는다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| 잠금 디렉터리 없음 | 자동 생성 |
| 잠금 파일 권한 없음 | 명확한 오류 |
| NFS 등 flock 미지원 | `doctor` 가 경고. 동작은 시도 |
| next --dry-run | reap 포함 어떤 쓰기도 없음 |
| pid 재사용 | `starttime` 비교로 구분 (표시용) |
| 회수 중 이슈가 done 으로 변함 | CAS 실패 → 회수 취소 (정상) |
| worktree 경로에 심볼릭 링크 | 정규화 후 해시 (macOS `/tmp` 주의) |
| worktree 삭제 후 잠금 파일 잔존 | 무해. 다음 획득이 성공 |
| 같은 세션이 여러 번 등록 시도 | 멱등. last_activity 만 갱신 |
| 훅 없음 | last_activity 로 판정. 오류 아님 |
| hook_pid 는 있는데 starttime 불일치 | pid 재사용. 사망 판정 |
| 시계가 뒤로 감 | last_activity 가 미래여도 생존으로 취급 |
| 잠금 파일 디렉터리가 읽기 전용 | 명확한 오류. 조용히 넘어가지 않음 |

### Exit criteria

- T4.1~T4.27 통과
- **상시 실행 프로세스가 코드에 존재하지 않음** (데몬 제거 확인)
- T4.7, T4.14, T4.16c 가 각각 코드 한 줄 되돌리면 실패함을 확인
- 사용자 대면 CLI 에 `session` 명령이 없음 (T4.1b)

---

## 단계 5 — next 스케줄러

### 목표

§7.2 알고리즘. 단, phase gate 는 아직 없음 (단계 9).

```
reap(잠금 확인) → open 수집 → depends_on 검사 → 정렬 → CAS claim → 실패 시 다음 후보
```

reap 이 `next` 안에 있는 것이 **유일한 자동 실행 지점**이다. 별도 주기 실행이 없다.

### 테스트

```
[T5.1] 후보 없음 → 빈 결과, exit 0
[T5.2] 우선순위 정렬 정확성
[T5.3] depends_on 미충족 이슈는 후보 제외
[T5.3b] priority none 은 후보 제외, 상태는 open 유지
[T5.3c] priority none → low 로 edit 하면 후보에 등장
[T5.4] 의존 대상이 archive 에 있어도 done 으로 인식
           (issues/ 만 보면 실패하는 회귀 테스트)
[T5.5] N=16 프로세스 동시 next --claim → 중복 배정 0건
[T5.6] 후보 M개, 에이전트 N>M → M개만 배정, 나머지는 빈 결과
[T5.7] CAS 실패 시 같은 이슈 재시도가 아니라 다음 후보로
```

#### fuzz (단계 5)

입력을 **구조화**해야 의미가 있다. 임의 바이트가 아니라 seed 로 보드 전체를 결정적으로 생성한다.

```
[F5.1] FuzzCandidates — 후보 선정 불변식
           seed → (plan, issues) 생성 후 Candidates() 결과 검증:
           - 모든 후보의 status 가 open
           - 모든 후보의 depends_on 이 전부 done
           - 순환 의존이 있어도 무한 루프/panic 없음

[F5.2] FuzzSortOrder — 비교 함수가 전순서(total order)인가
           plan priority → seq → priority → created_at 의 다단계 비교에서
           비일관 비교자를 만들면 sort.Slice 가 예측 불가능하게 동작한다.
           반사성/반대칭성/추이성을 무작위 3원소로 검증한다.
```

F5.2 는 겉보기에 사소하지만, 비일관 비교자는 **정렬 결과가 실행마다 달라지는** 형태로 나타나 재현이 매우 어렵다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| 순환 의존성 | 해당 이슈들이 영원히 후보에서 빠짐 + fsck 검출 |
| 자기 자신에 의존 | 거부 (add 시점) |
| 존재하지 않는 ID 에 의존 | 후보 제외 + fsck 보고 |
| 모든 후보가 CAS 실패 | 빈 결과. 오류 아님 |
| 후보 수천 개 | 정렬 성능. 전부 claim 시도하지 않고 상한 |
| depends_on 대상이 cancelled | done 으로 취급? → **아니다.** cancelled 는 의존 충족 아님 |
| priority none 이슈가 plan 에 속함 | gate 의 all_done 계산에는 포함 (open 이므로 미완) |
| priority none 이슈에 의존하는 이슈 | 영원히 후보 아님. fsck 경고 |

마지막 항목은 판단이 필요하다. 취소된 선행 작업 때문에 후속이 영원히 막히는 것도 문제이므로, **cancelled 는 의존을 충족하지 않되 fsck 가 경고**하도록 한다.

### Exit criteria

- T5.1~T5.7 통과
- T5.5 를 100회 반복해 중복 배정 0건

---

## 단계 6 — archive 와 다중 ref 트랜잭션

### 목표

종료 상태 이슈를 `issues/` → `archive/` 로 **단일 트랜잭션** 이동 (§4.4)

### 테스트

```
[T6.1] done 시 archive 로 이동, issues/ 에서 사라짐
[T6.2] 이동 중 SIGKILL → 양쪽에 동시 존재하거나 양쪽에서 사라지지 않음
[T6.3] archive 된 이슈의 history 가 보존됨
[T6.4] list 는 archive 를 제외, list --archived 는 포함
[T6.5] 의존성 검사가 archive 를 조회 (T5.4 와 연동)
[T6.6] 이동과 동시에 다른 프로세스가 같은 이슈 변경 시도 → 하나만 성공
```

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| archive 에 같은 ID 가 이미 존재 | 트랜잭션 거부. 덮어쓰지 않음 |
| archive 이동 후 그 이슈를 show | 정상 조회 |
| archive 된 이슈를 reopen | 지원 여부 결정. v1 은 미지원, 명시적 오류 |
| 트랜잭션 중 `.lock` 경쟁 | 전체 재시도 |
| archive 가 수천 개 | `list` 성능에 영향 없어야 함 (prefix 분리의 목적) |

**T6.2 가 §4.4 의 존재 이유다.** 개별 `update-ref` 두 번으로 구현하면 이 테스트가 실패한다. 실제로 그렇게 한 번 구현해보고 실패를 확인한 뒤 트랜잭션으로 고치는 것을 권한다.

### Exit criteria

- T6.1~T6.6 통과
- 개별 update-ref 2회 구현으로 되돌리면 T6.2 가 실패함을 확인

---

## 단계 7 — watch (polling)

### 목표

`for-each-ref` 결과를 주기적으로 비교해 변경 이벤트 생성 (§6.2)

### 테스트

```
[T7.1] 이슈 생성 → created 이벤트
[T7.2] 상태 변경 → updated 이벤트, old/new OID 포함
[T7.3] archive 이동 → deleted + created 이벤트
[T7.4] 변경 없으면 이벤트 없음
[T7.5] pack-refs 실행 후에도 정상 감지
[T7.6] 여러 변경이 한 주기에 발생 → 전부 보고
```

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| 한 주기 안에 A→B→A 변경 | 최종 상태만 보고 (중간 유실 허용, 문서화) |
| 첫 실행 (이전 스냅샷 없음) | 전체를 created 로 쏟지 않음. 베이스라인만 잡음 |
| 저장소가 매우 큼 | polling 간격 대비 조회 시간 확인 |
| watch 중 저장소 삭제 | 명확한 오류로 종료 |
| SIGINT | 즉시 정리 종료 |

**파일 mtime 이나 inotify 를 쓰지 않았는지 코드 리뷰로 확인한다** (§6.2). packed-refs 와 reftable 에서 조용히 깨진다.

### Exit criteria

- T7.1~T7.6 통과
- 코드에 `fsnotify`, `inotify`, `Stat().ModTime()` 사용 없음

---

## 단계 8 — export / fsck / history

### 목표

운영 도구. §5.4, §9.3

### 테스트

```
[T8.1] export json 이 유효한 JSON
[T8.2] export md 헤더에 생성 시각과 "파생물" 경고 포함
[T8.3] fsck 가 §9.3 의 각 항목을 실제로 검출 (항목당 테스트 1개)
[T8.4] fsck --fix 가 trailer 불일치를 복구
[T8.5] fsck --fix 가 데이터를 손실시키지 않음
[T8.6] 정상 저장소에서 fsck → 무결점, exit 0
[T8.7] history 가 이벤트 subject 를 그대로 보여줌
```

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| fsck 가 손상된 이슈를 만남 | 해당 항목만 보고, 나머지 계속 검사 |
| fsck --fix 중 다른 에이전트가 작업 중 | CAS 로 안전. 실패는 보고 |
| export 중 상태 변경 | 스냅샷 일관성 미보장 — 문서화 |
| history 가 매우 김 | 페이징 또는 `-n` 옵션 |
| trailer 와 issue.json 불일치 | issue.json 을 신뢰 (§3.3) |

### Exit criteria

- T8.1~T8.7 통과
- §9.3 의 검사 항목 전부에 대응 테스트 존재

---

## 단계 9 — plan 과 phase

### 목표

§3.7 전체. plan ref, gate 검사, 정렬 변경.

### 테스트

```
[T9.1] plan new / phase add / plan show
[T9.2] all_done gate: 직전 phase 에 open 이 있으면 막힘
[T9.3] any_done gate: 하나만 done 되어도 열림
[T9.4] manual gate: advance 전까지 막히고 이후 열림
[T9.5] task 없는 phase → gate 통과(공허참) + fsck 경고
[T9.6] N개 에이전트가 같은 plan 의 task 동시 claim
           → 중복 0건 **및 plan ref 쓰기 0회** (경쟁 분산 회귀)
[T9.7] plan close 후 소속 open task 가 후보에서 제외
[T9.8] seq 가 priority 를 앞섬: high/seq30 이 med/seq10 보다 나중
[T9.9] plan 에 속하지 않은 이슈가 계획 이슈와 함께 정렬됨
[T9.10] gate 로 막힌 task 의 status 가 open 이지 blocked 가 아님
```

**T9.6 이 §3.7.1 의 회귀 테스트다.** plan 에 진행률 필드를 추가하는 순간 이 테스트가 실패해야 한다.

**T9.10 도 중요하다.** gate 는 조회 시점 판단이지 저장된 상태가 아니다. 이걸 어기면 phase 가 열릴 때 되돌리는 일괄 쓰기가 필요해지고, 그게 plan 단위 경쟁으로 번진다.

#### fuzz (단계 9)

```
[F9.1] FuzzPhaseGate — gate 계산 불변식
           seed → 임의 phase 구성 + 임의 task 상태 분포
           - 첫 phase 는 항상 열림
           - all_done 인데 직전 phase 에 open 이 있으면 반드시 닫힘
           - manual 인데 advanced_phases 에 없으면 반드시 닫힘
           - phase 개수/상태와 무관하게 panic 없음

[F9.2] F5.1 을 plan 이 있는 보드로 확장 — gate 불변식 추가
```

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| phase 재정렬 후 기존 task | `phase` 필드는 id 참조라 영향 없음 |
| phase 삭제 | 소속 task 가 고아. 거부하거나 이동 강제 |
| seq 중복 | 동작함. created_at 으로 tie-break. fsck 경고 |
| seq 음수 | 허용 (선두 삽입) |
| plan 이 phase 0개 | 모든 task 가 후보 아님. fsck 경고 |
| 존재하지 않는 phase 를 advance | 거부 |
| plan cancel 후 task | 후보 제외 + fsck 보고 |
| 매우 많은 plan | `plan show` 가 plan 수만큼 ref 읽음 — 상한 확인 |

### Exit criteria

- T9.1~T9.10 통과
- plan 에 상태 필드를 추가하면 T9.6 이 실패함을 확인

---

## 단계 10 — hook

### 목표

`reference-transaction` hook 설치 + socket 수신 (§6.3). **선택 기능**이다.

### 테스트

```
[T10.1] hook 설치 후 ref 변경 → socket 에 이벤트
[T10.2] 우리 namespace 밖 ref 변경 → 이벤트 없음
[T10.3] 일반 git commit → 이벤트 없음, 성능 영향 없음
[T10.4] listener 없음 → git 명령이 정상 완료 (블로킹 안 됨)
[T10.5] listener 가 응답 안 함 → timeout 후 git 명령 완료
[T10.6] hook 미설치 → polling 만으로 정상 동작
[T10.7] SHA-256 저장소에서 created/deleted 판정 정확
[T10.8] preparing/prepared 단계에서는 이벤트 없음
[T10.9] core.hooksPath 설정 시 install 이 경고하고 올바른 위치 사용
[T10.10] 기존 reference-transaction hook 존재 → 덮어쓰지 않고 중단
```

**T10.4 와 T10.5 가 가장 중요하다.** 알림 실패가 쓰기 경로를 막으면 안 된다. 알림은 부가 기능이라는 설계 원칙(§1.2)의 실증이다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| hook 이 ref 를 쓰려 함 | 코드에 없어야 함. 리뷰로 확인 (재귀) |
| socket 파일이 stale | listener 시작 시 정리 |
| 여러 listener | 하나만. 중복 시 오류 |
| socat 없음 | polling 으로 자동 폴백 |
| hook 실행 권한 없음 | install 시 chmod, 실패 시 오류 |
| git fetch 로 수천 ref 갱신 | prefix 필터가 먼저 → 성능 영향 없음 |

### Exit criteria

- T10.1~T10.10 통과
- hook 을 삭제해도 전체 통합 테스트가 통과 (선택 기능 확인)

---

## 단계 12 — 결정 기록 (선택)

### 목표

불변 ADR 을 `refs/paperwork/decisions/` 에 저장 (§3.9).

- `decide`, `decisions list/show/history/--search`
- trailer 비정규화 (`Title`, `Supersedes`, `Issues`)
- `show <issue>` 에 연결된 결정 표시
- `export --decisions`

### 범위 밖

결정 상태 머신, 승인 워크플로우, 수정 명령. **불변이라는 성질이 이 단계의 전부다.**

### 테스트

```
[T12.1]  decide → ref 생성, ID 채번 (create-only CAS)
[T12.2]  같은 결정을 동시 N개 생성 → ID 충돌 없이 전부 성공
[T12.3]  edit 류 명령이 존재하지 않음 (불변 회귀)
[T12.4]  --supersedes → 새 결정 생성, 이전 결정은 변경 없음
[T12.5]  superseded_by 가 조회 시 계산됨 (저장 안 됨)
[T12.6]  decisions 기본 목록이 superseded 를 제외
[T12.7]  decisions --issue T001 이 연결된 것만 반환
[T12.8]  show T001 이 연결된 결정을 표시
[T12.9]  export --decisions 가 결정당 파일 하나, 파생물 헤더 포함
[T12.10] fsck 가 존재하지 않는 issue/plan/supersedes 참조를 검출
[T12.11] fsck 가 supersedes 순환을 검출
[T12.12] 3 worktree 에서 결정이 즉시 공유됨 (브랜치 무관)
```

**T12.3 과 T12.5 가 핵심이다.** 불변성과 단방향 엣지. 둘 다 "편의를 위해" 어기고 싶어지는 지점이다.

#### fuzz (단계 12)

```
[F12.1] FuzzDecisionRoundTrip — JSON 왕복 + 미지 필드 보존 (F1.2 와 동일 패턴)
[F12.2] FuzzSupersedesChain — 임의 supersedes 그래프에서 순환 검출이 panic 없이 종료
```

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| 존재하지 않는 결정을 supersede | 거부 |
| 자기 자신을 supersede | 거부 |
| 이미 superseded 된 결정을 다시 supersede | 허용 (분기). history 에 표시 |
| options 가 비어 있음 | 허용. 경고 |
| decision 이 options 에 없음 | 허용. 경고 (사후 추가된 선택지일 수 있음) |
| 연결 이슈가 archive 에 있음 | 정상 조회 (archive 포함) |
| export 대상 디렉터리에 같은 파일 존재 | 덮어씀 (파생물이므로) |

### Exit criteria

- T12.1~T12.12 통과
- **이 프로젝트의 `paperwork-decisions.md` D1~D16 을 `decide` 로 다시 기록할 수 있음**
  - D8 → D9 → D11 의 폐기 체인이 `--supersedes` 로 표현됨
  - D10 (기각) 이 표현됨 — 기각도 결정이다

마지막 항목이 스키마의 시금석이다. 실제 결정 열여섯 개를 담지 못하면 스키마가 부족한 것이다.

---

## fuzz 운영 방침

### 대상과 비대상

| 대상 | 단계 | 도구 |
|---|---|---|
| trailer / JSON 왕복 | 1 | fuzz |
| ref 이름 검증 (git 과 차등) | 0 | fuzz |
| `classifyRefError` 안전성 | 0 | fuzz |
| 후보 선정 / 정렬 불변식 | 5 | 구조화 fuzz |
| supersedes 순환 검출 | 12 | 구조화 fuzz |
| gate 계산 불변식 | 9 | 구조화 fuzz |
| CAS 동시성 | 2 | 결함 주입 + 스트레스 (fuzz 아님) |
| 상태 전이 | 3 | exhaustive table (fuzz 아님) |

### 규칙

1. **fuzz 가 찾은 입력은 반드시 `testdata/fuzz/` 에 커밋한다.** 그 순간부터 일반 단위 테스트로 계속 돌며 회귀를 막는다.
2. **CI 는 짧게, 로컬은 길게.** CI 에서는 corpus 재생만 (`go test` 기본 동작). 장시간 fuzzing 은 별도 잡이나 수동으로 돌린다.
3. **불변식은 "정확한 값" 이 아니라 "성질" 로 쓴다.** 왕복 일치, panic 없음, 안전 방향 오분류 없음, 전순서. 기대값을 하드코딩하면 fuzz 의 이점이 사라진다.
4. **거부는 유효한 결과다.** 입력이 잘못되어 함수가 오류를 반환하면 그대로 통과시킨다. 오류를 실패로 취급하면 대부분의 입력이 조기 종료되어 탐색이 안 된다.

---

## 단계 11 — 도구 훅 (선택)

### 목표

`SessionStart` / `SessionEnd` 훅으로 세션 등록과 즉시 정리 (§3.8 층 3). **선택 기능이다.**

- `hook install --claude-code` / `--codex` / `--agent-tools`
- `internal session-event` (훅 전용, help 비노출)
- stdin JSON 파싱

### 범위 밖

서브에이전트 이벤트. `SubagentStart`/`Stop` 에는 훅을 걸지 않는다 (§3.8).

### 테스트

```
[T11.1]  SessionStart 훅 → 세션 등록, hook_pid 기록
[T11.2]  SessionEnd 훅 → 해당 세션의 claimed 만 open 으로
[T11.2b] SessionEnd 훅 → working 은 그대로 (D15 회귀)
[T11.3]  stdin JSON 에서 session_id / cwd 정확히 파싱
[T11.4]  알 수 없는 stdin → 조용히 exit 0 (세션을 막지 않음)
[T11.5]  빈 stdin → exit 0
[T11.6]  SessionStart 가 ref 쓰기 1회로 완료 (속도)
[T11.7]  훅 미설치 상태에서 전체 워크플로우 정상 (선택 기능 확인)
[T11.8]  SessionEnd 가 오지 않아도 잠금 확인으로 회수됨
[T11.9]  claude-code 와 codex 에 같은 스크립트가 등록됨
[T11.10] 기존 .claude/settings.json 이 있으면 병합, 충돌 시 중단
[T11.11] SubagentStart/Stop 에 훅이 등록되지 않음
[T11.12] hook status 가 두 종류 훅을 구분해 표시
```

**T11.8 이 이 단계의 핵심이다.** 훅은 최적화이지 정합성의 근거가 아니다. `SessionEnd` 를 강제로 건너뛰어도 층 1이 처리해야 한다.

**T11.4 와 T11.5 도 중요하다.** 훅이 실패하면 도구 세션 자체가 막히거나 지연될 수 있다. 알 수 없는 입력은 조용히 통과시킨다.

### 엣지 케이스

| 상황 | 요구 동작 |
|---|---|
| stdin JSON 스키마가 바뀜 | 아는 필드만 읽고 나머지 무시 |
| `session_id` 없음 | 폴백 세션 사용, exit 0 |
| 훅이 worktree 밖에서 실행 | `cwd` 로 판단, 무관하면 exit 0 |
| SessionStart 없이 SessionEnd 만 옴 | 무해하게 처리 |
| SessionEnd 가 두 번 옴 | 멱등 |
| 훅 실행 중 저장소 없음 | exit 0 |
| Codex 훅이 신뢰 검토 대기 중 | install 이 안내, 동작은 층 1로 |
| Windows (Codex 훅 미지원) | install 이 명확히 안내 |
| 훅 스크립트 실행 권한 없음 | install 시 chmod |

### Exit criteria

- T11.1~T11.12 통과
- **훅을 전부 제거해도 전체 E2E 통과** (선택 기능 확인)
- `SessionStart` 훅 실행 시간이 측정되고 상한 이내

---

## 전 단계 공통 회귀 스위트

각 단계 완료 시마다 전부 재실행한다.

```
[R1] 모든 명령 실행 후 git status 가 clean
[R2] 모든 명령 실행 후 git log <branch> 에 변화 없음
[R3] 소스 commit 의 author 가 에이전트 ID 로 오염되지 않음
[R4] worktree 3개에서 교차 가시성
[R5] SHA-1 / SHA-256 양쪽에서 전체 통과
[R6] files backend / reftable backend 양쪽에서 전체 통과
[R7] pack-refs 실행 후에도 전체 통과
[R8] 동시성 테스트를 100회 반복해 flake 0
[R9] testdata/fuzz/ 의 전체 corpus 가 통과
[R10] FaultyRefStore 의 기록된 실패 seed 가 전부 통과
[R11] 설계 문서가 참조하는 D# 가 전부 decisions.md 에 존재
```

**R5~R7 을 CI 매트릭스로 돌린다.** 이 셋은 개발 환경에서 거의 발현되지 않다가 사용자 환경에서 터지는 종류다.

---

## 단계별 소요 추정

정확한 추정은 어렵지만 상대 비중은 다음과 같다.

| 단계 | 상대 비중 | 비고 |
|---|---|---|
| 0 | 크다 | 테스트 인프라(프로세스 spawn) 구축 포함 |
| 1 | 중간 | |
| 2 | **가장 크다** | 여기에 시간을 쓰는 것이 맞다 |
| 3 | 작다 | 2가 끝나면 기계적 |
| 4 | 중간 | |
| 5 | 중간 | |
| 6 | 작다 | |
| 7 | 작다 | |
| 8 | 중간 | fsck 항목이 많다 |
| 9 | 중간 | |
| 10 | 작다 | 스크립트는 이미 있음 |
| 11 | 작다 | 두 도구가 대칭이라 스크립트 하나 |
| 12 | 작다 | 이슈 인프라 재사용. 상태 머신 없음 |

단계 2 가 전체의 상당 부분을 차지하는 것이 **정상이다.** 여기서 아낀 시간은 나중에 재현 불가능한 동시성 버그로 돌아온다.
