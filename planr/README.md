# planr

규격화된 Markdown 계획을 분할 저장하고 진행 상태를 조회하는 Go CLI입니다.

## 사용

```sh
# 규격 초안 생성
go run . new platform-refresh

# 초안을 채운 뒤 plan으로 등록
go run . add platform-refresh.md

# 등록된 진행 중 plan 조회
go run . status

# 특정 plan 조회 (완료된 plan 포함)
go run . status platform-refresh
```

`new`는 생성한 초안 파일의 절대 경로를 출력합니다. `add --name <name>`으로
초안 frontmatter나 파일명 대신 plan 이름을 지정할 수 있습니다.

## 설정

리포지토리 루트의 `.planr.yaml`에서 plan 저장 위치를 지정합니다. 설정 파일은 현재
디렉터리부터 상위 디렉터리로 탐색합니다.

```yaml
plans_dirs:
  - plans-active
  - plans-archive
```

새 plan은 첫 번째 경로에 등록되고, `status`는 모든 경로를 조회합니다. 기존
`plans_dir` 단일 설정도 지원하며, 설정이 없으면 `plan/`을 사용합니다.

## 생성 구조

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

`PLAN.md` frontmatter에는 plan 수준 상태만 저장합니다. phase 목록은 문서 본문의
체크리스트와 링크로 관리됩니다.

```markdown
# Phases

- [ ] [Phase 00: Foundation](phases/00-foundation.md)
- [ ] [Phase 02: Benchmark Decision](phases/02-benchmark-decision.md)
```

## 초안 형식과 상태

초안에는 `GOALS`, `SCOPE`, `CONTEXT`, `PHASES`, `VERIFICATION`, `ORDERING`,
`NEXT` 섹션이 순서대로 필요합니다. 각 phase에는 YAML frontmatter로 번호, slug,
상태, 의존성을 정의합니다.

`status`는 완료된 plan을 기본적으로 숨깁니다. 진행 중 plan의 phase가 완료된 plan을
의존하면, 그 완료 plan은 함께 표시합니다. plan 이름을 지정해 조회하면 완료 여부와
관계없이 표시합니다.

## 초안 규격

초안 파일에는 선택적으로 다음 frontmatter를 둘 수 있습니다. plan 이름은 `add --name`
옵션, `plan_name` 또는 `name`, 파일명 순으로 결정됩니다.

```yaml
---
plan_name: checkout-v2
---
```

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

`PHASES`의 각 phase는 제목 직후 YAML 펜스를 두고, `계획된 작업`과 `완료 조건`을
모두 채웁니다. phase 번호는 비연속이어도 되지만 중복될 수 없고, `depends_on`은 같은
초안에 정의된 phase 번호만 참조할 수 있습니다.

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

`conditional` phase는 `entry_condition`에 착수 조건을 반드시 적습니다. `NEXT`에는
YAML 펜스의 `next_phase`와 다음 작업 설명을 작성합니다.

## 라이프사이클

1. `planr new <plan-name>`으로 초안을 생성하고, 목표·phase·검증 조건을 채웁니다.
2. `planr add <draft-file>`로 검증된 초안을 번호가 붙은 plan 디렉터리로 등록합니다.
   등록은 임시 디렉터리에서 준비한 후 이동하므로 파싱 오류가 나면 부분 파일을 남기지
   않습니다.
3. 작업을 시작하면 해당 `phases/*.md` frontmatter의 `status`를 `in-progress`로
   변경하고 구현·검증 결과를 기록합니다.
4. phase 완료 시 `status: done`으로 변경합니다. 다음 phase가 실측 조건을 만족하면
   `conditional` phase를 `in-progress`로 전환합니다.
5. 모든 phase가 끝나면 `PLAN.md` frontmatter의 `plan_status: done`으로 변경합니다.

phase 상태는 `planned`, `conditional`, `in-progress`, `done`을 사용합니다. `status`는
`phases/*.md`의 현재 frontmatter를 읽어 남은 phase와 `wait` 목록을 출력합니다.

## 재현 가능한 예제

checkout 출시 시나리오의 복합 상태 출력을 재현하려면 다음을 실행합니다.

```sh
./planr/test-planr.sh
```

스크립트는 `planr/test/work.*`에 새 격리 작업 디렉터리를 만들고, 완료된 인증 기반 plan,
진행 중 checkout plan, checkout을 기다리는 결제 plan, 숨겨지는 무관한 완료 plan을
생성한 뒤 `status` 출력을 보여 줍니다. 실행 결과는 Git에서 제외됩니다.

실행 결과를 정리하려면 다음을 사용합니다.

```sh
./planr/test-planr.sh clean
```
