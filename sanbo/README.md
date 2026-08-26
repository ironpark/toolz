# Sanbo

Sanbo는 [`getpaseo/paseo-relay`](https://github.com/getpaseo/paseo-relay)의
공개 HTTP/WebSocket 프로토콜과 운영 설정을 동일하게 제공하는 것을 목표로
하는 Go 구현체입니다. 배포 사업자 전용 기능은 코어에서 제외하며, 현재 호환
기준은 paseo-relay 커밋
`3fc41c96c8c63f3a7109e832899cc57d473c4531`입니다.

> 현재는 TDD 기반 초기 구현 단계입니다. 설정 로딩, 요청 검증, HTTP 운영
> 엔드포인트와 WebSocket 수락 기반은 구현되어 있지만 실제 프레임 중계,
> 세션 소유권, backpressure 및 다중 노드 동작은 아직 구현되지 않았습니다.
> 따라서 전체 호환 테스트는 의도적으로 실패합니다.

## 요구 사항

- Go 1.26.3 이상
- WebSocket 구현: [`github.com/coder/websocket`](https://github.com/coder/websocket)

## 실행

기본값으로 `127.0.0.1:4000`에서 실행합니다.

```sh
cd sanbo
go run .
```

포트와 공개 바인드 주소를 지정하려면 환경변수를 사용합니다.

```sh
PASEO_RELAY_HOST=0.0.0.0 \
PASEO_RELAY_PORT=8080 \
go run .
```

바이너리 빌드:

```sh
go build -o sanbo .
./sanbo
```

## HTTP 엔드포인트

| 경로 | 용도 | 현재 상태 |
| --- | --- | --- |
| `GET /health` | 프로세스 liveness | 구현됨 |
| `GET /ready` | 신규 작업 수락 가능 여부 | drain 및 최소 클러스터 설정의 기본 판정 구현됨 |
| `GET /metrics` | Prometheus 텍스트 메트릭 | 기본 readiness, drain, WebSocket gauge 구현됨 |
| `GET /ws` | paseo-relay 호환 WebSocket 업그레이드 | 쿼리 검증과 업그레이드만 구현됨 |

헬스 체크 예시:

```sh
curl -i http://127.0.0.1:4000/health
curl -i http://127.0.0.1:4000/ready
curl http://127.0.0.1:4000/metrics
```

## WebSocket 계약

모든 WebSocket은 `/ws`를 사용하며 `serverId`와 `role`이 필요합니다.
`serverId`와 `connectionId`는 각각 최대 256바이트입니다.

### v1

`v`가 없거나 `v=1`이면 v1 연결입니다.

```text
/ws?serverId=<server-id>&role=server
/ws?serverId=<server-id>&role=client
```

같은 `serverId`의 server/client 사이에서 텍스트 및 바이너리 프레임을 순서대로
중계하는 것이 목표 계약입니다.

### v2

```text
# daemon control
/ws?serverId=<server-id>&role=server&v=2

# client
/ws?serverId=<server-id>&role=client&v=2&connectionId=<connection-id>

# daemon data
/ws?serverId=<server-id>&role=server&v=2&connectionId=<connection-id>
```

v2 client에서 `connectionId`를 생략하면 relay가 `conn_` 접두사의 ID를
생성합니다. daemon control 입력은 최대 64KiB이며, data 메시지의 최대 payload는
`32MiB - 14바이트`입니다. 이 크기를 넘는 메시지는 최종 구현에서 WebSocket
close code `1009`로 종료되어야 합니다.

## 환경변수

| 환경변수 | 기본값 | 설명 |
| --- | ---: | --- |
| `PASEO_RELAY_HOST` | `127.0.0.1` | 공개 listener IP |
| `PASEO_RELAY_PORT` | `4000` | HTTP/WebSocket listener 포트 |
| `PASEO_RELAY_DRAIN` | `false` | 시작 시 신규 작업을 받지 않는 drain 모드 |
| `PASEO_RELAY_OWNERSHIP_TARGET` | `local` | 다른 노드에 광고하는 opaque 소유권 대상 |
| `PASEO_RELAY_REROUTE_HEADER` | `x-reroute-target` | 배포 어댑터가 읽는 reroute 응답 헤더 |
| `PASEO_RELAY_CLUSTER_QUERY` | 빈 값 | 클러스터 peer 탐색용 DNS query |
| `PASEO_RELAY_MIN_CLUSTER_SIZE` | `1` | 신규 세션 수락에 필요한 최소 노드 수 |
| `PASEO_RELAY_ACCEPTORS` | `100` | listener acceptor 수 |
| `PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR` | `200` | acceptor당 연결 수; 기본 WebSocket ceiling은 20,000 |
| `PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS` | `15000` | 업그레이드 전 HTTP idle timeout |
| `PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS` | `5000` | Capacity 상태 변경 제한 시간 |
| `PASEO_RELAY_INGRESS_BUDGET_BYTES` | `536870912` | 노드 단위 가중 ingress 예산 |
| `PASEO_RELAY_INGRESS_WEIGHT` | `4` | wire payload 바이트당 메모리 가중치 |
| `PASEO_RELAY_DELIVERY_TIMEOUT_MS` | `30000` | Writer 예약 및 전달 제한 시간 |
| `PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS` | `35000` | TCP 전송 제한 시간 |
| `PASEO_RELAY_CONTROL_QUEUE_BYTES` | `1048576` | 목적지별 control 알림 queue 제한 |
| `PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS` | `15000` | v2 daemon-data 연결 대기 시간 |
| `PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES` | `65536` | 소켓별 TCP 수신 버퍼 |
| `PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS` | `33554432` | 호환 설정으로 유지하는 WebSocket heap fuse 값 |
| `PASEO_RELAY_MEMORY_WATERMARK_BYTES` | `0` | 메모리 pressure watermark, `0`은 비활성화 |
| `RELEASE_NODE` | 빈 값 | 분산 노드 식별자 호환 설정 |
| `RELEASE_COOKIE` | 빈 값 | 분산 노드 인증 cookie 호환 설정 |

설정은 시작 시 검증됩니다. 특히 다음 조건을 만족해야 합니다.

- `PASEO_RELAY_DELIVERY_TIMEOUT_MS`는 transport send timeout보다 작아야 합니다.
- ingress 예산은 설정된 가중치로 최대 메시지 하나를 수용할 수 있어야 합니다.
- memory watermark의 `0`은 비활성화를 의미합니다.
- listener host는 hostname이 아닌 IP 주소여야 합니다.

## 테스트

전체 테스트 컴파일:

```sh
go test -run '^$' ./...
```

현재 구현된 설정, 쿼리 및 기본 운영 API 테스트:

```sh
go test -run '^(TestLoadConfig|TestEnvironmentVariableInventory|TestConfig|TestParseConnection|TestOperationsHealthIsAlwaysLive|TestOperationsReadyRefusesNewWorkDuringDrain|TestOperationsMetricsExposeStablePrometheusSurface|TestOperationsReadyWaitsForMinimumClusterSize|TestOperationsUnknownPathReturnsNotFound)$' ./...
```

전체 TDD 계약 실행:

```sh
go test ./...
```

원본 109개 중 Fly 어댑터 전용 9개를 제외한 100개 테스트 계약을 포팅했으며,
Go 전용 회귀 테스트를 포함해 총 111개 테스트가 있습니다. 미구현 기능 때문에
전체 실행은 현재 실패하는 것이 정상입니다. 자세한 기준과 테스트 그룹은
[`TESTING.md`](TESTING.md)를 참고하세요.

## 구현 상태

구현됨:

- 22개 환경변수와 기본값 및 교차 필드 검증
- v1/v2 query parsing과 route ID 길이 제한
- HTTP health, readiness, 기본 Prometheus 응답
- `coder/websocket` 기반 upgrade와 read limit
- TCP 수신 버퍼 및 전송 timeout 기반 listener wrapper
- 정상 종료를 위한 `Shutdown`

아직 구현되지 않음:

- v1 및 v2 양방향 프레임 forwarding
- v2 control sync, connected, disconnected 및 legacy ping/pong
- X25519 handshake key 검증과 관련 메트릭
- 단일/다중 노드 세션 ownership 및 opaque reroute
- 연결 및 메시지 Capacity ledger
- Writer queue, 전달 deadline과 slow-consumer 처리
- 메모리 pressure shedding 및 epoch 재시작
- black-box load client

호환 기능은 해당 테스트를 먼저 활성 상태로 유지한 채 구현합니다. 테스트를
skip하거나 기대값을 완화하는 방식으로 호환성을 맞추지 않습니다.
