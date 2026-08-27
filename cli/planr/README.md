# planr

규격화된 Markdown 계획을 저장하고 phase 단위로 진행 상태를 관리하는 Go CLI입니다.

`planr`는 하나의 초안을 `GOALS.md`, `CONTEXT.md`, `PLAN.md`, `phases/*.md`로 나누어
등록합니다. 이후 phase의 시작·완료, 의존성 대기, 전체 진행률과 완료 커밋을 같은
문서 집합에서 추적할 수 있습니다.

## 설치

Go 1.26.3 이상이 필요합니다.

```sh
go install github.com/ironpark/toolz/cli/planr@latest
```

설치 후 `go env GOPATH`의 `bin` 디렉터리가 `PATH`에 포함되어 있는지 확인합니다.

> [!IMPORTANT]
> plan 조회·변경 명령은 Git 저장소 안에서 실행해야 합니다. `config`, `doctor`,
> `completion`, `schema`는 진단을 위해 저장소 밖에서도 실행할 수 있습니다.

## 빠른 시작

Git 저장소 루트에서 다음 순서로 시작합니다.

```sh
# 1. 규격에 맞는 초안을 생성합니다.
planr new checkout-v2 --description "checkout flow refresh"

# 2. 출력된 Markdown 파일에 목표, phase, 검증 조건을 작성합니다.

# 3. 초안을 검증하고 plan으로 등록합니다.
planr apply checkout-v2.md

# 4. 다음 작업과 전체 진행률을 확인합니다.
planr show checkout-v2
planr overview

# 5. phase 상태를 변경합니다.
planr phase start checkout-v2 0
planr phase done checkout-v2 0
```

등록 결과는 기본적으로 다음과 같습니다.

```text
plan/
└── 00-checkout-v2/
    ├── GOALS.md
    ├── CONTEXT.md
    ├── PLAN.md
    └── phases/
        ├── 00-foundation.md
        └── 01-checkout-ui.md
```

phase 상태는 `planned`, `conditional`, `in-progress`, `done` 중 하나입니다. 마지막
phase가 완료되면 `PLAN.md`의 plan 상태도 자동으로 `done`이 됩니다.

## 자주 쓰는 명령

| 목적 | 명령 |
| --- | --- |
| 새 plan 초안 생성 | `planr new <plan> --description "..."` |
| 초안 검증·등록 | `planr apply <file>` |
| 다음 phase 확인 | `planr show <plan>` |
| 진행 중 plan 상세 조회 | `planr status [plan]` |
| 모든 plan 요약 | `planr overview [plan]` |
| phase 시작·완료 | `planr phase start\|done <plan> <number>` |
| 기존 문서 편집 | `planr edit <plan>#<number>` 또는 `planr edit <plan> --section <section>` |
| 설정과 문서 진단 | `planr doctor` |
| 완료 기록 조회 | `planr notes [plan]` |

plan 이름은 `checkout-v2`와 `00-checkout-v2` 형식을 모두 인식합니다. 자동화에서는 대부분의
조회·작성 명령에 제공되는 `--json`을 사용하고, 파일을 만들지 않으려면 `new`/`edit --json`과
`apply --stdin --json`을 조합할 수 있습니다.

## 설정 예시

저장소 루트의 `.planr.yaml`에서 문서 언어, 활성·보관 경로, 완료 검사 제외 경로와 훅을
설정합니다. 설정이 없으면 영어 문서를 `plan/`에 저장합니다.

```yaml
language: ko
plans_dirs:
  - plans-active
  - plans-archive
ignore:
  - generated/**
hooks:
  before:
    - on: [done]
      run: "go test ./..."
```

실제로 적용된 값은 `planr config`, 설정과 plan 문서의 정합성은 `planr doctor`로
확인할 수 있습니다.

## 문서

- [명령어 가이드](docs/commands.md) — 생성, 편집, 조회, phase 관리, 보관과 셸 자동 완성
- [설정과 저장 방식](docs/configuration.md) — `.planr.yaml`, 훅, 저장 경로와 Git note
- [plan 문서 규격](docs/document-format.md) — 초안 frontmatter, 섹션, phase와 의존성
- [개발 및 평가](docs/development.md) — 테스트, 시나리오 실행기와 Codex 평가 하네스

문서 전체의 짧은 색인은 [`docs/README.md`](docs/README.md)에서 볼 수 있습니다. CLI가
인식하는 최신 문서 계약이 필요하면 `planr schema` 또는 `planr schema --json`을 실행하세요.
