# 작업 지침

작업은 `planr`로 계획을 세우고 그 계획을 따라 진행합니다. `planr`는 이미 `PATH`에
설치되어 있습니다.

`planr`는 하나의 작업을 여러 phase로 나눠 Markdown 문서로 관리하는 CLI입니다. 무엇을
어떤 순서로 할지 먼저 적어 두고, 각 phase를 시작·완료할 때마다 상태를 갱신해 진행
상황이 문서에 남게 합니다.

## 명령

| 명령 | 용도 |
| --- | --- |
| `planr new <kebab-name> --description "짧은 설명"` | 계획 초안 파일 생성 |
| `planr add <draft-file>` | 초안을 검증하고 plan으로 등록 |
| `planr overview` | 모든 plan의 진행률 요약 |
| `planr status` | 남은 phase와 대기 중인 의존성 상세 |
| `planr phase start <plan-name> <number>` | phase 착수 |
| `planr phase done <plan-name> <number>` | phase 완료 |
| `planr phase add <plan-name> <title> --work "..." --done-when "..."` | 진행 중 plan에 phase 추가 |

`planr new`가 만든 초안에는 `GOALS`, `SCOPE`, `CONTEXT`, `PHASES`, `VERIFICATION`,
`ORDERING`, `NEXT` 섹션이 순서대로 있습니다. 각 phase는 제목 뒤 YAML 펜스에 `phase`,
`slug`, `status`, `depends_on`을 적고 `계획된 작업`과 `완료 조건`을 채웁니다. 초안
파일의 기존 구조를 그대로 따르면 되고, 형식이 어긋나면 `planr add`가 무엇이 잘못됐는지
알려 주며 등록을 거부합니다.

## 작업 흐름

1. 요청과 기존 코드·테스트를 먼저 읽고 무엇이 필요한지 파악합니다.
2. 코드를 고치기 전에 `planr new`로 초안을 만들고, 목표·검증 방법·phase를 실제 작업에
   맞게 채운 뒤 `planr add`로 등록합니다. phase는 각각 독립적으로 검증할 수 있는
   단위로 나눕니다.
3. `planr overview`로 등록 결과를 확인합니다.
4. phase마다 다음을 반복합니다.
   `planr phase start` → 구현 → 검증(테스트) → **변경 사항 커밋** → `planr phase done`
5. 진행 중 계획이 어긋나면 `planr phase add`로 phase를 추가하거나 계획 문서를 고쳐
   실제 작업과 맞춥니다.
6. 모든 phase가 끝나면 `planr overview`로 전부 `done`인지 확인합니다.

## 규칙

- `planr phase done`은 커밋되지 않은 변경이 있으면 실패합니다. 먼저 커밋하세요.
  `--force`로 이 검사를 우회하지 않습니다.
- 계획 문서와 코드·테스트를 함께 최신 상태로 유지합니다. 계획만 갱신하고 구현이
  없거나, 구현만 하고 phase 상태가 그대로면 안 됩니다.
- plan 문서와 `.planr.yaml`은 `planr` 명령으로 갱신합니다. 상태를 손으로 고쳐 맞추지
  않습니다.
