# 인증서·ACME·Web UI API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 공식 JSON-RPC 메서드를 기능별로 정리한다.
각 행은 공식 상세 페이지에서 추출한 최상위 호출 파라미터와 반환값을 보여 준다. 복합 객체의 전체 필드는 공식 상세 링크에서 확인한다.

**표기:** `name: type`은 인자의 순서와 타입이다. 공식 스키마가 명시한 기본값은 `(default: …)`로 표시하며, 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`acme.dns.authenticator`](https://api.truenas.com/v25.10/api_methods_acme.dns.authenticator.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`acme.dns.authenticator.authenticator_schemas`](https://api.truenas.com/v25.10/api_methods_acme.dns.authenticator.authenticator_schemas.html) | [] | `array of object` |
| [`acme.dns.authenticator.create`](https://api.truenas.com/v25.10/api_methods_acme.dns.authenticator.create.html) | [`dns_authenticator_create: object`] | `object (DNSAuthenticatorEntry)` |
| [`acme.dns.authenticator.delete`](https://api.truenas.com/v25.10/api_methods_acme.dns.authenticator.delete.html) | [`id: integer`] | `boolean` |
| [`acme.dns.authenticator.get_instance`](https://api.truenas.com/v25.10/api_methods_acme.dns.authenticator.get_instance.html) | [`id: integer`, `options: object`] | `object (DNSAuthenticatorEntry)` |
| [`acme.dns.authenticator.query`](https://api.truenas.com/v25.10/api_methods_acme.dns.authenticator.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`acme.dns.authenticator.update`](https://api.truenas.com/v25.10/api_methods_acme.dns.authenticator.update.html) | [`id: integer`, `dns_authenticator_update: object`] | `object (DNSAuthenticatorEntry)` |

## [`certificate`](https://api.truenas.com/v25.10/api_methods_certificate.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`certificate.acme_server_choices`](https://api.truenas.com/v25.10/api_methods_certificate.acme_server_choices.html) | [] | `object` |
| [`certificate.country_choices`](https://api.truenas.com/v25.10/api_methods_certificate.country_choices.html) | [] | `object` |
| [`certificate.create`](https://api.truenas.com/v25.10/api_methods_certificate.create.html) | [`certificate_create: object`] | `object (CertificateEntry)` |
| [`certificate.delete`](https://api.truenas.com/v25.10/api_methods_certificate.delete.html) | [`id: integer`, `force: boolean (default: false)`] | `boolean` |
| [`certificate.ec_curve_choices`](https://api.truenas.com/v25.10/api_methods_certificate.ec_curve_choices.html) | [] | `object` |
| [`certificate.extended_key_usage_choices`](https://api.truenas.com/v25.10/api_methods_certificate.extended_key_usage_choices.html) | [] | `object` |
| [`certificate.get_instance`](https://api.truenas.com/v25.10/api_methods_certificate.get_instance.html) | [`id: integer`, `options: object`] | `object (CertificateEntry)` |
| [`certificate.query`](https://api.truenas.com/v25.10/api_methods_certificate.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`certificate.update`](https://api.truenas.com/v25.10/api_methods_certificate.update.html) | [`id: integer`, `certificate_update: object`] | `object (CertificateEntry)` |

## [`webui.crypto`](https://api.truenas.com/v25.10/api_methods_webui.crypto.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`webui.crypto.csr_profiles`](https://api.truenas.com/v25.10/api_methods_webui.crypto.csr_profiles.html) | [] | `object (CSRProfilesModel)` |
| [`webui.crypto.get_certificate_domain_names`](https://api.truenas.com/v25.10/api_methods_webui.crypto.get_certificate_domain_names.html) | [`cert_id: integer`] | `array` |

## [`webui.enclosure`](https://api.truenas.com/v25.10/api_methods_webui.enclosure.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`webui.enclosure.dashboard`](https://api.truenas.com/v25.10/api_methods_webui.enclosure.dashboard.html) | [] | `array of object` |

## [`webui.main.dashboard`](https://api.truenas.com/v25.10/api_methods_webui.main.dashboard.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`webui.main.dashboard.sys_info`](https://api.truenas.com/v25.10/api_methods_webui.main.dashboard.sys_info.html) | [] | `object (SysInfo)` |

총 19개 메서드.
