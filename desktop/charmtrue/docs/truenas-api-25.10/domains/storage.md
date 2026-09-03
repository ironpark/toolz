# 스토리지·ZFS API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역을 메서드별 호출 시그니처와 반환 타입으로 정리한다. 복합 객체의 전체 필드는 공식 상세 문서에서 확인할 수 있다.

`Params`는 JSON-RPC positional tuple의 최상위 항목이며, 인자가 없으면 `[]`로 표시한다. `Returns`는 공식 JSON schema의 최상위 반환 타입/union이다.

`pool.scrub`는 공식 RST에서 scrub 관련 하위 메서드를 묶는 namespace 페이지로만 제공되며 직접 호출 schema가 없으므로 Params/Returns를 `N/A`로 표시한다.

## [`device`](https://api.truenas.com/v25.10/api_methods_device.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`device.get_info`](https://api.truenas.com/v25.10/api_methods_device.get_info.html) | `data: object` | `object / array of object` |

## [`disk`](https://api.truenas.com/v25.10/api_methods_disk.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`disk.details`](https://api.truenas.com/v25.10/api_methods_disk.details.html) | `data: object` | `array / object` |
| [`disk.get_used`](https://api.truenas.com/v25.10/api_methods_disk.get_used.html) | `join_partitions: boolean (default false)` | `array` |
| [`disk.query`](https://api.truenas.com/v25.10/api_methods_disk.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / DiskEntry / DiskQueryResultItem / integer` |
| [`disk.temperature_agg`](https://api.truenas.com/v25.10/api_methods_disk.temperature_agg.html) | `names: array of string`<br>`days: integer (default 7)` | `object` |
| [`disk.temperature_alerts`](https://api.truenas.com/v25.10/api_methods_disk.temperature_alerts.html) | `names: array of string` | `array of object` |
| [`disk.temperatures`](https://api.truenas.com/v25.10/api_methods_disk.temperatures.html) | `name: array of string`<br>`include_thresholds: boolean (default false)` | `object` |
| [`disk.update`](https://api.truenas.com/v25.10/api_methods_disk.update.html) | `id: string`<br>`data: object` | `object (DiskEntry)` |
| [`disk.wipe`](https://api.truenas.com/v25.10/api_methods_disk.wipe.html) | `dev: string`<br>`mode: enum (of string)`<br>`synccache: boolean (default true)` | `null` |

## [`enclosure.label`](https://api.truenas.com/v25.10/api_methods_enclosure.label.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`enclosure.label.set`](https://api.truenas.com/v25.10/api_methods_enclosure.label.set.html) | `id: string`<br>`label: string` | `null` |

## [`enclosure2`](https://api.truenas.com/v25.10/api_methods_enclosure2.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`enclosure2.query`](https://api.truenas.com/v25.10/api_methods_enclosure2.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / Enclosure2Entry / Enclosure2QueryResultItem / integer` |
| [`enclosure2.set_slot_status`](https://api.truenas.com/v25.10/api_methods_enclosure2.set_slot_status.html) | `Enclosure2SetSlotStatus: object` | `null` |

## [`filesystem`](https://api.truenas.com/v25.10/api_methods_filesystem.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`filesystem.chown`](https://api.truenas.com/v25.10/api_methods_filesystem.chown.html) | `filesystem_chown: object` | `null` |
| [`filesystem.get`](https://api.truenas.com/v25.10/api_methods_filesystem.get.html) | `path: string` | `null` |
| [`filesystem.get_zfs_attributes`](https://api.truenas.com/v25.10/api_methods_filesystem.get_zfs_attributes.html) | `path: string` | `object (ZFSFileAttrsData)` |
| [`filesystem.getacl`](https://api.truenas.com/v25.10/api_methods_filesystem.getacl.html) | `path: string`<br>`simplified: boolean (default true)`<br>`resolve_ids: boolean (default false)` | `NFS4ACLResult / POSIXACLResult / DISABLED_ACLResult` |
| [`filesystem.listdir`](https://api.truenas.com/v25.10/api_methods_filesystem.listdir.html) | `path: string`<br>`query_filters: array (default [])`<br>`query_options: object` | `array of object / FilesystemDirEntry / FilesystemDirQueryResultItem / integer` |
| [`filesystem.mkdir`](https://api.truenas.com/v25.10/api_methods_filesystem.mkdir.html) | `filesystem_mkdir: object` | `object (FilesystemDirEntry)` |
| [`filesystem.put`](https://api.truenas.com/v25.10/api_methods_filesystem.put.html) | `path: string`<br>`options: object (default {"append": false, "mode": null})` | `true` |
| [`filesystem.set_zfs_attributes`](https://api.truenas.com/v25.10/api_methods_filesystem.set_zfs_attributes.html) | `set_zfs_file_attributes: object` | `object (ZFSFileAttrsData)` |
| [`filesystem.setacl`](https://api.truenas.com/v25.10/api_methods_filesystem.setacl.html) | `filesystem_acl: object` | `NFS4ACLResult / POSIXACLResult / DISABLED_ACLResult` |
| [`filesystem.setperm`](https://api.truenas.com/v25.10/api_methods_filesystem.setperm.html) | `filesystem_setperm: object` | `null` |
| [`filesystem.stat`](https://api.truenas.com/v25.10/api_methods_filesystem.stat.html) | `path: string` | `object (FilesystemStatData)` |
| [`filesystem.statfs`](https://api.truenas.com/v25.10/api_methods_filesystem.statfs.html) | `path: string` | `object (FilesystemStatfsData)` |

## [`filesystem.acltemplate`](https://api.truenas.com/v25.10/api_methods_filesystem.acltemplate.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`filesystem.acltemplate.by_path`](https://api.truenas.com/v25.10/api_methods_filesystem.acltemplate.by_path.html) | `filesystem_acl: object` | `array of object` |
| [`filesystem.acltemplate.create`](https://api.truenas.com/v25.10/api_methods_filesystem.acltemplate.create.html) | `acltemplate_create: object` | `object (ACLTemplateEntry)` |
| [`filesystem.acltemplate.delete`](https://api.truenas.com/v25.10/api_methods_filesystem.acltemplate.delete.html) | `id: integer` | `true` |
| [`filesystem.acltemplate.get_instance`](https://api.truenas.com/v25.10/api_methods_filesystem.acltemplate.get_instance.html) | `id: integer`<br>`options: object` | `object (ACLTemplateEntry)` |
| [`filesystem.acltemplate.query`](https://api.truenas.com/v25.10/api_methods_filesystem.acltemplate.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / ACLTemplateEntry / ACLTemplateQueryResultItem / integer` |
| [`filesystem.acltemplate.update`](https://api.truenas.com/v25.10/api_methods_filesystem.acltemplate.update.html) | `id: integer`<br>`acltemplate_update: object` | `object (ACLTemplateEntry)` |

## [`pool`](https://api.truenas.com/v25.10/api_methods_pool.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`pool.attach`](https://api.truenas.com/v25.10/api_methods_pool.attach.html) | `oid: integer`<br>`options: object` | `null` |
| [`pool.attachments`](https://api.truenas.com/v25.10/api_methods_pool.attachments.html) | `id: integer` | `array of object` |
| [`pool.create`](https://api.truenas.com/v25.10/api_methods_pool.create.html) | `data: object` | `object (PoolEntry)` |
| [`pool.ddt_prefetch`](https://api.truenas.com/v25.10/api_methods_pool.ddt_prefetch.html) | `pool_name: string` | `null` |
| [`pool.ddt_prune`](https://api.truenas.com/v25.10/api_methods_pool.ddt_prune.html) | `options: object` | `null` |
| [`pool.detach`](https://api.truenas.com/v25.10/api_methods_pool.detach.html) | `id: integer`<br>`options: object` | `true` |
| [`pool.expand`](https://api.truenas.com/v25.10/api_methods_pool.expand.html) | `id: integer` | `null` |
| [`pool.export`](https://api.truenas.com/v25.10/api_methods_pool.export.html) | `id: integer`<br>`options: object` | `null` |
| [`pool.filesystem_choices`](https://api.truenas.com/v25.10/api_methods_pool.filesystem_choices.html) | `types: array of enum (of string) (default ["FILESYSTEM", "VOLUME"])` | `array of string` |
| [`pool.get_disks`](https://api.truenas.com/v25.10/api_methods_pool.get_disks.html) | `id: integer / null (default null)` | `array of string` |
| [`pool.get_instance`](https://api.truenas.com/v25.10/api_methods_pool.get_instance.html) | `id: integer`<br>`options: object` | `object (PoolEntry)` |
| [`pool.import_find`](https://api.truenas.com/v25.10/api_methods_pool.import_find.html) | `[]` | `array of object` |
| [`pool.import_pool`](https://api.truenas.com/v25.10/api_methods_pool.import_pool.html) | `pool_import: object` | `true` |
| [`pool.is_upgraded`](https://api.truenas.com/v25.10/api_methods_pool.is_upgraded.html) | `id: integer` | `boolean` |
| [`pool.offline`](https://api.truenas.com/v25.10/api_methods_pool.offline.html) | `id: integer`<br>`options: object` | `true` |
| [`pool.online`](https://api.truenas.com/v25.10/api_methods_pool.online.html) | `id: integer`<br>`options: object` | `true` |
| [`pool.processes`](https://api.truenas.com/v25.10/api_methods_pool.processes.html) | `id: integer` | `array of object` |
| [`pool.query`](https://api.truenas.com/v25.10/api_methods_pool.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / PoolEntry / PoolQueryResultItem / integer` |
| [`pool.remove`](https://api.truenas.com/v25.10/api_methods_pool.remove.html) | `id: integer`<br>`options: object` | `null` |
| [`pool.replace`](https://api.truenas.com/v25.10/api_methods_pool.replace.html) | `id: integer`<br>`options: object` | `true` |
| [`pool.scrub`](https://api.truenas.com/v25.10/api_methods_pool.scrub.html) | `N/A (namespace)` | `N/A (namespace)` |
| [`pool.update`](https://api.truenas.com/v25.10/api_methods_pool.update.html) | `id: integer`<br>`data: object` | `object (PoolEntry)` |
| [`pool.upgrade`](https://api.truenas.com/v25.10/api_methods_pool.upgrade.html) | `id: integer` | `true` |
| [`pool.validate_name`](https://api.truenas.com/v25.10/api_methods_pool.validate_name.html) | `pool_name: string` | `true` |

## [`pool.dataset`](https://api.truenas.com/v25.10/api_methods_pool.dataset.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`pool.dataset.attachments`](https://api.truenas.com/v25.10/api_methods_pool.dataset.attachments.html) | `id: string` | `array of object` |
| [`pool.dataset.change_key`](https://api.truenas.com/v25.10/api_methods_pool.dataset.change_key.html) | `id: string`<br>`options: object` | `null` |
| [`pool.dataset.checksum_choices`](https://api.truenas.com/v25.10/api_methods_pool.dataset.checksum_choices.html) | `[]` | `object (PoolDatasetChecksumChoicesResult)` |
| [`pool.dataset.compression_choices`](https://api.truenas.com/v25.10/api_methods_pool.dataset.compression_choices.html) | `[]` | `object` |
| [`pool.dataset.create`](https://api.truenas.com/v25.10/api_methods_pool.dataset.create.html) | `data: object` | `object (PoolDatasetEntry)` |
| [`pool.dataset.delete`](https://api.truenas.com/v25.10/api_methods_pool.dataset.delete.html) | `id: string`<br>`options: object` | `enum (of boolean or null)` |
| [`pool.dataset.details`](https://api.truenas.com/v25.10/api_methods_pool.dataset.details.html) | `[]` | `array of object` |
| [`pool.dataset.encryption_summary`](https://api.truenas.com/v25.10/api_methods_pool.dataset.encryption_summary.html) | `id: string`<br>`options: object` | `array of object` |
| [`pool.dataset.export_key`](https://api.truenas.com/v25.10/api_methods_pool.dataset.export_key.html) | `id: string`<br>`download: boolean (default false)` | `string / null` |
| [`pool.dataset.export_keys`](https://api.truenas.com/v25.10/api_methods_pool.dataset.export_keys.html) | `id: string` | `null` |
| [`pool.dataset.export_keys_for_replication`](https://api.truenas.com/v25.10/api_methods_pool.dataset.export_keys_for_replication.html) | `id: integer` | `null` |
| [`pool.dataset.get_instance`](https://api.truenas.com/v25.10/api_methods_pool.dataset.get_instance.html) | `id: string`<br>`options: object` | `object (PoolDatasetEntry)` |
| [`pool.dataset.get_quota`](https://api.truenas.com/v25.10/api_methods_pool.dataset.get_quota.html) | `dataset: string`<br>`quota_type: enum (of string)`<br>`filters: array (default [])`<br>`options: object` | `array` |
| [`pool.dataset.inherit_parent_encryption_properties`](https://api.truenas.com/v25.10/api_methods_pool.dataset.inherit_parent_encryption_properties.html) | `id: string` | `null` |
| [`pool.dataset.lock`](https://api.truenas.com/v25.10/api_methods_pool.dataset.lock.html) | `id: string`<br>`options: object` | `true` |
| [`pool.dataset.processes`](https://api.truenas.com/v25.10/api_methods_pool.dataset.processes.html) | `id: string` | `array of object` |
| [`pool.dataset.promote`](https://api.truenas.com/v25.10/api_methods_pool.dataset.promote.html) | `id: string` | `null` |
| [`pool.dataset.query`](https://api.truenas.com/v25.10/api_methods_pool.dataset.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / PoolDatasetEntry / PoolDatasetQueryResultItem / integer` |
| [`pool.dataset.recommended_zvol_blocksize`](https://api.truenas.com/v25.10/api_methods_pool.dataset.recommended_zvol_blocksize.html) | `pool: string` | `string` |
| [`pool.dataset.recordsize_choices`](https://api.truenas.com/v25.10/api_methods_pool.dataset.recordsize_choices.html) | `pool_name: string / null (default null)` | `array of string` |
| [`pool.dataset.rename`](https://api.truenas.com/v25.10/api_methods_pool.dataset.rename.html) | `id: string`<br>`data: object` | `null` |
| [`pool.dataset.set_quota`](https://api.truenas.com/v25.10/api_methods_pool.dataset.set_quota.html) | `dataset: string`<br>`quotas: array of object` | `null` |
| [`pool.dataset.snapshot_count`](https://api.truenas.com/v25.10/api_methods_pool.dataset.snapshot_count.html) | `dataset: string` | `integer` |
| [`pool.dataset.unlock`](https://api.truenas.com/v25.10/api_methods_pool.dataset.unlock.html) | `id: string`<br>`options: object` | `object (PoolDatasetUnlock)` |
| [`pool.dataset.update`](https://api.truenas.com/v25.10/api_methods_pool.dataset.update.html) | `id: string`<br>`data: object` | `object (PoolDatasetEntry)` |

## [`pool.resilver`](https://api.truenas.com/v25.10/api_methods_pool.resilver.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`pool.resilver.config`](https://api.truenas.com/v25.10/api_methods_pool.resilver.config.html) | `[]` | `object (PoolResilverEntry)` |
| [`pool.resilver.update`](https://api.truenas.com/v25.10/api_methods_pool.resilver.update.html) | `data: object` | `object (PoolResilverEntry)` |

## [`pool.scrub`](https://api.truenas.com/v25.10/api_methods_pool.scrub.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`pool.scrub.create`](https://api.truenas.com/v25.10/api_methods_pool.scrub.create.html) | `data: object` | `object (PoolScrubEntry)` |
| [`pool.scrub.delete`](https://api.truenas.com/v25.10/api_methods_pool.scrub.delete.html) | `id_: integer` | `true` |
| [`pool.scrub.get_instance`](https://api.truenas.com/v25.10/api_methods_pool.scrub.get_instance.html) | `id: integer`<br>`options: object` | `object (PoolScrubEntry)` |
| [`pool.scrub.query`](https://api.truenas.com/v25.10/api_methods_pool.scrub.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / PoolScrubEntry / PoolScrubQueryResultItem / integer` |
| [`pool.scrub.run`](https://api.truenas.com/v25.10/api_methods_pool.scrub.run.html) | `name: string`<br>`threshold: integer (default 35)` | `null` |
| [`pool.scrub.scrub`](https://api.truenas.com/v25.10/api_methods_pool.scrub.scrub.html) | `name: string`<br>`action: enum (of string) (default "START")` | `null` |
| [`pool.scrub.update`](https://api.truenas.com/v25.10/api_methods_pool.scrub.update.html) | `id_: integer`<br>`data: object` | `object (PoolScrubEntry)` |

## [`pool.snapshot`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`pool.snapshot.clone`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.clone.html) | `data: object` | `true` |
| [`pool.snapshot.create`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.create.html) | `data: object` | `object (PoolSnapshotCreateUpdateEntry)` |
| [`pool.snapshot.delete`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.delete.html) | `id: string`<br>`options: object` | `true` |
| [`pool.snapshot.get_instance`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.get_instance.html) | `id: string`<br>`options: object` | `object (PoolSnapshotEntry)` |
| [`pool.snapshot.hold`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.hold.html) | `id: string`<br>`options: object` | `null` |
| [`pool.snapshot.query`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / PoolSnapshotEntry / PoolSnapshotQueryResultItem / integer` |
| [`pool.snapshot.release`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.release.html) | `id: string`<br>`options: object` | `null` |
| [`pool.snapshot.rename`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.rename.html) | `id: string`<br>`options: object` | `null` |
| [`pool.snapshot.rollback`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.rollback.html) | `id: string`<br>`options: object` | `null` |
| [`pool.snapshot.update`](https://api.truenas.com/v25.10/api_methods_pool.snapshot.update.html) | `id: string`<br>`data: object` | `object (PoolSnapshotCreateUpdateEntry)` |

## [`pool.snapshottask`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`pool.snapshottask.create`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.create.html) | `data: object` | `object (PeriodicSnapshotTaskEntry)` |
| [`pool.snapshottask.delete`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.delete.html) | `id: integer`<br>`options: object` | `true` |
| [`pool.snapshottask.delete_will_change_retention_for`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.delete_will_change_retention_for.html) | `id: integer` | `object` |
| [`pool.snapshottask.get_instance`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.get_instance.html) | `id: integer`<br>`options: object` | `object (PeriodicSnapshotTaskEntry)` |
| [`pool.snapshottask.max_count`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.max_count.html) | `[]` | `integer` |
| [`pool.snapshottask.max_total_count`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.max_total_count.html) | `[]` | `integer` |
| [`pool.snapshottask.query`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / PeriodicSnapshotTaskEntry / PeriodicSnapshotTaskQueryResultItem / integer` |
| [`pool.snapshottask.run`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.run.html) | `id: integer` | `null` |
| [`pool.snapshottask.update`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.update.html) | `id: integer`<br>`data: object` | `object (PeriodicSnapshotTaskEntry)` |
| [`pool.snapshottask.update_will_change_retention_for`](https://api.truenas.com/v25.10/api_methods_pool.snapshottask.update_will_change_retention_for.html) | `id: integer`<br>`data: object` | `object` |

## [`zfs.resource`](https://api.truenas.com/v25.10/api_methods_zfs.resource.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`zfs.resource.query`](https://api.truenas.com/v25.10/api_methods_zfs.resource.query.html) | `data: object` | `array of object` |
