# 설정과 저장 방식

[README로 돌아가기](../README.md) · [명령어 가이드](commands.md) · [문서 규격](document-format.md)

## 설정 파일

`planr init`이 주석이 달린 `.planr.yaml`과 `plans_dirs` 디렉터리를 저장소 루트에
만듭니다. 하위 디렉터리에서 실행해도 파일은 저장소 루트에 생성되며, 이미 설정이 있으면
`--force` 없이는 덮어쓰지 않습니다.

```sh
planr init
planr init --language ko --plans-dir plans-active --plans-dir plans-archive
```

모든 설정에 기본값이 있으므로 `.planr.yaml` 없이도 planr은 동작합니다. `init`은 기본값을
파일로 꺼내 놓아 편집하기 쉽게 만드는 명령입니다.

Git 저장소가 아직 없어도 `init`은 현재 디렉터리에 설정을 만들고, 저장소가 필요하다는
경고를 표준 에러로 출력합니다. 프로젝트 설정을 `git init`보다 먼저 하는 경우가 많기
때문입니다. 나머지 명령은 완료 기록을 git note로 남기므로 저장소가 생길 때까지 실패합니다.

`.planr.yaml`은 현재 디렉터리에서 위로 탐색하되 Git worktree 루트를 넘어가지 않습니다.
경로 설정은 저장소 루트를 기준으로 해석합니다.

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

설정이 없으면 아래 기본값을 사용합니다.

| 키 | 기본값 | 설명 |
| --- | --- | --- |
| `language` | `en` | 새로 생성하는 plan 문서의 언어 |
| `plans_dirs` | `[plan]` | 조회·등록·보관에 사용할 상대 경로 목록 |
| `ignore` | `[]` | `phase done`의 미커밋 소스 검사에서 제외할 glob |
| `hooks.before` | `[]` | 변경 전에 실행할 훅 규칙 |
| `hooks.after` | `[]` | 변경 후에 실행할 훅 규칙 |
| `hooks.timeout` | `10m` | 훅 하나의 최대 실행 시간 |

기존의 단일 `plans_dir`도 호환을 위해 지원하지만 새 설정에는 `plans_dirs`를 권장합니다.
적용된 설정 파일과 최종값은 다음 명령으로 확인합니다.

```sh
planr config
planr config --json
```

`config --json`은 `config_file`, `repository_root`, `agent`, `language`, `plans_dirs`,
`ignore`, `hooks`를 반환합니다. 설정 파일이 없으면 `config_file`은 `null`입니다.

## 저장 경로

- 새 plan은 `plans_dirs`의 첫 번째 경로에 등록됩니다.
- 조회 명령은 모든 경로를 검색합니다.
- `archive`는 첫 번째 경로의 완료 plan을 마지막 경로로 이동합니다.
- plan 번호는 모든 경로를 함께 스캔한 최댓값 다음으로 배정됩니다.
- `plans_dirs`의 항목은 비어 있지 않은 상대 경로여야 하며 중복될 수 없습니다.

예를 들어 위 설정에서 생성된 문서는 다음과 같습니다.

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

`ignore`는 저장소 루트 기준 glob입니다. 생성물처럼 커밋하지 않고도 phase 완료를 허용할
파일만 좁게 지정하세요. plan 문서와 `.planr.yaml`은 별도 설정 없이 완료 검사에서 제외됩니다.

## 문서 언어

`language`는 `planr`가 새로 생성하는 문서에만 영향을 줍니다.

| 값 | `planr new` | phase 문서 | `PLAN.md` 본문 |
| --- | --- | --- | --- |
| `en` | 영어 스켈레톤 | `## Planned Work`, `## Done When` | `# Shared Verification` 등 |
| `ko` | 한국어 스켈레톤 | `## 계획된 작업`, `## 완료 조건` | `# 공통 검증` 등 |

명령 출력, 옵션과 오류 메시지는 설정과 관계없이 영어입니다. `apply`는 영어와 한국어
소제목을 모두 인식하므로 서로 다른 언어로 작성된 기존 문서도 읽을 수 있습니다. 설정을
바꿔도 이미 등록된 문서는 다시 쓰지 않습니다.

## 훅

