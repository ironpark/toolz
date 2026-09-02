# 파일 공유 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역 전체를 나열한다. 메서드 링크에서 인자, 반환 스키마, Job 여부와 권한을 확인한다.

표의 `Call parameters`와 `Return value`는 공식 v25.10.5 상세 문서의 최상위 JSON Schema(중첩 객체는 타입명과 상세 링크 참조)를 옮긴 것이다. 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`ftp`](https://api.truenas.com/v25.10/api_methods_ftp.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`ftp.config`](https://api.truenas.com/v25.10/api_methods_ftp.config.html) | `[]` | `object (FTPEntry)` |
| [`ftp.update`](https://api.truenas.com/v25.10/api_methods_ftp.update.html) | `[ftp_update: object]` | `object (FTPEntry)` |

## [`nfs`](https://api.truenas.com/v25.10/api_methods_nfs.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`nfs.bindip_choices`](https://api.truenas.com/v25.10/api_methods_nfs.bindip_choices.html) | `[]` | `object` |
| [`nfs.client_count`](https://api.truenas.com/v25.10/api_methods_nfs.client_count.html) | `[]` | `integer` |
| [`nfs.config`](https://api.truenas.com/v25.10/api_methods_nfs.config.html) | `[]` | `object (NFSEntry)` |
| [`nfs.get_nfs3_clients`](https://api.truenas.com/v25.10/api_methods_nfs.get_nfs3_clients.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nfs.get_nfs4_clients`](https://api.truenas.com/v25.10/api_methods_nfs.get_nfs4_clients.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`nfs.update`](https://api.truenas.com/v25.10/api_methods_nfs.update.html) | `[nfs_update: object]` | `object (NFSEntry)` |

## [`rsynctask`](https://api.truenas.com/v25.10/api_methods_rsynctask.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`rsynctask.create`](https://api.truenas.com/v25.10/api_methods_rsynctask.create.html) | `[rsync_task_create: object]` | `object (RsyncTaskEntry)` |
| [`rsynctask.delete`](https://api.truenas.com/v25.10/api_methods_rsynctask.delete.html) | `[id: integer]` | `boolean` |
| [`rsynctask.get_instance`](https://api.truenas.com/v25.10/api_methods_rsynctask.get_instance.html) | `[id: integer, options: object]` | `object (RsyncTaskEntry)` |
| [`rsynctask.query`](https://api.truenas.com/v25.10/api_methods_rsynctask.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`rsynctask.run`](https://api.truenas.com/v25.10/api_methods_rsynctask.run.html) | `[id: integer]` | `null` |
| [`rsynctask.update`](https://api.truenas.com/v25.10/api_methods_rsynctask.update.html) | `[id: integer, rsync_task_update: object]` | `object (RsyncTaskEntry)` |

## [`sharing.nfs`](https://api.truenas.com/v25.10/api_methods_sharing.nfs.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`sharing.nfs.create`](https://api.truenas.com/v25.10/api_methods_sharing.nfs.create.html) | `[data: object]` | `object (SharingNFSEntry)` |
| [`sharing.nfs.delete`](https://api.truenas.com/v25.10/api_methods_sharing.nfs.delete.html) | `[id: integer]` | `const(true)` |
| [`sharing.nfs.get_instance`](https://api.truenas.com/v25.10/api_methods_sharing.nfs.get_instance.html) | `[id: integer, options: object]` | `object (SharingNFSEntry)` |
| [`sharing.nfs.query`](https://api.truenas.com/v25.10/api_methods_sharing.nfs.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`sharing.nfs.update`](https://api.truenas.com/v25.10/api_methods_sharing.nfs.update.html) | `[id: integer, data: object]` | `object (SharingNFSEntry)` |

## [`sharing.smb`](https://api.truenas.com/v25.10/api_methods_sharing.smb.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`sharing.smb.create`](https://api.truenas.com/v25.10/api_methods_sharing.smb.create.html) | `[data: object]` | `object (SharingSMBEntry)` |
| [`sharing.smb.delete`](https://api.truenas.com/v25.10/api_methods_sharing.smb.delete.html) | `[id: integer]` | `const(true)` |
| [`sharing.smb.get_instance`](https://api.truenas.com/v25.10/api_methods_sharing.smb.get_instance.html) | `[id: integer, options: object]` | `object (SharingSMBEntry)` |
| [`sharing.smb.getacl`](https://api.truenas.com/v25.10/api_methods_sharing.smb.getacl.html) | `[smb_getacl: object]` | `object (SMBShareAcl)` |
| [`sharing.smb.presets`](https://api.truenas.com/v25.10/api_methods_sharing.smb.presets.html) | `[]` | `object` |
| [`sharing.smb.query`](https://api.truenas.com/v25.10/api_methods_sharing.smb.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`sharing.smb.setacl`](https://api.truenas.com/v25.10/api_methods_sharing.smb.setacl.html) | `[smb_setacl: object]` | `object (SMBShareAcl)` |
| [`sharing.smb.share_precheck`](https://api.truenas.com/v25.10/api_methods_sharing.smb.share_precheck.html) | `[smb_share_precheck: object]` | `null` |
| [`sharing.smb.update`](https://api.truenas.com/v25.10/api_methods_sharing.smb.update.html) | `[id: integer, data: object]` | `object (SharingSMBEntry)` |

## [`smb`](https://api.truenas.com/v25.10/api_methods_smb.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`smb.bindip_choices`](https://api.truenas.com/v25.10/api_methods_smb.bindip_choices.html) | `[]` | `object` |
| [`smb.config`](https://api.truenas.com/v25.10/api_methods_smb.config.html) | `[]` | `object (SMBEntry)` |
| [`smb.unixcharset_choices`](https://api.truenas.com/v25.10/api_methods_smb.unixcharset_choices.html) | `[]` | `object` |
| [`smb.update`](https://api.truenas.com/v25.10/api_methods_smb.update.html) | `[smb_update: object]` | `object (SMBEntry)` |

## [`ssh`](https://api.truenas.com/v25.10/api_methods_ssh.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`ssh.bindiface_choices`](https://api.truenas.com/v25.10/api_methods_ssh.bindiface_choices.html) | `[]` | `object` |
| [`ssh.config`](https://api.truenas.com/v25.10/api_methods_ssh.config.html) | `[]` | `object (SSHEntry)` |
| [`ssh.update`](https://api.truenas.com/v25.10/api_methods_ssh.update.html) | `[data: object]` | `object (SSHEntry)` |
