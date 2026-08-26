# planr

규격화된 Markdown 계획을 분할 저장하고 진행 상태를 조회하는 Go CLI입니다.

하나의 계획 초안을 작성해 등록하면 `GOALS.md`, `CONTEXT.md`, `PLAN.md`, `phases/*.md`로
나뉘어 저장되고, 이후에는 phase 단위로 상태를 바꾸며 진행 상황을 조회합니다.

- [설치](#설치)
- [빠른 시작](#빠른-시작)
- [명령](#명령) — [plan 등록](#plan-등록), [조회](#조회), [phase 관리](#phase-관리)
- [설정](#설정) — [plan 저장 위치](#plan-저장-위치), [훅](#훅)
- [저장 구조](#저장-구조)
- [초안 규격](#초안-규격) — [frontmatter](#frontmatter), [섹션](#섹션), [PHASE 블록](#phase-블록), [의존성](#의존성)
- [라이프사이클](#라이프사이클)
- [개발](#개발) — 기여자용: [실행기](#실행기), [실행 디렉터리](#실행-디렉터리), [Codex 평가](#codex-평가)

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
| `planr status [plan-name]` | 남은 phase와 대기 중인 의존성을 자세히 출력합니다 |
| `planr overview [plan-name]` | 모든 plan의 진행률을 한 줄씩 요약합니다 |
| `planr phase add <plan-name> <title>` | 열린 plan에 phase를 추가합니다 |
| `planr phase set <plan-name> <number> --status <status>` | phase 상태를 지정한 값으로 변경합니다 |
| `planr phase start\|done\|reset <plan-name> <number>` | phase 상태 변경 단축 명령입니다 |

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
```

**`status`** — `phases/*.md`의 현재 frontmatter를 읽어 남은 phase와 `wait` 목록을
출력합니다. 완료된 plan은 기본적으로 숨기지만, 진행 중 plan의 phase가 의존하는
완료 plan은 함께 표시합니다. plan 이름을 지정해 조회하면 완료 여부와 관계없이
표시합니다.

**`overview`** — 완료된 plan을 포함해 각 plan의 상태와 `완료 phase/전체 phase`
진행률, 다음 미완료 phase, 의존성 대기를 한눈에 보여 줍니다. 자세한 phase 목록과
기본 완료 plan 숨김 규칙이 필요하면 `status`를 사용합니다.

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

`phase done`은 plan 문서와 `.planr.yaml`을 제외한 미커밋 소스 변경이 있으면
실패합니다. 아직 커밋하지 않은 변경을 의도적으로 포함해야 할 때만 `--force`로
검사를 우회합니다.

## 설정

리포지토리 루트의 `.planr.yaml`에서 설정합니다. 설정 파일은 현재 디렉터리부터 상위
디렉터리로 탐색합니다.

```yaml
plans_dirs:
  - plans-active
  - plans-archive
ignore:
  - generated/**
  - tmp
hooks:
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

`PHASES`의 각 phase는 제목 직후 YAML 펜스를 두고, `계획된 작업`과 `완료 조건`을
모두 채웁니다.

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
있으므로, 나중에 추가할 plan을 미리 연결하는 것도 가능합니다.

검사 시점은 두 번입니다.

- **`new`** — plan 의존성의 형식, 중복, 자기 자신에 대한 의존성을 검사합니다.
- **`add`** — 위 검사를 다시 수행하고, phase가 같은 plan 안에서 다른 phase만
  의존하는지, 참조한 phase가 존재하는지, 의존성 순환이 없는지도 검사합니다.

문제가 있으면 어떤 plan·phase의 의존성이 잘못되었는지 오류 메시지로 안내하며 파일을
등록하지 않습니다.

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

시나리오는 `plan-scenario` 픽스처를 `planr/run/<타임스탬프>-scenario/`로 복사한 뒤,
다음 다섯 가지 plan을 만들고 상세한 `status`와 간단한 `overview` 출력을 차례로 보여
줍니다.

- 완료된 인증 기반 plan
- 진행 중 checkout plan
- checkout을 기다리는 결제 plan
- 일부 phase만 완료된 rollout plan
- `status`에서 숨겨지는 무관한 완료 plan

같은 초안을 여러 번 등록하면 동일한 진행 중 plan만 나오므로, 완료·대기·부분 완료
상태는 등록 후 frontmatter를 고쳐서 만들어 냅니다. 이 치환이 하나도 일치하지 않으면
plan 형식이 바뀐 것으로 보고 실패합니다. 실행 결과는 Git에서 제외됩니다.

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
| `FIXTURE.PROMPT.md` | **복사하지 않음.** 에이전트에게 보내는 요청으로만 사용됩니다 |
| `FIXTURE.AGENTS.md` | `AGENTS.md`로 설치됩니다 |
| 그 외 파일 | 그대로 복사됩니다 |

요청을 파일로 두지 않는 이유는, 디스크에서 다시 읽을 수 있는 과제 명세가 아니라 대화로
받은 요청만으로 일하는 상황을 재현하기 위해서입니다.

두 파일의 역할은 섞지 않습니다.

- `FIXTURE.PROMPT.md`는 **실제 사용자가 보낼 법한 요청**입니다. planr을 언급하지 않고,
  대신 "물어보지 말고 끝까지 알아서 진행해 달라"는 지시를 담습니다. 이후 개입이 없으므로
  완료 판단의 근거가 이 메시지뿐입니다.
- `FIXTURE.AGENTS.md`는 **저장소·과제와 무관한 planr 사용 지침**입니다. 명령 레퍼런스,
  초안 규격, 작업 흐름, 규칙만 담고 있어 다른 저장소에 그대로 옮겨도 됩니다.

요청 쪽에 워크플로를 다시 적으면 "에이전트가 AGENTS.md를 읽고 planr을 찾아 쓰는가"라는
측정 자체가 무의미해집니다. 이 분리는 단위 테스트로 고정되어 있습니다.

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
