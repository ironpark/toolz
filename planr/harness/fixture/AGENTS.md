# 저장소 작업 지침

이번 작업의 제품 요구사항은 저장소에 없습니다. 대화의 첫 메시지로 전달된 목표가
유일한 기준이며, turn이 바뀌어도 그 메시지를 다시 확인해 판단합니다.

## 작업 규칙

1. 먼저 `AGENT.md`, 첫 메시지의 목표, 기존 소스와 테스트를 읽습니다.
2. 코드를 수정하기 전에 `planr new <kebab-name> --description "짧은 설명"`으로 초안을
   만들고, 초안의 목표·검증·phase를 실제 작업에 맞게 채웁니다. 최소 세 개의 phase를
   정의한 뒤 `planr add <draft-file>`로 등록합니다.
3. 작업 전후에 `planr status`와 `planr overview`를 실행해 계획을 확인합니다.
4. 각 phase를 시작할 때 `planr phase start <plan-name> <phase-number>`를 사용하고,
   구현과 검증이 끝난 뒤 `planr phase done <plan-name> <phase-number>`를 사용합니다.
   `phase done` 전에 소스 변경을 커밋해야 하며, `--force`로 검사를 우회하지 않습니다.
5. 계획 문서와 소스·테스트를 함께 갱신하고, 모든 acceptance criteria와 테스트를
   만족한 뒤 모든 phase가 `done`인지 확인합니다. plan이 완료되기 전에는 제안만
   남기고 멈추지 않습니다.
6. 한 turn에서 작업이 끝나지 않으면 다음 turn에서 같은 작업을 자율적으로 이어갑니다.
   질문하거나 사용자 승인을 기다리지 말고, 현재 저장소에서 확인 가능한 근거로
   판단합니다.

## 격리 경계

- 이 저장소 안의 소스, 테스트, plan 문서만 수정합니다.
- `AGENT.md`, `AGENTS.md`와 `.harness/` 로그는 수정하지 않습니다.
- 외부 의존성을 추가하지 말고 표준 Go 도구와 이미 제공된 `planr` 바이너리를
  우선 사용합니다.
