# 시스템·설정·업데이트 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역을 메서드별 호출 시그니처와 반환 타입으로 정리한다. 복합 객체의 전체 필드는 공식 상세 문서에서 확인할 수 있다.

`Params`는 JSON-RPC positional tuple의 최상위 항목이며, 인자가 없으면 `[]`로 표시한다. `Returns`는 공식 JSON schema의 최상위 반환 타입/union이다.

`system.reboot`는 공식 RST에서 `system.reboot.info`를 묶는 namespace 페이지로만 제공되며 직접 호출 schema가 없으므로 Params/Returns를 `N/A`로 표시한다.

## [`boot`](https://api.truenas.com/v25.10/api_methods_boot.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`boot.attach`](https://api.truenas.com/v25.10/api_methods_boot.attach.html) | `dev: string`<br>`options: object` | `null` |
| [`boot.detach`](https://api.truenas.com/v25.10/api_methods_boot.detach.html) | `dev: string` | `null` |
| [`boot.get_disks`](https://api.truenas.com/v25.10/api_methods_boot.get_disks.html) | `[]` | `array of string` |
| [`boot.get_state`](https://api.truenas.com/v25.10/api_methods_boot.get_state.html) | `[]` | `object (BootGetState)` |
| [`boot.replace`](https://api.truenas.com/v25.10/api_methods_boot.replace.html) | `label: string`<br>`dev: string` | `null` |
| [`boot.scrub`](https://api.truenas.com/v25.10/api_methods_boot.scrub.html) | `[]` | `null` |
| [`boot.set_scrub_interval`](https://api.truenas.com/v25.10/api_methods_boot.set_scrub_interval.html) | `interval: integer` | `integer` |

## [`boot.environment`](https://api.truenas.com/v25.10/api_methods_boot.environment.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`boot.environment.activate`](https://api.truenas.com/v25.10/api_methods_boot.environment.activate.html) | `boot_environment_activate: object` | `object (BootEnvironmentEntry)` |
| [`boot.environment.clone`](https://api.truenas.com/v25.10/api_methods_boot.environment.clone.html) | `boot_environment_clone: object` | `object (BootEnvironmentEntry)` |
| [`boot.environment.destroy`](https://api.truenas.com/v25.10/api_methods_boot.environment.destroy.html) | `boot_environment_destroy: object` | `null` |
| [`boot.environment.get_instance`](https://api.truenas.com/v25.10/api_methods_boot.environment.get_instance.html) | `id: string`<br>`options: object` | `object (BootEnvironmentEntry)` |
| [`boot.environment.keep`](https://api.truenas.com/v25.10/api_methods_boot.environment.keep.html) | `boot_environment_destroy: object` | `object (BootEnvironmentEntry)` |
| [`boot.environment.query`](https://api.truenas.com/v25.10/api_methods_boot.environment.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / BootEnvironmentEntry / BootEnvironmentQueryResultItem / integer` |

## [`config`](https://api.truenas.com/v25.10/api_methods_config.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`config.reset`](https://api.truenas.com/v25.10/api_methods_config.reset.html) | `options: object (default {"reboot": true})` | `null` |
| [`config.save`](https://api.truenas.com/v25.10/api_methods_config.save.html) | `options: object` | `null` |
| [`config.upload`](https://api.truenas.com/v25.10/api_methods_config.upload.html) | `[]` | `null` |

## [`cronjob`](https://api.truenas.com/v25.10/api_methods_cronjob.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`cronjob.create`](https://api.truenas.com/v25.10/api_methods_cronjob.create.html) | `data: object` | `object (CronJobEntry)` |
| [`cronjob.delete`](https://api.truenas.com/v25.10/api_methods_cronjob.delete.html) | `id: integer` | `true` |
| [`cronjob.get_instance`](https://api.truenas.com/v25.10/api_methods_cronjob.get_instance.html) | `id: integer`<br>`options: object` | `object (CronJobEntry)` |
| [`cronjob.query`](https://api.truenas.com/v25.10/api_methods_cronjob.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / CronJobEntry / CronJobQueryResultItem / integer` |
| [`cronjob.run`](https://api.truenas.com/v25.10/api_methods_cronjob.run.html) | `id: integer`<br>`skip_disabled: boolean (default false)` | `null` |
| [`cronjob.update`](https://api.truenas.com/v25.10/api_methods_cronjob.update.html) | `id: integer`<br>`data: object` | `object (CronJobEntry)` |

## [`initshutdownscript`](https://api.truenas.com/v25.10/api_methods_initshutdownscript.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`initshutdownscript.create`](https://api.truenas.com/v25.10/api_methods_initshutdownscript.create.html) | `data: object` | `object (InitShutdownScriptEntry)` |
| [`initshutdownscript.delete`](https://api.truenas.com/v25.10/api_methods_initshutdownscript.delete.html) | `id: integer` | `true` |
| [`initshutdownscript.get_instance`](https://api.truenas.com/v25.10/api_methods_initshutdownscript.get_instance.html) | `id: integer`<br>`options: object` | `object (InitShutdownScriptEntry)` |
| [`initshutdownscript.query`](https://api.truenas.com/v25.10/api_methods_initshutdownscript.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / InitShutdownScriptEntry / InitShutdownScriptQueryResultItem / integer` |
| [`initshutdownscript.update`](https://api.truenas.com/v25.10/api_methods_initshutdownscript.update.html) | `id: integer`<br>`data: object` | `object (InitShutdownScriptEntry)` |

## [`service`](https://api.truenas.com/v25.10/api_methods_service.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`service.control`](https://api.truenas.com/v25.10/api_methods_service.control.html) | `verb: enum (of string)`<br>`service: string`<br>`options: object` | `boolean` |
| [`service.get_instance`](https://api.truenas.com/v25.10/api_methods_service.get_instance.html) | `id: integer`<br>`options: object` | `object (ServiceEntry)` |
| [`service.query`](https://api.truenas.com/v25.10/api_methods_service.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / ServiceEntry / ServiceQueryResultItem / integer` |
| [`service.reload`](https://api.truenas.com/v25.10/api_methods_service.reload.html) | `service: string`<br>`options: object` | `boolean` |
| [`service.restart`](https://api.truenas.com/v25.10/api_methods_service.restart.html) | `service: string`<br>`options: object` | `boolean` |
| [`service.start`](https://api.truenas.com/v25.10/api_methods_service.start.html) | `service: string`<br>`options: object` | `boolean` |
| [`service.started`](https://api.truenas.com/v25.10/api_methods_service.started.html) | `service: string` | `boolean` |
| [`service.started_or_enabled`](https://api.truenas.com/v25.10/api_methods_service.started_or_enabled.html) | `service: string` | `boolean` |
| [`service.stop`](https://api.truenas.com/v25.10/api_methods_service.stop.html) | `service: string`<br>`options: object` | `boolean` |
| [`service.update`](https://api.truenas.com/v25.10/api_methods_service.update.html) | `id_or_name: integer / string`<br>`service_update: object` | `integer` |

## [`system`](https://api.truenas.com/v25.10/api_methods_system.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.boot_id`](https://api.truenas.com/v25.10/api_methods_system.boot_id.html) | `[]` | `string` |
| [`system.debug`](https://api.truenas.com/v25.10/api_methods_system.debug.html) | `[]` | `null` |
| [`system.feature_enabled`](https://api.truenas.com/v25.10/api_methods_system.feature_enabled.html) | `feature: enum (of string)` | `boolean` |
| [`system.host_id`](https://api.truenas.com/v25.10/api_methods_system.host_id.html) | `[]` | `string` |
| [`system.info`](https://api.truenas.com/v25.10/api_methods_system.info.html) | `[]` | `object (SystemInfoResult)` |
| [`system.license_update`](https://api.truenas.com/v25.10/api_methods_system.license_update.html) | `license: string` | `null` |
| [`system.product_type`](https://api.truenas.com/v25.10/api_methods_system.product_type.html) | `[]` | `enum (of string)` |
| [`system.ready`](https://api.truenas.com/v25.10/api_methods_system.ready.html) | `[]` | `boolean` |
| [`system.reboot`](https://api.truenas.com/v25.10/api_methods_system.reboot.html) | `N/A (namespace)` | `N/A (namespace)` |
| [`system.release_notes_url`](https://api.truenas.com/v25.10/api_methods_system.release_notes_url.html) | `version_str: string / null (default null)` | `string` |
| [`system.shutdown`](https://api.truenas.com/v25.10/api_methods_system.shutdown.html) | `reason: string`<br>`options: object` | `null` |
| [`system.state`](https://api.truenas.com/v25.10/api_methods_system.state.html) | `[]` | `enum (of string)` |
| [`system.version`](https://api.truenas.com/v25.10/api_methods_system.version.html) | `[]` | `string` |
| [`system.version_short`](https://api.truenas.com/v25.10/api_methods_system.version_short.html) | `[]` | `string` |

## [`system.advanced`](https://api.truenas.com/v25.10/api_methods_system.advanced.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.advanced.config`](https://api.truenas.com/v25.10/api_methods_system.advanced.config.html) | `[]` | `object (SystemAdvancedEntry)` |
| [`system.advanced.get_gpu_pci_choices`](https://api.truenas.com/v25.10/api_methods_system.advanced.get_gpu_pci_choices.html) | `[]` | `object` |
| [`system.advanced.login_banner`](https://api.truenas.com/v25.10/api_methods_system.advanced.login_banner.html) | `[]` | `string` |
| [`system.advanced.sed_global_password`](https://api.truenas.com/v25.10/api_methods_system.advanced.sed_global_password.html) | `[]` | `string` |
| [`system.advanced.sed_global_password_is_set`](https://api.truenas.com/v25.10/api_methods_system.advanced.sed_global_password_is_set.html) | `[]` | `boolean` |
| [`system.advanced.serial_port_choices`](https://api.truenas.com/v25.10/api_methods_system.advanced.serial_port_choices.html) | `[]` | `object` |
| [`system.advanced.syslog_certificate_authority_choices`](https://api.truenas.com/v25.10/api_methods_system.advanced.syslog_certificate_authority_choices.html) | `[]` | `object (EmptyDict)` |
| [`system.advanced.syslog_certificate_choices`](https://api.truenas.com/v25.10/api_methods_system.advanced.syslog_certificate_choices.html) | `[]` | `object` |
| [`system.advanced.update`](https://api.truenas.com/v25.10/api_methods_system.advanced.update.html) | `data: object` | `object (SystemAdvancedEntry)` |
| [`system.advanced.update_gpu_pci_ids`](https://api.truenas.com/v25.10/api_methods_system.advanced.update_gpu_pci_ids.html) | `data: array of string` | `null` |

## [`system.general`](https://api.truenas.com/v25.10/api_methods_system.general.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.general.checkin`](https://api.truenas.com/v25.10/api_methods_system.general.checkin.html) | `[]` | `null` |
| [`system.general.checkin_waiting`](https://api.truenas.com/v25.10/api_methods_system.general.checkin_waiting.html) | `[]` | `integer / null` |
| [`system.general.config`](https://api.truenas.com/v25.10/api_methods_system.general.config.html) | `[]` | `object (SystemGeneralEntry)` |
| [`system.general.country_choices`](https://api.truenas.com/v25.10/api_methods_system.general.country_choices.html) | `[]` | `object` |
| [`system.general.kbdmap_choices`](https://api.truenas.com/v25.10/api_methods_system.general.kbdmap_choices.html) | `[]` | `object` |
| [`system.general.local_url`](https://api.truenas.com/v25.10/api_methods_system.general.local_url.html) | `[]` | `string` |
| [`system.general.timezone_choices`](https://api.truenas.com/v25.10/api_methods_system.general.timezone_choices.html) | `[]` | `object` |
| [`system.general.ui_address_choices`](https://api.truenas.com/v25.10/api_methods_system.general.ui_address_choices.html) | `[]` | `object` |
| [`system.general.ui_certificate_choices`](https://api.truenas.com/v25.10/api_methods_system.general.ui_certificate_choices.html) | `[]` | `object` |
| [`system.general.ui_httpsprotocols_choices`](https://api.truenas.com/v25.10/api_methods_system.general.ui_httpsprotocols_choices.html) | `[]` | `object` |
| [`system.general.ui_restart`](https://api.truenas.com/v25.10/api_methods_system.general.ui_restart.html) | `delay: integer (default 3)` | `null` |
| [`system.general.ui_v6address_choices`](https://api.truenas.com/v25.10/api_methods_system.general.ui_v6address_choices.html) | `[]` | `object` |
| [`system.general.update`](https://api.truenas.com/v25.10/api_methods_system.general.update.html) | `general_settings: object` | `object (SystemGeneralEntry)` |

## [`system.global`](https://api.truenas.com/v25.10/api_methods_system.global.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.global.id`](https://api.truenas.com/v25.10/api_methods_system.global.id.html) | `[]` | `string` |

## [`system.ntpserver`](https://api.truenas.com/v25.10/api_methods_system.ntpserver.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.ntpserver.create`](https://api.truenas.com/v25.10/api_methods_system.ntpserver.create.html) | `ntp_server_create: object` | `object (NTPServerEntry)` |
| [`system.ntpserver.delete`](https://api.truenas.com/v25.10/api_methods_system.ntpserver.delete.html) | `id: integer` | `true` |
| [`system.ntpserver.get_instance`](https://api.truenas.com/v25.10/api_methods_system.ntpserver.get_instance.html) | `id: integer`<br>`options: object` | `object (NTPServerEntry)` |
| [`system.ntpserver.query`](https://api.truenas.com/v25.10/api_methods_system.ntpserver.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / NTPServerEntry / NTPServerQueryResultItem / integer` |
| [`system.ntpserver.update`](https://api.truenas.com/v25.10/api_methods_system.ntpserver.update.html) | `id: integer`<br>`ntp_server_update: object` | `object (NTPServerEntry)` |

## [`system.reboot`](https://api.truenas.com/v25.10/api_methods_system.reboot.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.reboot.info`](https://api.truenas.com/v25.10/api_methods_system.reboot.info.html) | `[]` | `object (RebootInfo)` |

## [`system.security`](https://api.truenas.com/v25.10/api_methods_system.security.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.security.config`](https://api.truenas.com/v25.10/api_methods_system.security.config.html) | `[]` | `object (SystemSecurityEntry)` |
| [`system.security.update`](https://api.truenas.com/v25.10/api_methods_system.security.update.html) | `system_security_update: object` | `object (SystemSecurityEntry)` |

## [`system.security.info`](https://api.truenas.com/v25.10/api_methods_system.security.info.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`system.security.info.fips_available`](https://api.truenas.com/v25.10/api_methods_system.security.info.fips_available.html) | `[]` | `boolean` |
| [`system.security.info.fips_enabled`](https://api.truenas.com/v25.10/api_methods_system.security.info.fips_enabled.html) | `[]` | `boolean` |

## [`systemdataset`](https://api.truenas.com/v25.10/api_methods_systemdataset.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`systemdataset.config`](https://api.truenas.com/v25.10/api_methods_systemdataset.config.html) | `[]` | `object (SystemDatasetEntry)` |
| [`systemdataset.pool_choices`](https://api.truenas.com/v25.10/api_methods_systemdataset.pool_choices.html) | `include_current_pool: boolean (default true)` | `object` |
| [`systemdataset.update`](https://api.truenas.com/v25.10/api_methods_systemdataset.update.html) | `data: object` | `object (SystemDatasetEntry)` |

## [`tunable`](https://api.truenas.com/v25.10/api_methods_tunable.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`tunable.create`](https://api.truenas.com/v25.10/api_methods_tunable.create.html) | `data: object` | `object (TunableEntry)` |
| [`tunable.delete`](https://api.truenas.com/v25.10/api_methods_tunable.delete.html) | `id: integer` | `null` |
| [`tunable.get_instance`](https://api.truenas.com/v25.10/api_methods_tunable.get_instance.html) | `id: integer`<br>`options: object` | `object (TunableEntry)` |
| [`tunable.query`](https://api.truenas.com/v25.10/api_methods_tunable.query.html) | `filters: array (default [])`<br>`options: object` | `array of object / TunableEntry / TunableQueryResultItem / integer` |
| [`tunable.tunable_type_choices`](https://api.truenas.com/v25.10/api_methods_tunable.tunable_type_choices.html) | `[]` | `object (TunableTunableTypeChoices)` |
| [`tunable.update`](https://api.truenas.com/v25.10/api_methods_tunable.update.html) | `id: integer`<br>`data: object` | `object (TunableEntry)` |

## [`update`](https://api.truenas.com/v25.10/api_methods_update.html)

| Method | Params | Returns |
| --- | --- | --- |

| [`update.available_versions`](https://api.truenas.com/v25.10/api_methods_update.available_versions.html) | `[]` | `array of object` |
| [`update.config`](https://api.truenas.com/v25.10/api_methods_update.config.html) | `[]` | `object (UpdateEntry)` |
| [`update.download`](https://api.truenas.com/v25.10/api_methods_update.download.html) | `train: string / null (default null)`<br>`version: string / null (default null)` | `boolean` |
| [`update.file`](https://api.truenas.com/v25.10/api_methods_update.file.html) | `options: object (default {"resume": false, "destination": null})` | `null` |
| [`update.manual`](https://api.truenas.com/v25.10/api_methods_update.manual.html) | `path: string`<br>`options: object` | `null` |
| [`update.profile_choices`](https://api.truenas.com/v25.10/api_methods_update.profile_choices.html) | `[]` | `object` |
| [`update.run`](https://api.truenas.com/v25.10/api_methods_update.run.html) | `attrs: object` | `true` |
| [`update.status`](https://api.truenas.com/v25.10/api_methods_update.status.html) | `[]` | `object (UpdateStatus)` |
| [`update.update`](https://api.truenas.com/v25.10/api_methods_update.update.html) | `data: object` | `object (UpdateEntry)` |
