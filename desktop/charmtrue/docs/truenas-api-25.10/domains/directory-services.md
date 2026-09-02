# 디렉터리 서비스·Kerberos API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 공식 JSON-RPC 메서드를 기능별로 정리한다.
각 행은 공식 상세 페이지에서 추출한 최상위 호출 파라미터와 반환값을 보여 준다. 복합 객체의 전체 필드는 공식 상세 링크에서 확인한다.

**표기:** `name: type`은 인자의 순서와 타입이다. 공식 스키마가 명시한 기본값은 `(default: …)`로 표시하며, 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`directoryservices`](https://api.truenas.com/v25.10/api_methods_directoryservices.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`directoryservices.cache_refresh`](https://api.truenas.com/v25.10/api_methods_directoryservices.cache_refresh.html) | [] | `null` |
| [`directoryservices.certificate_choices`](https://api.truenas.com/v25.10/api_methods_directoryservices.certificate_choices.html) | [] | `object` |
| [`directoryservices.config`](https://api.truenas.com/v25.10/api_methods_directoryservices.config.html) | [] | `object (DirectoryServicesEntry)` |
| [`directoryservices.leave`](https://api.truenas.com/v25.10/api_methods_directoryservices.leave.html) | [`credential: object`] | `null` |
| [`directoryservices.status`](https://api.truenas.com/v25.10/api_methods_directoryservices.status.html) | [] | `object (DirectoryServicesStatusResult)` |
| [`directoryservices.sync_keytab`](https://api.truenas.com/v25.10/api_methods_directoryservices.sync_keytab.html) | [] | `null` |
| [`directoryservices.update`](https://api.truenas.com/v25.10/api_methods_directoryservices.update.html) | [`directoryservices_update: object`] | `object (DirectoryServicesEntry)` |

## [`idmap`](https://api.truenas.com/v25.10/api_methods_idmap.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`idmap.clear_idmap_cache`](https://api.truenas.com/v25.10/api_methods_idmap.clear_idmap_cache.html) | [] | `null` |

## [`kerberos`](https://api.truenas.com/v25.10/api_methods_kerberos.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`kerberos.config`](https://api.truenas.com/v25.10/api_methods_kerberos.config.html) | [] | `object (KerberosEntry)` |
| [`kerberos.update`](https://api.truenas.com/v25.10/api_methods_kerberos.update.html) | [`kerberos_update: object`] | `object (KerberosEntry)` |

## [`kerberos.keytab`](https://api.truenas.com/v25.10/api_methods_kerberos.keytab.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`kerberos.keytab.create`](https://api.truenas.com/v25.10/api_methods_kerberos.keytab.create.html) | [`data: object`] | `object (KerberosKeytabEntry)` |
| [`kerberos.keytab.delete`](https://api.truenas.com/v25.10/api_methods_kerberos.keytab.delete.html) | [`id: integer`] | `null` |
| [`kerberos.keytab.get_instance`](https://api.truenas.com/v25.10/api_methods_kerberos.keytab.get_instance.html) | [`id: integer`, `options: object`] | `object (KerberosKeytabEntry)` |
| [`kerberos.keytab.query`](https://api.truenas.com/v25.10/api_methods_kerberos.keytab.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`kerberos.keytab.update`](https://api.truenas.com/v25.10/api_methods_kerberos.keytab.update.html) | [`id: integer`, `data: object`] | `object (KerberosKeytabEntry)` |

## [`kerberos.realm`](https://api.truenas.com/v25.10/api_methods_kerberos.realm.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`kerberos.realm.create`](https://api.truenas.com/v25.10/api_methods_kerberos.realm.create.html) | [`data: object`] | `object (KerberosRealmEntry)` |
| [`kerberos.realm.delete`](https://api.truenas.com/v25.10/api_methods_kerberos.realm.delete.html) | [`id: integer`] | `null` |
| [`kerberos.realm.get_instance`](https://api.truenas.com/v25.10/api_methods_kerberos.realm.get_instance.html) | [`id: integer`, `options: object`] | `object (KerberosRealmEntry)` |
| [`kerberos.realm.query`](https://api.truenas.com/v25.10/api_methods_kerberos.realm.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`kerberos.realm.update`](https://api.truenas.com/v25.10/api_methods_kerberos.realm.update.html) | [`id: integer`, `data: object`] | `object (KerberosRealmEntry)` |

총 20개 메서드.
