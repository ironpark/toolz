# Goal: add JSON output to the greeting CLI

이 저장소의 작은 greeting CLI에 JSON 출력 모드를 추가한다. 기존 기본 동작은
계속 유지하고, 사용자가 결과를 스크립트에서 안정적으로 소비할 수 있어야 한다.

## Acceptance criteria

- `go run . --name Ada`는 기존과 같이 `Hello, Ada!`를 출력한다.
- `go run . --name Ada --format json`은 유효한 JSON 한 줄을 출력하며, 최소한
  `message` 문자열 필드에 `Hello, Ada!`를 담는다.
- 지원하지 않는 `--format` 값은 0이 아닌 종료 코드와 이해하기 쉬운 오류를 낸다.
- text/json 각 모드와 잘못된 format을 검증하는 단위 테스트가 있다.
- `README.md`에 두 출력 모드와 사용 예시가 문서화되어 있다.
- 마지막에는 `go test ./...`가 통과하고, planr의 모든 phase가 완료되어야 한다.

## Constraints

- 외부 Go 모듈을 추가하지 않는다.
- 기본 text 출력의 호환성을 깨지 않는다.
- 구현 세부사항보다 acceptance criteria를 우선한다.