`hooks.before`와 `hooks.after`는 규칙 배열입니다. 각 규칙의 `on`에 하나 이상의 이벤트를
쓰고 `run`에 셸 명령을 지정합니다. 규칙은 작성한 순서대로 저장소 루트에서 실행됩니다.

| 이벤트 | 발생 시점 |
| --- | --- |
| `new` | 초안 생성 |
| `add` | 새 plan 등록 |
| `phase_add` | 열린 plan에 phase 추가 |
| `start` | phase를 `in-progress`로 변경 |
| `done` | phase를 `done`으로 변경 |
| `reset` | phase를 `planned`로 변경 |
| `conditional` | phase를 `conditional`로 변경 |
| `plan_done` | 모든 phase가 처음 완료되는 순간 |

- `before` 훅이 실패하면 파일이나 상태를 변경하지 않고 작업을 중단합니다.
- `after` 훅은 변경이 기록된 뒤 실행하므로 실패해도 변경을 되돌리지 않습니다.
- `timeout`은 `30s`, `5m` 같은 Go duration 형식입니다.
- 한 번의 호출에서만 모든 훅을 끄려면 `--no-hooks`를 사용합니다.

훅은 다음 환경 변수를 받습니다.

| 환경 변수 | 값 |
| --- | --- |
| `PLANR_EVENT` | 이벤트 이름 |
| `PLANR_PLAN` | 번호가 붙은 plan 디렉터리 이름 |
| `PLANR_PHASE` | phase 번호. plan 단위 이벤트에서는 빈 문자열 |
| `PLANR_STATUS` | 변경된 상태 |
| `PLANR_AGENT` | 감지한 코딩 에이전트 이름. 사람이 직접 실행하면 빈 문자열 |
| `PLANR_AGENT_SESSION` | 알 수 있는 경우 에이전트 세션 또는 thread ID |
| `PLANR_AGENT_LEVEL` | `direct` 또는 `ambient` 감지 수준 |

예를 들어 사람이 직접 완료했을 때만 알림을 보내려면 다음처럼 분기할 수 있습니다.

```sh
[ -n "$PLANR_AGENT" ] || notify-team "$PLANR_PLAN $PLANR_PHASE done"
```

감지된 값과 근거 신호는 `planr config`와 `planr doctor`의 `agent:` 항목에서 확인할 수
있습니다. 이전의 단일 이벤트 맵 형식은 지원하지 않습니다.

## 완료 기록

phase 상태와 완료 이력은 plan 문서와 Git note에 나뉘어 기록됩니다.

1. `phase done`의 완료 시각은 해당 phase의 `completed_at`에 기록되고, 전체 완료 시
   `PLAN.md`에도 기록됩니다. 완료 phase를 다시 열면 오래된 완료 시각을 제거합니다.
2. `phase start`, `phase done`, 전체 plan 완료 시 현재 `HEAD`에 `refs/notes/planr` Git
   note가 추가됩니다. 커밋 객체와 히스토리는 바꾸지 않습니다.

```text
planr plan=00-checkout-v2 event=start phase=01 at=2026-08-27T02:00:00Z
planr plan=00-checkout-v2 event=done phase=01 at=2026-08-27T02:11:40Z
planr plan=00-checkout-v2 event=plan_done at=2026-08-27T02:11:40Z
```

```sh
planr notes
planr notes checkout-v2
git notes --ref=planr show HEAD
git log --notes=refs/notes/planr
```

note 기록에 실패해도 phase 완료는 유지되고 경고가 stderr로 출력됩니다. Git notes ref는
기본 push·fetch 대상이 아니므로 팀과 공유하려면 명시적으로 동기화합니다.

```sh
git push origin refs/notes/planr
git fetch origin refs/notes/planr:refs/notes/planr
```

## 정합성 검사

```sh
planr doctor
planr doctor --json
planr doctor --fix
```

`doctor`는 설정, Git 저장소, 경로, frontmatter, 의존성, phase 파일과 `PLAN.md`
checklist의 일치를 검사합니다. JSON 결과는 `issues` 배열이며 문제가 없어도 빈 배열을
반환합니다. `--fix`는 유효한 phase 문서에서 checklist를 재구성하는 작업만 수행합니다.
