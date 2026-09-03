# ppwk E2E 테스트 명세

실제 저장소·실제 프로세스·실제 `git` 바이너리 기준

버전 5.2 / 2026-09-02

설계: `ppwk-design.md` v5.2 · 구현: `ppwk-implementation.md` v5.2 · 기능: `ppwk-features.md` v5.2

---

## 0. 이 문서의 범위

### 0.1 E2E 의 정의

구현 문서의 테스트(T*, F*, D*)는 대부분 패키지 단위다. 이 문서는 그 위층을 다룬다.

| | 단위/통합 테스트 | E2E (이 문서) |
|---|---|---|
| 대상 | Go 함수, 패키지 | 빌드된 `ppwk` 바이너리 |
| 저장소 | 임시, 최소 | 실제 worktree 다중 구성 |
| 동시성 | goroutine 또는 소수 프로세스 | 실제 에이전트 프로세스 |
| 검증 | 반환값 | stdout/stderr/exit code + `git` 로 직접 확인 |
| 시간 | 초 | 분~시간 |

**E2E 는 반드시 빌드된 바이너리를 `exec` 해야 한다.** Go 함수를 직접 호출하면 종료 코드, 출력 형식, 플래그 파싱, 프로세스 경계가 검증되지 않는다. 이 시스템의 버그는 대부분 프로세스 경계에서 나온다.

### 0.2 검증의 이중화

각 시나리오는 두 층에서 확인한다.

1. **CLI 층** — `ppwk list --json` 등의 출력
2. **git 층** — `git for-each-ref`, `git cat-file`, `git log` 로 저장소 상태를 직접 확인

CLI 가 거짓말을 해도 git 층이 잡는다. 특히 오염 검사(§7)는 반드시 git 층에서 한다.

---

## 1. 테스트 하네스

### 1.1 픽스처

```
newFixture(t) *Fixture
    임시 디렉터리에 저장소 생성
    초기 commit 1개 (빈 저장소는 HEAD 가 없어 동작이 다름)
    ppwk init 실행
    t.Cleanup 으로 정리

f.AddWorktree(name, branch) *Worktree
    git worktree add 로 linked worktree 추가

f.Run(args...) Result
    빌드된 바이너리 실행. Result{Stdout, Stderr, ExitCode}

f.RunIn(wt, args...) Result
f.RunJSON(args...) (any, error)

f.Git(args...) string
    검증용 raw git 실행

f.Agent(name, wt) *Agent
    독립 프로세스로 에이전트 루프를 띄운 핸들
    별도 초기화 없이 첫 명령이 세션을 등록한다
    잠금은 이 프로세스 수명에 묶인다
    a.Kill()      SIGKILL — 정리 없이 즉사. OS 가 잠금 해제
    a.Stop()      SIGTERM — 정상 종료
    a.HoldsLock() 잠금 보유 여부 직접 확인
```

### 1.2 결정성 확보

E2E 는 flake 가 나기 쉽다. 다음을 통제한다.

| 요소 | 방법 |
|---|---|
| 시간 | 훅 경로는 대기가 없다. last_activity 경로는 PPWK_ACTIVITY_TTL 로 축소 |
| 랜덤 | 세션 nonce 는 랜덤 유지 (§4.3 목적) — 로그에 기록 |
| 순서 | 동시성 테스트는 결과의 **집합**을 검증. 순서 아님 |
| 대기 | `sleep` 금지. 조건 폴링 헬퍼 `waitFor(cond, timeout)` |
| 경로 | 절대 경로. 심볼릭 링크 있는 `/tmp` 주의 (macOS) |

**`sleep` 을 쓰면 느리거나 flake 하거나 둘 다다.** 항상 조건을 폴링한다.

### 1.3 매트릭스

전체 E2E 를 다음 조합에서 돌린다.

```
object format : sha1, sha256
ref backend   : files, reftable
OS            : linux, macos
git version   : 2.28 (최소), 최신 stable
```

