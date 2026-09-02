# paperwork 설계 결정 기록

설계 문서에서 "왜 X 가 아닌가" 를 분리한 것이다. 설계 문서는 현재 상태만 서술하고, 근거가 필요하면 여기의 번호를 참조한다.

형식: 맥락 → 검토한 선택지 → 결정 → 결과. 뒤집힌 결정도 남긴다 — 같은 길을 다시 가지 않기 위해서다.

---

## D1. 저장 위치: custom ref namespace

**맥락** — 여러 worktree 가 즉시 공유하고, 소스 히스토리와 섞이지 않는 저장소가 필요했다.

**검토** — tracked `TASKS.json`(브랜치마다 갈림), 전용 브랜치(checkout·conflict), `git notes`(단일 ref 경쟁, commit 종속), SQLite(Git 외부 상태).

**결정** — `refs/paperwork/*`. 모든 worktree 가 `$GIT_COMMON_DIR` 를 공유하므로 fetch 없이 즉시 보이고, 브랜치와 직교한다.

**결과** — `git log --all` 에 이슈 커밋이 섞인다. `--exclude` 로 완화만 가능.

---

## D2. 이슈당 ref 하나, 상태는 commit chain

**맥락** — 여러 에이전트가 서로 다른 이슈를 동시에 갱신한다.

**결정** — `refs/paperwork/issues/<id>` 가 commit 을 가리키고, parent 체인이 이력이다. 이슈마다 ref 가 다르므로 경쟁이 분산된다.

**결과** — `git log <ref>` 가 곧 `history`. 별도 이력 구조가 없다.

---

## D3. CAS 는 `git update-ref <ref> <new> <old>`

**맥락** — 프로세스 간 원자적 갱신이 필요하다.

**결정** — git 의 CAS 를 그대로 쓴다. lock 실패(`cannot lock ref`)와 CAS 실패(`but expected`)를 구분해 전자는 재시도, 후자는 재판단.

**결과** — 문자열 매칭이 취약하다. 분류 실패 시 기본값을 일반 오류로 두고, 잘못된 재시도를 막는다.

---

## D4. Agent-Session trailer 로 OID 충돌 방지

**맥락** — content-addressed 라 같은 parent·tree·author·시각이면 OID 가 같아지고, 두 CAS 가 모두 "성공" 한다.

**결정** — commit message trailer 에 세션 고유값을 넣어 content 를 갈라놓는다.

**결과** — 회귀 테스트: trailer 를 빼면 반드시 실패해야 한다.

---

## D5. trailer 비정규화

**맥락** — 목록 조회가 이슈 수 × 3 object 를 읽고 있었다.

**결정** — 상태 필드를 commit message trailer 에 복제해 `for-each-ref --format` 한 번으로 목록을 만든다. `issue.json` 이 진실, trailer 는 인덱스.

**결과** — `fsck` 가 불일치를 검출한다.

---

## D6. plan 은 상태를 갖지 않는다

**맥락** — plan ref 하나에 phase 와 task 진행률을 담고 싶어진다.

**검토** — 그렇게 하면 모든 task 상태 변경이 같은 plan ref 를 쓰고, D2 의 경쟁 분산이 무너진다. D1 에서 `git notes` 를 기각한 이유와 같다.

**결정** — plan 은 구조만. 진행률·현재 phase 는 task 를 읽어 계산한다. task 가 plan 을 가리키고 plan 은 task 목록을 갖지 않는다.

**결과** — 회귀 테스트: plan 의 여러 task 를 동시 claim 해도 plan ref 쓰기 0회.

---

## D7. go-git 하이브리드

**맥락** — Go 로 구현하며 go-git 을 쓴다.

**검토** — go-git 의 `CheckAndSetReference` 는 read-then-write 이며 프로세스 간 원자성이 없다. 훅도 실행하지 않고, 다중 ref 트랜잭션도 없다.

**결정** — 읽기와 객체 생성은 go-git, ref 갱신만 `git` CLI exec. `RefStore` 인터페이스 뒤에 가둔다.

