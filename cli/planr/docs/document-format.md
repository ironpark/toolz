# plan 문서 규격

[README로 돌아가기](../README.md) · [명령어 가이드](commands.md) · [설정](configuration.md)

사용 가능한 필드와 규칙을 기계가 읽을 수 있는 형태로 확인하려면 `planr schema --json`을
사용하세요. 이 문서는 사람이 초안과 등록 문서를 작성할 때 필요한 규칙을 설명합니다.

## 초안에서 등록 문서까지

`planr new`가 만드는 단일 Markdown 초안을 `planr apply`에 전달하면 다음 문서로 나뉩니다.

| 초안 내용 | 등록 문서 |
| --- | --- |
| `GOALS` | `GOALS.md` |
| `SCOPE`, `CONTEXT` | `CONTEXT.md` |
| phase 목록과 공통 검증·순서·다음 작업 | `PLAN.md` |
| 각 `PHASE` 블록 | `phases/NN-slug.md` |

등록은 임시 디렉터리에서 모두 준비한 뒤 최종 위치로 이동합니다. 파싱이나 검증이 실패하면
부분적으로 생성된 plan을 남기지 않습니다.

## plan 초안

### frontmatter

초안의 YAML frontmatter에는 다음 값을 둘 수 있습니다. `description`은 `new`로 만들 때
필수이며 공백을 포함해 200자 이하여야 합니다.

```yaml
---
plan_name: checkout-v2
description: "checkout flow refresh"
depends_on: [platform-refresh, api-foundation#2]
---
```

plan 이름은 `plan_name`, 호환 필드 `name`, 파일명 순으로 결정됩니다. 이름은 kebab-case를
사용합니다. `depends_on`은 생략할 수 있습니다.

### 최상위 섹션

아래 섹션을 이 순서로 한 번씩 작성합니다.

```markdown
# GOALS
# SCOPE
# CONTEXT
# PHASES
# VERIFICATION
# ORDERING
# NEXT
```

`NEXT`에는 YAML fence의 `next_phase`와 다음 작업 설명이 필요합니다. `next_phase`는 초안에
정의된 phase 번호를 가리켜야 합니다.

### PHASE 블록

각 phase는 제목 바로 아래 YAML fence, 계획된 작업, 완료 조건으로 구성합니다.

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

| 필드 | 규칙 |
| --- | --- |
| `phase` | 0 이상의 고유 번호입니다. 비연속이어도 됩니다 |
| `slug` | 파일명에 사용할 kebab-case 값입니다 |
| `perf_phase` | 성능 검증 phase인지 나타내는 boolean입니다 |
| `depends_on` | 같은 초안에 있는 다른 phase 번호의 목록입니다 |
| `status` | 새 plan에서는 `planned` 또는 `conditional`입니다 |
| `entry_condition` | `conditional` phase일 때 필수인 착수 조건입니다 |

계획과 완료 조건 소제목은 아래 두 언어 중 한 쌍을 사용합니다. 저장소의 `language`와 달라도
읽을 수 있지만 한 phase 안에서는 같은 언어 쌍을 쓰는 편이 명확합니다.

| 언어 | 계획 | 완료 조건 |
| --- | --- | --- |
| `en` | `### Planned Work` | `### Done When` |
| `ko` | `### 계획된 작업` | `### 완료 조건` |

## 의존성

### plan 간 의존성

- `plan-name`은 해당 plan 전체가 끝날 때까지 기다립니다.
- `plan-name#phase-number`는 지정한 phase가 끝날 때까지 기다립니다.
- 아직 등록되지 않은 plan 이름도 초안에 기록할 수 있지만 실제 phase 시작은 차단됩니다.
- plan은 자기 자신에 의존할 수 없고 같은 의존성을 중복 지정할 수 없습니다.

### phase 간 의존성

새 plan 초안에서는 같은 초안의 phase 번호를 참조합니다. 기존 plan에 phase를 추가하는
초안에서는 기존 phase 번호 또는 slug를 사용할 수 있습니다. 자기 참조, 존재하지 않는
phase와 순환 의존성은 허용하지 않습니다.

검사는 세 시점에 이루어집니다.

1. `new`가 plan 의존성 표기, 중복과 자기 참조를 검사합니다.
2. `apply`가 모든 필드와 phase 참조, 순환 의존성을 검사합니다.
3. `phase start`와 `phase done`이 등록된 선행 phase와 plan의 완료 여부를 검사합니다.

실행 시점의 차단 이유는 `planr status`의 `wait`에도 표시됩니다. 상태 변경 검사는
`--force`로 우회할 수 있지만 문서 형식 오류는 먼저 수정해야 합니다.

## 등록된 문서

### PLAN.md

`PLAN.md` frontmatter에는 설명, 등록 시각, plan 상태와 plan 의존성이 저장됩니다.

```yaml
---
description: "checkout flow refresh"
registered_at: "2026-08-26T17:30:00Z"
plan_status: in-progress
depends_on: [platform-refresh, api-foundation#2]
---
```

`registered_at`은 초안 생성 시각이 아니라 등록 시각이며 UTC RFC3339 형식입니다. 빈 값은
직렬화하지 않지만 `false`와 `0`은 의미 있는 값이므로 유지합니다.

phase 목록은 본문의 checklist입니다.

```markdown
# Phases

- [ ] [Phase 00: Foundation](phases/00-foundation.md)
- [ ] [Phase 02: Benchmark Decision](phases/02-benchmark-decision.md)
```

이 영역과 `plan_status`는 phase 문서에서 파생됩니다. `edit --section plan` checkout에서
보호 marker를 직접 바꾸지 말고 상태는 `phase set`, `start`, `done`, `reset`으로 변경하세요.

### phase 문서

등록된 phase 번호는 `phases/NN-slug.md` 파일명과 PLAN 링크에 저장됩니다. frontmatter에는
`status`, `entry_condition`, `perf_phase`, `depends_on`, `blocks`와 완료 시각이 들어갑니다.
본문은 `# <title>`, `## Planned Work`, `## Done When` 구조를 사용하며 설정 언어에 따라
한국어 제목을 쓸 수 있습니다.

phase를 삭제해도 나머지 번호는 다시 매기지 않습니다. 따라서 `00`, `02`처럼 빈 번호가
있는 것은 정상입니다.

## 상태 라이프사이클

```text
planned ── start ──> in-progress ── done ──> done
   ▲                       │                    │
   └──────── reset ────────┴────── reset ──────┘

conditional ── start ──> in-progress
```

- `conditional`에는 `entry_condition`이 필요합니다.
- 상태 변경은 phase frontmatter와 `PLAN.md` checklist를 함께 갱신합니다.
- 모든 phase가 `done`이면 plan도 `done`이 됩니다.
- 완료 phase를 다시 열면 plan은 `in-progress`로 돌아가고 오래된 완료 시각이 제거됩니다.

## 검증 오류 확인

사람이 읽을 오류는 기본 출력으로, 자동화에서 처리할 오류는 JSON으로 확인합니다.

```sh
planr apply draft.md --dry-run
planr apply draft.md --dry-run --json
planr schema --json
```

JSON 검증 오류는 `rule`, `section`, `phase`, `line`, `detail`을 제공하므로 원본 문서에서
수정할 위치와 위반 규칙을 함께 찾을 수 있습니다.
