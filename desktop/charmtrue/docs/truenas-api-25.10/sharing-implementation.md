# 파일 공유 API 구현 현황

TrueNAS API v25.10.5 기준. `go generate ./internal/truenas`로 생성한다.

| 상태 | 메서드 | 종류 | 위험 | Wrapper |
|---|---|---|---|---|
| ✅ | `ftp.config` | read | false | `FTPService.Config` |
| ✅ | `ftp.update` | change | false | `FTPService.Update` |
| ✅ | `nfs.bindip_choices` | read | false | `NFSService.BindipChoices` |
| ✅ | `nfs.client_count` | read | false | `NFSService.ClientCount` |
| ✅ | `nfs.config` | read | false | `NFSService.Config` |
| ✅ | `nfs.get_nfs3_clients` | read | false | `NFSService.GetNfs3Clients` |
| ✅ | `nfs.get_nfs4_clients` | read | false | `NFSService.GetNfs4Clients` |
| ✅ | `nfs.update` | change | false | `NFSService.Update` |
| ✅ | `rsynctask.create` | create | false | `RsyncTaskService.Create` |
| ✅ | `rsynctask.delete` | destructive | true | `RsyncTaskService.Delete` |
| ✅ | `rsynctask.get_instance` | read | false | `RsyncTaskService.GetInstance` |
| ✅ | `rsynctask.query` | read | false | `RsyncTaskService.Query` |
| ✅ | `rsynctask.run` | change | false | `RsyncTaskService.Run` |
| ✅ | `rsynctask.update` | change | false | `RsyncTaskService.Update` |
| ✅ | `sharing.nfs.create` | create | false | `NFSShareService.Create` |
| ✅ | `sharing.nfs.delete` | destructive | true | `NFSShareService.Delete` |
| ✅ | `sharing.nfs.get_instance` | read | false | `NFSShareService.GetInstance` |
| ✅ | `sharing.nfs.query` | read | false | `NFSShareService.Query` |
| ✅ | `sharing.nfs.update` | change | false | `NFSShareService.Update` |
| ✅ | `sharing.smb.create` | create | false | `SMBShareService.Create` |
| ✅ | `sharing.smb.delete` | destructive | true | `SMBShareService.Delete` |
| ✅ | `sharing.smb.get_instance` | read | false | `SMBShareService.GetInstance` |
| ✅ | `sharing.smb.getacl` | read | false | `SMBShareService.Getacl` |
| ✅ | `sharing.smb.presets` | read | false | `SMBShareService.Presets` |
| ✅ | `sharing.smb.query` | read | false | `SMBShareService.Query` |
| ✅ | `sharing.smb.setacl` | destructive | true | `SMBShareService.Setacl` |
| ✅ | `sharing.smb.share_precheck` | change | false | `SMBShareService.SharePrecheck` |
| ✅ | `sharing.smb.update` | change | false | `SMBShareService.Update` |
| ✅ | `smb.bindip_choices` | read | false | `SMBService.BindipChoices` |
| ✅ | `smb.config` | read | false | `SMBService.Config` |
| ✅ | `smb.unixcharset_choices` | read | false | `SMBService.UnixcharsetChoices` |
| ✅ | `smb.update` | change | false | `SMBService.Update` |
| ✅ | `ssh.bindiface_choices` | read | false | `SSHService.BindifaceChoices` |
| ✅ | `ssh.config` | read | false | `SSHService.Config` |
| ✅ | `ssh.update` | change | false | `SSHService.Update` |
