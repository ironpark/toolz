# Paseo Relay 프로토콜

이 문서는 `references/paseo-relay`의 현재 Elixir 구현을 분석해 정리한
WebSocket 공개 동작 계약이다. 구현 자체도 “internal protocols may change”라고
명시하므로, 호환 클라이언트는 배포본/소스의 변경을 함께 확인해야 한다. 이 문서의
범위는 릴레이가 관찰하거나 생성하는 메타데이터와 전송 동작이며, 애플리케이션
페이로드의 E2EE 형식은 정의하지 않는다.

근거가 되는 주 구현 파일은
`references/paseo-relay/lib/paseo_relay/{connection,socket,ownership,handshake_validation,protocol,capacity}.ex`와
`delivery/writer.ex`이며, 동작 예시는 `test/relay_protocol_test.exs`에 있다.

마지막 절 “sanbo(Go) 구현 차이”는 이 계약을 기준으로 삼되 현재 Go 릴레이가
다르게 동작하는 지점을 따로 모아 둔 것이다.

## 전송과 업그레이드

- 엔드포인트: `GET /ws` WebSocket upgrade (HTTP/1 전용, `protocols: [:http]`).
- `/ws` 이외의 경로는 WebSocket이 아니라 운영 엔드포인트로 처리된다. method와
  무관하게 `/health`는 `200 {"status":"ok"}`, `/ready`는 `200 {"status":"ready"}`
  또는 `503 {"status":"unready"}`, `/metrics`는 Prometheus 텍스트를 반환하고,
  그 외 경로는 `404 not found`다.
- upgrade가 완료되기 전까지의 HTTP 요청에는 `PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS`
  (기본 15초)가 적용된다. upgrade 이후 WebSocket에는 idle timeout이 없다.

`/ws` 요청은 다음 순서로 판정되며, 앞선 단계에서 실패하면 뒤 단계는 평가되지 않는다.

1. WebSocket upgrade 요청인가 → 아니면 `426 Expected WebSocket upgrade`.
   query가 잘못되어 있어도 upgrade가 아니면 `400`이 아니라 `426`이다.
2. query 파라미터 검증 → 실패 시 `400`과 아래의 오류 문자열.
3. 소유권 조회 → 다른 노드가 소유하면 `409`와 구성된 reroute 헤더
   (기본 `x-reroute-target`). 응답 본문은 비어 있다. 클라이언트나 앞단 프록시는
   원래 upgrade 요청을 그 대상에 재시도해야 한다.
4. 연결 budget admission → 실패 시 `503`.
5. WebSocket upgrade → 실패하면 연결 budget을 되돌리고 소유권을 건드리지 않는다.
6. 소유자 확보 → upgrade 성공 뒤 원자적으로 claim한다. 이때 claim에 실패하거나
   다른 노드가 소유권을 가져갔다면 이미 열린 소켓을 `1012 Session expired`로
   닫는다.

즉 reroute(`409`) 판정은 연결 용량 검사보다 **먼저** 일어나므로, 노드가 포화
상태여도 다른 노드 소유 세션은 `503`이 아니라 `409`로 안내된다.

### Prometheus 메트릭

`/metrics`는 method와 무관하게 참조 구현과 같은 17개 일반 metric family의 HELP/TYPE와
handshake counter family를 포함한다. delivery 대기 시간 bucket은
`0.001`, `0.01`, `0.1`, `1`, `10`초이고, frame 크기 bucket은 `1024`,
`65536`, `1048576`, `8388608`, `33554418`바이트이며 모두 누적값이다.
Capacity 상태를 사용할 수 없으면 다음 네 gauge family를 응답에서 완전히
생략한다: `active_websockets`, `ingress_reserved_bytes`,
`inflight_delivery_bytes`, `backpressured_sources`.

이 릴레이 코드에는 TLS, Origin, Authorization/Bearer token, Cookie를 검증하는
인증·인가 단계가 없다. 그런 보호가 필요하면 TLS 종료/인증 프록시 등 외부 계층이
맡아야 한다.

### Query string

