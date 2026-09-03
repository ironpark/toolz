package truenas

import (
	"context"
	"encoding/json"
)

// ManagementCall preserves the positional JSON-RPC argument order documented
// by TrueNAS. Generated wrappers validate the method against the pinned API.
type ManagementCall struct{ Params []any }

type managementCaller struct{ client *Client }

func managementCall(ctx context.Context, c managementCaller, domain, namespace, method string, req ManagementCall) (json.RawMessage, error) {
	full := namespace + "." + method
	var ok bool
	if domain == "system" {
		_, ok = SystemMethodByName(full)
	} else {
		_, ok = IdentityMethodByName(full)
	}
	if !ok {
		return nil, &ValidationError{Field: "method", Message: "is not a TrueNAS 25.10 " + domain + " method"}
	}
	var result json.RawMessage
	if err := c.client.Call(ctx, full, req.Params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type SystemService struct {
	Boot                BootService
	BootEnvironments    BootEnvironmentService
	Config              ConfigService
	CronJobs            CronJobService
	InitShutdownScripts InitShutdownScriptService
	Services            DaemonService
	System              SystemCoreService
	Advanced            SystemAdvancedService
	General             SystemGeneralService
	Global              SystemGlobalService
	NTPServers          NTPServerService
	Reboot              SystemRebootService
	Security            SystemSecurityService
	SecurityInfo        SystemSecurityInfoService
	Dataset             SystemDatasetService
	Tunables            TunableService
	Updates             UpdateService
}
type BootService struct{ managementCaller }
type BootEnvironmentService struct{ managementCaller }
type ConfigService struct{ managementCaller }
type CronJobService struct{ managementCaller }
type InitShutdownScriptService struct{ managementCaller }
type DaemonService struct{ managementCaller }
type SystemCoreService struct{ managementCaller }
type SystemAdvancedService struct{ managementCaller }
type SystemGeneralService struct{ managementCaller }
type SystemGlobalService struct{ managementCaller }
type NTPServerService struct{ managementCaller }
type SystemRebootService struct{ managementCaller }
type SystemSecurityService struct{ managementCaller }
type SystemSecurityInfoService struct{ managementCaller }
type SystemDatasetService struct{ managementCaller }
type TunableService struct{ managementCaller }
type UpdateService struct{ managementCaller }

func (c *Client) System() SystemService {
	b := managementCaller{c}
	return SystemService{BootService{b}, BootEnvironmentService{b}, ConfigService{b}, CronJobService{b}, InitShutdownScriptService{b}, DaemonService{b}, SystemCoreService{b}, SystemAdvancedService{b}, SystemGeneralService{b}, SystemGlobalService{b}, NTPServerService{b}, SystemRebootService{b}, SystemSecurityService{b}, SystemSecurityInfoService{b}, SystemDatasetService{b}, TunableService{b}, UpdateService{b}}
}

type IdentityService struct {
	APIKeys    APIKeyService
	Auth       AuthService
	TwoFactor  TwoFactorService
	Groups     GroupService
	Privileges PrivilegeService
	Users      UserService
}
type APIKeyService struct{ managementCaller }
type AuthService struct{ managementCaller }
type TwoFactorService struct{ managementCaller }
type GroupService struct{ managementCaller }
type PrivilegeService struct{ managementCaller }
type UserService struct{ managementCaller }

func (c *Client) Identity() IdentityService {
	b := managementCaller{c}
	return IdentityService{APIKeyService{b}, AuthService{b}, TwoFactorService{b}, GroupService{b}, PrivilegeService{b}, UserService{b}}
}

type ServiceEntry struct {
	ID      int    `json:"id"`
	Service string `json:"service"`
	State   string `json:"state"`
	Enable  bool   `json:"enable"`
}
type UserEntry struct {
	ID                     int              `json:"id"`
	UID                    int              `json:"uid"`
	Username               string           `json:"username"`
	FullName               string           `json:"full_name"`
	Email                  string           `json:"email"`
	Home                   string           `json:"home"`
	Shell                  string           `json:"shell"`
	Local                  bool             `json:"local"`
	Builtin                bool             `json:"builtin"`
	Immutable              bool             `json:"immutable"`
	PasswordDisabled       bool             `json:"password_disabled"`
	SSHPasswordEnabled     bool             `json:"ssh_password_enabled"`
	SSHPublicKey           *string          `json:"sshpubkey"`
	Locked                 bool             `json:"locked"`
	SMB                    bool             `json:"smb"`
	UserNSIDMap            any              `json:"userns_idmap"`
	Group                  UserPrimaryGroup `json:"group"`
	Groups                 []int            `json:"groups"`
	SudoCommands           []string         `json:"sudo_commands"`
	SudoCommandsNoPassword []string         `json:"sudo_commands_nopasswd"`
}
type UserPrimaryGroup struct {
	ID    int    `json:"id"`
	GID   int    `json:"gid"`
	Group string `json:"group"`
}
type GroupEntry struct {
	ID      int    `json:"id"`
	GID     int    `json:"gid"`
	Group   string `json:"group"`
	Local   bool   `json:"local"`
	Builtin bool   `json:"builtin"`
	SMB     bool   `json:"smb"`
	Users   []int  `json:"users"`
}
type APIKeyEntry struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Username  string  `json:"username"`
	CreatedAt any     `json:"created_at"`
	ExpiresAt *string `json:"expires_at"`
	Revoked   bool    `json:"revoked"`
}

func (s DaemonService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]ServiceEntry, error) {
	return Query[ServiceEntry](ctx, s.client, "service.query", f, o)
}
func (s UserService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]UserEntry, error) {
	return Query[UserEntry](ctx, s.client, "user.query", f, o)
}
func (s GroupService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]GroupEntry, error) {
	return Query[GroupEntry](ctx, s.client, "group.query", f, o)
}
func (s APIKeyService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]APIKeyEntry, error) {
	return Query[APIKeyEntry](ctx, s.client, "api_key.query", f, o)
}

//go:generate go run ./cmd/genmanagement -root ../../docs/truenas-api-25.10 -out .
