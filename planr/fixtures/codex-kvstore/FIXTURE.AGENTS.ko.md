# 작업 지침

작업은 `planr`로 계획을 세우고 그 계획을 따라 진행합니다. `planr`는 이미 `PATH`에
설치되어 있습니다.

`planr`는 하나의 작업을 여러 phase로 나누어 Markdown 문서로 관리합니다. 가능한 경우
JSON과 stdin/stdout 인터페이스를 사용해 메모리에서 처리하고, plan 문서나 `.planr.yaml`을
직접 열지 않습니다.

## 명령

```sh
planr schema # 문서 계약·상태·의존성 규칙
planr new <kebab-name> --description "200 글자 이내 짧은 설명" --json
planr apply --stdin # 완성한 Markdown을 표준 입력으로 적용
planr overview # 모든 plan 진행률
planr status # 남은 phase와 대기 중인 의존성
planr show <plan-name> [<number>] # phase 문서 하나
planr show <plan-name> --all --json # plan 전체 문서를 한 번에
planr edit <plan-name>#<number> --json # phase 하나를 메모리로 checkout
planr edit <plan-name> --section plan --json # 편집할 plan section checkout
planr new <plan-name>#<title> --json # 새 phase 초안 생성
planr phase start <plan-name> <number> # phase 착수
planr phase done <plan-name> <number> # phase 완료
```

기본 출력을 그대로 읽으십시오. `--json`과 같은 사실을 더 적은 토큰으로 담고 있습니다.
`--json`은 구조가 값을 하는 세 곳에서만 씁니다. 실패한 `apply`, `show --all`(문서 본문에
Markdown 제목이 들어 있어 필드로만 문서 경계를 나눌 수 있습니다), 그리고 읽을 요약이
아니라 다룰 문서를 돌려주는 `new`·`edit`입니다.

`new --json`은 selector와 `template` 문자열을 반환합니다. 모든 `TODO(planr)` 표시를
실제 내용으로 바꾼 뒤 그 문자열을 `planr apply --stdin`으로 보냅니다. phase 초안에는
작업, 완료 조건, `depends_on`, `status`, `entry_condition`, `perf_phase`, 편집 가능한
slug가 들어가며, phase 번호는 기존 최대 번호 다음으로 `apply`가 지정합니다.

문서를 넘길 때는 셸 따옴표 안에 넣지 말고 임시 파일에 쓴 뒤 리다이렉션합니다. 계획
본문에는 백틱·따옴표·빈 줄이 들어 있어 따옴표로 감싼 `-c` 문자열에서 깨지고, 문서
안의 명령 예시가 셸에서 실행될 수 있습니다.

```sh
cat > .planr-draft <<'PLANR'
<문서 전체>
PLANR
planr apply --stdin < .planr-draft && rm .planr-draft
```

적용한 뒤에는 임시 파일을 지웁니다. 저장소에 남은 미추적 파일은 `planr phase done`을
막습니다.

`apply`가 실패하면 `--json`을 붙입니다. 어떤 규칙을 어느 섹션·phase에서 어겼는지가
구조화된 오류로 돌아오므로 문서를 다시 읽는 것보다 빠릅니다.

`edit --json`은 checkout 문서와 `planr_base` hash를 반환합니다. frontmatter의 식별
필드를 보존하고 수정한 `document`를 `planr apply --stdin`으로 보냅니다. 그동안 다른
명령이 대상 문서를 바꿨으면 적용이 거부되므로 다시 checkout합니다. phase 상태 변경은
`phase start`, `phase done`, `phase reset`, `phase set`만 사용합니다.

plan 초안에는 `GOALS`, `SCOPE`, `CONTEXT`, `PHASES`, `VERIFICATION`, `ORDERING`, `NEXT`가
순서대로 있습니다. 각 phase는 제목 뒤 YAML 펜스에 `phase`, `slug`, `status`,
`depends_on`을 적고, 초안에 있는 제목 아래 작업과 완료 조건을 채웁니다. `new`가 반환한
구조를 따르며, `apply --json`은 규칙·section·phase·줄 번호가 있는 검증 오류를
반환합니다.

## 작업 흐름

1. 요청과 기존 코드·테스트를 먼저 읽고 필요한 일을 파악합니다.
2. `new --json`으로 plan을 만들고 template을 채운 뒤 stdin으로 적용합니다. 각 phase를
   독립적으로 검증할 수 있는 단위로 나눕니다.
3. `overview`, `status`, `show`로 결과를 확인합니다.
4. phase마다 `planr phase start` → 구현 → 검증(테스트) → **변경 사항 커밋** →
   `planr phase done`을 반복합니다.
5. 계획이 실제 작업과 달라지면 `new plan#title`로 phase 초안을 만들거나, `edit`로 해당
   phase/section을 checkout해 수정하고 적용합니다.
6. 모든 phase가 끝나면 `overview`로 모두 `done`인지 확인합니다.

## 규칙

- `planr phase done`은 커밋되지 않은 소스 변경이 있으면 실패합니다. 먼저 커밋하고
  `--force`로 우회하지 않습니다.
- `planr phase start`와 `planr phase done`은 선행 phase가 `done`이 아니면 실패합니다.
  계획한 순서대로 작업하고 `--force`로 우회하지 않습니다.
- plan 문서와 코드·테스트를 함께 최신 상태로 유지합니다. plan만 고치거나 구현만 하고
  phase 상태를 옮기지 않는 것은 허용되지 않습니다.
- phase checklist와 `plan_status`는 derived 값입니다. section checkout의 checklist는
  보호된 marker로 나타나므로 marker를 바꾸지 않습니다.