파라미터는 **`role` → `serverId` → `v` → `connectionId`** 순으로 검증되며, 첫
실패의 메시지만 반환된다. 예를 들어 `role`과 `serverId`가 모두 잘못되면 응답은
`Missing or invalid role parameter`다.

| 이름 | 필수 | 허용값과 처리 |
| --- | --- | --- |
| `serverId` | 예 | 1–256 **bytes** 문자열. 누락 시 `Missing serverId parameter`, 초과 시 `serverId is too long`. 동일 세션을 묶는 라우팅 키다. trim하지 않는다. |
| `role` | 예 | 정확히 `server` 또는 `client`. 그 외/누락은 `Missing or invalid role parameter`. |
| `v` | 아니오 | trim한 뒤 판정한다. 생략, 빈 문자열, `1`은 v1. `2`는 v2. 그 외는 `Invalid v parameter (expected 1 or 2)`. |
| `connectionId` | v2에서 역할별 | v1에서는 무시된다. v2에서는 공백을 trim한 뒤 최대 256 bytes. 초과 시 `connectionId is too long`. |

query 파싱 세부:

- 값이 없는 파라미터(`?v`처럼 `=`가 없는 형태)는 빈 문자열로 취급된다.
- 같은 키가 여러 번 오면 **마지막 값**이 쓰인다.
- 길이 상한은 문자 수가 아니라 UTF-8 byte 수다.

v2의 `client`가 `connectionId`를 생략하거나 빈 값으로 보내면 릴레이가
`conn_` + 암호학적 난수 8 bytes의 소문자 hex(16자) ID를 내부 생성한다. 이 값은
HTTP upgrade 응답으로 돌려주지 않으므로, 상호 연결이 필요한 클라이언트는 보통
명시적인 `connectionId`를 보내야 한다. v2 `server`의 빈 `connectionId`는 유효하며
제어 소켓을 뜻한다. 길이 검사는 생성보다 먼저 수행되므로, 256 bytes를 넘는
`connectionId`는 client라도 생성으로 대체되지 않고 `400`이 된다.

예:

```text
ws://relay.example/ws?serverId=daemon-a&role=server&v=2
ws://relay.example/ws?serverId=daemon-a&role=server&v=2&connectionId=client-17
ws://relay.example/ws?serverId=daemon-a&role=client&v=2&connectionId=client-17
```

## 버전별 라우팅

### v1

각 `serverId`에는 `server`와 `client`가 각각 하나씩만 현재 연결로 존재한다.

- `client`의 text/binary 메시지는 `server`로, `server`의 것은 `client`로 전달된다.
- WebSocket opcode(text/binary)와 payload bytes는 바꾸지 않는다.
- 같은 역할이 새로 연결되면 기존 소켓은 `1008 Replaced by new connection`으로
  닫힌다.
- 상대 역할이 아직 붙지 않았으면 대기하지 않는다. 목적지 목록이 비어 있는
  전달은 **성공으로 처리되고 메시지는 조용히 버려진다.** v1에는 버퍼링이 없다.

### v2

v2는 하나의 `serverId` 아래에서 control, data, client route를 분리한다.

| 접속 | 의미 | 전달 대상 |
| --- | --- | --- |
| `role=server`, 빈 `connectionId` | daemon control 소켓 | 제어 JSON 수신 |
| `role=server`, 비어 있지 않은 `connectionId` | 해당 client route의 daemon data 소켓 | 같은 ID의 모든 client |
| `role=client`, `connectionId=id` | client route | `id`의 daemon data 소켓 |

하나의 `connectionId`에는 client WebSocket이 여러 개 붙을 수 있다.

**client → data**: data 소켓이 아직 붙지 않았다면 source마다 한 메시지만 in-flight로
둔 채 붙기를 기다린다. 릴레이 내부에 제한 없는 route별 프레임 버퍼는 두지 않으며,
source의 다음 프레임은 읽기 루프가 대기를 끝낼 때까지 TCP 수신 쪽에 남는다. 대기
한도는 `min(남은 delivery deadline, PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS)`이며, 기본값
기준으로는 15초다. data가 도착하면 대기 중이던 메시지가 입력 순서대로 전달된다.
대기 한도가 지나면 source client는 `1013 Data route unavailable`으로 닫힌다.
대기 시작 시점에 delivery deadline이 이미 소진되어 있으면 대기하지 않고
`1013 Delivery unavailable`이 된다.

