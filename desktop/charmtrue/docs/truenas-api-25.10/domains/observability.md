# 관찰·알림·지원 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 공식 JSON-RPC 메서드를 기능별로 정리한다.
각 행은 공식 상세 페이지에서 추출한 최상위 호출 파라미터와 반환값을 보여 준다. 복합 객체의 전체 필드는 공식 상세 링크에서 확인한다.

**표기:** `name: type`은 인자의 순서와 타입이다. 공식 스키마가 명시한 기본값은 `(default: …)`로 표시하며, 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`alert`](https://api.truenas.com/v25.10/api_methods_alert.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`alert.dismiss`](https://api.truenas.com/v25.10/api_methods_alert.dismiss.html) | [`uuid: string`] | `null` |
| [`alert.list`](https://api.truenas.com/v25.10/api_methods_alert.list.html) | [] | `array of object` |
| [`alert.list_categories`](https://api.truenas.com/v25.10/api_methods_alert.list_categories.html) | [] | `array of object` |
| [`alert.list_policies`](https://api.truenas.com/v25.10/api_methods_alert.list_policies.html) | [] | `array of string` |
| [`alert.restore`](https://api.truenas.com/v25.10/api_methods_alert.restore.html) | [`uuid: string`] | `null` |

## [`alertclasses`](https://api.truenas.com/v25.10/api_methods_alertclasses.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`alertclasses.config`](https://api.truenas.com/v25.10/api_methods_alertclasses.config.html) | [] | `object (AlertClassesEntry)` |
| [`alertclasses.update`](https://api.truenas.com/v25.10/api_methods_alertclasses.update.html) | [`alert_class_update: object`] | `object (AlertClassesEntry)` |

## [`alertservice`](https://api.truenas.com/v25.10/api_methods_alertservice.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`alertservice.create`](https://api.truenas.com/v25.10/api_methods_alertservice.create.html) | [`alert_service_create: object`] | `object (AlertServiceEntry)` |
| [`alertservice.delete`](https://api.truenas.com/v25.10/api_methods_alertservice.delete.html) | [`id: integer`] | `boolean` |
| [`alertservice.get_instance`](https://api.truenas.com/v25.10/api_methods_alertservice.get_instance.html) | [`id: integer`, `options: object`] | `object (AlertServiceEntry)` |
| [`alertservice.query`](https://api.truenas.com/v25.10/api_methods_alertservice.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`alertservice.test`](https://api.truenas.com/v25.10/api_methods_alertservice.test.html) | [`alert_service_create: object`] | `boolean` |
| [`alertservice.update`](https://api.truenas.com/v25.10/api_methods_alertservice.update.html) | [`id: integer`, `alert_service_update: object`] | `object (AlertServiceEntry)` |

## [`audit`](https://api.truenas.com/v25.10/api_methods_audit.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`audit.config`](https://api.truenas.com/v25.10/api_methods_audit.config.html) | [] | `object (AuditEntry)` |
| [`audit.download_report`](https://api.truenas.com/v25.10/api_methods_audit.download_report.html) | [`data: object`] | `null` |
| [`audit.export`](https://api.truenas.com/v25.10/api_methods_audit.export.html) | [`data: object`] | `string` |
| [`audit.query`](https://api.truenas.com/v25.10/api_methods_audit.query.html) | [`data: object`] | `integer` |
| [`audit.update`](https://api.truenas.com/v25.10/api_methods_audit.update.html) | [`data: object`] | `object (AuditEntry)` |

## [`mail`](https://api.truenas.com/v25.10/api_methods_mail.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`mail.config`](https://api.truenas.com/v25.10/api_methods_mail.config.html) | [] | `object (MailEntry)` |
| [`mail.local_administrator_email`](https://api.truenas.com/v25.10/api_methods_mail.local_administrator_email.html) | [] | `string` |
| [`mail.send`](https://api.truenas.com/v25.10/api_methods_mail.send.html) | [`message: object`, `config: object`] | `boolean` |
| [`mail.update`](https://api.truenas.com/v25.10/api_methods_mail.update.html) | [`data: object`] | `object (MailEntry)` |

## [`reporting`](https://api.truenas.com/v25.10/api_methods_reporting.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`reporting.config`](https://api.truenas.com/v25.10/api_methods_reporting.config.html) | [] | `object (ReportingEntry)` |
| [`reporting.get_data`](https://api.truenas.com/v25.10/api_methods_reporting.get_data.html) | [`graphs: array of object`, `query: object`] | `array of object` |
| [`reporting.graph`](https://api.truenas.com/v25.10/api_methods_reporting.graph.html) | [`str: string`, `query: object`] | `array of object` |
| [`reporting.netdata_get_data`](https://api.truenas.com/v25.10/api_methods_reporting.netdata_get_data.html) | [`graphs: array of object`, `query: object`] | `array of object` |
| [`reporting.netdata_graph`](https://api.truenas.com/v25.10/api_methods_reporting.netdata_graph.html) | [`str: string`, `query: object`] | `array of object` |
| [`reporting.update`](https://api.truenas.com/v25.10/api_methods_reporting.update.html) | [`reporting_update: object`] | `object (ReportingEntry)` |

## [`reporting.exporters`](https://api.truenas.com/v25.10/api_methods_reporting.exporters.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`reporting.exporters.create`](https://api.truenas.com/v25.10/api_methods_reporting.exporters.create.html) | [`reporting_exporter_create: object`] | `object (ReportingExportsEntry)` |
| [`reporting.exporters.delete`](https://api.truenas.com/v25.10/api_methods_reporting.exporters.delete.html) | [`id: integer`] | `boolean` |
| [`reporting.exporters.exporter_schemas`](https://api.truenas.com/v25.10/api_methods_reporting.exporters.exporter_schemas.html) | [] | `array of object` |
| [`reporting.exporters.get_instance`](https://api.truenas.com/v25.10/api_methods_reporting.exporters.get_instance.html) | [`id: integer`, `options: object`] | `object (ReportingExportsEntry)` |
| [`reporting.exporters.query`](https://api.truenas.com/v25.10/api_methods_reporting.exporters.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`reporting.exporters.update`](https://api.truenas.com/v25.10/api_methods_reporting.exporters.update.html) | [`id: integer`, `reporting_exporter_update: object`] | `object (ReportingExportsEntry)` |

## [`snmp`](https://api.truenas.com/v25.10/api_methods_snmp.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`snmp.config`](https://api.truenas.com/v25.10/api_methods_snmp.config.html) | [] | `object (SNMPEntry)` |
| [`snmp.update`](https://api.truenas.com/v25.10/api_methods_snmp.update.html) | [`snmp_update: object`] | `object (SNMPEntry)` |

## [`support`](https://api.truenas.com/v25.10/api_methods_support.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`support.attach_ticket`](https://api.truenas.com/v25.10/api_methods_support.attach_ticket.html) | [`data: object`] | `null` |
| [`support.attach_ticket_max_size`](https://api.truenas.com/v25.10/api_methods_support.attach_ticket_max_size.html) | [] | `integer` |
| [`support.config`](https://api.truenas.com/v25.10/api_methods_support.config.html) | [] | `object (SupportEntry)` |
| [`support.fields`](https://api.truenas.com/v25.10/api_methods_support.fields.html) | [] | `array of array` |
| [`support.is_available`](https://api.truenas.com/v25.10/api_methods_support.is_available.html) | [] | `boolean` |
| [`support.is_available_and_enabled`](https://api.truenas.com/v25.10/api_methods_support.is_available_and_enabled.html) | [] | `boolean` |
| [`support.new_ticket`](https://api.truenas.com/v25.10/api_methods_support.new_ticket.html) | [`data: object (variants: SupportNewTicketEnterprise, SupportNewTicketCommunity)`] | `object (SupportNewTicketResult)` |
| [`support.similar_issues`](https://api.truenas.com/v25.10/api_methods_support.similar_issues.html) | [`query: string`] | `array of object` |
| [`support.update`](https://api.truenas.com/v25.10/api_methods_support.update.html) | [`data: object`] | `object (SupportEntry)` |

## [`ups`](https://api.truenas.com/v25.10/api_methods_ups.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`ups.config`](https://api.truenas.com/v25.10/api_methods_ups.config.html) | [] | `object (UPSEntry)` |
| [`ups.driver_choices`](https://api.truenas.com/v25.10/api_methods_ups.driver_choices.html) | [] | `object` |
| [`ups.port_choices`](https://api.truenas.com/v25.10/api_methods_ups.port_choices.html) | [] | `array of string` |
| [`ups.update`](https://api.truenas.com/v25.10/api_methods_ups.update.html) | [`ups_update: object`] | `object (UPSEntry)` |

총 49개 메서드.
