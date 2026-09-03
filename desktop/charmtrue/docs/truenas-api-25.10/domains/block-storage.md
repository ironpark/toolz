# 블록 스토리지 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역 전체를 나열한다. 메서드 링크에서 인자, 반환 스키마, Job 여부와 권한을 확인한다.

표의 `Call parameters`와 `Return value`는 공식 v25.10.5 상세 문서의 최상위 JSON Schema(중첩 객체는 타입명과 상세 링크 참조)를 옮긴 것이다. 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`fc`](https://api.truenas.com/v25.10/api_methods_fc.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`fc.capable`](https://api.truenas.com/v25.10/api_methods_fc.capable.html) | `[]` | `boolean` |

## [`fc.fc_host`](https://api.truenas.com/v25.10/api_methods_fc.fc_host.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`fc.fc_host.create`](https://api.truenas.com/v25.10/api_methods_fc.fc_host.create.html) | `[fc_host_create: object]` | `object (FCHostEntry)` |
| [`fc.fc_host.delete`](https://api.truenas.com/v25.10/api_methods_fc.fc_host.delete.html) | `[id: integer]` | `const(true)` |
| [`fc.fc_host.get_instance`](https://api.truenas.com/v25.10/api_methods_fc.fc_host.get_instance.html) | `[id: integer, options: object]` | `object (FCHostEntry)` |
| [`fc.fc_host.query`](https://api.truenas.com/v25.10/api_methods_fc.fc_host.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`fc.fc_host.update`](https://api.truenas.com/v25.10/api_methods_fc.fc_host.update.html) | `[id: integer, fc_host_update: object]` | `object (FCHostEntry)` |

## [`fcport`](https://api.truenas.com/v25.10/api_methods_fcport.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`fcport.create`](https://api.truenas.com/v25.10/api_methods_fcport.create.html) | `[fc_Port_create: object]` | `object (FCPortEntry)` |
| [`fcport.delete`](https://api.truenas.com/v25.10/api_methods_fcport.delete.html) | `[id: integer]` | `const(true)` |
| [`fcport.get_instance`](https://api.truenas.com/v25.10/api_methods_fcport.get_instance.html) | `[id: integer, options: object]` | `object (FCPortEntry)` |
| [`fcport.port_choices`](https://api.truenas.com/v25.10/api_methods_fcport.port_choices.html) | `[include_used: boolean (default true)]` | `object` |
| [`fcport.query`](https://api.truenas.com/v25.10/api_methods_fcport.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`fcport.status`](https://api.truenas.com/v25.10/api_methods_fcport.status.html) | `[filters: array (default []), options: object]` | `array` |
| [`fcport.update`](https://api.truenas.com/v25.10/api_methods_fcport.update.html) | `[id: integer, fc_Port_update: object]` | `object (FCPortEntry)` |

## [`iscsi.auth`](https://api.truenas.com/v25.10/api_methods_iscsi.auth.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`iscsi.auth.create`](https://api.truenas.com/v25.10/api_methods_iscsi.auth.create.html) | `[data: object]` | `object (iSCSITargetAuthCredentialEntry)` |
| [`iscsi.auth.delete`](https://api.truenas.com/v25.10/api_methods_iscsi.auth.delete.html) | `[id: integer]` | `const(true)` |
| [`iscsi.auth.get_instance`](https://api.truenas.com/v25.10/api_methods_iscsi.auth.get_instance.html) | `[id: integer, options: object]` | `object (iSCSITargetAuthCredentialEntry)` |
| [`iscsi.auth.query`](https://api.truenas.com/v25.10/api_methods_iscsi.auth.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`iscsi.auth.update`](https://api.truenas.com/v25.10/api_methods_iscsi.auth.update.html) | `[id: integer, data: object]` | `object (iSCSITargetAuthCredentialEntry)` |

## [`iscsi.extent`](https://api.truenas.com/v25.10/api_methods_iscsi.extent.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`iscsi.extent.create`](https://api.truenas.com/v25.10/api_methods_iscsi.extent.create.html) | `[iscsi_extent_create: object]` | `object (iSCSITargetExtentEntry)` |
| [`iscsi.extent.delete`](https://api.truenas.com/v25.10/api_methods_iscsi.extent.delete.html) | `[id: integer, remove: boolean (default false), force: boolean (default false)]` | `const(true)` |
| [`iscsi.extent.disk_choices`](https://api.truenas.com/v25.10/api_methods_iscsi.extent.disk_choices.html) | `[]` | `object` |
| [`iscsi.extent.get_instance`](https://api.truenas.com/v25.10/api_methods_iscsi.extent.get_instance.html) | `[id: integer, options: object]` | `object (iSCSITargetExtentEntry)` |
| [`iscsi.extent.query`](https://api.truenas.com/v25.10/api_methods_iscsi.extent.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`iscsi.extent.update`](https://api.truenas.com/v25.10/api_methods_iscsi.extent.update.html) | `[id: integer, iscsi_extent_update: object]` | `object (iSCSITargetExtentEntry)` |

## [`iscsi.global`](https://api.truenas.com/v25.10/api_methods_iscsi.global.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`iscsi.global.alua_enabled`](https://api.truenas.com/v25.10/api_methods_iscsi.global.alua_enabled.html) | `[]` | `boolean` |
| [`iscsi.global.client_count`](https://api.truenas.com/v25.10/api_methods_iscsi.global.client_count.html) | `[]` | `integer` |
| [`iscsi.global.config`](https://api.truenas.com/v25.10/api_methods_iscsi.global.config.html) | `[]` | `object (ISCSIGlobalEntry)` |
| [`iscsi.global.iser_enabled`](https://api.truenas.com/v25.10/api_methods_iscsi.global.iser_enabled.html) | `[]` | `boolean` |
| [`iscsi.global.sessions`](https://api.truenas.com/v25.10/api_methods_iscsi.global.sessions.html) | `[filters: array (default []), options: object]` | `array of object` |
| [`iscsi.global.update`](https://api.truenas.com/v25.10/api_methods_iscsi.global.update.html) | `[iscsi_update: object]` | `object (ISCSIGlobalEntry)` |

## [`iscsi.initiator`](https://api.truenas.com/v25.10/api_methods_iscsi.initiator.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`iscsi.initiator.create`](https://api.truenas.com/v25.10/api_methods_iscsi.initiator.create.html) | `[iscsi_initiator_create: object]` | `object (iSCSITargetAuthorizedInitiatorEntry)` |
| [`iscsi.initiator.delete`](https://api.truenas.com/v25.10/api_methods_iscsi.initiator.delete.html) | `[id: integer]` | `const(true)` |
| [`iscsi.initiator.get_instance`](https://api.truenas.com/v25.10/api_methods_iscsi.initiator.get_instance.html) | `[id: integer, options: object]` | `object (iSCSITargetAuthorizedInitiatorEntry)` |
| [`iscsi.initiator.query`](https://api.truenas.com/v25.10/api_methods_iscsi.initiator.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`iscsi.initiator.update`](https://api.truenas.com/v25.10/api_methods_iscsi.initiator.update.html) | `[id: integer, iscsi_initiator_update: object]` | `object (iSCSITargetAuthorizedInitiatorEntry)` |

## [`iscsi.portal`](https://api.truenas.com/v25.10/api_methods_iscsi.portal.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`iscsi.portal.create`](https://api.truenas.com/v25.10/api_methods_iscsi.portal.create.html) | `[iscsi_portal_create: object]` | `object (ISCSIPortalEntry)` |
| [`iscsi.portal.delete`](https://api.truenas.com/v25.10/api_methods_iscsi.portal.delete.html) | `[id: integer]` | `const(true)` |
| [`iscsi.portal.get_instance`](https://api.truenas.com/v25.10/api_methods_iscsi.portal.get_instance.html) | `[id: integer, options: object]` | `object (ISCSIPortalEntry)` |
| [`iscsi.portal.listen_ip_choices`](https://api.truenas.com/v25.10/api_methods_iscsi.portal.listen_ip_choices.html) | `[]` | `object` |
| [`iscsi.portal.query`](https://api.truenas.com/v25.10/api_methods_iscsi.portal.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`iscsi.portal.update`](https://api.truenas.com/v25.10/api_methods_iscsi.portal.update.html) | `[id: integer, iscsi_portal_update: object]` | `object (ISCSIPortalEntry)` |

## [`iscsi.target`](https://api.truenas.com/v25.10/api_methods_iscsi.target.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`iscsi.target.create`](https://api.truenas.com/v25.10/api_methods_iscsi.target.create.html) | `[iscsi_target_create: object]` | `object (iSCSITargetEntry)` |
| [`iscsi.target.delete`](https://api.truenas.com/v25.10/api_methods_iscsi.target.delete.html) | `[id: integer, force: boolean (default false), delete_extents: boolean (default false)]` | `const(true)` |
| [`iscsi.target.get_instance`](https://api.truenas.com/v25.10/api_methods_iscsi.target.get_instance.html) | `[id: integer, options: object]` | `object (iSCSITargetEntry)` |
| [`iscsi.target.query`](https://api.truenas.com/v25.10/api_methods_iscsi.target.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`iscsi.target.update`](https://api.truenas.com/v25.10/api_methods_iscsi.target.update.html) | `[id: integer, iscsi_target_update: object]` | `object (iSCSITargetEntry)` |
| [`iscsi.target.validate_name`](https://api.truenas.com/v25.10/api_methods_iscsi.target.validate_name.html) | `[name: string, existing_id: integer \| null (default null)]` | `string \| null` |

## [`iscsi.targetextent`](https://api.truenas.com/v25.10/api_methods_iscsi.targetextent.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`iscsi.targetextent.create`](https://api.truenas.com/v25.10/api_methods_iscsi.targetextent.create.html) | `[iscsi_target_to_extent_create: object]` | `object (iSCSITargetToExtentEntry)` |
| [`iscsi.targetextent.delete`](https://api.truenas.com/v25.10/api_methods_iscsi.targetextent.delete.html) | `[id: integer, force: boolean (default false)]` | `const(true)` |
| [`iscsi.targetextent.get_instance`](https://api.truenas.com/v25.10/api_methods_iscsi.targetextent.get_instance.html) | `[id: integer, options: object]` | `object (iSCSITargetToExtentEntry)` |
| [`iscsi.targetextent.query`](https://api.truenas.com/v25.10/api_methods_iscsi.targetextent.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`iscsi.targetextent.update`](https://api.truenas.com/v25.10/api_methods_iscsi.targetextent.update.html) | `[id: integer, iscsi_target_to_extent_update: object]` | `object (iSCSITargetToExtentEntry)` |

## [`jbof`](https://api.truenas.com/v25.10/api_methods_jbof.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`jbof.create`](https://api.truenas.com/v25.10/api_methods_jbof.create.html) | `[data: object]` | `object (JBOFEntry)` |
| [`jbof.delete`](https://api.truenas.com/v25.10/api_methods_jbof.delete.html) | `[id: integer, force: boolean (default false)]` | `const(true)` |
| [`jbof.get_instance`](https://api.truenas.com/v25.10/api_methods_jbof.get_instance.html) | `[id: integer, options: object]` | `object (JBOFEntry)` |
| [`jbof.licensed`](https://api.truenas.com/v25.10/api_methods_jbof.licensed.html) | `[]` | `integer` |
| [`jbof.query`](https://api.truenas.com/v25.10/api_methods_jbof.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`jbof.reapply_config`](https://api.truenas.com/v25.10/api_methods_jbof.reapply_config.html) | `[]` | `null` |
| [`jbof.update`](https://api.truenas.com/v25.10/api_methods_jbof.update.html) | `[id: integer, data: object]` | `object (JBOFEntry)` |

## [`nvmet.global`](https://api.truenas.com/v25.10/api_methods_nvmet.global.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nvmet.global.config`](https://api.truenas.com/v25.10/api_methods_nvmet.global.config.html) | `[]` | `object (NVMetGlobalEntry)` |
| [`nvmet.global.update`](https://api.truenas.com/v25.10/api_methods_nvmet.global.update.html) | `[nvmet_update: object]` | `object (NVMetGlobalEntry)` |

## [`nvmet.host`](https://api.truenas.com/v25.10/api_methods_nvmet.host.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nvmet.host.create`](https://api.truenas.com/v25.10/api_methods_nvmet.host.create.html) | `[nvmet_host_create: object]` | `object (NVMetHostEntry)` |
| [`nvmet.host.delete`](https://api.truenas.com/v25.10/api_methods_nvmet.host.delete.html) | `[id: integer, options: object]` | `const(true)` |
| [`nvmet.host.dhchap_dhgroup_choices`](https://api.truenas.com/v25.10/api_methods_nvmet.host.dhchap_dhgroup_choices.html) | `[]` | `array of enum (of string)` |
| [`nvmet.host.dhchap_hash_choices`](https://api.truenas.com/v25.10/api_methods_nvmet.host.dhchap_hash_choices.html) | `[]` | `array of enum (of string)` |
| [`nvmet.host.generate_key`](https://api.truenas.com/v25.10/api_methods_nvmet.host.generate_key.html) | `[dhchap_hash: enum (of string) (default "SHA-256"), nqn: string \| null (default null)]` | `string` |
| [`nvmet.host.get_instance`](https://api.truenas.com/v25.10/api_methods_nvmet.host.get_instance.html) | `[id: integer, options: object]` | `object (NVMetHostEntry)` |
| [`nvmet.host.query`](https://api.truenas.com/v25.10/api_methods_nvmet.host.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nvmet.host.update`](https://api.truenas.com/v25.10/api_methods_nvmet.host.update.html) | `[id: integer, nvmet_host_update: object]` | `object (NVMetHostEntry)` |

## [`nvmet.host_subsys`](https://api.truenas.com/v25.10/api_methods_nvmet.host_subsys.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nvmet.host_subsys.create`](https://api.truenas.com/v25.10/api_methods_nvmet.host_subsys.create.html) | `[nvmet_host_subsys_create: object]` | `object (NVMetHostSubsysEntry)` |
| [`nvmet.host_subsys.delete`](https://api.truenas.com/v25.10/api_methods_nvmet.host_subsys.delete.html) | `[id: integer]` | `const(true)` |
| [`nvmet.host_subsys.get_instance`](https://api.truenas.com/v25.10/api_methods_nvmet.host_subsys.get_instance.html) | `[id: integer, options: object]` | `object (NVMetHostSubsysEntry)` |
| [`nvmet.host_subsys.query`](https://api.truenas.com/v25.10/api_methods_nvmet.host_subsys.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nvmet.host_subsys.update`](https://api.truenas.com/v25.10/api_methods_nvmet.host_subsys.update.html) | `[id: integer, nvmet_host_subsys_update: object]` | `object (NVMetHostSubsysEntry)` |

## [`nvmet.namespace`](https://api.truenas.com/v25.10/api_methods_nvmet.namespace.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nvmet.namespace.create`](https://api.truenas.com/v25.10/api_methods_nvmet.namespace.create.html) | `[nvmet_namespace_create: object]` | `object (NVMetNamespaceEntry)` |
| [`nvmet.namespace.delete`](https://api.truenas.com/v25.10/api_methods_nvmet.namespace.delete.html) | `[id: integer, options: object]` | `const(true)` |
| [`nvmet.namespace.get_instance`](https://api.truenas.com/v25.10/api_methods_nvmet.namespace.get_instance.html) | `[id: integer, options: object]` | `object (NVMetNamespaceEntry)` |
| [`nvmet.namespace.query`](https://api.truenas.com/v25.10/api_methods_nvmet.namespace.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nvmet.namespace.update`](https://api.truenas.com/v25.10/api_methods_nvmet.namespace.update.html) | `[id: integer, nvmet_namespace_update: object]` | `object (NVMetNamespaceEntry)` |

## [`nvmet.port`](https://api.truenas.com/v25.10/api_methods_nvmet.port.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nvmet.port.create`](https://api.truenas.com/v25.10/api_methods_nvmet.port.create.html) | `[nvmet_port_create: object]` | `object (NVMetPortEntry)` |
| [`nvmet.port.delete`](https://api.truenas.com/v25.10/api_methods_nvmet.port.delete.html) | `[id: integer, options: object]` | `const(true)` |
| [`nvmet.port.get_instance`](https://api.truenas.com/v25.10/api_methods_nvmet.port.get_instance.html) | `[id: integer, options: object]` | `object (NVMetPortEntry)` |
| [`nvmet.port.query`](https://api.truenas.com/v25.10/api_methods_nvmet.port.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nvmet.port.transport_address_choices`](https://api.truenas.com/v25.10/api_methods_nvmet.port.transport_address_choices.html) | `[addr_trtype: enum (of string), force_ana: boolean (default false)]` | `object` |
| [`nvmet.port.update`](https://api.truenas.com/v25.10/api_methods_nvmet.port.update.html) | `[id: integer, nvmet_port_update: object (NVMetPortUpdateRDMATCP) \| object (NVMetPortUpdateFC)]` | `object (NVMetPortEntry)` |

## [`nvmet.port_subsys`](https://api.truenas.com/v25.10/api_methods_nvmet.port_subsys.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nvmet.port_subsys.create`](https://api.truenas.com/v25.10/api_methods_nvmet.port_subsys.create.html) | `[nvmet_port_subsys_create: object]` | `object (NVMetPortSubsysEntry)` |
| [`nvmet.port_subsys.delete`](https://api.truenas.com/v25.10/api_methods_nvmet.port_subsys.delete.html) | `[id: integer]` | `const(true)` |
| [`nvmet.port_subsys.get_instance`](https://api.truenas.com/v25.10/api_methods_nvmet.port_subsys.get_instance.html) | `[id: integer, options: object]` | `object (NVMetPortSubsysEntry)` |
| [`nvmet.port_subsys.query`](https://api.truenas.com/v25.10/api_methods_nvmet.port_subsys.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nvmet.port_subsys.update`](https://api.truenas.com/v25.10/api_methods_nvmet.port_subsys.update.html) | `[id: integer, nvmet_port_subsys_update: object]` | `object (NVMetPortSubsysEntry)` |

## [`nvmet.subsys`](https://api.truenas.com/v25.10/api_methods_nvmet.subsys.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nvmet.subsys.create`](https://api.truenas.com/v25.10/api_methods_nvmet.subsys.create.html) | `[nvmet_subsys_create: object]` | `object (NVMetSubsysEntry)` |
| [`nvmet.subsys.delete`](https://api.truenas.com/v25.10/api_methods_nvmet.subsys.delete.html) | `[id: integer, options: object]` | `const(true)` |
| [`nvmet.subsys.get_instance`](https://api.truenas.com/v25.10/api_methods_nvmet.subsys.get_instance.html) | `[id: integer, options: object]` | `object (NVMetSubsysEntry)` |
| [`nvmet.subsys.query`](https://api.truenas.com/v25.10/api_methods_nvmet.subsys.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nvmet.subsys.update`](https://api.truenas.com/v25.10/api_methods_nvmet.subsys.update.html) | `[id: integer, nvmet_subsys_update: object]` | `object (NVMetSubsysEntry)` |

## [`rdma`](https://api.truenas.com/v25.10/api_methods_rdma.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`rdma.capable_protocols`](https://api.truenas.com/v25.10/api_methods_rdma.capable_protocols.html) | `[]` | `array of enum (of string)` |
| [`rdma.get_card_choices`](https://api.truenas.com/v25.10/api_methods_rdma.get_card_choices.html) | `[]` | `array of object` |
