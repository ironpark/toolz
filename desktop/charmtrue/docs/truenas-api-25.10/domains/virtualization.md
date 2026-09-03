# 가상화 API

[← 문서 홈](../README.md)

이 문서는 TrueNAS API v25.10.5의 해당 기능 영역 전체를 나열한다. 메서드 링크에서 인자, 반환 스키마, Job 여부와 권한을 확인한다.

표의 `Call parameters`와 `Return value`는 공식 v25.10.5 상세 문서의 최상위 JSON Schema(중첩 객체는 타입명과 상세 링크 참조)를 옮긴 것이다. 인자가 없으면 `[]`, 반환값이 없으면 `null`로 표시한다.

## [`hardware.virtualization`](https://api.truenas.com/v25.10/api_methods_hardware.virtualization.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`hardware.virtualization.variant`](https://api.truenas.com/v25.10/api_methods_hardware.virtualization.variant.html) | `[]` | `string` |

## [`vm`](https://api.truenas.com/v25.10/api_methods_vm.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`vm.bootloader_options`](https://api.truenas.com/v25.10/api_methods_vm.bootloader_options.html) | `[]` | `object (VMBootloaderOptionsResult)` |
| [`vm.bootloader_ovmf_choices`](https://api.truenas.com/v25.10/api_methods_vm.bootloader_ovmf_choices.html) | `[]` | `object (VMBootloaderOvmfChoicesResult)` |
| [`vm.clone`](https://api.truenas.com/v25.10/api_methods_vm.clone.html) | `[id: integer, name: string \| null (default null)]` | `boolean` |
| [`vm.cpu_model_choices`](https://api.truenas.com/v25.10/api_methods_vm.cpu_model_choices.html) | `[]` | `object (VMCpuModelChoicesResult)` |
| [`vm.create`](https://api.truenas.com/v25.10/api_methods_vm.create.html) | `[vm_create: object]` | `object (VMEntry)` |
| [`vm.delete`](https://api.truenas.com/v25.10/api_methods_vm.delete.html) | `[id: integer, options: object (default {"zvols": false, "force": false})]` | `boolean` |
| [`vm.flags`](https://api.truenas.com/v25.10/api_methods_vm.flags.html) | `[]` | `object (VMFlagsResult)` |
| [`vm.get_available_memory`](https://api.truenas.com/v25.10/api_methods_vm.get_available_memory.html) | `[overcommit: boolean (default false)]` | `integer` |
| [`vm.get_console`](https://api.truenas.com/v25.10/api_methods_vm.get_console.html) | `[id: integer]` | `string` |
| [`vm.get_display_devices`](https://api.truenas.com/v25.10/api_methods_vm.get_display_devices.html) | `[id: integer]` | `array of object` |
| [`vm.get_display_web_uri`](https://api.truenas.com/v25.10/api_methods_vm.get_display_web_uri.html) | `[id: integer, host: string (default ""), options: object (default {"protocol": "HTTP"})]` | `object (VMGetDisplayWebUriResult)` |
| [`vm.get_instance`](https://api.truenas.com/v25.10/api_methods_vm.get_instance.html) | `[id: integer, options: object]` | `object (VMEntry)` |
| [`vm.get_memory_usage`](https://api.truenas.com/v25.10/api_methods_vm.get_memory_usage.html) | `[id: integer]` | `integer` |
| [`vm.get_vm_memory_info`](https://api.truenas.com/v25.10/api_methods_vm.get_vm_memory_info.html) | `[id: integer]` | `object (VMGetVmMemoryInfoResult)` |
| [`vm.get_vmemory_in_use`](https://api.truenas.com/v25.10/api_methods_vm.get_vmemory_in_use.html) | `[]` | `object (VMGetVmemoryInUseResult)` |
| [`vm.guest_architecture_and_machine_choices`](https://api.truenas.com/v25.10/api_methods_vm.guest_architecture_and_machine_choices.html) | `[]` | `object (VMGuestArchitectureAndMachineChoicesResult)` |
| [`vm.log_file_download`](https://api.truenas.com/v25.10/api_methods_vm.log_file_download.html) | `[id: integer]` | `null` |
| [`vm.log_file_path`](https://api.truenas.com/v25.10/api_methods_vm.log_file_path.html) | `[id: integer]` | `string \| null` |
| [`vm.maximum_supported_vcpus`](https://api.truenas.com/v25.10/api_methods_vm.maximum_supported_vcpus.html) | `[]` | `integer` |
| [`vm.port_wizard`](https://api.truenas.com/v25.10/api_methods_vm.port_wizard.html) | `[]` | `object (VMPortWizardResult)` |
| [`vm.poweroff`](https://api.truenas.com/v25.10/api_methods_vm.poweroff.html) | `[id: integer]` | `null` |
| [`vm.query`](https://api.truenas.com/v25.10/api_methods_vm.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`vm.random_mac`](https://api.truenas.com/v25.10/api_methods_vm.random_mac.html) | `[]` | `string` |
| [`vm.resolution_choices`](https://api.truenas.com/v25.10/api_methods_vm.resolution_choices.html) | `[]` | `object` |
| [`vm.restart`](https://api.truenas.com/v25.10/api_methods_vm.restart.html) | `[id: integer]` | `null` |
| [`vm.resume`](https://api.truenas.com/v25.10/api_methods_vm.resume.html) | `[id: integer]` | `null` |
| [`vm.start`](https://api.truenas.com/v25.10/api_methods_vm.start.html) | `[id: integer, options: object (default {"overcommit": false})]` | `null` |
| [`vm.status`](https://api.truenas.com/v25.10/api_methods_vm.status.html) | `[id: integer]` | `object (VMStatus)` |
| [`vm.stop`](https://api.truenas.com/v25.10/api_methods_vm.stop.html) | `[id: integer, options: object]` | `null` |
| [`vm.supports_virtualization`](https://api.truenas.com/v25.10/api_methods_vm.supports_virtualization.html) | `[]` | `boolean` |
| [`vm.suspend`](https://api.truenas.com/v25.10/api_methods_vm.suspend.html) | `[id: integer]` | `null` |
| [`vm.update`](https://api.truenas.com/v25.10/api_methods_vm.update.html) | `[id: integer, vm_update: object]` | `object (VMEntry)` |
| [`vm.virtualization_details`](https://api.truenas.com/v25.10/api_methods_vm.virtualization_details.html) | `[]` | `object (VMVirtualizationDetailsResult)` |

## [`vm.device`](https://api.truenas.com/v25.10/api_methods_vm.device.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`vm.device.bind_choices`](https://api.truenas.com/v25.10/api_methods_vm.device.bind_choices.html) | `[]` | `object (VMDeviceBindChoicesResult)` |
| [`vm.device.convert`](https://api.truenas.com/v25.10/api_methods_vm.device.convert.html) | `[vm_convert: object]` | `boolean` |
| [`vm.device.create`](https://api.truenas.com/v25.10/api_methods_vm.device.create.html) | `[vm_device_create: object]` | `object (VMDeviceEntry)` |
| [`vm.device.delete`](https://api.truenas.com/v25.10/api_methods_vm.device.delete.html) | `[id: integer, options: object]` | `boolean` |
| [`vm.device.disk_choices`](https://api.truenas.com/v25.10/api_methods_vm.device.disk_choices.html) | `[]` | `object (VMDeviceDiskChoices)` |
| [`vm.device.get_instance`](https://api.truenas.com/v25.10/api_methods_vm.device.get_instance.html) | `[id: integer, options: object]` | `object (VMDeviceEntry)` |
| [`vm.device.iommu_enabled`](https://api.truenas.com/v25.10/api_methods_vm.device.iommu_enabled.html) | `[]` | `boolean` |
| [`vm.device.iotype_choices`](https://api.truenas.com/v25.10/api_methods_vm.device.iotype_choices.html) | `[]` | `object (VMDeviceIotypeChoicesResult)` |
| [`vm.device.nic_attach_choices`](https://api.truenas.com/v25.10/api_methods_vm.device.nic_attach_choices.html) | `[]` | `object (VMDeviceNicAttachChoicesResult)` |
| [`vm.device.passthrough_device`](https://api.truenas.com/v25.10/api_methods_vm.device.passthrough_device.html) | `[device: string]` | `object (VMDevicePassthroughDevice)` |
| [`vm.device.passthrough_device_choices`](https://api.truenas.com/v25.10/api_methods_vm.device.passthrough_device_choices.html) | `[]` | `object (VMDevicePassthroughInfo)` |
| [`vm.device.query`](https://api.truenas.com/v25.10/api_methods_vm.device.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`vm.device.update`](https://api.truenas.com/v25.10/api_methods_vm.device.update.html) | `[id: integer, vm_device_update: object]` | `object (VMDeviceEntry)` |
| [`vm.device.usb_controller_choices`](https://api.truenas.com/v25.10/api_methods_vm.device.usb_controller_choices.html) | `[]` | `object (VMDeviceUsbControllerChoicesResult)` |
| [`vm.device.usb_passthrough_choices`](https://api.truenas.com/v25.10/api_methods_vm.device.usb_passthrough_choices.html) | `[]` | `object (USBPassthroughInfo)` |
| [`vm.device.usb_passthrough_device`](https://api.truenas.com/v25.10/api_methods_vm.device.usb_passthrough_device.html) | `[device: string]` | `object (USBPassthroughDevice)` |
| [`vm.device.virtual_size`](https://api.truenas.com/v25.10/api_methods_vm.device.virtual_size.html) | `[vm_virtual_size: object]` | `integer` |

## [`vmware`](https://api.truenas.com/v25.10/api_methods_vmware.html)

| 메서드 | Call parameters | Return value |
|---|---|---|
| [`vmware.create`](https://api.truenas.com/v25.10/api_methods_vmware.create.html) | `[vmware_create: object]` | `object (VMWareEntry)` |
| [`vmware.dataset_has_vms`](https://api.truenas.com/v25.10/api_methods_vmware.dataset_has_vms.html) | `[dataset: string, recursive: boolean]` | `boolean` |
| [`vmware.delete`](https://api.truenas.com/v25.10/api_methods_vmware.delete.html) | `[id: integer]` | `const(true)` |
| [`vmware.get_datastores`](https://api.truenas.com/v25.10/api_methods_vmware.get_datastores.html) | `[vmware-creds: object]` | `array of string` |
| [`vmware.get_instance`](https://api.truenas.com/v25.10/api_methods_vmware.get_instance.html) | `[id: integer, options: object]` | `object (VMWareEntry)` |
| [`vmware.match_datastores_with_datasets`](https://api.truenas.com/v25.10/api_methods_vmware.match_datastores_with_datasets.html) | `[vmware-creds: object]` | `object (VMWareMatchDatastoresWithDatasetsResult)` |
| [`vmware.query`](https://api.truenas.com/v25.10/api_methods_vmware.query.html) | `[filters: array (default []), options: object]` | `array of object \| object \| integer` |
| [`vmware.update`](https://api.truenas.com/v25.10/api_methods_vmware.update.html) | `[id: integer, vmware_update: object]` | `object (VMWareEntry)` |