CI 는 `linux × sha1 × files × 최신` 을 모든 PR 에서, 나머지는 야간에 돌린다. 전 조합은 릴리스 전 필수다.

---

## 2. 기본 워크플로우

### E2E-1: 단일 에이전트 전체 수명주기

```
init → add → list → claim → start → done → archive 확인
```

검증:

```
CLI: 각 단계에서 list/show 의 status 가 기대값
git: refs/ppwk/issues/T001 이 done 후 사라짐
git: refs/ppwk/archive/T001 이 존재
git: git log <ref> 가 create/claim/start/done 4개 commit
git: 각 commit 의 author 가 agent id
git: commit message trailer 의 Status 가 issue.json 과 일치
```

### E2E-1b: 배정 흐름 (오케스트레이터 모델)

```
worktree A 에서 add "작업1" → T001
worktree B 에서 show T001 → start (claim 겸함) → done
```

검증:

```
A 가 add 한 직후 T001 이 open, owner 없음
B 가 claim 한 후 owner 가 B
A 는 T001 에 대해 아무 소유권도 갖지 않음
git: create/start/done 3개 commit (start 가 claim 을 겸함)
```

**A 가 B 몫으로 예약하는 경로가 없어야 한다** (§8.0).

### E2E-1c: 배정 대상 변경

```
A 가 add → T001
B 에게 전달했으나 B 가 응답 없음 (아무 명령도 실행 안 함)
C 가 claim
```

검증:

```
B 가 응답하지 않는 동안 T001 은 open 유지
정리 명령 없이 C 가 곧바로 claim 가능
B 의 흔적이 ref 에 남지 않음
```

**배정 상태를 ref 에 기록하면 여기서 정리 책임이 생긴다.**

### E2E-2: 3 worktree 교차 가시성

```
worktree A 에서 add
worktree B 에서 list  → 즉시 보임 (fetch 없이)
worktree C 에서 claim
worktree A 에서 show  → owner 가 C
```

검증:

```
각 worktree 의 git rev-parse --git-dir 이 서로 다름
각 worktree 의 git rev-parse --git-common-dir 이 동일
어느 worktree 에서 봐도 ref OID 가 동일
```

**이 테스트가 설계 §3.1 의 전제를 직접 검증한다.** commondir 해석이 깨지면 반드시 실패해야 한다.

### E2E-3: 브랜치 독립성

```
worktree A: feature/foo 체크아웃
worktree B: feature/bar 체크아웃
양쪽에서 이슈 생성·상태 변경
브랜치 전환 (git checkout main)
```

검증:

```
브랜치 전환 후에도 이슈 목록 동일
git status 가 clean (워킹 디렉터리에 파일 안 생김)
git log feature/foo 에 이슈 commit 없음
git diff main feature/foo 에 이슈 관련 변화 없음
```

---

## 3. 동시성

### E2E-4: 동시 claim 경쟁

```
이슈 1개, 에이전트 16개를 동시에 spawn
전원이 ppwk claim T001 실행
```

검증:

```
exit 0 인 프로세스가 정확히 1개
나머지 15개는 exit 4 (CAS 경쟁) 또는 exit 3 (이미 claimed)
git: refs/.../T001 의 commit 이 정확히 1개 추가됨
git: owner 가 성공한 에이전트와 일치
어떤 프로세스도 panic 하지 않음
```

**반복 100회.** flake 0 이어야 한다.

### E2E-5: 동시 next --claim 분산

```
이슈 20개 (전부 open, 의존성 없음)
에이전트 8개가 동시에 next --claim 을 각각 3회
```

검증:

```
배정된 이슈에 중복 0건
총 배정 수 ≤ 20
각 이슈의 owner 가 정확히 1명
git: 각 이슈의 claim commit 이 1개씩
```

### E2E-6: 경쟁 후 다음 후보로 이동

```
이슈 2개, 에이전트 2개
둘 다 동시에 next --claim
```

검증:

```
두 에이전트가 서로 다른 이슈를 획득
둘 다 exit 0 (아무도 빈손이 아님)
```

