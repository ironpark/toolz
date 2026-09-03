# 전송과 호출

[← 문서 홈](../README.md)

TrueNAS 25.10은 WebSocket 위의 JSON-RPC 2.0을 사용한다. 기본 엔드포인트는
`wss://<host>/api/current`다. 과거 REST API와 `/websocket` 레거시 프로토콜은
새 코드에서 사용하지 않는다. 한 메시지에는 한 호출만 담으며 배치는 지원하지 않는다.

```json
{"jsonrpc":"2.0","id":1,"method":"system.info","params":[]}
```

- `params`는 배열이다. 객체 인자 하나도 배열 안에 넣는다.
- 고유한 `id`로 동시 호출과 응답을 매칭한다.
- 성공 응답은 같은 `id`와 `result`, 실패 응답은 `id`와 `error`를 가진다.
- 인증된 WebSocket 하나를 재사용하고 연결 종료를 모든 대기 호출에 전파한다.
- 연결 후 `system.info`로 실제 서버 버전을 확인한다.

공식 문서: [JSON-RPC](https://api.truenas.com/v25.10/jsonrpc.html)