**data → client**: 대기하지 않는다. 그 `connectionId`에 붙어 있는 client 전원에게
fan-out되며, client가 하나도 없으면 목적지 없는 전달로서 성공 처리되고 메시지는
버려진다.

fan-out 시 목적지 중 **하나라도** 전달에 성공하면 source 입장에서는 성공이다.
느린 client 하나가 `1013 Slow consumer`로 잘려도 다른 목적지가 받았다면 source는
닫히지 않는다. 모든 목적지가 실패해야 source가 `1013 Delivery unavailable`로 닫힌다.

동일한 v2 control 또는 동일 ID의 data 소켓을 새로 연결하면 이전 소켓은
`1008 Replaced by new connection`으로 닫힌다. client는 같은 ID에 여러 개가
공존하므로 교체되지 않는다. 마지막 client가 route에서 떠나면 해당 data 소켓은
`1001 Client disconnected`으로 닫힌다. data 소켓이 사라지면 그 ID의 client들은
`1012 Server disconnected`으로 닫힌다.

## v2 control 메시지

control 메시지는 모두 **text WebSocket frame의 JSON**이다. outbound 알림은 다음과
같다.

| 시점 | JSON |
| --- | --- |
| control 접속 | `{"type":"sync","connectionIds":[...]}` |
| client route에 client가 붙을 때마다 | `{"type":"connected","connectionId":"id"}` |
| route의 마지막 client 연결 해제 | `{"type":"disconnected","connectionId":"id"}` |
| client 접속 10초 뒤에도 data 없음 | `sync` 재전송 |

`connected`는 **client 소켓이 붙을 때마다** 발송된다. 같은 `connectionId`에 client
두 개가 붙으면 같은 ID로 `connected`가 두 번 온다. 반면 `disconnected`는 그 route의
마지막 client가 떠날 때 한 번만 발송된다. 따라서 두 알림은 짝이 맞지 않으며,
control 소켓은 `connected` 수를 세는 방식으로 client 수를 추적해서는 안 된다.

`connectionIds`는 **client가 하나 이상 붙어 있는 route ID 목록**이며 data 소켓의
ID 목록이 아니다. 배열 순서는 구현상 map key 순서이므로 계약된 순서가 아니다.
control 접속 직후의 `sync`는 그 시점에 이미 존재하던 route를 포함한다.

client가 붙고 10초가 지나도 그 route에 data가 없으면 control로 `sync`를 다시 보낸다.
그 `sync` 후에도 5초 동안 data가 붙지 않으면 control 소켓은
`1011 Control unresponsive`으로 닫힌다. 두 검사 모두 “해당 route에 client는 있고
data는 없다”를 재확인한 뒤에만 동작하므로, 그 사이 data가 붙었으면 아무 일도
일어나지 않는다.

inbound로 의미 있게 처리하는 것은 text JSON의 `{"type":"ping"}`뿐이다
(추가 필드는 허용). 응답은 같은 control 소켓으로 다음을 보낸다.

```json
{"type":"pong","ts":1720000000000}
```

`ts`는 밀리초 Unix epoch 정수다. 그 외 text payload는 무시되고, control 소켓의
**binary frame은 아무 처리 없이 버려진다**(frame 관찰 counter와 ingress 예산도
소비하지 않는다).
표준 WebSocket ping/pong/close control frame은 Cowboy WebSocket 계층이 처리하며,
릴레이 애플리케이션은 별도의 의미를 부여하지 않는다.

JSON ping의 pong도 control delivery queue를 통해 전송되므로
`PASEO_RELAY_CONTROL_QUEUE_BYTES`의 제한을 받는다. queue 또는 delivery가 실패하면
control 소켓을 `1013 Delivery unavailable`으로 닫는다.