**같은 이슈를 재시도하는 구현이면 한쪽이 빈손이 되어 실패한다** (§7.2).

### E2E-7: OID 충돌 회귀

```
동일 tree/parent 를 갖는 상태에서
두 프로세스가 같은 초에 동시 claim
```

검증:

```
두 commit 의 OID 가 다름
정확히 1개만 CAS 성공
```

**`Agent-Session` trailer 를 제거한 빌드로 돌리면 이 테스트가 실패해야 한다.** 실패하지 않으면 테스트가 잘못됐다.

---

## 4. 장애와 복구

### E2E-8: 에이전트 SIGKILL 후 즉시 회수

```
agent-b 가 T003 을 claim + start
agent-b 를 SIGKILL (정리 없이)
다른 에이전트가 즉시 next 호출
```

검증:

```
대기 시간 없이 T003 이 open 으로 복귀
git: 회수 commit 이 chain 에 추가됨 (이력 보존)
git: 회수 commit 의 author 가 회수를 수행한 에이전트
T003 이 곧바로 재배정 가능
잠금 파일이 OS 에 의해 해제되었음 (직접 flock 시도로 확인)
```

**TTL 대기가 없다.** 이전 TTL 설계에서는 최대 10분을 기다려야 했다.

### E2E-9: 유휴 상태에서도 손실 없음

```
에이전트 3개 전원이 working 상태로 장시간 작업
아무도 next 를 호출하지 않음
그 중 하나를 SIGKILL
장시간 방치
이후 누군가 next 호출
```

검증:

```
next 호출 시점에 즉시 회수됨
방치 시간과 무관하게 정확히 회수
```

**게으른 확인의 정당성을 검증한다.** 아무도 원하지 않는 동안 방치되어도 손해가 없고, 필요해진 순간 확인된다.

### E2E-10: 세션 재시작

```
agent-b (session S1) 가 T003 claim
agent-b 를 SIGKILL
같은 이름 agent-b (session S2) 로 재시작 (잠금 재획득)
다른 에이전트가 next 호출
```

검증:

```
S1 의 claim 이 회수됨
S2 는 정상적으로 잠금 보유 (alive)
agents 명령에서 agent-b 가 alive 이지만 T003 은 회수됨
```

**잠금만 보고 session 을 비교하지 않는 구현은 실패한다.** 새 프로세스가 잠금을 잡았으므로 "생존" 으로 보이지만, 이슈의 session 은 죽은 쪽이다 (§3.6).

### E2E-10b: worktree 배타 (암묵 세션)

```
worktree A 에서 agent-a1 이 claim (초기화 명령 없이)
같은 worktree A 에서 agent-a2 가 claim 시도
```

검증:

```
두 번째가 거부됨, 명확한 오류 메시지
agent-a1 은 영향 없이 계속 동작
agent-a1 종료 후 agent-a2 가 정상 동작
--allow-shared-worktree 로는 통과
```

**초기화 명령 없이도 배타가 강제되어야 한다.**

### E2E-10d: 암묵 세션 자동 등록

```
아무 초기화 없이 곧바로 next --claim
```

검증:

```
세션이 자동 등록됨 (잠금 파일 생성)
agents 에 나타남
doctor 가 세션 등록 상태를 표시
이후 명령들이 같은 세션으로 묶임
```

### E2E-10e: 등록 경쟁

```
같은 worktree 에서 N=16 프로세스가 동시에 첫 claim 시도
```

검증:

```
정확히 1개만 worktree 확보
나머지는 명확한 거부 오류
잠금 파일이 손상되지 않음
```

**read-modify-write 가 원자적이지 않으면 실패한다** (§3.6).

### E2E-10h: 장시간 작업 중 회수 방지

```
훅 없음. B 가 start T001 후 CLI 호출 없이 7시간 경과 (시계 조작)
다른 에이전트가 next 호출
```

검증:

```
T001 이 회수되지 않음 (working, owner=B 유지)
9시간 경과 후에는 회수됨
```

