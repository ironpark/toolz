# 백업·복제·키 관리 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 공식 JSON-RPC 메서드를 기능별로 정리한다.
각 행은 공식 상세 페이지에서 추출한 최상위 호출 파라미터와 반환값을 보여 준다. 복합 객체의 전체 필드는 공식 상세 링크에서 확인한다.

**표기:** `name: type`은 인자의 순서와 타입이다. 공식 스키마가 명시한 기본값은 `(default: …)`로 표시하며, 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`cloud_backup`](https://api.truenas.com/v25.10/api_methods_cloud_backup.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`cloud_backup.abort`](https://api.truenas.com/v25.10/api_methods_cloud_backup.abort.html) | [`id: integer`] | `boolean` |
| [`cloud_backup.create`](https://api.truenas.com/v25.10/api_methods_cloud_backup.create.html) | [`cloud_backup: object`] | `object (CloudBackupEntry)` |
| [`cloud_backup.delete`](https://api.truenas.com/v25.10/api_methods_cloud_backup.delete.html) | [`id: integer`] | `const (true)` |
| [`cloud_backup.delete_snapshot`](https://api.truenas.com/v25.10/api_methods_cloud_backup.delete_snapshot.html) | [`id: integer`, `snapshot_id: string`] | `null` |
| [`cloud_backup.get_instance`](https://api.truenas.com/v25.10/api_methods_cloud_backup.get_instance.html) | [`id: integer`, `options: object`] | `object (CloudBackupEntry)` |
| [`cloud_backup.list_snapshot_directory`](https://api.truenas.com/v25.10/api_methods_cloud_backup.list_snapshot_directory.html) | [`id: integer`, `snapshot_id: string`, `path: string`] | `array of object` |
| [`cloud_backup.list_snapshots`](https://api.truenas.com/v25.10/api_methods_cloud_backup.list_snapshots.html) | [`id: integer`] | `array of object` |
| [`cloud_backup.query`](https://api.truenas.com/v25.10/api_methods_cloud_backup.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`cloud_backup.restore`](https://api.truenas.com/v25.10/api_methods_cloud_backup.restore.html) | [`id: integer`, `snapshot_id: string`, `subfolder: string`, `destination_path: string`, `options: object (default: { "exclude": [], "include": [], "rate_limit": null })`] | `null` |
| [`cloud_backup.sync`](https://api.truenas.com/v25.10/api_methods_cloud_backup.sync.html) | [`id: integer`, `options: object`] | `null` |
| [`cloud_backup.transfer_setting_choices`](https://api.truenas.com/v25.10/api_methods_cloud_backup.transfer_setting_choices.html) | [] | `array of enum (of string)` |
| [`cloud_backup.update`](https://api.truenas.com/v25.10/api_methods_cloud_backup.update.html) | [`id: integer`, `data: object`] | `object (CloudBackupEntry)` |

## [`cloudsync`](https://api.truenas.com/v25.10/api_methods_cloudsync.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`cloudsync.abort`](https://api.truenas.com/v25.10/api_methods_cloudsync.abort.html) | [`id: integer`] | `boolean` |
| [`cloudsync.create`](https://api.truenas.com/v25.10/api_methods_cloudsync.create.html) | [`cloud_sync_create: object`] | `object (CloudSyncEntry)` |
| [`cloudsync.create_bucket`](https://api.truenas.com/v25.10/api_methods_cloudsync.create_bucket.html) | [`credentials_id: integer`, `name: string`] | `null` |
| [`cloudsync.delete`](https://api.truenas.com/v25.10/api_methods_cloudsync.delete.html) | [`id: integer`] | `const (true)` |
| [`cloudsync.get_instance`](https://api.truenas.com/v25.10/api_methods_cloudsync.get_instance.html) | [`id: integer`, `options: object`] | `object (CloudSyncEntry)` |
| [`cloudsync.list_buckets`](https://api.truenas.com/v25.10/api_methods_cloudsync.list_buckets.html) | [`credentials_id: integer`] | `array of object` |
| [`cloudsync.list_directory`](https://api.truenas.com/v25.10/api_methods_cloudsync.list_directory.html) | [`cloud_sync_ls: object`] | `array of object` |
| [`cloudsync.onedrive_list_drives`](https://api.truenas.com/v25.10/api_methods_cloudsync.onedrive_list_drives.html) | [`onedrive_list_drives: object`] | `array of object` |
| [`cloudsync.providers`](https://api.truenas.com/v25.10/api_methods_cloudsync.providers.html) | [] | `array of object` |
| [`cloudsync.query`](https://api.truenas.com/v25.10/api_methods_cloudsync.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`cloudsync.restore`](https://api.truenas.com/v25.10/api_methods_cloudsync.restore.html) | [`id: integer`, `opts: object`] | `object (CloudSyncEntry)` |
| [`cloudsync.sync`](https://api.truenas.com/v25.10/api_methods_cloudsync.sync.html) | [`id: integer`, `cloud_sync_sync_options: object`] | `null` |
| [`cloudsync.sync_onetime`](https://api.truenas.com/v25.10/api_methods_cloudsync.sync_onetime.html) | [`cloud_sync_sync_onetime: object`, `cloud_sync_sync_onetime_options: object`] | `null` |
| [`cloudsync.update`](https://api.truenas.com/v25.10/api_methods_cloudsync.update.html) | [`id: integer`, `cloud_sync_update: object`] | `object (CloudSyncEntry)` |

## [`cloudsync.credentials`](https://api.truenas.com/v25.10/api_methods_cloudsync.credentials.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`cloudsync.credentials.create`](https://api.truenas.com/v25.10/api_methods_cloudsync.credentials.create.html) | [`cloud_sync_credentials_create: object`] | `object (CredentialsEntry)` |
| [`cloudsync.credentials.delete`](https://api.truenas.com/v25.10/api_methods_cloudsync.credentials.delete.html) | [`id: integer`] | `boolean` |
| [`cloudsync.credentials.get_instance`](https://api.truenas.com/v25.10/api_methods_cloudsync.credentials.get_instance.html) | [`id: integer`, `options: object`] | `object (CredentialsEntry)` |
| [`cloudsync.credentials.query`](https://api.truenas.com/v25.10/api_methods_cloudsync.credentials.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`cloudsync.credentials.update`](https://api.truenas.com/v25.10/api_methods_cloudsync.credentials.update.html) | [`id: integer`, `cloud_sync_credentials_update: object`] | `object (CredentialsEntry)` |
| [`cloudsync.credentials.verify`](https://api.truenas.com/v25.10/api_methods_cloudsync.credentials.verify.html) | [`cloud_sync_credentials_create: object`] | `object (CredentialsVerifyResult)` |

## [`keychaincredential`](https://api.truenas.com/v25.10/api_methods_keychaincredential.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`keychaincredential.create`](https://api.truenas.com/v25.10/api_methods_keychaincredential.create.html) | [`keychain_credential_create: object (variants: KeychainCredentialCreateSSHKeyPairEntry, KeychainCredentialCreateSSHCredentialsEntry)`] | `object` |
| [`keychaincredential.delete`](https://api.truenas.com/v25.10/api_methods_keychaincredential.delete.html) | [`id: integer`, `options: object (default: {"cascade": false})`] | `null` |
| [`keychaincredential.generate_ssh_key_pair`](https://api.truenas.com/v25.10/api_methods_keychaincredential.generate_ssh_key_pair.html) | [] | `object (KeychainCredentialGenerateSshKeyPairResult)` |
| [`keychaincredential.get_instance`](https://api.truenas.com/v25.10/api_methods_keychaincredential.get_instance.html) | [`id: integer`, `options: object`] | `object (KeychainCredentialEntry)` |
| [`keychaincredential.query`](https://api.truenas.com/v25.10/api_methods_keychaincredential.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`keychaincredential.remote_ssh_host_key_scan`](https://api.truenas.com/v25.10/api_methods_keychaincredential.remote_ssh_host_key_scan.html) | [`keychain_remote_ssh_host_key_scan: object`] | `string` |
| [`keychaincredential.remote_ssh_semiautomatic_setup`](https://api.truenas.com/v25.10/api_methods_keychaincredential.remote_ssh_semiautomatic_setup.html) | [`data: object`] | `object (SSHCredentialsEntry)` |
| [`keychaincredential.setup_ssh_connection`](https://api.truenas.com/v25.10/api_methods_keychaincredential.setup_ssh_connection.html) | [`options: object`] | `object (SSHCredentialsEntry)` |
| [`keychaincredential.update`](https://api.truenas.com/v25.10/api_methods_keychaincredential.update.html) | [`id: integer`, `keychain_credential_update: object (variants: KeychainCredentialUpdateSSHKeyPairEntry, KeychainCredentialUpdateSSHCredentialsEntry)`] | `object` |
| [`keychaincredential.used_by`](https://api.truenas.com/v25.10/api_methods_keychaincredential.used_by.html) | [`id: integer`] | `array of object` |

## [`kmip`](https://api.truenas.com/v25.10/api_methods_kmip.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`kmip.clear_sync_pending_keys`](https://api.truenas.com/v25.10/api_methods_kmip.clear_sync_pending_keys.html) | [] | `null` |
| [`kmip.config`](https://api.truenas.com/v25.10/api_methods_kmip.config.html) | [] | `object (KMIPEntry)` |
| [`kmip.kmip_sync_pending`](https://api.truenas.com/v25.10/api_methods_kmip.kmip_sync_pending.html) | [] | `boolean` |
| [`kmip.sync_keys`](https://api.truenas.com/v25.10/api_methods_kmip.sync_keys.html) | [] | `null` |
| [`kmip.update`](https://api.truenas.com/v25.10/api_methods_kmip.update.html) | [`kmip_update: object`] | `object (KMIPEntry)` |

## [`replication`](https://api.truenas.com/v25.10/api_methods_replication.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`replication.count_eligible_manual_snapshots`](https://api.truenas.com/v25.10/api_methods_replication.count_eligible_manual_snapshots.html) | [`count_eligible_manual_snapshots: object`] | `object (ReplicationCountEligibleManualSnapshotsResult)` |
| [`replication.create`](https://api.truenas.com/v25.10/api_methods_replication.create.html) | [`replication_create: object`] | `object (ReplicationEntry)` |
| [`replication.create_dataset`](https://api.truenas.com/v25.10/api_methods_replication.create_dataset.html) | [`dataset: string`, `transport: enum (of string)`, `ssh_credentials: integer or null (default: null)`] | `null` |
| [`replication.delete`](https://api.truenas.com/v25.10/api_methods_replication.delete.html) | [`id: integer`] | `boolean` |
| [`replication.get_instance`](https://api.truenas.com/v25.10/api_methods_replication.get_instance.html) | [`id: integer`, `options: object`] | `object (ReplicationEntry)` |
| [`replication.list_datasets`](https://api.truenas.com/v25.10/api_methods_replication.list_datasets.html) | [`transport: enum (of string)`, `ssh_credentials: integer or null (default: null)`] | `array of string` |
| [`replication.list_naming_schemas`](https://api.truenas.com/v25.10/api_methods_replication.list_naming_schemas.html) | [] | `array of string` |
| [`replication.query`](https://api.truenas.com/v25.10/api_methods_replication.query.html) | [`filters: array (default: [])`, `options: object (default: {extra: {}, order_by: [], select: [], count: false, get: false, offset: 0, limit: 0, force_sql_filters: false})`] | `array of object` |
| [`replication.restore`](https://api.truenas.com/v25.10/api_methods_replication.restore.html) | [`id: integer`, `replication_restore: object`] | `object (ReplicationEntry)` |
| [`replication.run`](https://api.truenas.com/v25.10/api_methods_replication.run.html) | [`id: integer`] | `null` |
| [`replication.run_onetime`](https://api.truenas.com/v25.10/api_methods_replication.run_onetime.html) | [`replication_run_onetime: object`] | `null` |
| [`replication.target_unmatched_snapshots`](https://api.truenas.com/v25.10/api_methods_replication.target_unmatched_snapshots.html) | [`direction: enum (of string)`, `source_datasets: array of string`, `target_dataset: string`, `transport: enum (of string)`, `ssh_credentials: integer or null (default: null)`] | `object` |
| [`replication.update`](https://api.truenas.com/v25.10/api_methods_replication.update.html) | [`id: integer`, `replication_update: object`] | `object (ReplicationEntry)` |

## [`replication.config`](https://api.truenas.com/v25.10/api_methods_replication.config.html)

| 메서드 | Call parameters | Return value |
| --- | --- | --- |
| [`replication.config.config`](https://api.truenas.com/v25.10/api_methods_replication.config.config.html) | [] | `object (ReplicationConfigEntry)` |
| [`replication.config.update`](https://api.truenas.com/v25.10/api_methods_replication.config.update.html) | [`replication_config_update: object`] | `object (ReplicationConfigEntry)` |

총 62개 메서드.