control 소켓은 `role=server`이므로 아래의 client handshake 검사 대상이 아니다.

## 프레임, 크기, 순서

- 압축은 비활성화되어 있다 (`permessage-deflate` 없음).
- 릴레이에 의한 idle WebSocket timeout은 없다.
- 일반 data 메시지의 호환 상한은 masked client wire frame 32 MiB이다. 최대 14-byte
  client header를 제외한 payload/message 상한은 **33,554,418 bytes**
  (`32 * 1024 * 1024 - 14`)이다.
- Cowboy는 이 상한을 개별 frame과 재조립한 fragmented message에 적용하며, 초과는
  `1009` close가 된다.
- v2 control 소켓의 inbound payload 상한은 **65,536 bytes**다.
- text/binary의 완성 메시지만 릴레이가 처리하며, 조각난 메시지는 Cowboy가 먼저
  재조립한다. 전달 시 opcode와 bytes가 보존된다.

### ingress 예산

frame 크기 상한과 **별개로** 노드 단위 ingress 예산이 있다. 메시지 하나는
`payload_bytes × PASEO_RELAY_INGRESS_WEIGHT`(기본 4)만큼 예산을 점유하며, 전달이
끝나면 반환된다. 다음 두 경우 모두 source가 `1013 Relay ingress capacity`로 닫힌다.

- 가중치를 적용한 크기가 예산 총량(`PASEO_RELAY_INGRESS_BUDGET_BYTES`, 기본 512 MiB)
  자체를 넘는 경우 — 노드가 비어 있어도 항상 실패한다.
- 현재 점유량과 합쳐 예산을 넘는 경우 — 일시적이며 재연결 후 성공할 수 있다.

기본값에서는 가중 상한 128 MiB가 frame 상한 33,554,418 bytes보다 크므로 frame
상한이 먼저 걸린다. 예산을 낮추거나 가중치를 올리면 **frame 상한 이내의 메시지도
거부될 수 있다.** v2 control 소켓의 text frame도 같은 예산을 소비한다.

### 순서 보장

source별로 한 번에 하나의 메시지만 delivery 단계에 들어가고, 목적지 Writer가
전송 완료 barrier를 확인할 때까지 source 읽기를 중단한다. 이후 대기열을 입력
순서대로 재개한다. 목적지별 Writer도 한 번에 하나의 write만 수행한다. 이 규칙은
source별 순서를 보장하지만, 서로 다른 source 간 전역 순서를 보장하지는 않는다.

## 클라이언트 handshake 검사

릴레이는 payload를 복호화하지 않는다. 단, **client role**이 보내는 text 또는 binary
frame을 JSON으로 해석할 수 있고 top-level이 JSON **object**이며 `type`이 `hello`
또는 `e2ee_hello`이면 `key`만 sanity check한다. 이것은 인증이 아니라 잘못된 X25519
공개키의 조기 차단이다.

`key`는 다음을 모두 만족해야 한다.

1. canonical padded Base64이고 decode 후 정확히 32 bytes여야 한다.
2. 다시 Base64로 encode했을 때 원 문자열과 같아야 한다.
3. 32 bytes를 little-endian X25519 coordinate로 해석했을 때 `2^255 - 19`보다 작아야 한다.
4. 구현이 거부하는 7개의 low-order/unsupported public-key encoding 중 하나가 아니어야 한다.

`key`가 문자열이 아니면(누락, `null`, 숫자 등) 그대로 무효다.

유효한 handshake는 원문 그대로 전달된다. `type`은 일치하지만 `key`가 없거나
유효하지 않으면 전달하지 않고 source를 `1008 Invalid handshake key`로 닫는다.
JSON이 아니거나, JSON이지만 object가 아니거나(배열·문자열·숫자), object이지만
`type`이 다르면 불투명 payload로 취급해 그대로 전달한다. `capabilities` 등 다른
handshake 필드는 검사하지 않는다.

## 메모리 pressure