**배정 모델의 핵심 회귀다** (D11). 임계값을 짧게 바꾸면 산 작업이 회수된다.

### E2E-22g: SessionEnd 가 working 을 보존

```
훅 설치. B 가 T001 start, T002 claim (시작 안 함)
SessionEnd 발생
```

검증:

```
T002 → open (claimed 였으므로 반납)
T001 → working 유지 (미커밋 작업 보호)
```

### E2E-10g: 생존 판정 2수준

같은 시나리오를 세 구성에서 돌린다.

```
구성 A: 아무 통합 없음   → last_activity
구성 B: 도구 훅 설치     → hook_pid
```

각 구성에서 에이전트를 SIGKILL 하고 회수를 확인한다.

검증:

```
A: PPWK_ACTIVITY_TTL(테스트에서 축소) 경과 후 회수
B: 즉시 회수
두 구성 모두 최종 상태가 동일
doctor 가 각 구성의 판정 근거를 정확히 표시
```

**감지 속도는 통합 수준에 비례하되, 정합성은 두 구성에서 동일해야 한다** (§3.6).

### E2E-10f: 세션 명령 부재

```
--help 및 하위 명령 목록 확인
```

검증:

```
session begin / end / exec / status 가 존재하지 않음
internal 은 help 에 노출되지 않음
초기화 없이 전체 워크플로우가 동작
```

### E2E-10c: 조회는 잠금과 무관

```
agent-a1 이 worktree A 에서 session 보유 중
같은 worktree 에서 list / show / watch / export 를 동시에 여러 개 실행
```

검증:

```
전부 정상 동작
agent-a1 의 잠금에 영향 없음
```

### E2E-11: 쓰기 중 SIGKILL

```
claim 수행 중 무작위 시점에 SIGKILL (여러 시점 반복)
```

검증:

```
저장소가 항상 정합 상태
이슈가 중간 상태로 남지 않음 (claim 되었거나 안 되었거나)
git fsck 통과
dangling commit 은 허용 (gc 대상)
```

객체를 먼저 만들고 ref 를 나중에 바꾸므로 부분 상태가 없어야 한다 (§4.1).

### E2E-12: archive 이동 중 SIGKILL

```
done 수행 중 SIGKILL, 여러 시점 반복
```

검증:

```
issues/ 와 archive/ 양쪽에 동시 존재하지 않음
양쪽에서 동시에 사라지지 않음
정확히 한쪽에만 존재
```

**개별 update-ref 2회로 구현하면 실패한다** (§4.4).

### E2E-13: stale lock

```
.lock 파일을 수동 생성 후 CAS 시도
```

검증:

```
명확한 오류 메시지
도구가 .lock 을 자동 삭제하지 않음
doctor 가 stale lock 을 보고
fsck 가 경고
```

---

## 5. plan 과 phase

### E2E-14: phase 진행

```
plan P01, phase p1(2 task) / p2(2 task) / p3(manual, 2 task)
에이전트들이 계속 next --claim
```

검증:

```
p1 의 task 가 먼저 전부 배정됨
p1 완료 전까지 p2 의 task 는 후보에 안 나옴
p1 완료 후 p2 의 task 등장
p2 완료 후에도 p3 는 막힘 (manual)
plan advance P01 p3 후 p3 등장
각 시점의 plan show 진행률이 정확
```

### E2E-15: plan 경쟁 분산

```
plan 1개, 같은 phase 에 task 10개
에이전트 8개가 동시에 next --claim
```

검증:

```
중복 배정 0건
git: refs/ppwk/plans/P01 의 OID 가 시작과 끝이 동일
```

**plan ref 쓰기 0회가 핵심이다** (§3.7.1). plan 에 진행률 필드를 추가하면 반드시 실패한다.

### E2E-16: seq 우선 정렬

```
같은 phase 에
  seq 10, priority low
  seq 20, priority high
```

검증:

```
seq 10 이 먼저 배정됨
```

### E2E-17: 의존성과 archive

```
T001 (독립), T002 (depends_on T001)
T001 을 done → archive 로 이동
```