**결과** — 쓰기당 fork 1회. 셸 구현(5회)보다 적다.

---

## D8. 생존 판정 — TTL heartbeat (폐기)

**맥락** — 죽은 에이전트의 claim 을 회수해야 한다.

**결정 (당시)** — 에이전트마다 heartbeat 데몬이 15초마다 lease ref 를 갱신하고 TTL 로 판정.

**폐기 이유** — 데몬 프로세스가 하나씩 더 필요하고, 감지가 TTL 만큼 지연되며, 오래 걸리는 정상 작업이 오탐 회수된다. 유휴 구간(아무도 `next` 를 안 부르는 동안)이 사각지대가 되어 데몬에 reap 까지 얹어야 했다.

**대체** — D9 → D10 → D11.

---

## D9. 생존 판정 — 세션 수명 동안 flock (폐기)

**결정 (당시)** — `session begin` 이 flock 을 잡고 프로세스 수명 동안 유지. OS 가 종료 시 해제하므로 즉시 감지.

**폐기 이유** — (1) 코딩 에이전트 도구 안에서는 매 명령이 새 프로세스라 긴 잠금을 유지할 주체가 없다. (2) 살아있지만 멈춘 프로세스가 잠금을 영구히 쥔다. (3) `session begin` 을 잊으면 조용히 틀린다.

**남긴 것** — flock 은 잠금 파일 read-modify-write 보호용으로만 쓴다.

---

## D10. 생존 판정 — 프로세스 트리 탐색 (기각)

**검토** — 부모를 거슬러 올라가 `claude` / `codex` 이름을 찾는다.

**기각 이유** — 래퍼 스크립트가 같은 이름일 수 있고 실제 프로세스명이 `node` 일 수 있다. `comm` 은 15자에서 잘린다. tmux·컨테이너·sudo 를 거치면 트리가 다르다. 무엇보다 **이름은 "살아있는가" 판정에 기여하지 않는다.** 실패하면 조용히 틀린다.

**결과** — 회귀 테스트: 코드에 프로세스 이름 조회가 존재하지 않아야 한다.

---

## D11. 생존 판정 — hook_pid 또는 last_activity (현재)

**결정** — 훅이 있으면 `SessionStart` 훅이 기록한 `hook_pid` (훅의 부모는 구조적으로 도구 프로세스). 없으면 `last_activity` 임계값.

**임계값 8시간** — 초기 30분은 폴링 워커 전제였다. 배정 모델(D14)에서는 `start` 후 몇 시간 동안 CLI 호출이 없을 수 있어 산 작업이 회수된다. "죽은 작업 방치" 보다 "산 작업 회수" 가 훨씬 나쁜 오류이므로 관대하게 잡는다.

**결과** — 훅 없는 환경에서 자동 회수는 사실상 하루 단위다. `doctor` 가 WARN 하고, 정리는 오케스트레이터(D14)와 사람의 몫이 된다.

---

## D12. 세션 명령 없음

**맥락** — `session begin/end/exec/status` 가 있었다.

**검토** — `begin` 은 첫 명령이 대신한다. `end` 는 생략해도 무해. `status` 는 `doctor` 와 중복. `exec` 만 고유했으나 D9 의 이유로 오히려 위험하다.

**결정** — 전부 제거. 세션은 부수적으로 발생하는 사실이지 관리 대상이 아니다. 훅용 진입점은 `internal session-event` 로 숨긴다.

**결과** — 회귀 테스트: `--help` 에 session 계열 명령이 없어야 한다.

---

## D13. lease ref 없음

**맥락** — `refs/paperwork/agents/<name>` 을 "다른 worktree 에서 현황을 보기 위해" 두었다.

**검토** — 잠금 파일이 `$GIT_COMMON_DIR/paperwork/locks/` 에 있어 이미 모든 worktree 가 읽는다. 같은 정보를 두 곳에 쓰고 있었고, "런타임 생존 신호는 데이터가 아니다" 원칙과 어긋난다.

