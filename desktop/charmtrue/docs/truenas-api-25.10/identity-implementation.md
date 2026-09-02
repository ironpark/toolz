# Identity API 구현 현황

TrueNAS API v25.10.5 기준. `go generate ./internal/truenas`로 생성한다.

| 상태 | 메서드 | 종류 | 위험 | Wrapper |
|---|---|---|---|---|
| ✅ | `api_key.create` | create | false | `APIKeyService.Create` |
| ✅ | `api_key.delete` | destructive | true | `APIKeyService.Delete` |
| ✅ | `api_key.get_instance` | read | false | `APIKeyService.GetInstance` |
| ✅ | `api_key.my_keys` | read | false | `APIKeyService.MyKeys` |
| ✅ | `api_key.query` | read | false | `APIKeyService.Query` |
| ✅ | `api_key.update` | change | false | `APIKeyService.Update` |
| ✅ | `auth.generate_onetime_password` | change | false | `AuthService.GenerateOnetimePassword` |
| ✅ | `auth.generate_token` | change | false | `AuthService.GenerateToken` |
| ✅ | `auth.login` | change | false | `AuthService.Login` |
| ✅ | `auth.login_ex` | change | false | `AuthService.LoginEx` |
| ✅ | `auth.login_ex_continue` | change | false | `AuthService.LoginExContinue` |
| ✅ | `auth.login_with_api_key` | change | false | `AuthService.LoginWithApiKey` |
| ✅ | `auth.login_with_token` | change | false | `AuthService.LoginWithToken` |
| ✅ | `auth.logout` | change | false | `AuthService.Logout` |
| ✅ | `auth.me` | read | false | `AuthService.Me` |
| ✅ | `auth.mechanism_choices` | change | false | `AuthService.MechanismChoices` |
| ✅ | `auth.sessions` | read | false | `AuthService.Sessions` |
| ✅ | `auth.set_attribute` | change | false | `AuthService.SetAttribute` |
| ✅ | `auth.terminate_other_sessions` | destructive | true | `AuthService.TerminateOtherSessions` |
| ✅ | `auth.terminate_session` | destructive | true | `AuthService.TerminateSession` |
| ✅ | `auth.twofactor.config` | read | false | `TwoFactorService.Config` |
| ✅ | `auth.twofactor.update` | change | false | `TwoFactorService.Update` |
| ✅ | `group.create` | create | false | `GroupService.Create` |
| ✅ | `group.delete` | destructive | true | `GroupService.Delete` |
| ✅ | `group.get_group_obj` | change | false | `GroupService.GetGroupObj` |
| ✅ | `group.get_instance` | read | false | `GroupService.GetInstance` |
| ✅ | `group.get_next_gid` | change | false | `GroupService.GetNextGid` |
| ✅ | `group.has_password_enabled_user` | change | false | `GroupService.HasPasswordEnabledUser` |
| ✅ | `group.query` | read | false | `GroupService.Query` |
| ✅ | `group.update` | change | false | `GroupService.Update` |
| ✅ | `privilege.create` | create | false | `PrivilegeService.Create` |
| ✅ | `privilege.delete` | destructive | true | `PrivilegeService.Delete` |
| ✅ | `privilege.get_instance` | read | false | `PrivilegeService.GetInstance` |
| ✅ | `privilege.query` | read | false | `PrivilegeService.Query` |
| ✅ | `privilege.roles` | read | false | `PrivilegeService.Roles` |
| ✅ | `privilege.update` | change | false | `PrivilegeService.Update` |
| ✅ | `user.create` | create | false | `UserService.Create` |
| ✅ | `user.delete` | destructive | true | `UserService.Delete` |
| ✅ | `user.get_instance` | read | false | `UserService.GetInstance` |
| ✅ | `user.get_next_uid` | change | false | `UserService.GetNextUid` |
| ✅ | `user.get_user_obj` | change | false | `UserService.GetUserObj` |
| ✅ | `user.has_local_administrator_set_up` | change | false | `UserService.HasLocalAdministratorSetUp` |
| ✅ | `user.query` | read | false | `UserService.Query` |
| ✅ | `user.renew_2fa_secret` | change | false | `UserService.Renew2faSecret` |
| ✅ | `user.set_password` | destructive | true | `UserService.SetPassword` |
| ✅ | `user.setup_local_administrator` | change | false | `UserService.SetupLocalAdministrator` |
| ✅ | `user.shell_choices` | change | false | `UserService.ShellChoices` |
| ✅ | `user.unset_2fa_secret` | destructive | true | `UserService.Unset2faSecret` |
| ✅ | `user.update` | change | false | `UserService.Update` |
