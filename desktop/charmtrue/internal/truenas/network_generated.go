// Network method manifest pinned to TrueNAS API v25.10.5.
package truenas

import (
	"context"
	"encoding/json"
)

type NetworkMethod struct {
	Name, Service, Kind string
	Destructive         bool
}

var NetworkMethods = [...]NetworkMethod{
	{Name: "dns.query", Service: "DNSService", Kind: "read"},
	{Name: "interface.bridge_members_choices", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.cancel_rollback", Service: "InterfaceService", Kind: "change", Destructive: true},
	{Name: "interface.checkin", Service: "InterfaceService", Kind: "change", Destructive: true},
	{Name: "interface.checkin_waiting", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.choices", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.commit", Service: "InterfaceService", Kind: "change", Destructive: true},
	{Name: "interface.create", Service: "InterfaceService", Kind: "create", Destructive: true},
	{Name: "interface.delete", Service: "InterfaceService", Kind: "destructive", Destructive: true},
	{Name: "interface.get_instance", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.has_pending_changes", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.ip_in_use", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.lacpdu_rate_choices", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.lag_ports_choices", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.network_config_to_be_removed", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.query", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.rollback", Service: "InterfaceService", Kind: "change", Destructive: true},
	{Name: "interface.save_network_config", Service: "InterfaceService", Kind: "change", Destructive: true},
	{Name: "interface.services_restarted_on_sync", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.update", Service: "InterfaceService", Kind: "change", Destructive: true},
	{Name: "interface.vlan_parent_interface_choices", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.websocket_interface", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.websocket_local_ip", Service: "InterfaceService", Kind: "read"},
	{Name: "interface.xmit_hash_policy_choices", Service: "InterfaceService", Kind: "read"},
	{Name: "network.configuration.activity_choices", Service: "NetworkConfigurationService", Kind: "read"},
	{Name: "network.configuration.config", Service: "NetworkConfigurationService", Kind: "read"},
	{Name: "network.configuration.update", Service: "NetworkConfigurationService", Kind: "change", Destructive: true},
	{Name: "network.general.summary", Service: "NetworkGeneralService", Kind: "read"},
	{Name: "route.ipv4gw_reachable", Service: "RouteService", Kind: "read"},
	{Name: "route.system_routes", Service: "RouteService", Kind: "read"},
	{Name: "staticroute.create", Service: "StaticRouteService", Kind: "create", Destructive: true},
	{Name: "staticroute.delete", Service: "StaticRouteService", Kind: "destructive", Destructive: true},
	{Name: "staticroute.get_instance", Service: "StaticRouteService", Kind: "read"},
	{Name: "staticroute.query", Service: "StaticRouteService", Kind: "read"},
	{Name: "staticroute.update", Service: "StaticRouteService", Kind: "change", Destructive: true},
}

func NetworkMethodByName(name string) (NetworkMethod, bool) {
	for _, method := range NetworkMethods {
		if method.Name == name {
			return method, true
		}
	}
	return NetworkMethod{}, false
}

func (s DNSService) Query(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "query", r)
}
func (s InterfaceService) BridgeMembersChoices(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "bridge_members_choices", r)
}
func (s InterfaceService) CancelRollback(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "cancel_rollback", r)
}
func (s InterfaceService) Checkin(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "checkin", r)
}
func (s InterfaceService) CheckinWaiting(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "checkin_waiting", r)
}
func (s InterfaceService) Choices(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "choices", r)
}
func (s InterfaceService) Commit(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "commit", r)
}
func (s InterfaceService) Create(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "create", r)
}
func (s InterfaceService) Delete(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "delete", r)
}
func (s InterfaceService) GetInstance(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "get_instance", r)
}
func (s InterfaceService) HasPendingChanges(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "has_pending_changes", r)
}
func (s InterfaceService) IPInUse(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "ip_in_use", r)
}
func (s InterfaceService) LACPduRateChoices(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "lacpdu_rate_choices", r)
}
func (s InterfaceService) LAGPortsChoices(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "lag_ports_choices", r)
}
func (s InterfaceService) NetworkConfigToBeRemoved(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "network_config_to_be_removed", r)
}
func (s InterfaceService) Query(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "query", r)
}
func (s InterfaceService) Rollback(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "rollback", r)
}
func (s InterfaceService) SaveNetworkConfig(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "save_network_config", r)
}
func (s InterfaceService) ServicesRestartedOnSync(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "services_restarted_on_sync", r)
}
func (s InterfaceService) Update(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s InterfaceService) VLANParentInterfaceChoices(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "vlan_parent_interface_choices", r)
}
func (s InterfaceService) WebsocketInterface(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "websocket_interface", r)
}
func (s InterfaceService) WebsocketLocalIP(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "websocket_local_ip", r)
}
func (s InterfaceService) XmitHashPolicyChoices(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "xmit_hash_policy_choices", r)
}
func (s NetworkConfigurationService) ActivityChoices(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "activity_choices", r)
}
func (s NetworkConfigurationService) Config(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "config", r)
}
func (s NetworkConfigurationService) Update(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
func (s NetworkGeneralService) Summary(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "summary", r)
}
func (s RouteService) IPv4GatewayReachable(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "ipv4gw_reachable", r)
}
func (s RouteService) SystemRoutes(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "system_routes", r)
}
func (s StaticRouteService) Create(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "create", r)
}
func (s StaticRouteService) Delete(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "delete", r)
}
func (s StaticRouteService) GetInstance(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "get_instance", r)
}
func (s StaticRouteService) Query(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "query", r)
}
func (s StaticRouteService) Update(ctx context.Context, r NetworkCall) (json.RawMessage, error) {
	return s.Call(ctx, "update", r)
}
