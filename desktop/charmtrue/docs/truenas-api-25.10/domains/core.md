# RPC 핵심 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 공식 JSON-RPC 메서드를 기능별로 정리한다.
각 행은 공식 상세 페이지에서 추출한 최상위 호출 파라미터와 반환값을 보여 준다. 복합 객체의 전체 필드는 공식 상세 링크에서 확인한다.

**표기:** `name: type`은 인자의 순서와 타입이다. 공식 스키마가 명시한 기본값은 `(default: …)`로 표시하며, 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`core`](https://api.truenas.com/v25.10/api_methods_core.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`core.arp`](https://api.truenas.com/v25.10/api_methods_core.arp.html) | [`options: object`] | `object` |
| [`core.bulk`](https://api.truenas.com/v25.10/api_methods_core.bulk.html) | [`method: string`, `params: array of array`, `description: string or null (default: null)`] | `array of object` |
| [`core.download`](https://api.truenas.com/v25.10/api_methods_core.download.html) | [`method: string`, `args: array`, `filename: string`, `buffered: boolean (default: false)`] | `array` |
| [`core.get_jobs`](https://api.truenas.com/v25.10/api_methods_core.get_jobs.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`core.get_methods`](https://api.truenas.com/v25.10/api_methods_core.get_methods.html) | [`service: string or null (default: null)`, `target: enum (of string) (default: "WS")`] | `object` |
| [`core.get_services`](https://api.truenas.com/v25.10/api_methods_core.get_services.html) | [`target: enum (of string) (default: "WS")`] | `object` |
| [`core.job_abort`](https://api.truenas.com/v25.10/api_methods_core.job_abort.html) | [`id: integer`] | `null` |
| [`core.job_download_logs`](https://api.truenas.com/v25.10/api_methods_core.job_download_logs.html) | [`id: integer`, `filename: string`, `buffered: boolean (default: false)`] | `string` |
| [`core.job_wait`](https://api.truenas.com/v25.10/api_methods_core.job_wait.html) | [`id: integer`] | `object` |
| [`core.ping`](https://api.truenas.com/v25.10/api_methods_core.ping.html) | [] | `const ("pong")` |
| [`core.ping_remote`](https://api.truenas.com/v25.10/api_methods_core.ping_remote.html) | [`options: object`] | `boolean` |
| [`core.resize_shell`](https://api.truenas.com/v25.10/api_methods_core.resize_shell.html) | [`id: string`, `cols: integer`, `rows: integer`] | `null` |
| [`core.set_options`](https://api.truenas.com/v25.10/api_methods_core.set_options.html) | [`options: object`] | `object (CoreOptions)` |
| [`core.subscribe`](https://api.truenas.com/v25.10/api_methods_core.subscribe.html) | [`event: string`] | `string` |
| [`core.unsubscribe`](https://api.truenas.com/v25.10/api_methods_core.unsubscribe.html) | [`id_: string`] | `null` |

총 15개 메서드.