`PASEO_RELAY_MEMORY_WATERMARK_BYTES`가 `0`이 아니면 릴레이는 BEAM 총 메모리를
watermark와 비교한다. watermark에 도달하면 pressure 상태로 들어가고, **한 번에
전부가 아니라 batch 단위로 점진적으로** 소켓을 골라 `1013 Relay memory pressure`로
닫는다. batch 크기는 직전 shedding이 회수한 메모리에 따라 조정되며, 회수가 없으면
배로 늘어난다.

pressure는 메모리가 `watermark - 최대 메시지 크기`(recovery threshold) 이하로
내려갈 때까지 유지된다. 그 동안:

- 새 upgrade는 `503 Relay memory pressure`로 거부된다.
- 이미 붙어 있는 소켓이 보내는 메시지는 admission 단계에서 막혀
  `1013 Relay ingress capacity`가 된다.
- delivery 시작 단계에서 막히면 `1013 Relay memory pressure`가 된다.

즉 메모리 pressure는 단계에 따라 서로 다른 close reason으로 나타난다.

각 WebSocket 소켓 프로세스에는 `PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS`(기본
33,554,432 words) 힙 상한이 `kill: true`로 걸려 있다. 이 상한을 넘긴 소켓은
close frame 없이 종료되므로, 클라이언트는 정상 close code 없는 연결 종료도
처리할 수 있어야 한다. Go 구현이 이 상한을 어떻게 재는지는 아래
“소켓별 힙 상한의 등가 구현”을 참고한다.

## 실패와 close code

`1013`은 대체로 일시적 과부하/전달 불능이며 릴레이는 자동 재시도를 하지 않는다.
클라이언트는 새 WebSocket 연결을 시도해야 한다.

### HTTP 응답

| 코드 | 본문 | 발생 조건 |
| --- | --- | --- |
| 400 | 위 query 오류 문자열 | 잘못된 route query |
| 409 | 빈 본문 + reroute header | 원격 노드가 `serverId` 소유 |
| 426 | `Expected WebSocket upgrade` | WebSocket upgrade가 아님 |
| 503 | `draining` | 노드가 drain 중 |
| 503 | `cluster` | 활성 노드 수 < `PASEO_RELAY_MIN_CLUSTER_SIZE` |
| 503 | `owner` | session owner 프로세스 확보/예약 실패 |
| 503 | `Relay connection capacity` | 노드 WebSocket 연결 상한 도달 |
| 503 | `Relay memory pressure` | 메모리 pressure 중 |
| 503 | `Relay capacity configuration` | 연결 budget 설정 불일치 |
| 503 | `Relay capacity unavailable` | capacity 프로세스에 접근 불가 |

`draining`/`cluster`/`owner`는 내부 atom을 그대로 문자열로 쓴 것이라 다른 항목과
대소문자 규칙이 다르다. 클라이언트는 본문 문자열을 파싱하기보다 상태 코드로
판단하는 편이 안전하다.

### WebSocket close code

| 코드 | reason | 닫히는 쪽 | 발생 조건 |
| --- | --- | --- | --- |
| 1008 | `Invalid handshake key` | source | client `hello`/`e2ee_hello` key가 무효 |
| 1008 | `Replaced by new connection` | 기존 소켓 | v1 동일 역할 또는 v2 중복 control/data 교체 |
| 1001 | `Client disconnected` | data 소켓 | v2 route의 마지막 client가 해제됨 |
| 1011 | `Control unresponsive` | control 소켓 | `sync` 재전송 후 5초간 data 미부착 |
| 1012 | `Session expired` | 신규 소켓 | upgrade 뒤 capacity attach/writer 시작/owner attach 실패 |
| 1012 | `Session owner moved` | 부착된 소켓 전부 | session owner 프로세스 종료/이동 |
| 1012 | `Server disconnected` | client 소켓 | 대응 v2 data 소켓 해제 |
| 1013 | `Data route unavailable` | source client | client → data attach 대기 한도 초과 |
| 1013 | `Delivery unavailable` | source | 모든 목적지 전달 실패, owner/writer 종료, control pong 전송 실패 |
| 1013 | `Slow consumer` | **목적지** | 목적지 write deadline 초과 또는 control queue(`PASEO_RELAY_CONTROL_QUEUE_BYTES`) 초과 |
| 1013 | `Relay ingress capacity` | source | ingress 예산 초과 또는 pressure 중 메시지 admission 실패 |
| 1013 | `Relay memory pressure` | 선정된 소켓 | 메모리 pressure shedding, delivery 시작 실패 |
| 1013 | `Relay capacity unavailable` | 부착된 소켓 | capacity 프로세스 종료 감지 |
| 1009 | 구현 의존 reason | 초과한 쪽 | payload 크기 상한 초과 |
| (없음) | — | 해당 소켓 | 소켓 프로세스 힙 상한 초과로 강제 종료 |

