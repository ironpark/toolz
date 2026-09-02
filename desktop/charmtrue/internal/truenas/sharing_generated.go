// Code generated from TrueNAS API v25.10.5; DO NOT EDIT.
package truenas

import (
	"context"
	"encoding/json"
)

type SharingMethod struct {
	Name, Service, Kind string
	Destructive         bool
}

var SharingMethods = [...]SharingMethod{
	{Name: "ftp.config", Service: "FTPService", Kind: "read", Destructive: false},
	{Name: "ftp.update", Service: "FTPService", Kind: "change", Destructive: false},
	{Name: "nfs.bindip_choices", Service: "NFSService", Kind: "read", Destructive: false},
	{Name: "nfs.client_count", Service: "NFSService", Kind: "read", Destructive: false},
	{Name: "nfs.config", Service: "NFSService", Kind: "read", Destructive: false},
	{Name: "nfs.get_nfs3_clients", Service: "NFSService", Kind: "read", Destructive: false},
	{Name: "nfs.get_nfs4_clients", Service: "NFSService", Kind: "read", Destructive: false},
	{Name: "nfs.update", Service: "NFSService", Kind: "change", Destructive: false},
	{Name: "rsynctask.create", Service: "RsyncTaskService", Kind: "create", Destructive: false},
	{Name: "rsynctask.delete", Service: "RsyncTaskService", Kind: "destructive", Destructive: true},
	{Name: "rsynctask.get_instance", Service: "RsyncTaskService", Kind: "read", Destructive: false},
	{Name: "rsynctask.query", Service: "RsyncTaskService", Kind: "read", Destructive: false},
	{Name: "rsynctask.run", Service: "RsyncTaskService", Kind: "change", Destructive: false},
	{Name: "rsynctask.update", Service: "RsyncTaskService", Kind: "change", Destructive: false},
	{Name: "sharing.nfs.create", Service: "NFSShareService", Kind: "create", Destructive: false},
	{Name: "sharing.nfs.delete", Service: "NFSShareService", Kind: "destructive", Destructive: true},
	{Name: "sharing.nfs.get_instance", Service: "NFSShareService", Kind: "read", Destructive: false},
	{Name: "sharing.nfs.query", Service: "NFSShareService", Kind: "read", Destructive: false},
	{Name: "sharing.nfs.update", Service: "NFSShareService", Kind: "change", Destructive: false},
	{Name: "sharing.smb.create", Service: "SMBShareService", Kind: "create", Destructive: false},
	{Name: "sharing.smb.delete", Service: "SMBShareService", Kind: "destructive", Destructive: true},
	{Name: "sharing.smb.get_instance", Service: "SMBShareService", Kind: "read", Destructive: false},
	{Name: "sharing.smb.getacl", Service: "SMBShareService", Kind: "read", Destructive: false},
	{Name: "sharing.smb.presets", Service: "SMBShareService", Kind: "read", Destructive: false},
	{Name: "sharing.smb.query", Service: "SMBShareService", Kind: "read", Destructive: false},
	{Name: "sharing.smb.setacl", Service: "SMBShareService", Kind: "destructive", Destructive: true},
	{Name: "sharing.smb.share_precheck", Service: "SMBShareService", Kind: "change", Destructive: false},
	{Name: "sharing.smb.update", Service: "SMBShareService", Kind: "change", Destructive: false},
	{Name: "smb.bindip_choices", Service: "SMBService", Kind: "read", Destructive: false},
	{Name: "smb.config", Service: "SMBService", Kind: "read", Destructive: false},
	{Name: "smb.unixcharset_choices", Service: "SMBService", Kind: "read", Destructive: false},
	{Name: "smb.update", Service: "SMBService", Kind: "change", Destructive: false},
	{Name: "ssh.bindiface_choices", Service: "SSHService", Kind: "read", Destructive: false},
	{Name: "ssh.config", Service: "SSHService", Kind: "read", Destructive: false},
	{Name: "ssh.update", Service: "SSHService", Kind: "change", Destructive: false},
}

func SharingMethodByName(n string) (SharingMethod, bool) {
	for _, m := range SharingMethods {
		if m.Name == n {
			return m, true
		}
	}
	return SharingMethod{}, false
}
func (s FTPService) Config(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "config", r)
}
func (s FTPService) Update(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s NFSService) BindipChoices(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "bindip_choices", r)
}
func (s NFSService) ClientCount(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "client_count", r)
}
func (s NFSService) Config(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "config", r)
}
func (s NFSService) GetNfs3Clients(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "get_nfs3_clients", r)
}
func (s NFSService) GetNfs4Clients(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "get_nfs4_clients", r)
}
func (s NFSService) Update(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s RsyncTaskService) Create(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "create", r)
}
func (s RsyncTaskService) Delete(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "delete", r)
}
func (s RsyncTaskService) GetInstance(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "get_instance", r)
}
func (s RsyncTaskService) Query(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "query", r)
}
func (s RsyncTaskService) Run(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "run", r)
}
func (s RsyncTaskService) Update(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s NFSShareService) Create(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "create", r)
}
func (s NFSShareService) Delete(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "delete", r)
}
func (s NFSShareService) GetInstance(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "get_instance", r)
}
func (s NFSShareService) Query(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "query", r)
}
func (s NFSShareService) Update(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s SMBShareService) Create(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "create", r)
}
func (s SMBShareService) Delete(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "delete", r)
}
func (s SMBShareService) GetInstance(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "get_instance", r)
}
func (s SMBShareService) Getacl(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "getacl", r)
}
func (s SMBShareService) Presets(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "presets", r)
}
func (s SMBShareService) Query(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "query", r)
}
func (s SMBShareService) Setacl(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "setacl", r)
}
func (s SMBShareService) SharePrecheck(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "share_precheck", r)
}
func (s SMBShareService) Update(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s SMBService) BindipChoices(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "bindip_choices", r)
}
func (s SMBService) Config(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "config", r)
}
func (s SMBService) UnixcharsetChoices(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "unixcharset_choices", r)
}
func (s SMBService) Update(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s SSHService) BindifaceChoices(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "bindiface_choices", r)
}
func (s SSHService) Config(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "config", r)
}
func (s SSHService) Update(ctx context.Context, r SharingCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