**결정** — ref 를 없애고 `agents` 명령이 잠금 파일을 직접 읽는다.

**결과** — ref 쓰기 하나 감소. v2(여러 머신)에서 필요해지면 그때 추가한다.

---

## D14. 배정은 오케스트레이터가

**맥락** — A 가 이슈를 만들어 B 에게 넘긴다. `assign` 명령과 `assigned` 상태를 검토했다.

**검토** — 배정을 ref 에 기록하면 메시지와 상태가 두 곳에 생긴다. A 가 C 에게 다시 보내면 `assigned=B` 는 거짓이 되고, `unassign` 을 기억해야 한다. `assigned` 는 소유자가 없는 상태라 회수 규칙에 예외가 필요하다.

**결정** — `assign` 없음. 메시지는 오케스트레이터 도구로 전달하고, 이슈는 받는 쪽이 `claim` 할 때까지 `open`. "아무도 받지 않았다" 를 정확히 표현한다.

**결과** — 명령이 늘지 않았다. 회귀 테스트: B 가 무응답이어도 정리 없이 C 가 곧바로 claim 가능.

---

## D15. SessionEnd 는 claimed 만 반납

**맥락** — 훅의 `SessionEnd` 가 세션의 claim 을 전부 반납하고 있었다.

**검토** — 사용자가 도구를 닫았다 다시 열어 같은 작업을 잇는 것은 흔하다. `working` 에는 worktree 의 미커밋 변경이 있다. 반납하면 다른 에이전트가 다른 worktree 에서 처음부터 한다.

**결정** — `SessionEnd` 는 `claimed`(시작 전)만 `open` 으로. `working` 은 두고 D11 에 맡긴다. 같은 worktree 에서 다시 열면 이어진다.

---

## D16. start 가 open 을 허용

**맥락** — `claimed` 상태는 D8 시절 "예약만 하고 안 시작한 것" 을 짧은 TTL 로 회수하려고 만들었다. 지금은 에이전트가 `claim` 직후 `start` 를 부른다.

**결정** — `start` 를 `open` 에서도 허용한다 (claim + start 를 한 CAS 로). `claim` 은 "예약만" 용도로 남긴다.

**결과** — 에이전트 단계가 `show → start → done` 셋이 된다.

---

## D17. 결정 기록을 도구 안에

**맥락** — 에이전트가 `feature/a` 에서 내린 결정을 `feature/b` 의 에이전트가 merge 전까지 모른다. tracked 파일은 브랜치마다 갈린다 (D1 과 같은 문제).

**검토** — 이슈에 `--label decision` 으로 대신하기. 동작은 하지만 불변성·supersede 연결·전용 export 가 없다. 결정은 상태가 전이하지 않으므로 이슈와 성질이 다르다.

**결정** — `refs/paperwork/decisions/` 에 불변 ADR. `edit` 없음, `--supersedes` 로 대체. 엣지는 결정 → 이슈 한 방향. 상태 머신 없음. `export --decisions` 로 tracked 파일을 파생.

**결과** — `show <issue>` 가 연결된 결정을 표시해 세션 간 논의 반복을 막는다. 시금석: 이 파일의 D1~D17 을 `decide` 로 다시 기록할 수 있어야 한다.

---

## D18. 백로그는 상태가 아니라 priority none

**맥락** — "당분간 안 할 것" 을 표현할 방법이 필요했다.

**검토** — `backlog` 상태 추가. 전이 표 +2행, gate 계산 예외, 회수 규칙 예외, fsck 항목이 따라온다.

**결정** — `priority` 에 `none` 을 추가하고 `next` 가 제외한다. 상태는 `open` 그대로. "당분간 안 함" 은 작업의 속성이지 상태가 아니다 — D6, D14 와 같은 구분이다.

**결과** — `next` 필터 한 줄. 전이·gate·회수 규칙에 예외가 없다.
