// Code generated from TrueNAS API v25.10.5; DO NOT EDIT.
package truenas

import (
	"context"
	"encoding/json"
)

type IdentityMethod struct {
	Name, Service, Kind string
	Destructive         bool
}

var IdentityMethods = [...]IdentityMethod{
	{Name: "api_key.create", Service: "APIKeyService", Kind: "create", Destructive: false},
	{Name: "api_key.delete", Service: "APIKeyService", Kind: "destructive", Destructive: true},
	{Name: "api_key.get_instance", Service: "APIKeyService", Kind: "read", Destructive: false},
	{Name: "api_key.my_keys", Service: "APIKeyService", Kind: "read", Destructive: false},
	{Name: "api_key.query", Service: "APIKeyService", Kind: "read", Destructive: false},
	{Name: "api_key.update", Service: "APIKeyService", Kind: "change", Destructive: false},
	{Name: "auth.generate_onetime_password", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.generate_token", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.login", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.login_ex", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.login_ex_continue", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.login_with_api_key", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.login_with_token", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.logout", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.me", Service: "AuthService", Kind: "read", Destructive: false},
	{Name: "auth.mechanism_choices", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.sessions", Service: "AuthService", Kind: "read", Destructive: false},
	{Name: "auth.set_attribute", Service: "AuthService", Kind: "change", Destructive: false},
	{Name: "auth.terminate_other_sessions", Service: "AuthService", Kind: "destructive", Destructive: true},
	{Name: "auth.terminate_session", Service: "AuthService", Kind: "destructive", Destructive: true},
	{Name: "auth.twofactor.config", Service: "TwoFactorService", Kind: "read", Destructive: false},
	{Name: "auth.twofactor.update", Service: "TwoFactorService", Kind: "change", Destructive: false},
	{Name: "group.create", Service: "GroupService", Kind: "create", Destructive: false},
	{Name: "group.delete", Service: "GroupService", Kind: "destructive", Destructive: true},
	{Name: "group.get_group_obj", Service: "GroupService", Kind: "change", Destructive: false},
	{Name: "group.get_instance", Service: "GroupService", Kind: "read", Destructive: false},
	{Name: "group.get_next_gid", Service: "GroupService", Kind: "change", Destructive: false},
	{Name: "group.has_password_enabled_user", Service: "GroupService", Kind: "change", Destructive: false},
	{Name: "group.query", Service: "GroupService", Kind: "read", Destructive: false},
	{Name: "group.update", Service: "GroupService", Kind: "change", Destructive: false},
	{Name: "privilege.create", Service: "PrivilegeService", Kind: "create", Destructive: false},
	{Name: "privilege.delete", Service: "PrivilegeService", Kind: "destructive", Destructive: true},
	{Name: "privilege.get_instance", Service: "PrivilegeService", Kind: "read", Destructive: false},
	{Name: "privilege.query", Service: "PrivilegeService", Kind: "read", Destructive: false},
	{Name: "privilege.roles", Service: "PrivilegeService", Kind: "read", Destructive: false},
	{Name: "privilege.update", Service: "PrivilegeService", Kind: "change", Destructive: false},
	{Name: "user.create", Service: "UserService", Kind: "create", Destructive: false},
	{Name: "user.delete", Service: "UserService", Kind: "destructive", Destructive: true},
	{Name: "user.get_instance", Service: "UserService", Kind: "read", Destructive: false},
	{Name: "user.get_next_uid", Service: "UserService", Kind: "change", Destructive: false},
	{Name: "user.get_user_obj", Service: "UserService", Kind: "change", Destructive: false},
	{Name: "user.has_local_administrator_set_up", Service: "UserService", Kind: "change", Destructive: false},
	{Name: "user.query", Service: "UserService", Kind: "read", Destructive: false},
	{Name: "user.renew_2fa_secret", Service: "UserService", Kind: "change", Destructive: false},
	{Name: "user.set_password", Service: "UserService", Kind: "destructive", Destructive: true},
	{Name: "user.setup_local_administrator", Service: "UserService", Kind: "change", Destructive: false},
	{Name: "user.shell_choices", Service: "UserService", Kind: "change", Destructive: false},
	{Name: "user.unset_2fa_secret", Service: "UserService", Kind: "destructive", Destructive: true},
	{Name: "user.update", Service: "UserService", Kind: "change", Destructive: false},
}

func IdentityMethodByName(n string) (IdentityMethod, bool) {
	for _, m := range IdentityMethods {
		if m.Name == n {
			return m, true
		}
	}
	return IdentityMethod{}, false
}
func (s APIKeyService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "api_key", "create", r)
}
func (s APIKeyService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "api_key", "delete", r)
}
func (s APIKeyService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "api_key", "get_instance", r)
}
func (s APIKeyService) MyKeys(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "api_key", "my_keys", r)
}
func (s APIKeyService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "api_key", "query", r)
}
func (s APIKeyService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "api_key", "update", r)
}
func (s AuthService) GenerateOnetimePassword(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "generate_onetime_password", r)
}
func (s AuthService) GenerateToken(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "generate_token", r)
}
func (s AuthService) Login(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "login", r)
}
func (s AuthService) LoginEx(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "login_ex", r)
}
func (s AuthService) LoginExContinue(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "login_ex_continue", r)
}
func (s AuthService) LoginWithApiKey(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "login_with_api_key", r)
}
func (s AuthService) LoginWithToken(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "login_with_token", r)
}
func (s AuthService) Logout(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "logout", r)
}
func (s AuthService) Me(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "me", r)
}
func (s AuthService) MechanismChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "mechanism_choices", r)
}
func (s AuthService) Sessions(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "sessions", r)
}
func (s AuthService) SetAttribute(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "set_attribute", r)
}
func (s AuthService) TerminateOtherSessions(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "terminate_other_sessions", r)
}
func (s AuthService) TerminateSession(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth", "terminate_session", r)
}
func (s TwoFactorService) Config(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth.twofactor", "config", r)
}
func (s TwoFactorService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "auth.twofactor", "update", r)
}
func (s GroupService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "create", r)
}
func (s GroupService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "delete", r)
}
func (s GroupService) GetGroupObj(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "get_group_obj", r)
}
func (s GroupService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "get_instance", r)
}
func (s GroupService) GetNextGid(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "get_next_gid", r)
}
func (s GroupService) HasPasswordEnabledUser(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "has_password_enabled_user", r)
}
func (s GroupService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "query", r)
}
func (s GroupService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "group", "update", r)
}
func (s PrivilegeService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "privilege", "create", r)
}
func (s PrivilegeService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "privilege", "delete", r)
}
func (s PrivilegeService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "privilege", "get_instance", r)
}
func (s PrivilegeService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "privilege", "query", r)
}
func (s PrivilegeService) Roles(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "privilege", "roles", r)
}
func (s PrivilegeService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "privilege", "update", r)
}
func (s UserService) Create(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "create", r)
}
func (s UserService) Delete(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "delete", r)
}
func (s UserService) GetInstance(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "get_instance", r)
}
func (s UserService) GetNextUid(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "get_next_uid", r)
}
func (s UserService) GetUserObj(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "get_user_obj", r)
}
func (s UserService) HasLocalAdministratorSetUp(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "has_local_administrator_set_up", r)
}
func (s UserService) Query(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "query", r)
}
func (s UserService) Renew2faSecret(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "renew_2fa_secret", r)
}
func (s UserService) SetPassword(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "set_password", r)
}
func (s UserService) SetupLocalAdministrator(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "setup_local_administrator", r)
}
func (s UserService) ShellChoices(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "shell_choices", r)
}
func (s UserService) Unset2faSecret(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "unset_2fa_secret", r)
}
func (s UserService) Update(ctx context.Context, r ManagementCall) (json.RawMessage, error) {
	return managementCall(ctx, s.managementCaller, "identity", "user", "update", r)
}