검증:

```
T002 가 후보에 등장
issues/ 만 조회하는 구현이면 T002 가 영원히 안 나옴
```

---

## 5.5 결정 기록

### E2E-17b: 브랜치 간 결정 공유

```
worktree A (feature/a): decide "저장소는 SQLite" --issue T001 → D001
worktree B (feature/b): decisions
```

검증:

```
B 에서 D001 이 즉시 보임 (merge 없이)
B 에서 show T001 에 D001 이 표시됨
git log feature/a, feature/b 어느 쪽에도 결정 커밋 없음
```

**결정을 tracked 파일로 두면 이 시나리오가 실패한다** (§3.9).

### E2E-17c: supersede 체인

```
D001 결정 → D002 --supersedes D001 → D003 --supersedes D002
```

검증:

```
decisions 기본 목록에 D003 만
decisions --all 에 셋 다
decisions history D003 이 체인 전체
D001 의 ref OID 가 처음과 동일 (불변)
```

### E2E-17d: export 후 커밋

```
export --decisions -o docs/decisions/
git add docs/decisions && git commit
```

검증:

```
결정당 파일 하나
파일 헤더에 파생물 경고
커밋 후 git status clean
ref 와 파일 내용이 일치
```

## 6. 알림

### E2E-18: hook 알림

```
watch --hook 시작
다른 worktree 에서 claim
```

검증:

```
이벤트가 1초 이내 수신
old/new OID 가 실제 ref 변경과 일치
kind 가 정확
```

### E2E-19: hook 이 쓰기를 막지 않음

```
hook 설치, listener 없음
claim 실행
```

검증:

```
claim 이 정상 완료 (블로킹 없음)
실행 시간이 EMIT_TIMEOUT 이내
```

```
listener 가 소켓을 열고 응답하지 않음 (accept 후 read 안 함)
claim 실행
```

검증:

```
timeout 후 claim 완료
```

**알림 실패가 쓰기 경로를 막으면 안 된다** (§1.2 설계 원칙).

### E2E-20: hook 없이 동작

```
hook 미설치 상태로 전체 워크플로우 (E2E-1, E2E-5)
watch (polling) 로 변경 감지
```

검증:

```
모든 기능 정상
polling 이 변경을 놓치지 않음
```

### E2E-21: 무관한 ref 는 무시

```
hook 설치 상태에서
일반 git commit, git branch, git tag, git fetch 수행
```

검증:

```
소켓에 이벤트 없음
git 명령의 실행 시간이 hook 미설치 대비 유의미하게 늘지 않음
```

### E2E-22: pack-refs 후 감지

```
watch 실행 중 git pack-refs --all
이후 이슈 상태 변경
```

검증:

```
변경이 정상 감지됨
```

**파일 mtime/inotify 기반 구현이면 실패한다** (§6.2).

---

## 6.5 도구 통합

도구 훅은 선택 기능이므로, **없을 때도 전부 동작하는지**가 주된 검증 대상이다.

### E2E-22b: 환경변수 감지 (훅 없이)

```
CLAUDE_CODE_SESSION_ID / CLAUDECODE 를 설정한 환경에서 전체 워크플로우
```

검증:

```
agent-id 가 claude-code:<worktree>
같은 세션 ID 로 실행된 명령들이 같은 세션으로 묶임
list --mine 이 해당 이슈만 반환
doctor 가 감지 근거를 표시
```

### E2E-22c: 감지 실패 폴백

```
관련 환경변수를 전부 제거한 환경에서 전체 워크플로우
```

검증:

```
<hostname>:<worktree> 로 폴백
모든 기능 정상 동작
doctor 가 폴백 사용을 정보로 표시 (FAIL 아님)
```

**환경변수 이름은 도구 버전에 따라 바뀔 수 있다.** 이 시나리오가 그 변화에 대한 내성을 검증한다.

### E2E-22d: SessionEnd 정상 경로

```
SessionStart 훅 → 이슈 여러 개 claim → SessionEnd 훅
```

검증:

