# planr

규격화된 Markdown 계획을 분할 저장하고 진행 상태를 조회하는 Go CLI입니다.

하나의 계획 초안을 작성해 등록하면 `GOALS.md`, `CONTEXT.md`, `PLAN.md`, `phases/*.md`로
나뉘어 저장되고, 이후에는 phase 단위로 상태를 바꾸며 진행 상황을 조회합니다.

- [설치](#설치)
- [빠른 시작](#빠른-시작)
- [명령](#명령) — [plan 등록](#plan-등록), [조회](#조회), [설정 확인과 진단](#설정-확인과-진단), [phase 관리](#phase-관리)
- [설정](#설정) — [plan 저장 위치](#plan-저장-위치), [문서 언어](#문서-언어), [훅](#훅)
- [저장 구조](#저장-구조)
- [초안 규격](#초안-규격) — [frontmatter](#frontmatter), [섹션](#섹션), [PHASE 블록](#phase-블록), [의존성](#의존성)
- [라이프사이클](#라이프사이클)
- [개발](#개발) — 기여자용: [실행기](#실행기), [실행 디렉터리](#실행-디렉터리), [Codex 평가](#codex-평가)

## 요구 사항

planr의 plan 조회·변경 명령은 git 저장소 안에서 동작합니다. 완료 기록을 git note로
남기고, phase 완료 시 작업 트리 상태를 확인하기 때문입니다. `config`와 `doctor`는
설정과 저장소 상태를 진단할 수 있도록 저장소 밖에서도 실행되며, `doctor`가 git
저장소 없음 문제를 non-zero로 보고합니다. 그 밖의 명령을 저장소 밖에서 실행하면
다음과 같이 중단됩니다.

```text
planr requires a git repository, but /tmp/scratch is not inside one; run `git init` at your project root first
```

## 설치

Go가 설치되어 있다면 다음 명령으로 최신 버전을 설치할 수 있습니다.

```sh
go install github.com/ironpark/toolz/planr@latest
```

Go의 바이너리 설치 경로가 `PATH`에 포함되어 있으면 어디서든 `planr` 명령을
사용할 수 있습니다.

## 빠른 시작

```sh
# 1. 초안 생성 — 생성된 파일의 절대 경로를 출력합니다
planr new checkout-v2 --description "checkout flow refresh"

# 2. 초안 파일을 열어 목표·phase·검증 조건을 채웁니다

# 3. plan으로 등록
planr add checkout-v2.md

# 4. 진행 상태 조회
planr overview

# 5. phase 단위로 진행
planr phase start checkout-v2 1
planr phase done checkout-v2 1
```

## 명령

| 명령 | 설명 |
| --- | --- |
| `planr new <plan-name>` | 규격 초안 파일을 생성합니다 |
| `planr add <draft-file>` | 초안을 검증하고 plan 디렉터리로 등록합니다 |
| `planr config` | 실제 적용된 설정 파일과 effective 설정값을 출력합니다 |
| `planr doctor [--fix]` | 설정·저장소·등록된 plan 문서의 정합성을 진단합니다 |
| `planr status [plan-name] [--json]` | 남은 phase와 대기 중인 의존성을 자세히 출력합니다 |
| `planr overview [plan-name] [--json]` | 모든 plan의 진행률을 한 줄씩 요약합니다 |
| `planr phase add <plan-name> <title>` | 열린 plan에 phase를 추가합니다 |
| `planr phase set <plan-name> <number> --status <status>` | phase 상태를 지정한 값으로 변경합니다 |
| `planr phase start\|done\|reset <plan-name> <number>` | phase 상태 변경 단축 명령입니다 |
| `planr notes [plan-name] [--json]` | 커밋에 연결된 완료 기록을 조회합니다 |
| `planr --version` | 설치된 버전을 출력합니다 |

plan 이름을 받는 모든 명령은 `checkout-v2`와 `00-checkout-v2`를 모두 인식합니다.

### plan 등록

```sh
# 규격 초안 생성
planr new platform-refresh --description "platform plan refresh"

# 다른 plan이 끝난 뒤 시작하는 초안 (여러 번 지정 가능)
planr new checkout-v2 --description "checkout flow refresh" \
  --depends-on platform-refresh --depends-on api-foundation#2

# 초안을 채운 뒤 plan으로 등록
planr add platform-refresh.md
```

| 옵션 | 명령 | 설명 |
| --- | --- | --- |
| `--description` | `new` | **필수.** 공백 포함 200자 이내의 짧은 설명을 초안 frontmatter에 기록합니다. plan 이름 뒤의 두 번째 인자로도 지정할 수 있습니다. |
| `--depends-on` | `new` | 선행 plan 또는 phase를 지정합니다. 옵션을 반복하거나 쉼표로 구분하며, 특정 phase는 `plan-name#phase-number` 형식을 씁니다. |
| `--name` | `add` | 초안 frontmatter나 파일명 대신 plan 이름을 지정합니다. |

`--depends-on` 정보는 초안과 등록된 `PLAN.md`의 frontmatter에 그대로 보존됩니다.

### 조회

```sh
# 등록된 진행 중 plan 조회
planr status

# 특정 plan 조회 (완료된 plan 포함)
planr status platform-refresh

# 전체 plan 간단 요약 (완료된 plan 포함)
planr overview
planr overview checkout-v2

# 스크립트/에이전트용 JSON 출력
planr status --json
planr overview --json
planr notes --json
```

**`status`** — `phases/*.md`의 현재 frontmatter를 읽어 남은 phase와 `wait` 목록을
출력합니다. 완료된 plan은 기본적으로 숨기지만, 진행 중 plan의 phase가 의존하는
완료 plan은 함께 표시합니다. plan 이름을 지정해 조회하면 완료 여부와 관계없이
표시합니다.

**`overview`** — 완료된 plan을 포함해 각 plan의 상태와 `완료 phase/전체 phase`
진행률, 다음 미완료 phase, 의존성 대기를 한눈에 보여 줍니다. 자세한 phase 목록과
기본 완료 plan 숨김 규칙이 필요하면 `status`를 사용합니다.

`status`, `overview`, `notes`는 `--json`을 지정하면 텍스트 표 대신 JSON 한 객체를
출력합니다. 텍스트 출력은 기존 형식을 유지하며, JSON의 필드명은 다음처럼
snake_case로 고정됩니다. plan이 없거나 완료 기록이 없을 때도 배열은 `null`이 아닌
빈 배열입니다.

`status --json`의 최상위 필드는 `plans`이며 각 항목은 `name`, `directory`, `status`,
`done_phases`, `total_phases`, `remaining`, `wait`를 가집니다. `remaining` 항목은
`phase_number`, `slug`, `title`, `status`를 가집니다. 텍스트 `status`와 같은 가시성
규칙(완료 plan 숨김)을 적용합니다.

`overview --json`도 `plans`를 사용하며 각 항목은 `name`, `directory`, `status`,
`done_phases`, `total_phases`, `next_phase`, `wait`를 가집니다. `next_phase`는 다음
미완료 phase 제목이며 없으면 빈 문자열입니다.

`notes --json`의 최상위 필드는 `notes`이며 각 항목은 `completed_at`, `plan`, `event`,
`phase`, `commit`, `short_commit`, `subject`를 가집니다. plan 단위 기록의 `phase`는
빈 문자열입니다.

### 설정 확인과 진단

```sh
# 현재 명령에 실제 적용된 설정 확인
planr config

# 읽기 전용 정합성 진단
planr doctor

# PLAN.md 체크리스트와 phase 파일의 불일치까지 복구
planr doctor --fix
```

`config`는 `config_file`에 적용된 `.planr.yaml`의 절대 경로를 출력하고,
`language`, `plans_dirs`, `ignore`, `hooks.before`, `hooks.after`, `hooks.timeout`의
최종값을 보여 줍니다. 설정 파일이 없으면 `config_file: none (using defaults)`라고
표시하고 기본값(`language: en`, `plans_dirs: [plan]`, 10분 훅 타임아웃)을 사용한다고
알립니다. `.planr.yaml`은 현재 디렉터리에서 시작해 git worktree 루트까지 검색합니다.

`doctor`는 `.planr.yaml`의 파싱·검증, `plans_dirs` 경로의 존재, git 저장소 여부,
등록된 plan의 PLAN/phase frontmatter, plan·phase 의존성, `PLAN.md` 체크리스트와
`phases/*.md`의 파일명·제목·상태·링크 일치를 검사합니다. 문제가 하나라도 있으면
non-zero로 종료합니다. 기본 동작은 읽기 전용이며, `--fix`를 지정한 경우에만 유효한
phase 파일을 기준으로 체크리스트를 다시 씁니다. frontmatter 오류나 의존성 오류는
자동으로 변경하지 않습니다.

### phase 관리

```sh
# 상태 변경 단축 명령
planr phase start checkout-v2 1
planr phase done checkout-v2 1
planr phase done checkout-v2 1 --force

# 기타 상태로 직접 변경
planr phase set checkout-v2 1 --status conditional

# 진행 중 plan에 phase 추가
planr phase add checkout-v2 "Cache Warmup" \
  --work "캐시 워밍업 경로를 추가한다." \
  --done-when "캐시 적중률 검증이 통과한다." \
  --depends-on 1
```

| 단축 명령 | 변경되는 상태 |
| --- | --- |
| `phase start` | `in-progress` |
| `phase done` | `done` |
| `phase reset` | `planned` |

`phase add`의 `--work`와 `--done-when`은 필수입니다. 번호는 기존 phase의 가장 큰
번호 다음으로 자동 지정되고, `--depends-on`에는 같은 plan의 기존 phase 번호를 하나
이상 지정할 수 있습니다. 새 phase 문서와 `PLAN.md` 체크리스트가 함께 생성되며,
완료된 plan에는 phase를 추가할 수 없습니다.

#### 진행 전 검사

`phase start`와 `phase done`은 계획된 순서를 지키는지 먼저 확인합니다. `add`가
검증한 의존성 그래프를 실행 시점에도 그대로 적용하는 것입니다.

- **의존성** — 해당 phase의 `depends_on`에 있는 phase, 그리고 `PLAN.md`의
  `depends_on`에 있는 선행 plan이 모두 `done`이어야 합니다. 아직 등록되지 않은
  plan에 의존하고 있으면 `not registered`로 막힙니다. `status`의 `wait` 목록에
  나오는 항목이 그대로 차단 사유가 됩니다.
- **미커밋 소스** — `phase done`은 plan 문서와 `.planr.yaml`을 제외한 미커밋 소스
  변경이 있으면 실패합니다.

```text
$ planr phase start checkout-v2 1
cannot set 00-checkout-v2 phase 01 to in-progress while its dependencies are unfinished:
  - phase 00 "API Contract" (planned)
finish them first or use --force
```

두 검사 모두 `--force`로 한 번에 우회합니다. 의도적으로 순서를 벗어나거나 커밋하지
않은 변경을 포함해야 할 때만 사용합니다. `phase reset`처럼 상태를 되돌리는 방향은
검사하지 않습니다.

## 설정

리포지토리 루트의 `.planr.yaml`에서 설정합니다. 설정 파일은 현재 디렉터리부터 상위
디렉터리로 탐색하되 git worktree 루트를 넘어가지는 않습니다.

```yaml
language: ko
plans_dirs:
  - plans-active
  - plans-archive
ignore:
  - generated/**
  - tmp
hooks:
  timeout: 30s
  before:
    - on: [new, add]
      run: "echo plan command started"
    - on: [done]
      run: "go test ./..."
  after:
    - on: [add, done]
      run: "echo plan command finished"
    - on: [plan_done]
      run: "echo all phases completed"
```

### plan 저장 위치

- `plans_dirs` — 새 plan은 **첫 번째 경로**에 등록되고, 조회는 모든 경로를 대상으로
  합니다. 기존 `plans_dir` 단일 설정도 지원하며, 설정이 없으면 `plan/`을 사용합니다.
- `ignore` — 리포지토리 루트 기준 glob 패턴으로, `phase done`의 미커밋 소스 검사에서
  제외할 경로를 지정합니다.

### 문서 언어

`language`는 planr이 **생성하는 문서**의 언어를 정합니다. 지원 값은 `en`과 `ko`이고,
설정하지 않으면 `en`입니다.

| 값 | `planr new` 초안 | phase 문서 | `PLAN.md` 본문 |
| --- | --- | --- | --- |
| `en` (기본) | 영어 스켈레톤 | `## Planned Work` / `## Done When` | `# Shared Verification` 등 |
| `ko` | 한국어 스켈레톤 | `## 계획된 작업` / `## 완료 조건` | `# 공통 검증` 등 |

명령 출력·옵션·오류 메시지는 언어 설정과 무관하게 항상 영어입니다. 훅과 스크립트가
어느 저장소에서나 같은 문자열을 보게 하기 위해서입니다.

**읽기는 언어를 가리지 않습니다.** `planr add`는 지원하는 모든 언어의 phase 제목을
인식하므로, `ko`로 설정된 저장소에서도 영어로 작성된 초안을 그대로 등록할 수 있고 그
반대도 됩니다. 언어 설정은 새로 만드는 문서에만 적용되며, 이미 등록된 plan의 문서를
다시 쓰지 않습니다.

### 훅

`hooks.before`와 `hooks.after`에 실행 시점별 규칙을 작성합니다. `on`에는 여러 이벤트를
배열로 지정할 수 있고, 같은 시점에 여러 명령이 필요하면 규칙을 여러 개 작성합니다.
규칙은 위에서 아래 순서로 실행됩니다.

| 이벤트 | 발생 시점 |
| --- | --- |
| `new` | 초안 생성 |
| `add` | plan 등록 |
| `phase_add` | 열린 plan에 phase 추가 |
| `start` / `done` / `reset` / `conditional` | 해당 phase 상태로 변경 |
| `plan_done` | 모든 phase가 완료되는 순간 (plan당 한 번) |

- **`before`** 훅은 상태나 파일을 기록하기 전에 실행되며, 실패하면 작업을 중단합니다.
- **`after`** 훅은 작업이 기록된 뒤 실행되며, 실패해도 기록을 되돌리지 않고 오류만
  알립니다.
- `hooks.timeout`은 훅 하나의 최대 실행 시간이며 `30s`, `5m`처럼 Go duration 형식으로
  지정합니다. 생략하면 기존 기본값인 **10분**을 사용하고, 넘으면 중단되며 어느 훅이
  멈췄는지 알려 줍니다.

모든 훅은 리포지토리 루트에서 셸 명령으로 실행되고 다음 환경 변수를 받습니다.

| 환경 변수 | 값 |
| --- | --- |
| `PLANR_EVENT` | 이벤트 이름 |
| `PLANR_PLAN` | plan 디렉터리 이름 |
| `PLANR_PHASE` | phase 번호 (`new`, `add`, `plan_done`처럼 plan 단위 이벤트에서는 비어 있음) |
| `PLANR_STATUS` | 변경된 상태 |

> 이전 단일 이벤트 맵 형식은 사용하지 않고 이 규칙 형식만 사용합니다.

## 저장 구조

plan은 모든 설정 경로를 기준으로 순번을 찾아 `00-`, `01-` 접두사를 붙여 생성합니다.

```text
plans-active/
└── 00-platform-refresh/
    ├── GOALS.md
    ├── CONTEXT.md
    ├── PLAN.md
    └── phases/
        ├── 00-foundation.md
        └── 02-benchmark-decision.md
```

`PLAN.md` frontmatter에는 plan 수준 설명·등록 시각·의존성·상태를 저장합니다.
`registered_at`은 초안 생성 시각이 아니라 `add`로 plan을 등록한 시각이 UTC RFC3339
형식으로 자동 기록됩니다.

값이 비어 있는 항목은 frontmatter에 쓰지 않습니다. 의존성이 없으면 `depends_on: []`
대신 키 자체가 없고, `succeeded_by: null` 같은 줄도 남지 않습니다. `false`와 `0`은
의미 있는 값이므로 그대로 유지됩니다.

## 완료 기록

phase나 plan이 `done`이 되면 두 가지가 자동으로 기록됩니다.

**1. `completed_at` frontmatter** — 완료 시각(UTC RFC3339)이 해당 phase 문서와,
plan 전체가 끝난 경우 `PLAN.md`에 기록됩니다. phase를 다시 열면 두 값 모두 지워지므로
오래된 날짜가 남지 않습니다.

**2. git note** — 완료 시점의 `HEAD` 커밋에 `refs/notes/planr` 노트를 붙여
"이 완료가 어느 커밋에서 이뤄졌는지"를 남깁니다. 노트는 커밋 객체를 바꾸지 않으므로
히스토리를 다시 쓰지 않습니다.

```text
planr plan=00-checkout-v2 event=done phase=01 at=2026-08-27T02:11:40Z
planr plan=00-checkout-v2 event=plan_done at=2026-08-27T02:11:40Z
```

기록된 내용은 `planr notes`로 조회합니다.

```sh
# 전체 완료 기록
planr notes

# 특정 plan만 (두 표기 모두 동작)
planr notes checkout-v2
planr notes 00-checkout-v2
```

```text
COMPLETED             PLAN          EVENT      COMMIT   SUBJECT
2026-08-27T02:11:40Z  00-demo-plan  done 00    50eb786  add demo plan
2026-08-27T02:11:40Z  00-demo-plan  plan_done  50eb786  add demo plan
```

노트는 go-git으로 직접 기록하므로 `git` 실행 파일이 없어도 동작하고, 표준 git notes
포맷이라 `git notes --ref=planr show <commit>` 이나 `git log --notes=refs/notes/planr`
로도 그대로 읽힙니다. 노트를 남기지 못해도 완료 처리 자체는 유지되고 경고만 표준 오류로
출력합니다.

`refs/notes/*`는 git이 기본으로 주고받는 ref가 아니므로, 완료 기록을 팀과 공유하려면
명시적으로 push·fetch 합니다.

```sh
git push origin refs/notes/planr
git fetch origin refs/notes/planr:refs/notes/planr
```

```yaml
---
description: "checkout flow refresh"
registered_at: "2026-08-26T17:30:00Z"
plan_status: in-progress
depends_on: [platform-refresh, api-foundation#2]
---
```

phase 목록은 문서 본문의 체크리스트와 링크로 관리됩니다.

```markdown
# Phases

- [ ] [Phase 00: Foundation](phases/00-foundation.md)
- [ ] [Phase 02: Benchmark Decision](phases/02-benchmark-decision.md)
```

## 초안 규격

### frontmatter

초안 파일에는 선택적으로 다음 frontmatter를 둘 수 있습니다. plan 이름은
`add --name` 옵션, `plan_name` 또는 `name`, 파일명 순으로 결정됩니다.

```yaml
---
plan_name: checkout-v2
description: "checkout flow refresh"
depends_on: [platform-refresh, api-foundation#2]
---
```

### 섹션

모든 최상위 섹션은 아래 순서로 한 번씩 작성합니다.

```markdown
# GOALS
# SCOPE
# CONTEXT
# PHASES
# VERIFICATION
# ORDERING
# NEXT
```

`NEXT`에는 YAML 펜스의 `next_phase`와 다음 작업 설명을 작성합니다.

### PHASE 블록

`PHASES`의 각 phase는 제목 직후 YAML 펜스를 두고, 계획된 작업과 완료 조건을 모두
채웁니다. 두 소제목은 언어별로 다음 쌍 중 **하나**를 씁니다. 저장소의 `language`
설정과 관계없이 두 쌍 모두 인식하므로, 다른 언어로 작성된 초안도 그대로 등록됩니다.

| 언어 | 소제목 |
| --- | --- |
| `en` | `### Planned Work` / `### Done When` |
| `ko` | `### 계획된 작업` / `### 완료 조건` |

````markdown
## PHASE — Checkout UI

```yaml
phase: 1
slug: checkout-ui
perf_phase: false
depends_on: [0]
status: planned
entry_condition: null
```

### 계획된 작업

- 새 checkout UI를 feature flag 뒤에 구현한다.

### 완료 조건

- E2E checkout 시나리오가 통과한다.
````

- phase 번호는 비연속이어도 되지만 중복될 수 없습니다.
- `depends_on`은 같은 초안에 정의된 자신 이외의 phase 번호만 참조할 수 있습니다.
- `conditional` phase는 `entry_condition`에 착수 조건을 반드시 적습니다.

### 의존성

plan 의존성은 plan 이름을 기준으로 하며, `plan-name#phase-number`를 사용하면 특정
phase까지 기다리도록 지정할 수 있습니다. 완료되지 않은 선행 plan 또는 phase가 있으면
`status` 출력의 `wait` 목록에 표시됩니다. 존재하지 않는 이름도 초안에는 기록할 수
있으므로, 나중에 추가할 plan을 미리 연결하는 것도 가능합니다. 다만 등록되지 않은
plan에 의존하는 phase는 [진행 전 검사](#진행-전-검사)에서 막히므로, 실제로 작업을
시작하려면 그 plan이 먼저 등록되고 완료되어야 합니다.

검사 시점은 세 번입니다.

- **`new`** — plan 의존성의 형식, 중복, 자기 자신에 대한 의존성을 검사합니다.
- **`add`** — 위 검사를 다시 수행하고, phase가 같은 plan 안에서 다른 phase만
  의존하는지, 참조한 phase가 존재하는지, 의존성 순환이 없는지도 검사합니다.
- **`phase start` / `phase done`** — 등록된 그래프대로 선행 phase와 선행 plan이 모두
  완료되었는지 검사합니다. `--force`로 우회합니다.

앞의 두 검사에서 문제가 있으면 어떤 plan·phase의 의존성이 잘못되었는지 오류 메시지로
안내하며 파일을 등록하지 않습니다.

## 라이프사이클

1. `planr new <plan-name> --description "짧은 설명"`으로 초안을 생성하고, 목표·phase·검증
   조건을 채웁니다.
2. `planr add <draft-file>`로 검증된 초안을 번호가 붙은 plan 디렉터리로 등록합니다.
   등록은 임시 디렉터리에서 준비한 후 이동하므로 파싱 오류가 나면 부분 파일을 남기지
   않습니다.
3. 작업을 시작하면 해당 `phases/*.md` frontmatter의 `status`를 `in-progress`로
   변경하고 구현·검증 결과를 기록합니다.
4. phase 완료 시 `status: done`으로 변경합니다. 다음 phase가 실측 조건을 만족하면
   `conditional` phase를 `in-progress`로 전환합니다.
5. 모든 phase를 `phase done`으로 완료하면 `PLAN.md`의 `plan_status: done`이 자동으로
   기록됩니다.

phase 상태는 `planned`, `conditional`, `in-progress`, `done`을 사용합니다. 상태를
변경하면 해당 phase 문서의 frontmatter와 `PLAN.md`의 체크리스트가 함께 갱신됩니다.
모든 phase를 `done`으로 변경하면 `PLAN.md`의 `plan_status`도 자동으로 `done`이 되며,
완료된 phase를 다시 미완료 상태로 바꾸면 plan은 `in-progress`로 돌아갑니다.

---

## 개발

여기부터는 `planr` 자체를 수정하거나 검증할 때 필요한 내용입니다.

### 실행기

`planr`를 검증하는 스크립트는 모두 [`planr/scripts`](scripts)의 uv 프로젝트에 있고,
단일 진입점 [`planr/scripts/main.py`](scripts/main.py)의 하위 명령으로 실행합니다.

| 명령 | 하는 일 | 필요한 것 |
| --- | --- | --- |
| `main.py scenario` | checkout 출시 시나리오의 `status`·`overview` 출력을 재현 | `python3`, `go` |
| `main.py scenario clean` | 시나리오 실행 디렉터리 삭제 | `python3` |
| `main.py codex` | 격리 저장소에서 Codex 평가 실행 | `uv`, `go`, `git`, Codex 로그인 |
| `main.py codex analyze <dir>` | 이전 실행 재분석 | `python3` |
| `main.py codex clean` | Codex 실행 디렉터리와 작업공간 삭제 | `python3` |

Codex SDK는 실제로 평가를 실행할 때만 불러오므로, `scenario`와 `codex`의
`analyze`·`clean`은 `python3`로 바로 실행됩니다. `codex` 실행만 uv가 필요하고, SDK
없이 호출하면 실행할 명령을 알려 주고 종료합니다.

오류 메시지와 종료 코드는 모두 [`main.py`](scripts/main.py) 한 곳에서 처리하며, 두
실행기가 공유하는 준비 과정(픽스처 복사, `planr` 빌드, 실행 디렉터리 생성·정리)은
[`common.py`](scripts/common.py)에 있습니다.

### 실행 디렉터리

모든 실행은 `planr/run/` 아래에 자기 디렉터리를 하나 만들고 그 안에만 산출물을
남깁니다. 이름은 `<UTC 타임스탬프>-<실행기>` 형식이라 디렉터리 목록이 곧 시간순
정렬입니다. 같은 초에 같은 종류의 실행이 겹칠 때만 뒤에 번호가 붙습니다.

```text
planr/run/
├── 20260826-123846-scenario/   시나리오 작업 디렉터리 겸 산출물
└── 20260826-123851-codex/      Codex 평가 산출물 (REPORT.md, transcript.md, session.jsonl, state/, metrics.json)
```

Codex 평가에서 **에이전트의 작업공간은 `planr/run/` 밖**, 시스템 임시 디렉터리에
만들어집니다. 작업공간을 실행 디렉터리 안이나 옆에 두면 에이전트가 `..`를 읽어 자기
평가 리포트와 transcript를 볼 수 있기 때문입니다. 작업공간 경로는 실행 디렉터리의
`metadata.env`에 `workspace=`로 기록되고, `clean`이 이 기록을 따라가 작업공간까지 함께
삭제합니다. `planr/run/`은 Git에서 제외됩니다.

### 테스트 픽스처

실행기가 사용하는 픽스처는 모두 [`planr/fixtures`](fixtures) 아래에 있습니다.

| 픽스처 | 내용 |
| --- | --- |
| [`plan-scenario`](fixtures/plan-scenario) | `scenario.py`가 쓰는 단순 plan 생성 시나리오 |
| [`codex-harness`](fixtures/codex-harness) | Codex 평가 실행기가 복사하는 샘플 Git·Go 저장소 |

각 픽스처의 용도와 구성 파일은 [`MANIFEST.yaml`](fixtures/MANIFEST.yaml)에 정리되어
있으며, 픽스처를 추가하면 이 파일에도 항목을 추가합니다. 실행기는 픽스처를
실행별 격리 디렉터리로 복사한 뒤 그 안에서만 파일을 수정하므로 픽스처 원본은
변경되지 않습니다.

### 시나리오 재현

checkout 출시 시나리오의 복합 상태 출력을 재현하려면 다음을 실행합니다.

```sh
python3 planr/scripts/main.py scenario

# 실행 결과 정리
python3 planr/scripts/main.py scenario clean
```

시나리오는 `plan-scenario` 픽스처를 `planr/run/<타임스탬프>-scenario/`로 복사하고 git
저장소로 초기화한 뒤, 다음 다섯 가지 plan을 만들고 상세한 `status`, 간단한 `overview`,
`notes` 출력을 차례로 보여 줍니다.

- 완료된 인증 기반 plan
- 진행 중 checkout plan
- checkout을 기다리는 결제 plan
- 일부 phase만 완료된 rollout plan
- `status`에서 숨겨지는 무관한 완료 plan

**상태를 만드는 방법** — 시나리오는 planr이 만든 문서를 고치지 않습니다. plan마다
초안을 따로 쓰면서 의존성을 **초안 frontmatter에 적어 넣고**(입력을 다듬고), 완료·부분
완료 상태는 실제 `planr phase done` 호출로 만듭니다.

생성된 frontmatter를 정규식으로 치환하면 내부 파일 형식에 시나리오가 묶이고, 무엇보다
**CLI가 거부할 배치를 화면에 만들어 낼 수 있습니다** — 선행 phase보다 먼저 완료된
phase 같은 것입니다. 반면 초안 형식은 문서화된 인터페이스이므로, 형식이 바뀌면 시나리오도
분명한 이유로 함께 실패합니다. 실행 결과는 Git에서 제외됩니다.

### 테스트

```sh
# Go 코드와 단위 테스트
go test ./...
go vet ./...

# 실행기의 Python 단위 테스트
uv run --with pytest --project planr/scripts python -m pytest planr/scripts -q
```

### Codex 평가

실행기는 매번 시스템 임시 디렉터리에 격리된 Git 리포지토리를 만들고, 그 안에
`codex-harness` 픽스처의 `AGENTS.md`, 샘플 Go 프로젝트와 `planr` 바이너리를
준비합니다. 산출물은 이 리포지토리가 아니라 `planr/run/<타임스탬프>-codex/`에
쌓입니다.

픽스처의 `FIXTURE.` 접두사 파일은 평가 설정이지 저장소 내용이 아니므로 워크스페이스로
그대로 복사되지 않습니다.

| 픽스처 파일 | 워크스페이스 |
| --- | --- |
| `FIXTURE.PROMPT.<언어>.md` | **복사하지 않음.** 실행 언어에 해당하는 파일이 요청으로만 사용됩니다 |
| `FIXTURE.AGENTS.<언어>.md` | 실행 언어에 해당하는 파일만 `AGENTS.md`로 설치됩니다 |
| 그 외 파일 | 그대로 복사됩니다 |

요청을 파일로 두지 않는 이유는, 디스크에서 다시 읽을 수 있는 과제 명세가 아니라 대화로
받은 요청만으로 일하는 상황을 재현하기 위해서입니다.

두 파일의 역할은 섞지 않습니다.

- `FIXTURE.PROMPT.<언어>.md`는 **실제 사용자가 보낼 법한 요청**입니다. planr을 언급하지 않고,
  대신 "물어보지 말고 끝까지 알아서 진행해 달라"는 지시를 담습니다. 이후 개입이 없으므로
  완료 판단의 근거가 이 메시지뿐입니다.
- `FIXTURE.AGENTS.<언어>.md`는 **저장소·과제와 무관한 planr 사용 지침**입니다. 명령
  레퍼런스, 초안 규격, 작업 흐름, 규칙만 담고 있어 다른 저장소에 그대로 옮겨도 됩니다.

요청 쪽에 워크플로를 다시 적으면 "에이전트가 AGENTS.md를 읽고 planr을 찾아 쓰는가"라는
측정 자체가 무의미해집니다. 이 분리는 단위 테스트로 고정되어 있습니다.

#### 실행 언어

요청과 지침을 언어마다 하나씩 두고, 실행 언어에 맞는 파일만 사용합니다.

| 언어 | 요청 | 지침 |
| --- | --- | --- |
| `en` | `FIXTURE.PROMPT.en.md` | `FIXTURE.AGENTS.en.md` |
| `ko` | `FIXTURE.PROMPT.ko.md` | `FIXTURE.AGENTS.ko.md` |

둘을 함께 고르는 이유는, 영어 요청에 한국어 지침이 붙으면 실행이 어느 한 언어가 아니라
그 혼합을 측정하기 때문입니다. 언어는 다음 순서로 정해집니다.

1. `--language en|ko` — 지정하면 이 값이 이깁니다. 픽스처를 고치지 않고 같은 과제를
   두 언어로 평가할 수 있습니다. (`PLANR_HARNESS_LANGUAGE`로도 지정합니다.)
2. 픽스처 `.planr.yaml`의 `language` 값
3. 둘 다 없으면 planr 기본값인 `en`

```sh
planr-codex                             # 픽스처 설정을 따름
planr-codex --language ko               # 픽스처가 en이어도 ko로 실행
planr-codex --fixture codex-greenfield --language en
```

정해진 언어는 지침 선택에만 쓰이지 않고, 워크스페이스 `.planr.yaml`의 `language`에도
그대로 기록됩니다. 그러지 않으면 `--language`가 **에이전트가 읽는 문서만 바꾸고 planr이
생성하는 문서는 픽스처 언어 그대로** 남아, 실행이 언어가 아니라 그 모순을 측정하게
됩니다.

실행 언어는 `metadata.env`와 리포트의 `Document language`에 남습니다. 서로 다른 실행을
비교할 때는 이 값이 같은지 먼저 확인해야 합니다.

구현은 [`codex.py`](scripts/codex.py)에 있고, `openai-codex` 공식 Python SDK의
`AsyncCodex`를 사용합니다. 따라서 별도의 `codex exec` 셸 호출 없이 원본 SDK 알림을 모두
보존합니다.

요청은 **한 번만** 보내고, 이후에는 개입하지 않습니다. "계속해" 같은 후속 프롬프트가
없으므로 에이전트가 스스로 완료를 판단해야 하며, 리포트는 그 결과를 측정합니다.
따라서 `--timeout`(기본 3600초)은 **작업 전체**에 주어지는 시간입니다.
기본 모델은 `gpt-5.6-luna`, reasoning은 `medium`입니다. SDK 사용법은
[공식 Python SDK 문서](https://github.com/openai/codex/tree/main/sdk/python)를 기준으로
합니다.

처음 clone한 환경에서는 의존성을 동기화합니다.

```sh
uv sync --project planr/scripts
```

Codex 인증은 로컬 Codex 설정을 사용하므로, 실제 실행 전 Codex 로그인이 되어 있어야
합니다. 승인은 자동으로 거부하고 샌드박스는 격리 작업공간에 한정합니다.

```sh
# 아래 예시는 이 별칭을 사용합니다
alias planr-codex='uv run --locked --project planr/scripts python planr/scripts/main.py codex'

# 기본 실행 (에이전트가 완료를 판단할 때까지 진행)
planr-codex

# 모델과 reasoning을 명시
planr-codex --model gpt-5.6-luna --reasoning low

# 문서 언어를 지정 (생략하면 픽스처의 language 설정)
planr-codex --language ko

# 오래 걸리는 과제라면 제한 시간을 늘립니다
planr-codex --timeout 7200

# Codex를 호출하지 않고 격리 저장소와 산출물 경로만 점검
planr-codex --dry-run

# 진행 로그 없이 실행
planr-codex --quiet

# 이전 실행 재분석 또는 임시 실행공간 정리
planr-codex analyze planr/run/20260826-123851-codex
planr-codex clean
```

#### 진행 로그

한 번의 세션이 최대 `--timeout`(기본 3600초)까지 이어지므로, 실행 중에는 진행 상황을
**stderr**로 계속 출력합니다. 최종 결과 경로는 stdout으로 나가므로 `> paths.txt`처럼
갈라 받아도 로그는 그대로 볼 수 있습니다. `--quiet`로 끌 수 있습니다.

```text
[   0:00] preparing isolated repository (copy fixture, build planr, git init)
[   0:02] run directory 20260827-101500-codex; workspace /var/folders/.../planr-codex-ab12
[   0:03] thread th_01J started on gpt-5.6-luna (reasoning low)
[   0:03] session started (612 prompt chars)
[   0:24]   think  Read AGENTS.md, then plan the work
[   0:31]   cmd  planr new json-output --description "..."
[   1:02]   edit  main.go, main_test.go
[   1:14]   cmd  go test ./... (exit 1)
[   1:48]   say  Phase 0 done: added the parser and its tests.
...
[41:52] session completed in 41:49 · 87 items · 268.4k tokens (in 240.1k / out 28.3k)
[42:10] running final verification (go test) and capturing end state
[42:18] final go test exit 0; writing report
```

- 맨 앞은 실행 시작 이후 경과 시간입니다.
- 들여쓴 줄은 에이전트가 한 일입니다. `cmd`(명령 실행), `edit`(파일 변경),
  `say`(에이전트 메시지), `think`(추론), `tool`, `search`, `todo`, `error`로 구분됩니다.
- 세션 종료 줄에는 소요 시간, 항목 수, 토큰 사용량이 함께 표시됩니다.
- 타임아웃·실패도 각각 한 줄로 남으므로, 로그가 멈춰 있으면 그때는 실제로 모델이
  응답을 기다리는 중입니다.

전체 알림 원본이 필요하면 실행 중에도 `session.jsonl`을 직접 `tail -f` 할 수 있습니다.

실행이 끝나면 실행 디렉터리에 다음 산출물이 남습니다.

| 파일 | 내용 |
| --- | --- |
| `REPORT.md` | plan 완료 여부, 세션 exit·이벤트·input/output/total token, 관찰된 `planr` 명령과 도구 호출 누락, 지침이 불명확했을 가능성이 있는 지점, 반복 명령으로 추정되는 토큰 낭비 신호 |
| `transcript.md` | 대화와 명령 추출본 |
| `session.jsonl` | SDK가 전달한 원본 알림과 정규화한 세션 결과 |
| `session.prompt.md` | 에이전트에게 보낸 요청 |
| `metrics.json` | 서로 다른 모델·설정 실행을 비교할 때 사용 |

실행기는 격리된 작업공간에서만 파일을 수정하며, 분석이 끝난 뒤에도 결과를 직접
확인할 수 있도록 실행 디렉터리와 작업공간을 자동 삭제하지 않습니다. 정리는
`planr-codex clean`으로 합니다.
