# HA·하드웨어 관리 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 공식 JSON-RPC 메서드를 기능별로 정리한다.
각 행은 공식 상세 페이지에서 추출한 최상위 호출 파라미터와 반환값을 보여 준다. 복합 객체의 전체 필드는 공식 상세 링크에서 확인한다.

**표기:** `name: type`은 인자의 순서와 타입이다. 공식 스키마가 명시한 기본값은 `(default: …)`로 표시하며, 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`failover`](https://api.truenas.com/v25.10/api_methods_failover.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`failover.become_passive`](https://api.truenas.com/v25.10/api_methods_failover.become_passive.html) | [] | `null` |
| [`failover.config`](https://api.truenas.com/v25.10/api_methods_failover.config.html) | [] | `object (FailoverEntry)` |
| [`failover.get_ips`](https://api.truenas.com/v25.10/api_methods_failover.get_ips.html) | [] | `array of string` |
| [`failover.licensed`](https://api.truenas.com/v25.10/api_methods_failover.licensed.html) | [] | `boolean` |
| [`failover.node`](https://api.truenas.com/v25.10/api_methods_failover.node.html) | [] | `string` |
| [`failover.status`](https://api.truenas.com/v25.10/api_methods_failover.status.html) | [] | `string` |
| [`failover.sync_from_peer`](https://api.truenas.com/v25.10/api_methods_failover.sync_from_peer.html) | [] | `null` |
| [`failover.sync_to_peer`](https://api.truenas.com/v25.10/api_methods_failover.sync_to_peer.html) | [`options: object (default: {"reboot": false})`] | `null` |
| [`failover.update`](https://api.truenas.com/v25.10/api_methods_failover.update.html) | [`data: object`] | `object (FailoverEntry)` |
| [`failover.upgrade`](https://api.truenas.com/v25.10/api_methods_failover.upgrade.html) | [`failover_upgrade: object (default: { "train": null, "version": null, "resume": false, "resume_manual": false })`] | `boolean` |

## [`failover.disabled`](https://api.truenas.com/v25.10/api_methods_failover.disabled.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`failover.disabled.reasons`](https://api.truenas.com/v25.10/api_methods_failover.disabled.reasons.html) | [] | `array of string` |

## [`failover.reboot`](https://api.truenas.com/v25.10/api_methods_failover.reboot.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`failover.reboot.info`](https://api.truenas.com/v25.10/api_methods_failover.reboot.info.html) | [] | `object (FailoverRebootInfoResult)` |
| [`failover.reboot.other_node`](https://api.truenas.com/v25.10/api_methods_failover.reboot.other_node.html) | [`options: object (default: { "reason": "System upgrade", "graceful": false })`] | `null` |

## [`ipmi`](https://api.truenas.com/v25.10/api_methods_ipmi.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`ipmi.is_loaded`](https://api.truenas.com/v25.10/api_methods_ipmi.is_loaded.html) | [] | `boolean` |

## [`ipmi.chassis`](https://api.truenas.com/v25.10/api_methods_ipmi.chassis.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`ipmi.chassis.identify`](https://api.truenas.com/v25.10/api_methods_ipmi.chassis.identify.html) | [`data: object`] | `null` |
| [`ipmi.chassis.info`](https://api.truenas.com/v25.10/api_methods_ipmi.chassis.info.html) | [`data: object`] | `object` |

## [`ipmi.lan`](https://api.truenas.com/v25.10/api_methods_ipmi.lan.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`ipmi.lan.channels`](https://api.truenas.com/v25.10/api_methods_ipmi.lan.channels.html) | [] | `array of integer` |
| [`ipmi.lan.query`](https://api.truenas.com/v25.10/api_methods_ipmi.lan.query.html) | [`data: object`] | `array of object` |
| [`ipmi.lan.update`](https://api.truenas.com/v25.10/api_methods_ipmi.lan.update.html) | [`channel: integer`, `data: object`] | `integer` |

## [`ipmi.sel`](https://api.truenas.com/v25.10/api_methods_ipmi.sel.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`ipmi.sel.clear`](https://api.truenas.com/v25.10/api_methods_ipmi.sel.clear.html) | [] | `null` |
| [`ipmi.sel.elist`](https://api.truenas.com/v25.10/api_methods_ipmi.sel.elist.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`ipmi.sel.info`](https://api.truenas.com/v25.10/api_methods_ipmi.sel.info.html) | [] | `object` |

총 22개 메서드.