```
SessionEnd 시점에 claim 전부 반납됨
다른 에이전트가 즉시 가져갈 수 있음
대기 시간 없음
```

### E2E-22e: SessionEnd 누락 시 폴백

```
SessionStart 훅 → claim → SessionEnd 를 건너뛰고 프로세스 SIGKILL
```

검증:

```
다음 next 호출 시 잠금 확인으로 회수
E2E-8 과 동일한 결과
```

**훅이 정합성의 근거가 아님을 검증한다.** 층 1 단독으로 처리되어야 한다.

### E2E-22f: 훅이 세션을 막지 않음

```
훅에 빈 stdin / 깨진 JSON / 저장소 밖 cwd 를 전달
```

검증:

```
전부 exit 0
도구 세션이 정상 시작
SessionStart 실행 시간이 상한 이내
```

---

## 7. 오염 검사

모든 시나리오 종료 시 공통으로 실행한다.

### E2E-23: 소스 히스토리 무오염

```
전체 워크플로우 실행 후
```

검증:

```
git status --porcelain 이 빈 출력
    (단 init 직후의 AGENTS.md 와 docs/ppwk/*.md 는 예외 —
     untracked 로 나타난다. E2E 픽스처는 init 후 이들을 커밋하고
     베이스라인을 잡는다)
git log --format=%an <branch> 에 에이전트 ID 없음
git log <branch> 의 commit 수가 시작과 동일
git diff <시작 시점 commit> HEAD 가 빈 출력
워킹 디렉터리에 새 파일 없음
.gitignore 변경 없음
```

### E2E-24: 원격 미노출

```
bare 원격 생성, git push (기본 refspec)
```

검증:

```
원격에 refs/ppwk/* 없음
git clone 한 사본에도 없음
```

```
git push --mirror 수행
```

검증:

```
원격에 refs/ppwk/* 존재 (경고 문구의 사실 확인)
```

두 번째는 "안 되어야 한다" 가 아니라 **문서의 경고가 사실인지** 확인하는 것이다 (§9.1).

---

## 8. 환경 호환성

### E2E-25: SHA-256 저장소

```
git init --object-format=sha256 로 생성 후 전체 워크플로우
```

검증:

```
모든 기능 정상
hook 의 created/deleted 판정 정확 (64자리 zero OID)
```

**40자리 zero OID 만 비교하는 hook 이면 전부 틀린다** (§6.3).

### E2E-26: reftable backend

```
git init --ref-format=reftable 로 생성 후 전체 워크플로우
```

검증:

```
모든 기능 정상
watch 가 변경 감지 (ref 파일이 없는 환경)
```

### E2E-27: core.hooksPath 충돌

```
git config core.hooksPath /some/other/dir 설정 후 init --hooks
```

검증:

```
경고 출력
hook 이 올바른 위치에 설치되거나 설치 거부
doctor 가 충돌 보고
```

### E2E-28: 최소 git 버전

```
git 2.27 환경 (또는 모킹)
```

검증:

```
init 또는 doctor 가 명확히 거부
```

---

## 9. 장기 실행

### E2E-29: 24시간 무중단

완성 판정 기준의 필수 항목이다 (기능 명세 §10).

```
worktree 3개, 에이전트 3개
이슈를 지속 생성하는 producer 1개
24시간 연속 실행
```

검증 (주기적):

```
메모리 사용량이 선형 증가하지 않음
파일 디스크립터 누수 없음 (잠금 fd 포함)
에이전트 프로세스 외에 상시 실행 프로세스가 없음
loose ref 개수가 통제됨 (또는 gc 로 관리 가능)
중복 배정 누적 0건
lease 가 드리프트하지 않음
git fsck 통과
```

**단위 테스트로는 안 잡히는 종류가 여기서 나온다.** 특히 세션 잠금 fd 누수와 watch 의 스냅샷 메모리 증가.

### E2E-30: 대량 이슈

```
이슈 10,000개 생성
```

검증:

