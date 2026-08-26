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

## 초안 형식과 상태

초안에는 `GOALS`, `SCOPE`, `CONTEXT`, `PHASES`, `VERIFICATION`, `ORDERING`,
`NEXT` 섹션이 순서대로 필요합니다. 각 phase에는 YAML frontmatter로 번호, slug,
상태, 의존성을 정의합니다.

`status`는 완료된 plan을 기본적으로 숨깁니다. 진행 중 plan의 phase가 완료된 plan을
의존하면, 그 완료 plan은 함께 표시합니다. plan 이름을 지정해 조회하면 완료 여부와
관계없이 표시합니다.
