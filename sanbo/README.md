# Sanbo

Sanbo는 [`getpaseo/paseo-relay`](https://github.com/getpaseo/paseo-relay)의
공개 HTTP/WebSocket 프로토콜과 운영 설정을 동일하게 제공하는 것을 목표로
하는 Go 구현체입니다. 배포 사업자 전용 기능은 코어에서 제외하며, 현재 호환
기준은 paseo-relay 커밋
`3fc41c96c8c63f3a7109e832899cc57d473c4531`입니다.

> Fly 배포 어댑터를 제외한 provider-neutral 계약을 구현했습니다. v1/v2 프레임
> 중계, 독립 relay 프로세스 사이의 lease 기반 소유권과 opaque reroute, Capacity,
> bounded delivery/backpressure, handshake 및 운영 메트릭을 실제 HTTP/WebSocket과
> 다중 프로세스 테스트로 검증합니다.

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
| `/health` (any method) | 프로세스 liveness | 구현됨 |
| `/ready` (any method) | 신규 작업 수락 가능 여부 | drain, 클러스터 floor, 연결 ceiling, capacity/pressure 반영 |
| `/metrics` (any method) | Prometheus 텍스트 메트릭 | 연결·세션·전달·용량·handshake·histogram 전체 surface |
| `GET /ws` | paseo-relay 호환 WebSocket 업그레이드 | v1/v2 routing, ownership 및 opaque reroute 구현 |

`/metrics`는 참조 구현과 같은 HELP/TYPE 메타데이터와 누적 histogram bucket을
노출합니다. Capacity 상태를 읽을 수 없을 때는 `active_websockets`,
`ingress_reserved_bytes`, `inflight_delivery_bytes`, `backpressured_sources` 네
게이지를 응답에서 생략합니다.

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
중계합니다.

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
`32MiB - 14바이트`입니다. 이 크기를 넘는 메시지는 WebSocket close code
`1009`로 종료됩니다.

## 환경변수

| 환경변수 | 기본값 | 설명 |
| --- | ---: | --- |
| `PASEO_RELAY_HOST` | `127.0.0.1` | 공개 listener IP |
| `PASEO_RELAY_PORT` | `4000` | HTTP/WebSocket listener 포트 |
| `PASEO_RELAY_DRAIN` | `false` | 시작 시 신규 작업을 받지 않는 drain 모드 |
| `PASEO_RELAY_OWNERSHIP_TARGET` | `local` | 다른 노드에 광고하는 opaque 소유권 대상 |
| `PASEO_RELAY_REROUTE_HEADER` | `x-reroute-target` | 배포 어댑터가 읽는 reroute 응답 헤더 |
| `PASEO_RELAY_CLUSTER_QUERY` | 빈 값 | 클러스터 namespace를 구분하는 호환 query |
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

### 메모리 pressure

`PASEO_RELAY_MEMORY_WATERMARK_BYTES`가 설정되면 relay는 250ms마다 Go 런타임이
OS로부터 확보한 메모리(`runtime/metrics`의 `총 메모리 - 반환된 heap`)를
샘플링합니다. watermark에 도달하면

- `/ready`와 WebSocket admission이 닫히고,
- data attach를 기다리던 in-flight frame을 모두 폐기하고 예약된 ingress를 반환하며,
- 연결된 모든 peer를 `1013 Relay memory pressure`로 종료한 뒤
  (`paseo_relay_memory_pressure_disconnects_total` 증가) `runtime.GC()`를 호출합니다.

Shedding은 watermark를 넘는 순간 한 번만 수행됩니다. 사용량이 watermark의 90%
아래로 내려가야 pressure가 해제되므로, 경계에 머무는 heap이 admission을
반복적으로 여닫지 않습니다. Ownership reroute는 pressure보다 우선하므로 다른
노드가 소유한 세션은 pressure 중에도 `409`로 안내됩니다.

### Capacity 조정

`PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS` 주기마다 relay는 ingress 원장을 실제
상태와 대조합니다. 예약된 바이트는 항상 (data attach를 기다리거나 라우팅 중인
바이트)와 일치해야 하며, 예약이 이를 초과하면 유실된 예약으로
간주합니다. 이 경우

- admission을 닫고 (`capacity_unavailable`),
- capacity epoch을 증가시키고,
- 초과분을 예산에 반환한 뒤 admission을 다시 엽니다.

진행 중인 예약은 `reserveIngress`가 예약보다 먼저 등록하고 반환보다 나중에
해제하므로, 조정 시점의 스냅샷은 항상 과다 계상 방향으로만 어긋납니다. 즉
정상적으로 라우팅 중인 프레임이 유실로 오인되어 회수되는 일은 없습니다.
이 값은 "불일치가 정정되기까지 허용되는 최대 시간"으로 읽으면 됩니다.

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

전체 호환 계약 실행:

```sh
go test -count=1 ./...
```

원본 109개 중 Fly 어댑터 전용 9개를 제외한 100개 테스트 계약을 포팅했으며,
Go 전용 회귀 테스트를 포함해 총 133개 테스트와 5개 fuzz seed target이 있습니다.
race detector까지 포함한 검증 명령은 다음과 같습니다.

```sh
go test -race -count=1 ./...
go vet ./...
```

자세한 기준과 테스트 그룹은
[`TESTING.md`](TESTING.md)를 참고하세요.

## 구현 상태

구현됨:

- 22개 환경변수와 기본값 및 교차 필드 검증
- v1/v2 query parsing과 route ID 길이 제한
- HTTP health, readiness, 기본 Prometheus 응답
- `coder/websocket` 기반 upgrade와 read limit
- v1 및 v2 양방향 프레임 forwarding과 per-destination 직렬화
- v2 control sync, connected, disconnected 및 legacy ping/pong의 기본 경로
- canonical X25519 handshake key 검증
- client-originated handshake만 검증하는 role boundary와 outcome counter
- 가중 ingress reservation, source-blocking data-route attach wait 및 reservation 정리
- 전달 deadline, slow-consumer fail-closed 처리 및 capacity reconciliation
- 단일 프로세스 내 다중 relay node ownership, 원자적 claim 및 opaque reroute
- query와 release cookie로 격리된 host-level 다중 프로세스 lease registry
- 실제 member heartbeat 수를 사용하는 minimum-cluster readiness
- process 종료 후 lease 만료와 remote ownership reclaim
- WebSocket upgrade 후 원자적 claim과 소유권 상실 시 WebSocket `1012` 수렴
- 연결 ceiling, memory-pressure admission/shedding 및 session reclamation
- 전체 Prometheus counter/gauge/histogram surface
- 실제 socket lifecycle을 사용하는 provider-neutral load 시나리오
- TCP 수신 버퍼 및 전송 timeout 기반 listener wrapper
- 정상 종료를 위한 `Shutdown`

클러스터 설정(`PASEO_RELAY_CLUSTER_QUERY`, `RELEASE_NODE`, `RELEASE_COOKIE`)이 모두
있으면 OS 임시 디렉터리의 locked lease registry를 사용합니다. cluster query와
cookie 조합이 namespace이며, 100ms heartbeat와 750ms lease로 실제 membership과
owner liveness를 판단합니다. 설정이 없으면 개발 및 단일 프로세스 호환을 위해
기존 in-process registry를 사용합니다. 이 backend는 동일 호스트 또는 공유
파일시스템을 사용하는 프로세스를 대상으로 하며, 공유 저장소가 없는 서로 다른
호스트의 DNS transport는 현재 범위에 포함되지 않습니다.

`multinode_integration_test.go`는 실제 relay subprocess 2~3개로 join/leave,
동시 claim, opaque reroute, owner failover, 격리와 ownership surge를 검증합니다.

Fly deployment adapter 동작은 의도적으로 범위에서 제외됩니다. 호환 테스트는
skip하거나 기대값을 완화하지 않으며, scheduler·transport·memory fault는 실제
relay state에 결정적으로 주입한 뒤 socket과 production counter를 관찰합니다.
