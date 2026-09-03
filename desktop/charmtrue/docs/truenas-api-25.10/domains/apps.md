# 앱·컨테이너 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역 전체를 나열한다. 메서드 링크에서 인자, 반환 스키마, Job 여부와 권한을 확인한다.

표의 `Call parameters`와 `Return value`는 공식 v25.10.5 상세 문서의 최상위 JSON Schema(중첩 객체는 타입명과 상세 링크 참조)를 옮긴 것이다. 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`app`](https://api.truenas.com/v25.10/api_methods_app.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`app.available_space`](https://api.truenas.com/v25.10/api_methods_app.available_space.html) | `[]` | `integer` |
| [`app.categories`](https://api.truenas.com/v25.10/api_methods_app.categories.html) | `[]` | `array of string` |
| [`app.certificate_choices`](https://api.truenas.com/v25.10/api_methods_app.certificate_choices.html) | `[]` | `array of object` |
| [`app.config`](https://api.truenas.com/v25.10/api_methods_app.config.html) | `[app_name: string]` | `object` |
| [`app.container_console_choices`](https://api.truenas.com/v25.10/api_methods_app.container_console_choices.html) | `[app_name: string]` | `object (AppContainerResponse)` |
| [`app.container_ids`](https://api.truenas.com/v25.10/api_methods_app.container_ids.html) | `[app_name: string, options: object (default {"alive_only": true})]` | `object (AppContainerResponse)` |
| [`app.convert_to_custom`](https://api.truenas.com/v25.10/api_methods_app.convert_to_custom.html) | `[app_name: string]` | `object (AppEntry)` |
| [`app.create`](https://api.truenas.com/v25.10/api_methods_app.create.html) | `[app_create: object]` | `object (AppEntry)` |
| [`app.delete`](https://api.truenas.com/v25.10/api_methods_app.delete.html) | `[app_name: string, options: object]` | `const(true)` |
| [`app.get_instance`](https://api.truenas.com/v25.10/api_methods_app.get_instance.html) | `[id: string, options: object]` | `object (AppEntry)` |
| [`app.gpu_choices`](https://api.truenas.com/v25.10/api_methods_app.gpu_choices.html) | `[]` | `object (AppGPUResponse)` |
| [`app.ip_choices`](https://api.truenas.com/v25.10/api_methods_app.ip_choices.html) | `[]` | `object` |
| [`app.outdated_docker_images`](https://api.truenas.com/v25.10/api_methods_app.outdated_docker_images.html) | `[app_name: string]` | `array of string` |
| [`app.pull_images`](https://api.truenas.com/v25.10/api_methods_app.pull_images.html) | `[app_name: string, options: object (default {"redeploy": true})]` | `null` |
| [`app.query`](https://api.truenas.com/v25.10/api_methods_app.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`app.redeploy`](https://api.truenas.com/v25.10/api_methods_app.redeploy.html) | `[app_name: string]` | `object (AppEntry)` |
| [`app.rollback`](https://api.truenas.com/v25.10/api_methods_app.rollback.html) | `[app_name: string, options: object]` | `object (AppEntry)` |
| [`app.rollback_versions`](https://api.truenas.com/v25.10/api_methods_app.rollback_versions.html) | `[app_name: string]` | `array of string` |
| [`app.similar`](https://api.truenas.com/v25.10/api_methods_app.similar.html) | `[app_name: string, train: string]` | `array of object` |
| [`app.start`](https://api.truenas.com/v25.10/api_methods_app.start.html) | `[app_name: string]` | `null` |
| [`app.stop`](https://api.truenas.com/v25.10/api_methods_app.stop.html) | `[app_name: string]` | `null` |
| [`app.update`](https://api.truenas.com/v25.10/api_methods_app.update.html) | `[app_name: string, update: object]` | `object (AppEntry)` |
| [`app.upgrade`](https://api.truenas.com/v25.10/api_methods_app.upgrade.html) | `[app_name: string, options: object]` | `object (AppEntry)` |
| [`app.upgrade_summary`](https://api.truenas.com/v25.10/api_methods_app.upgrade_summary.html) | `[app_name: string, options: object (default {"app_version": "latest"})]` | `object (AppUpgradeSummaryResult)` |
| [`app.used_host_ips`](https://api.truenas.com/v25.10/api_methods_app.used_host_ips.html) | `[]` | `object` |
| [`app.used_ports`](https://api.truenas.com/v25.10/api_methods_app.used_ports.html) | `[]` | `array of integer` |

## [`app.image`](https://api.truenas.com/v25.10/api_methods_app.image.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`app.image.delete`](https://api.truenas.com/v25.10/api_methods_app.image.delete.html) | `[image_id: string, options: object (default {"force": false})]` | `const(true)` |
| [`app.image.dockerhub_rate_limit`](https://api.truenas.com/v25.10/api_methods_app.image.dockerhub_rate_limit.html) | `[]` | `object (AppImageDockerhubRateLimitResult)` |
| [`app.image.get_instance`](https://api.truenas.com/v25.10/api_methods_app.image.get_instance.html) | `[id: string, options: object]` | `object (AppImageEntry)` |
| [`app.image.pull`](https://api.truenas.com/v25.10/api_methods_app.image.pull.html) | `[image_pull: object]` | `null` |
| [`app.image.query`](https://api.truenas.com/v25.10/api_methods_app.image.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |

## [`app.ix_volume`](https://api.truenas.com/v25.10/api_methods_app.ix_volume.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`app.ix_volume.exists`](https://api.truenas.com/v25.10/api_methods_app.ix_volume.exists.html) | `[name: string]` | `boolean` |
| [`app.ix_volume.query`](https://api.truenas.com/v25.10/api_methods_app.ix_volume.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |

## [`app.registry`](https://api.truenas.com/v25.10/api_methods_app.registry.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`app.registry.create`](https://api.truenas.com/v25.10/api_methods_app.registry.create.html) | `[app_registry_create: object]` | `object (AppRegistryEntry)` |
| [`app.registry.delete`](https://api.truenas.com/v25.10/api_methods_app.registry.delete.html) | `[id: integer]` | `null` |
| [`app.registry.get_instance`](https://api.truenas.com/v25.10/api_methods_app.registry.get_instance.html) | `[id: integer, options: object]` | `object (AppRegistryEntry)` |
| [`app.registry.query`](https://api.truenas.com/v25.10/api_methods_app.registry.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`app.registry.update`](https://api.truenas.com/v25.10/api_methods_app.registry.update.html) | `[id: integer, data: object]` | `object (AppRegistryEntry)` |

## [`catalog`](https://api.truenas.com/v25.10/api_methods_catalog.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`catalog.apps`](https://api.truenas.com/v25.10/api_methods_catalog.apps.html) | `[catalog_apps_options: object]` | `object` |
| [`catalog.config`](https://api.truenas.com/v25.10/api_methods_catalog.config.html) | `[]` | `object (CatalogEntry)` |
| [`catalog.get_app_details`](https://api.truenas.com/v25.10/api_methods_catalog.get_app_details.html) | `[app_name: string, app_version_details: object]` | `object (CatalogAppInfo)` |
| [`catalog.sync`](https://api.truenas.com/v25.10/api_methods_catalog.sync.html) | `[]` | `null` |
| [`catalog.trains`](https://api.truenas.com/v25.10/api_methods_catalog.trains.html) | `[]` | `array of string` |
| [`catalog.update`](https://api.truenas.com/v25.10/api_methods_catalog.update.html) | `[catalog_update: object]` | `object (CatalogEntry)` |

## [`docker`](https://api.truenas.com/v25.10/api_methods_docker.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`docker.backup`](https://api.truenas.com/v25.10/api_methods_docker.backup.html) | `[backup_name: string \| null (default null)]` | `string` |
| [`docker.backup_to_pool`](https://api.truenas.com/v25.10/api_methods_docker.backup_to_pool.html) | `[target_pool: string]` | `null` |
| [`docker.config`](https://api.truenas.com/v25.10/api_methods_docker.config.html) | `[]` | `object (DockerEntry)` |
| [`docker.delete_backup`](https://api.truenas.com/v25.10/api_methods_docker.delete_backup.html) | `[backup_name: string]` | `null` |
| [`docker.list_backups`](https://api.truenas.com/v25.10/api_methods_docker.list_backups.html) | `[]` | `object (DockerBackupInfo)` |
| [`docker.nvidia_present`](https://api.truenas.com/v25.10/api_methods_docker.nvidia_present.html) | `[]` | `boolean` |
| [`docker.restore_backup`](https://api.truenas.com/v25.10/api_methods_docker.restore_backup.html) | `[backup_name: string]` | `null` |
| [`docker.status`](https://api.truenas.com/v25.10/api_methods_docker.status.html) | `[]` | `object (StatusResult)` |
| [`docker.update`](https://api.truenas.com/v25.10/api_methods_docker.update.html) | `[docker_update: object]` | `object (DockerEntry)` |

## [`docker.network`](https://api.truenas.com/v25.10/api_methods_docker.network.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`docker.network.get_instance`](https://api.truenas.com/v25.10/api_methods_docker.network.get_instance.html) | `[id: string \| null, options: object]` | `object (DockerNetworkEntry)` |
| [`docker.network.query`](https://api.truenas.com/v25.10/api_methods_docker.network.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
