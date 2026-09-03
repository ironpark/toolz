# 원격 관리·TrueNAS 서비스 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 공식 JSON-RPC 메서드를 기능별로 정리한다.
각 행은 공식 상세 페이지에서 추출한 최상위 호출 파라미터와 반환값을 보여 준다. 복합 객체의 전체 필드는 공식 상세 링크에서 확인한다.

**표기:** `name: type`은 인자의 순서와 타입이다. 공식 스키마가 명시한 기본값은 `(default: …)`로 표시하며, 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`tn_connect`](https://api.truenas.com/v25.10/api_methods_tn_connect.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`tn_connect.config`](https://api.truenas.com/v25.10/api_methods_tn_connect.config.html) | [] | `object (TrueNASConnectEntry)` |
| [`tn_connect.generate_claim_token`](https://api.truenas.com/v25.10/api_methods_tn_connect.generate_claim_token.html) | [] | `string` |
| [`tn_connect.get_registration_uri`](https://api.truenas.com/v25.10/api_methods_tn_connect.get_registration_uri.html) | [] | `string` |
| [`tn_connect.update`](https://api.truenas.com/v25.10/api_methods_tn_connect.update.html) | [`tn_connect_update: object`] | `object (TrueNASConnectEntry)` |

## [`truecommand`](https://api.truenas.com/v25.10/api_methods_truecommand.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`truecommand.config`](https://api.truenas.com/v25.10/api_methods_truecommand.config.html) | [] | `object (TruecommandEntry)` |
| [`truecommand.update`](https://api.truenas.com/v25.10/api_methods_truecommand.update.html) | [`truecommand_update: object`] | `object (TruecommandEntry)` |

## [`truenas`](https://api.truenas.com/v25.10/api_methods_truenas.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`truenas.accept_eula`](https://api.truenas.com/v25.10/api_methods_truenas.accept_eula.html) | [] | `null` |
| [`truenas.get_chassis_hardware`](https://api.truenas.com/v25.10/api_methods_truenas.get_chassis_hardware.html) | [] | `string` |
| [`truenas.get_eula`](https://api.truenas.com/v25.10/api_methods_truenas.get_eula.html) | [] | `string` |
| [`truenas.is_eula_accepted`](https://api.truenas.com/v25.10/api_methods_truenas.is_eula_accepted.html) | [] | `boolean` |
| [`truenas.is_ix_hardware`](https://api.truenas.com/v25.10/api_methods_truenas.is_ix_hardware.html) | [] | `boolean` |
| [`truenas.is_production`](https://api.truenas.com/v25.10/api_methods_truenas.is_production.html) | [] | `boolean` |
| [`truenas.managed_by_truecommand`](https://api.truenas.com/v25.10/api_methods_truenas.managed_by_truecommand.html) | [] | `boolean` |
| [`truenas.set_production`](https://api.truenas.com/v25.10/api_methods_truenas.set_production.html) | [`production: boolean`, `attach_debug: boolean (default: false)`] | `object` |

총 14개 메서드.
