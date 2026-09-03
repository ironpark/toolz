# 인증·계정·권한 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역을 메서드별 호출 시그니처와 반환 타입으로 정리한다. 복합 객체의 전체 필드는 공식 상세 문서에서 확인할 수 있다.

`Params`는 JSON-RPC positional tuple의 최상위 항목이며, 인자가 없으면 `[]`로 표시한다. `Returns`는 공식 JSON schema의 최상위 반환 타입/union이다.

## [`api_key`](https://api.truenas.com/v25.10/api_methods_api_key.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`api_key.create`](https://api.truenas.com/v25.10/api_methods_api_key.create.html) | `api_key_create: object` | `object (ApiKeyEntryWithKey)` |
| [`api_key.delete`](https://api.truenas.com/v25.10/api_methods_api_key.delete.html) | `id: integer` | `true` |
| [`api_key.get_instance`](https://api.truenas.com/v25.10/api_methods_api_key.get_instance.html) | `id: integer`<br>`options: object` | `object (ApiKeyEntry)` |
| [`api_key.my_keys`](https://api.truenas.com/v25.10/api_methods_api_key.my_keys.html) | `[]` | `array of object` |
| [`api_key.query`](https://api.truenas.com/v25.10/api_methods_api_key.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / ApiKeyEntry / ApiKeyQueryResultItem / integer` |
| [`api_key.update`](https://api.truenas.com/v25.10/api_methods_api_key.update.html) | `id: integer`<br>`api_key_update: object` | `ApiKeyEntryWithKey / ApiKeyEntry` |

## [`auth`](https://api.truenas.com/v25.10/api_methods_auth.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`auth.generate_onetime_password`](https://api.truenas.com/v25.10/api_methods_auth.generate_onetime_password.html) | `generate_single_use_password: object` | `string` |
| [`auth.generate_token`](https://api.truenas.com/v25.10/api_methods_auth.generate_token.html) | `ttl: integer / null (default 600)`<br>`attrs: object (default {})`<br>`match_origin: boolean (default true)`<br>`single_use: boolean (default false)` | `string` |
| [`auth.login`](https://api.truenas.com/v25.10/api_methods_auth.login.html) | `username: string`<br>`password: string`<br>`otp_token: string / null (default "")` | `boolean` |
| [`auth.login_ex`](https://api.truenas.com/v25.10/api_methods_auth.login_ex.html) | `login_data: object` | `AuthRespSuccess / AuthRespAuthErr / AuthRespExpired / AuthRespOTPRequired / AuthRespAuthRedirect` |
| [`auth.login_ex_continue`](https://api.truenas.com/v25.10/api_methods_auth.login_ex_continue.html) | `login_data: object` | `AuthRespSuccess / AuthRespAuthErr / AuthRespExpired / AuthRespOTPRequired / AuthRespAuthRedirect` |
| [`auth.login_with_api_key`](https://api.truenas.com/v25.10/api_methods_auth.login_with_api_key.html) | `api_key: string` | `boolean` |
| [`auth.login_with_token`](https://api.truenas.com/v25.10/api_methods_auth.login_with_token.html) | `token: string` | `boolean` |
| [`auth.logout`](https://api.truenas.com/v25.10/api_methods_auth.logout.html) | `[]` | `true` |
| [`auth.me`](https://api.truenas.com/v25.10/api_methods_auth.me.html) | `[]` | `object (AuthMeResult)` |
| [`auth.mechanism_choices`](https://api.truenas.com/v25.10/api_methods_auth.mechanism_choices.html) | `[]` | `array of string` |
| [`auth.sessions`](https://api.truenas.com/v25.10/api_methods_auth.sessions.html) | `filters: array (default [])`<br>`options: object` | `array of object / AuthSessionsEntry / AuthSessionsQueryResultItem / integer` |
| [`auth.set_attribute`](https://api.truenas.com/v25.10/api_methods_auth.set_attribute.html) | `key: string`<br>`value: object` | `null` |
| [`auth.terminate_other_sessions`](https://api.truenas.com/v25.10/api_methods_auth.terminate_other_sessions.html) | `[]` | `true` |
| [`auth.terminate_session`](https://api.truenas.com/v25.10/api_methods_auth.terminate_session.html) | `id: string` | `boolean` |

## [`auth.twofactor`](https://api.truenas.com/v25.10/api_methods_auth.twofactor.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`auth.twofactor.config`](https://api.truenas.com/v25.10/api_methods_auth.twofactor.config.html) | `[]` | `object (TwoFactorAuthEntry)` |
| [`auth.twofactor.update`](https://api.truenas.com/v25.10/api_methods_auth.twofactor.update.html) | `auth_twofactor_update: object` | `object (TwoFactorAuthEntry)` |

## [`group`](https://api.truenas.com/v25.10/api_methods_group.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`group.create`](https://api.truenas.com/v25.10/api_methods_group.create.html) | `group_create: object` | `integer` |
| [`group.delete`](https://api.truenas.com/v25.10/api_methods_group.delete.html) | `id: integer`<br>`options: object (default {"delete_users": false})` | `integer` |
| [`group.get_group_obj`](https://api.truenas.com/v25.10/api_methods_group.get_group_obj.html) | `get_group_obj: object` | `object (GroupGetGroupObjResult)` |
| [`group.get_instance`](https://api.truenas.com/v25.10/api_methods_group.get_instance.html) | `id: integer`<br>`options: object` | `object (GroupEntry)` |
| [`group.get_next_gid`](https://api.truenas.com/v25.10/api_methods_group.get_next_gid.html) | `[]` | `integer` |
| [`group.has_password_enabled_user`](https://api.truenas.com/v25.10/api_methods_group.has_password_enabled_user.html) | `gids: array of integer`<br>`exclude_user_ids: array of integer (default [])` | `boolean` |
| [`group.query`](https://api.truenas.com/v25.10/api_methods_group.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / GroupEntry / GroupQueryResultItem / integer` |
| [`group.update`](https://api.truenas.com/v25.10/api_methods_group.update.html) | `id: integer`<br>`group_update: object` | `integer` |

## [`privilege`](https://api.truenas.com/v25.10/api_methods_privilege.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`privilege.create`](https://api.truenas.com/v25.10/api_methods_privilege.create.html) | `privilege_create: object` | `object (PrivilegeEntry)` |
| [`privilege.delete`](https://api.truenas.com/v25.10/api_methods_privilege.delete.html) | `id: integer` | `boolean` |
| [`privilege.get_instance`](https://api.truenas.com/v25.10/api_methods_privilege.get_instance.html) | `id: integer`<br>`options: object` | `object (PrivilegeEntry)` |
| [`privilege.query`](https://api.truenas.com/v25.10/api_methods_privilege.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / PrivilegeEntry / PrivilegeQueryResultItem / integer` |
| [`privilege.roles`](https://api.truenas.com/v25.10/api_methods_privilege.roles.html) | `filters: array (default [])`<br>`options: object` | `array of object / PrivilegeRolesEntry / PrivilegeRolesQueryResultItem / integer` |
| [`privilege.update`](https://api.truenas.com/v25.10/api_methods_privilege.update.html) | `id: integer`<br>`privilege_update: object` | `object (PrivilegeEntry)` |

## [`user`](https://api.truenas.com/v25.10/api_methods_user.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`user.create`](https://api.truenas.com/v25.10/api_methods_user.create.html) | `user_create: object` | `object (UserCreateUpdateResult)` |
| [`user.delete`](https://api.truenas.com/v25.10/api_methods_user.delete.html) | `id: integer`<br>`options: object` | `integer` |
| [`user.get_instance`](https://api.truenas.com/v25.10/api_methods_user.get_instance.html) | `id: integer`<br>`options: object` | `object (UserEntry)` |
| [`user.get_next_uid`](https://api.truenas.com/v25.10/api_methods_user.get_next_uid.html) | `[]` | `integer` |
| [`user.get_user_obj`](https://api.truenas.com/v25.10/api_methods_user.get_user_obj.html) | `get_user_obj: object` | `object (UserGetUserObj)` |
| [`user.has_local_administrator_set_up`](https://api.truenas.com/v25.10/api_methods_user.has_local_administrator_set_up.html) | `[]` | `boolean` |
| [`user.query`](https://api.truenas.com/v25.10/api_methods_user.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / UserEntry / UserQueryResultItem / integer` |
| [`user.renew_2fa_secret`](https://api.truenas.com/v25.10/api_methods_user.renew_2fa_secret.html) | `username: string`<br>`twofactor_options: object` | `object (UserRenew2faSecretResult)` |
| [`user.set_password`](https://api.truenas.com/v25.10/api_methods_user.set_password.html) | `set_password_data: object` | `null` |
| [`user.setup_local_administrator`](https://api.truenas.com/v25.10/api_methods_user.setup_local_administrator.html) | `username: enum (of string)`<br>`password: string`<br>`options: object` | `null` |
| [`user.shell_choices`](https://api.truenas.com/v25.10/api_methods_user.shell_choices.html) | `group_ids: array of integer (default [])` | `object` |
| [`user.unset_2fa_secret`](https://api.truenas.com/v25.10/api_methods_user.unset_2fa_secret.html) | `username: string` | `null` |
| [`user.update`](https://api.truenas.com/v25.10/api_methods_user.update.html) | `id: integer`<br>`user_update: object` | `object (UserCreateUpdateResult)` |
