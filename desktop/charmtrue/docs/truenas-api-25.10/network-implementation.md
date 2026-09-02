# 네트워크 API 구현 현황

TrueNAS API v25.10.5 기준. 아래 35개 호출 메서드는 `Client.Network()` 서비스와 메서드 메타데이터에 등록되어 있다.

| 상태 | 네임스페이스 | 메서드 | 분류 |
|---|---|---|---|
| ✅ | DNS | `dns.query` | 조회 |
| ✅ | 인터페이스 | `interface.bridge_members_choices` | 조회 |
| ✅ | 인터페이스 | `interface.cancel_rollback` | 위험 변경 |
| ✅ | 인터페이스 | `interface.checkin` | 위험 변경 |
| ✅ | 인터페이스 | `interface.checkin_waiting` | 조회 |
| ✅ | 인터페이스 | `interface.choices` | 조회 |
| ✅ | 인터페이스 | `interface.commit` | 위험 변경 |
| ✅ | 인터페이스 | `interface.create` | 위험 생성 |
| ✅ | 인터페이스 | `interface.delete` | 파괴 |
| ✅ | 인터페이스 | `interface.get_instance` | 조회 |
| ✅ | 인터페이스 | `interface.has_pending_changes` | 조회 |
| ✅ | 인터페이스 | `interface.ip_in_use` | 조회 |
| ✅ | 인터페이스 | `interface.lacpdu_rate_choices` | 조회 |
| ✅ | 인터페이스 | `interface.lag_ports_choices` | 조회 |
| ✅ | 인터페이스 | `interface.network_config_to_be_removed` | 조회 |
| ✅ | 인터페이스 | `interface.query` | 조회 |
| ✅ | 인터페이스 | `interface.rollback` | 위험 변경 |
| ✅ | 인터페이스 | `interface.save_network_config` | 위험 변경 |
| ✅ | 인터페이스 | `interface.services_restarted_on_sync` | 조회 |
| ✅ | 인터페이스 | `interface.update` | 위험 변경 |
| ✅ | 인터페이스 | `interface.vlan_parent_interface_choices` | 조회 |
| ✅ | 인터페이스 | `interface.websocket_interface` | 조회 |
| ✅ | 인터페이스 | `interface.websocket_local_ip` | 조회 |
| ✅ | 인터페이스 | `interface.xmit_hash_policy_choices` | 조회 |
| ✅ | 전역 설정 | `network.configuration.activity_choices` | 조회 |
| ✅ | 전역 설정 | `network.configuration.config` | 조회 |
| ✅ | 전역 설정 | `network.configuration.update` | 위험 변경 |
| ✅ | 요약 | `network.general.summary` | 조회 |
| ✅ | 라우트 | `route.ipv4gw_reachable` | 조회 |
| ✅ | 라우트 | `route.system_routes` | 조회 |
| ✅ | 정적 라우트 | `staticroute.create` | 위험 생성 |
| ✅ | 정적 라우트 | `staticroute.delete` | 파괴 |
| ✅ | 정적 라우트 | `staticroute.get_instance` | 조회 |
| ✅ | 정적 라우트 | `staticroute.query` | 조회 |
| ✅ | 정적 라우트 | `staticroute.update` | 위험 변경 |