```
list 응답 시간이 실용 범위
next 응답 시간이 실용 범위
gc --pack-refs 후 성능 개선 확인
디스크 사용량 보고
```

---

## 10. 실행 정책

### 10.1 티어

| 티어 | 시나리오 | 실행 시점 |
|---|---|---|
| smoke | E2E-1, 2, 3 | 모든 커밋 |
| core | 1~24 (17b~17d, 22b~22g 포함) | 모든 PR |
| matrix | 1~28 × 전 조합 | 야간 |
| soak | 29, 30 | 릴리스 전, 주 1회 |

### 10.2 flake 정책

**E2E 의 flake 를 재시도로 덮지 않는다.** 이 시스템의 flake 는 대부분 진짜 동시성 버그다. 실패하면:

1. 재현 정보를 보존 (저장소 tarball, 전 프로세스의 stdout/stderr, 세션 nonce)
2. 원인 규명 전까지 해당 시나리오를 skip 하지 않는다
3. 원인 규명 후 결정적 단위 테스트로 축소해 회귀에 고정

### 10.3 실패 시 수집물

```
저장소 전체 tarball (.git 포함)
git for-each-ref 전체 출력
각 이슈의 git log
모든 프로세스의 stdout/stderr (에이전트별 분리)
각 프로세스의 세션 nonce
타임라인 (각 명령의 시작/종료 시각)
doctor 출력
fsck 출력
```

세션 nonce 가 특히 중요하다. 어느 프로세스가 어느 commit 을 만들었는지 추적하는 유일한 단서다.

---

## 11. 커버리지 대조표

설계 문서의 핵심 주장이 E2E 로 검증되는지 확인한다.

| 설계 절 | 주장 | 검증 |
|---|---|---|
| §3.1 | worktree 간 즉시 공유 | E2E-2 |
| §3.5 | 상태 전이 규칙 | E2E-1 |
| §3.6 | 잠금이 생존을 판정 | E2E-8 |
| §3.6 | session 비교 필요성 | E2E-10 |
| §3.6 | worktree 배타 | E2E-10b |
| §3.6 | 암묵 세션 자동 등록 | E2E-10d |
| §3.6 | 등록의 원자성 | E2E-10e |
| §3.6 | 세션 명령 부재 | E2E-10f |
| §3.6 | 판정 2수준, 정합성 동일 | E2E-10g |
| D11 | 장시간 작업 회수 방지 | E2E-10h |
| D15 | SessionEnd 는 claimed 만 | E2E-22g |
| D13 | agents ref 없음 | T4.14 |
| §3.7.1 | plan 은 상태 없음 | E2E-15 |
| §3.9 | 결정은 브랜치 무관 즉시 공유 | E2E-17b |
| §3.9 | 결정 불변 | E2E-17c |
| §4.1 | CAS 원자성 | E2E-4 |
| §4.2 | lock/CAS 실패 구분 | E2E-4, 6 |
| §4.3 | OID 충돌 방지 | E2E-7 |
| §4.4 | 다중 ref 트랜잭션 | E2E-12 |
| §4.5 | 게으른 확인으로 충분 | E2E-9 |
| §4.5 | 데몬 없음 | E2E-29 |
| §6.1 | 알림은 부가 기능 | E2E-19, 20 |
| §6.2 | polling, mtime 금지 | E2E-22 |
| §6.3 | git hook 제약 | E2E-19, 21, 25 |
| §3.8 | 층 2 환경변수 감지 | E2E-22b |
| §3.8 | 감지 실패 폴백 | E2E-22c |
| §3.8 | 훅은 최적화일 뿐 | E2E-22e |
| §7.2 | 경쟁 시 다음 후보로 | E2E-6 |
| §8.0 | 배정은 오케스트레이터 담당 | E2E-1b, 1c |
| §9.1 | 가시성 범위 | E2E-24 |
| §14.4 | commondir 필요 | E2E-2 |

**대조표에 빈칸이 생기면 설계 주장이 검증되지 않은 것이다.** 설계 문서를 수정할 때 이 표를 함께 갱신한다.
