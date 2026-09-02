// Code generated from TrueNAS API v25.10.5; DO NOT EDIT.
package truenas

import (
	"context"
	"encoding/json"
)

type SystemMethod struct {
	Name, Service, Kind string
	Destructive         bool
}

var SystemMethods = [...]SystemMethod{
	{Name: "boot.attach", Service: "BootService", Kind: "change", Destructive: false},
	{Name: "boot.detach", Service: "BootService", Kind: "destructive", Destructive: true},
	{Name: "boot.environment.activate", Service: "BootEnvironmentService", Kind: "change", Destructive: false},
	{Name: "boot.environment.clone", Service: "BootEnvironmentService", Kind: "change", Destructive: false},
	{Name: "boot.environment.destroy", Service: "BootEnvironmentService", Kind: "destructive", Destructive: true},
	{Name: "boot.environment.get_instance", Service: "BootEnvironmentService", Kind: "read", Destructive: false},
	{Name: "boot.environment.keep", Service: "BootEnvironmentService", Kind: "change", Destructive: false},
	{Name: "boot.environment.query", Service: "BootEnvironmentService", Kind: "read", Destructive: false},
	{Name: "boot.get_disks", Service: "BootService", Kind: "change", Destructive: false},
	{Name: "boot.get_state", Service: "BootService", Kind: "change", Destructive: false},
	{Name: "boot.replace", Service: "BootService", Kind: "destructive", Destructive: true},
	{Name: "boot.scrub", Service: "BootService", Kind: "change", Destructive: false},
	{Name: "boot.set_scrub_interval", Service: "BootService", Kind: "change", Destructive: false},
	{Name: "config.reset", Service: "ConfigService", Kind: "destructive", Destructive: true},
	{Name: "config.save", Service: "ConfigService", Kind: "change", Destructive: false},
	{Name: "config.upload", Service: "ConfigService", Kind: "change", Destructive: false},
	{Name: "cronjob.create", Service: "CronJobService", Kind: "create", Destructive: false},
	{Name: "cronjob.delete", Service: "CronJobService", Kind: "destructive", Destructive: true},
	{Name: "cronjob.get_instance", Service: "CronJobService", Kind: "read", Destructive: false},
	{Name: "cronjob.query", Service: "CronJobService", Kind: "read", Destructive: false},
	{Name: "cronjob.run", Service: "CronJobService", Kind: "change", Destructive: false},
	{Name: "cronjob.update", Service: "CronJobService", Kind: "change", Destructive: false},
	{Name: "initshutdownscript.create", Service: "InitShutdownScriptService", Kind: "create", Destructive: false},
	{Name: "initshutdownscript.delete", Service: "InitShutdownScriptService", Kind: "destructive", Destructive: true},
	{Name: "initshutdownscript.get_instance", Service: "InitShutdownScriptService", Kind: "read", Destructive: false},
	{Name: "initshutdownscript.query", Service: "InitShutdownScriptService", Kind: "read", Destructive: false},
	{Name: "initshutdownscript.update", Service: "InitShutdownScriptService", Kind: "change", Destructive: false},
	{Name: "service.control", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "service.get_instance", Service: "DaemonService", Kind: "read", Destructive: false},
	{Name: "service.query", Service: "DaemonService", Kind: "read", Destructive: false},
	{Name: "service.reload", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "service.restart", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "service.start", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "service.started", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "service.started_or_enabled", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "service.stop", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "service.update", Service: "DaemonService", Kind: "change", Destructive: false},
	{Name: "system.advanced.config", Service: "SystemAdvancedService", Kind: "read", Destructive: false},
	{Name: "system.advanced.get_gpu_pci_choices", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.login_banner", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.sed_global_password", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.sed_global_password_is_set", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.serial_port_choices", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.syslog_certificate_authority_choices", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.syslog_certificate_choices", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.update", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.advanced.update_gpu_pci_ids", Service: "SystemAdvancedService", Kind: "change", Destructive: false},
	{Name: "system.boot_id", Service: "SystemCoreService", Kind: "change", Destructive: false},
	{Name: "system.debug", Service: "SystemCoreService", Kind: "change", Destructive: false},
	{Name: "system.feature_enabled", Service: "SystemCoreService", Kind: "change", Destructive: false},
	{Name: "system.general.checkin", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.checkin_waiting", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.config", Service: "SystemGeneralService", Kind: "read", Destructive: false},
	{Name: "system.general.country_choices", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.kbdmap_choices", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.local_url", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.timezone_choices", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.ui_address_choices", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.ui_certificate_choices", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.ui_httpsprotocols_choices", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.ui_restart", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.ui_v6address_choices", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.general.update", Service: "SystemGeneralService", Kind: "change", Destructive: false},
	{Name: "system.global.id", Service: "SystemGlobalService", Kind: "change", Destructive: false},
	{Name: "system.host_id", Service: "SystemCoreService", Kind: "change", Destructive: false},
	{Name: "system.info", Service: "SystemCoreService", Kind: "read", Destructive: false},
	{Name: "system.license_update", Service: "SystemCoreService", Kind: "change", Destructive: false},
	{Name: "system.ntpserver.create", Service: "NTPServerService", Kind: "create", Destructive: false},
	{Name: "system.ntpserver.delete", Service: "NTPServerService", Kind: "destructive", Destructive: true},
	{Name: "system.ntpserver.get_instance", Service: "NTPServerService", Kind: "read", Destructive: false},
	{Name: "system.ntpserver.query", Service: "NTPServerService", Kind: "read", Destructive: false},
	{Name: "system.ntpserver.update", Service: "NTPServerService", Kind: "change", Destructive: false},
	{Name: "system.product_type", Service: "SystemCoreService", Kind: "change", Destructive: false},
	{Name: "system.ready", Service: "SystemCoreService", Kind: "read", Destructive: false},
	{Name: "system.reboot.info", Service: "SystemRebootService", Kind: "read", Destructive: false},
	{Name: "system.release_notes_url", Service: "SystemCoreService", Kind: "change", Destructive: false},
	{Name: "system.security.config", Service: "SystemSecurityService", Kind: "read", Destructive: false},
	{Name: "system.security.info.fips_available", Service: "SystemSecurityInfoService", Kind: "change", Destructive: false},
	{Name: "system.security.info.fips_enabled", Service: "SystemSecurityInfoService", Kind: "change", Destructive: false},
	{Name: "system.security.update", Service: "SystemSecurityService", Kind: "change", Destructive: false},
	{Name: "system.shutdown", Service: "SystemCoreService", Kind: "destructive", Destructive: true},
	{Name: "system.state", Service: "SystemCoreService", Kind: "read", Destructive: false},
	{Name: "system.version", Service: "SystemCoreService", Kind: "read", Destructive: false},
	{Name: "system.version_short", Service: "SystemCoreService", Kind: "read", Destructive: false},
	{Name: "systemdataset.config", Service: "SystemDatasetService", Kind: "read", Destructive: false},
	{Name: "systemdataset.pool_choices", Service: "SystemDatasetService", Kind: "change", Destructive: false},
	{Name: "systemdataset.update", Service: "SystemDatasetService", Kind: "change", Destructive: false},
	{Name: "tunable.create", Service: "TunableService", Kind: "create", Destructive: false},
	{Name: "tunable.delete", Service: "TunableService", Kind: "destructive", Destructive: true},
	{Name: "tunable.get_instance", Service: "TunableService", Kind: "read", Destructive: false},
	{Name: "tunable.query", Service: "TunableService", Kind: "read", Destructive: false},
	{Name: "tunable.tunable_type_choices", Service: "TunableService", Kind: "change", Destructive: false},
	{Name: "tunable.update", Service: "TunableService", Kind: "change", Destructive: false},
	{Name: "update.available_versions", Service: "UpdateService", Kind: "change", Destructive: false},
	{Name: "update.config", Service: "UpdateService", Kind: "read", Destructive: false},
	{Name: "update.download", Service: "UpdateService", Kind: "change", Destructive: false},
	{Name: "update.file", Service: "UpdateService", Kind: "change", Destructive: false},
	{Name: "update.manual", Service: "UpdateService", Kind: "change", Destructive: false},
	{Name: "update.profile_choices", Service: "UpdateService", Kind: "change", Destructive: false},
	{Name: "update.run", Service: "UpdateService", Kind: "change", Destructive: false},
	{Name: "update.status", Service: "UpdateService", Kind: "read", Destructive: false},
	{Name: "update.update", Service: "UpdateService", Kind: "change", Destructive: false},
}

func SystemMethodByName(n string) (SystemMethod, bool) {
	for _, m := range SystemMethods {
		if m.Name == n {
			return m, true
		}
	}
	return SystemMethod{}, false
}
func (s BootService) Attach(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot", "attach", r)
}
func (s BootService) Detach(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot", "detach", r)
}
func (s BootEnvironmentService) Activate(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot.environment", "activate", r)
}
func (s BootEnvironmentService) Clone(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot.environment", "clone", r)
}
func (s BootEnvironmentService) Destroy(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot.environment", "destroy", r)
}
func (s BootEnvironmentService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot.environment", "get_instance", r)
}
func (s BootEnvironmentService) Keep(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot.environment", "keep", r)
}
func (s BootEnvironmentService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot.environment", "query", r)
}
func (s BootService) GetDisks(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot", "get_disks", r)
}
func (s BootService) GetState(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot", "get_state", r)
}
func (s BootService) Replace(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot", "replace", r)
}
func (s BootService) Scrub(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot", "scrub", r)
}
func (s BootService) SetScrubInterval(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "boot", "set_scrub_interval", r)
}
func (s ConfigService) Reset(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "config", "reset", r)
}
func (s ConfigService) Save(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "config", "save", r)
}
func (s ConfigService) Upload(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "config", "upload", r)
}
func (s CronJobService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "cronjob", "create", r)
}
func (s CronJobService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "cronjob", "delete", r)
}
func (s CronJobService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "cronjob", "get_instance", r)
}
func (s CronJobService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "cronjob", "query", r)
}
func (s CronJobService) Run(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "cronjob", "run", r)
}
func (s CronJobService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "cronjob", "update", r)
}
func (s InitShutdownScriptService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "initshutdownscript", "create", r)
}
func (s InitShutdownScriptService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "initshutdownscript", "delete", r)
}
func (s InitShutdownScriptService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "initshutdownscript", "get_instance", r)
}
func (s InitShutdownScriptService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "initshutdownscript", "query", r)
}
func (s InitShutdownScriptService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "initshutdownscript", "update", r)
}
func (s DaemonService) Control(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "control", r)
}
func (s DaemonService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "get_instance", r)
}
func (s DaemonService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "query", r)
}
func (s DaemonService) Reload(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "reload", r)
}
func (s DaemonService) Restart(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "restart", r)
}
func (s DaemonService) Start(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "start", r)
}
func (s DaemonService) Started(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "started", r)
}
func (s DaemonService) StartedOrEnabled(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "started_or_enabled", r)
}
func (s DaemonService) Stop(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "stop", r)
}
func (s DaemonService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "service", "update", r)
}
func (s SystemAdvancedService) Config(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "config", r)
}
func (s SystemAdvancedService) GetGpuPciChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "get_gpu_pci_choices", r)
}
func (s SystemAdvancedService) LoginBanner(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "login_banner", r)
}
func (s SystemAdvancedService) SedGlobalPassword(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "sed_global_password", r)
}
func (s SystemAdvancedService) SedGlobalPasswordIsSet(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "sed_global_password_is_set", r)
}
func (s SystemAdvancedService) SerialPortChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "serial_port_choices", r)
}
func (s SystemAdvancedService) SyslogCertificateAuthorityChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "syslog_certificate_authority_choices", r)
}
func (s SystemAdvancedService) SyslogCertificateChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "syslog_certificate_choices", r)
}
func (s SystemAdvancedService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "update", r)
}
func (s SystemAdvancedService) UpdateGpuPciIds(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.advanced", "update_gpu_pci_ids", r)
}
func (s SystemCoreService) BootId(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "boot_id", r)
}
func (s SystemCoreService) Debug(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "debug", r)
}
func (s SystemCoreService) FeatureEnabled(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "feature_enabled", r)
}
func (s SystemGeneralService) Checkin(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "checkin", r)
}
func (s SystemGeneralService) CheckinWaiting(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "checkin_waiting", r)
}
func (s SystemGeneralService) Config(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "config", r)
}
func (s SystemGeneralService) CountryChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "country_choices", r)
}
func (s SystemGeneralService) KbdmapChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "kbdmap_choices", r)
}
func (s SystemGeneralService) LocalUrl(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "local_url", r)
}
func (s SystemGeneralService) TimezoneChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "timezone_choices", r)
}
func (s SystemGeneralService) UiAddressChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "ui_address_choices", r)
}
func (s SystemGeneralService) UiCertificateChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "ui_certificate_choices", r)
}
func (s SystemGeneralService) UiHttpsprotocolsChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "ui_httpsprotocols_choices", r)
}
func (s SystemGeneralService) UiRestart(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "ui_restart", r)
}
func (s SystemGeneralService) UiV6addressChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "ui_v6address_choices", r)
}
func (s SystemGeneralService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.general", "update", r)
}
func (s SystemGlobalService) Id(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.global", "id", r)
}
func (s SystemCoreService) HostId(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "host_id", r)
}
func (s SystemCoreService) Info(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "info", r)
}
func (s SystemCoreService) LicenseUpdate(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "license_update", r)
}
func (s NTPServerService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.ntpserver", "create", r)
}
func (s NTPServerService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.ntpserver", "delete", r)
}
func (s NTPServerService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.ntpserver", "get_instance", r)
}
func (s NTPServerService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.ntpserver", "query", r)
}
func (s NTPServerService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.ntpserver", "update", r)
}
func (s SystemCoreService) ProductType(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "product_type", r)
}
func (s SystemCoreService) Ready(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "ready", r)
}
func (s SystemRebootService) Info(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.reboot", "info", r)
}
func (s SystemCoreService) ReleaseNotesUrl(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "release_notes_url", r)
}
func (s SystemSecurityService) Config(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.security", "config", r)
}
func (s SystemSecurityInfoService) FipsAvailable(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.security.info", "fips_available", r)
}
func (s SystemSecurityInfoService) FipsEnabled(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.security.info", "fips_enabled", r)
}
func (s SystemSecurityService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system.security", "update", r)
}
func (s SystemCoreService) Shutdown(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "shutdown", r)
}
func (s SystemCoreService) State(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "state", r)
}
func (s SystemCoreService) Version(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "version", r)
}
func (s SystemCoreService) VersionShort(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "system", "version_short", r)
}
func (s SystemDatasetService) Config(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "systemdataset", "config", r)
}
func (s SystemDatasetService) PoolChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "systemdataset", "pool_choices", r)
}
func (s SystemDatasetService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "systemdataset", "update", r)
}
func (s TunableService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "tunable", "create", r)
}
func (s TunableService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "tunable", "delete", r)
}
func (s TunableService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "tunable", "get_instance", r)
}
func (s TunableService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "tunable", "query", r)
}
func (s TunableService) TunableTypeChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "tunable", "tunable_type_choices", r)
}
func (s TunableService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "tunable", "update", r)
}
func (s UpdateService) AvailableVersions(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "available_versions", r)
}
func (s UpdateService) Config(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "config", r)
}
func (s UpdateService) Download(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "download", r)
}
func (s UpdateService) File(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "file", r)
}
func (s UpdateService) Manual(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "manual", r)
}
func (s UpdateService) ProfileChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "profile_choices", r)
}
func (s UpdateService) Run(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "run", r)
}
func (s UpdateService) Status(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "status", r)
}
func (s UpdateService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "system", "update", "update", r)
}
