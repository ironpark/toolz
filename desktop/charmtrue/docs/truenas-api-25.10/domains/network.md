# 네트워크 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역 전체를 나열한다. 메서드 링크에서 인자, 반환 스키마, Job 여부와 권한을 확인한다.

표의 `Call parameters`와 `Return value`는 공식 v25.10.5 상세 문서의 최상위 JSON Schema(중첩 객체는 타입명과 상세 링크 참조)를 옮긴 것이다. 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`dns`](https://api.truenas.com/v25.10/api_methods_dns.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`dns.query`](https://api.truenas.com/v25.10/api_methods_dns.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |

## [`interface`](https://api.truenas.com/v25.10/api_methods_interface.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`interface.bridge_members_choices`](https://api.truenas.com/v25.10/api_methods_interface.bridge_members_choices.html) | `[id: string \| null (default null)]` | `object` |
| [`interface.cancel_rollback`](https://api.truenas.com/v25.10/api_methods_interface.cancel_rollback.html) | `[]` | `null` |
| [`interface.checkin`](https://api.truenas.com/v25.10/api_methods_interface.checkin.html) | `[]` | `null` |
| [`interface.checkin_waiting`](https://api.truenas.com/v25.10/api_methods_interface.checkin_waiting.html) | `[]` | `integer \| null` |
| [`interface.choices`](https://api.truenas.com/v25.10/api_methods_interface.choices.html) | `[options: object]` | `object` |
| [`interface.commit`](https://api.truenas.com/v25.10/api_methods_interface.commit.html) | `[options: object]` | `null` |
| [`interface.create`](https://api.truenas.com/v25.10/api_methods_interface.create.html) | `[data: object]` | `object (InterfaceEntry)` |
| [`interface.delete`](https://api.truenas.com/v25.10/api_methods_interface.delete.html) | `[id: string]` | `string` |
| [`interface.get_instance`](https://api.truenas.com/v25.10/api_methods_interface.get_instance.html) | `[id: string, options: object]` | `object (InterfaceEntry)` |
| [`interface.has_pending_changes`](https://api.truenas.com/v25.10/api_methods_interface.has_pending_changes.html) | `[]` | `boolean` |
| [`interface.ip_in_use`](https://api.truenas.com/v25.10/api_methods_interface.ip_in_use.html) | `[options: object]` | `array of object` |
| [`interface.lacpdu_rate_choices`](https://api.truenas.com/v25.10/api_methods_interface.lacpdu_rate_choices.html) | `[]` | `object (InterfaceLacpduRateChoicesResult)` |
| [`interface.lag_ports_choices`](https://api.truenas.com/v25.10/api_methods_interface.lag_ports_choices.html) | `[id: string \| null (default null)]` | `object` |
| [`interface.network_config_to_be_removed`](https://api.truenas.com/v25.10/api_methods_interface.network_config_to_be_removed.html) | `[]` | `array of enum (of string)` |
| [`interface.query`](https://api.truenas.com/v25.10/api_methods_interface.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`interface.rollback`](https://api.truenas.com/v25.10/api_methods_interface.rollback.html) | `[]` | `null` |
| [`interface.save_network_config`](https://api.truenas.com/v25.10/api_methods_interface.save_network_config.html) | `[config: object]` | `null` |
| [`interface.services_restarted_on_sync`](https://api.truenas.com/v25.10/api_methods_interface.services_restarted_on_sync.html) | `[]` | `array of object` |
| [`interface.update`](https://api.truenas.com/v25.10/api_methods_interface.update.html) | `[id: string, data: object]` | `object (InterfaceEntry)` |
| [`interface.vlan_parent_interface_choices`](https://api.truenas.com/v25.10/api_methods_interface.vlan_parent_interface_choices.html) | `[]` | `object` |
| [`interface.websocket_interface`](https://api.truenas.com/v25.10/api_methods_interface.websocket_interface.html) | `[]` | `object \| null` |
| [`interface.websocket_local_ip`](https://api.truenas.com/v25.10/api_methods_interface.websocket_local_ip.html) | `[]` | `const("") \| string \| null` |
| [`interface.xmit_hash_policy_choices`](https://api.truenas.com/v25.10/api_methods_interface.xmit_hash_policy_choices.html) | `[]` | `object (InterfaceXmitHashPolicyChoicesResult)` |

## [`network.configuration`](https://api.truenas.com/v25.10/api_methods_network.configuration.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`network.configuration.activity_choices`](https://api.truenas.com/v25.10/api_methods_network.configuration.activity_choices.html) | `[]` | `array of array` |
| [`network.configuration.config`](https://api.truenas.com/v25.10/api_methods_network.configuration.config.html) | `[]` | `object (NetworkConfigurationEntry)` |
| [`network.configuration.update`](https://api.truenas.com/v25.10/api_methods_network.configuration.update.html) | `[data: object]` | `object (NetworkConfigurationEntry)` |

## [`network.general`](https://api.truenas.com/v25.10/api_methods_network.general.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`network.general.summary`](https://api.truenas.com/v25.10/api_methods_network.general.summary.html) | `[]` | `object (NetworkGeneralSummaryResult)` |

## [`route`](https://api.truenas.com/v25.10/api_methods_route.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`route.ipv4gw_reachable`](https://api.truenas.com/v25.10/api_methods_route.ipv4gw_reachable.html) | `[ipv4_gateway: string]` | `boolean` |
| [`route.system_routes`](https://api.truenas.com/v25.10/api_methods_route.system_routes.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |

## [`staticroute`](https://api.truenas.com/v25.10/api_methods_staticroute.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`staticroute.create`](https://api.truenas.com/v25.10/api_methods_staticroute.create.html) | `[data: object]` | `object (StaticRouteEntry)` |
| [`staticroute.delete`](https://api.truenas.com/v25.10/api_methods_staticroute.delete.html) | `[id: integer]` | `boolean` |
| [`staticroute.get_instance`](https://api.truenas.com/v25.10/api_methods_staticroute.get_instance.html) | `[id: integer, options: object]` | `object (StaticRouteEntry)` |
| [`staticroute.query`](https://api.truenas.com/v25.10/api_methods_staticroute.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`staticroute.update`](https://api.truenas.com/v25.10/api_methods_staticroute.update.html) | `[id: integer, data: object]` | `object (StaticRouteEntry)` |