`Slow consumer`는 **느린 목적지 소켓**을 닫는 것이지 source를 닫는 것이 아니다.
source는 모든 목적지가 실패했을 때만 `Delivery unavailable`을 받는다.

session owner에 대한 내부 호출은 5초 타임아웃이며, 초과하면 owner 프로세스를
강제 종료한다. 그 결과 그 세션에 붙어 있던 모든 소켓이
`1012 Session owner moved`를 받는다.

reference의 소유권은 Syn registry scope와 DNSCluster가 관리한다. 네트워크 분할 중
동일 `serverId`의 owner가 양쪽에 잠시 생길 수 있으며, registry가 수렴하면 진 쪽의
소켓은 `1012`로 닫혀 재연결이 필요하다. 기존 WebSocket의 투명한 노드 간
migration/forwarding은 없다.

sanbo의 ownership backend는 이 분산 registry를 구현하지 않는다.
`PASEO_RELAY_CLUSTER_QUERY`, `RELEASE_NODE`, `RELEASE_COOKIE`가 모두 설정된 경우에만
query+cookie로 정한 OS 임시 디렉터리의 locked file-lease registry를 사용하고,
100ms heartbeat와 750ms lease로 동일 호스트 프로세스의 member/owner liveness를
판단한다. 설정이 빠지면 process-local registry를 사용한다. 따라서 cluster query는
DNS 조회가 아니며, 서로 다른 호스트 사이의 reference peer discovery, 분산
membership, cross-host ownership convergence는 알려진 미지원 범위다.

## 비목표와 구현 주의점

- relay는 payload의 E2EE 내용을 해석·저장·재암호화하지 않는다.
- relay는 메시지 또는 연결을 자체 재시도하지 않는다.
- 내부 owner 예약은 5초, 비어 있는 owner의 idle 종료는 30초, owner 호출 타임아웃은
  5초다. 이들은 server 내부 lifecycle 값이지 peer가 의존해야 할 wire negotiation은
  아니다.
- delivery 전체에는 기본 30초의 단일 deadline이 적용된다. owner 조회, data attach,
  Writer reservation, write barrier가 모두 이 deadline을 공유한다. 따라서 data attach
  대기는 `PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS`만으로 결정되지 않고 남은 deadline에
  의해 더 짧아질 수 있다.

## sanbo(Go) 구현 차이

이 저장소의 Go 릴레이는 위 계약을 목표로 하지만 현재 다음 지점이 다르다. 각 항목은
Go 소스 또는 실제 실행으로 확인한 것이다.

| 항목 | 참조(Elixir) | sanbo(Go) |
| --- | --- | --- |
| `connectionId`당 client roster | map key 순서 | `sync`의 `connectionIds`를 정렬해 보낸다. 계약이 순서를 보장하지 않으므로 호환 범위 안이다 |
| 소켓별 힙 상한 | BEAM 소켓 프로세스의 `max_heap_size` (`kill: true`) | 소켓별 **메모리 회계**로 등가 구현. 아래 참고 |
| ownership registry | Syn scope + DNSCluster peer discovery | process-local registry 또는 동일 호스트/shared filesystem file-lease registry. cross-host distributed clustering은 미지원 |
| 503 본문 | 위 표의 7종 | `draining`, `cluster ownership unavailable`, `relay capacity unavailable` 3종 |

client fan-out, control 초기 `sync` roster, 10초/5초 control watchdog, handshake
좌표 범위 검사, batch 단위 메모리 pressure shedding, 단일 보유 role의 소켓 교체,
data 소켓 해제 시 client 정리, control queue 상한, pressure 중 admission/delivery
거부, shedding 희생자 선정 순서, 런타임 drain은 계약대로 동작한다.

### 소켓별 힙 상한의 등가 구현

Go에는 프로세스별 힙이 없으므로 `PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS`를 소켓별
메모리 회계로 대체했다. 상한은 `words × 8` bytes이며, 각 소켓에 대해 다음을
합산한다.

- 그 소켓이 라우팅 중인 프레임의 payload bytes
- data 소켓 부착을 기다리는 source의 in-flight frame payload bytes
- 그 소켓으로 나가는 write에 들어간 payload bytes와, 진행 중인 write 뒤에 큐잉된
  control 알림 bytes

합계가 상한을 넘으면 **그 소켓만** close frame 없이 즉시 끊는다(`CloseNow`).
클라이언트가 관찰하는 결과 — “close code 없는 연결 종료” — 는 참조와 같다.

정확한 등가가 아닌 부분:

- 참조는 BEAM 힙 전체(스택, 프로세스 힙, 공유 binary 포함)를 재는 반면 Go 회계는
  릴레이가 그 소켓 몫으로 명시적으로 붙잡고 있는 payload 메모리만 잰다. 따라서
  Go 쪽 값이 항상 더 작고, 같은 설정값에서 퓨즈가 더 늦게 걸린다.
- 참조는 힙 초과를 스케줄러가 감지해 프로세스를 죽이므로 시점이 GC와 묶여 있다.
  Go는 회계 시점(프레임 라우팅 시작, write 시작)에만 판정한다.
- 참조의 `words`는 BEAM word 크기에 의존한다. Go는 64-bit word(8 bytes)로 고정
  해석한다.

### 런타임 drain

drain은 참조와 마찬가지로 **프로세스 로컬 admission 상태**이며, drain 전용 HTTP
엔드포인트는 없다(`references/paseo-relay/OPERATIONS.md:99-101`). 부팅 시
`PASEO_RELAY_DRAIN`으로 초기화되고, 실행 중에는 `Relay.BeginDrain()` /
`Relay.CancelDrain()` / `Relay.Draining()`으로 토글한다.

drain 중에는:

- `/ready`가 `503`이 되고 `paseo_relay_draining`이 `1`이 된다.
- 이 노드가 아직 소유하지 않은 `serverId`의 신규 upgrade는 `503 draining`이다.
- 이미 이 노드가 소유한 세션은 유지되며, 그 세션에 붙는 새 소켓도 계속 받는다.
  노드가 한 번에 끊기지 않고 점진적으로 비워지도록 하기 위한 것이다.

## 소스 근거

- Query/버전/ID 검증: `references/paseo-relay/lib/paseo_relay/connection.ex`
- HTTP upgrade 판정 순서, frame 처리, control ping/pong, close: `references/paseo-relay/lib/paseo_relay/socket.ex`
- v1/v2 topology, control notification, watchdog, owner lifecycle: `references/paseo-relay/lib/paseo_relay/ownership.ex`
- frame limits: `references/paseo-relay/lib/paseo_relay/protocol.ex`
- handshake key validation: `references/paseo-relay/lib/paseo_relay/handshake_validation.ex`
- ingress 예산, 메모리 pressure shedding, admission 상태: `references/paseo-relay/lib/paseo_relay/capacity.ex`
- ordered/backpressured destination write, fan-out 성공 판정: `references/paseo-relay/lib/paseo_relay/delivery.ex`, `delivery/writer.ex`
- 운영 엔드포인트: `references/paseo-relay/lib/paseo_relay/operations.ex`
- 설정 기본값과 범위: `references/paseo-relay/lib/paseo_relay/config.ex`
- executable behavior examples: `references/paseo-relay/test/relay_protocol_test.exs`,
  `references/paseo-relay/test/relay_backpressure_test.exs`
