package truenas

import (
	"context"
	"encoding/json"
)

// NetworkService groups every TrueNAS 25.10 network namespace.
type NetworkService struct {
	DNS           DNSService
	Interfaces    InterfaceService
	Configuration NetworkConfigurationService
	General       NetworkGeneralService
	Routes        RouteService
	StaticRoutes  StaticRouteService
}

type networkCaller struct{ client *Client }
type NetworkCall struct{ Params []any }
type DNSService struct{ networkCaller }
type InterfaceService struct{ networkCaller }
type NetworkConfigurationService struct{ networkCaller }
type NetworkGeneralService struct{ networkCaller }
type RouteService struct{ networkCaller }
type StaticRouteService struct{ networkCaller }

func (c *Client) Network() NetworkService {
	b := networkCaller{client: c}
	return NetworkService{DNSService{b}, InterfaceService{b}, NetworkConfigurationService{b}, NetworkGeneralService{b}, RouteService{b}, StaticRouteService{b}}
}

func networkServiceCall(ctx context.Context, caller networkCaller, namespace, method string, request NetworkCall) (json.RawMessage, error) {
	full := namespace + "." + method
	if _, ok := NetworkMethodByName(full); !ok {
		return nil, &ValidationError{Field: "method", Message: "is not a TrueNAS 25.10 network method"}
	}
	var result json.RawMessage
	if err := caller.client.Call(ctx, full, request.Params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s DNSService) Call(ctx context.Context, method string, request NetworkCall) (json.RawMessage, error) {
	return networkServiceCall(ctx, s.networkCaller, "dns", method, request)
}
func (s InterfaceService) Call(ctx context.Context, method string, request NetworkCall) (json.RawMessage, error) {
	return networkServiceCall(ctx, s.networkCaller, "interface", method, request)
}
func (s NetworkConfigurationService) Call(ctx context.Context, method string, request NetworkCall) (json.RawMessage, error) {
	return networkServiceCall(ctx, s.networkCaller, "network.configuration", method, request)
}
func (s NetworkGeneralService) Call(ctx context.Context, method string, request NetworkCall) (json.RawMessage, error) {
	return networkServiceCall(ctx, s.networkCaller, "network.general", method, request)
}
func (s RouteService) Call(ctx context.Context, method string, request NetworkCall) (json.RawMessage, error) {
	return networkServiceCall(ctx, s.networkCaller, "route", method, request)
}
func (s StaticRouteService) Call(ctx context.Context, method string, request NetworkCall) (json.RawMessage, error) {
	return networkServiceCall(ctx, s.networkCaller, "staticroute", method, request)
}

type InterfaceEntry struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	Fake                bool             `json:"fake"`
	Type                string           `json:"type"`
	State               InterfaceState   `json:"state"`
	Aliases             []InterfaceAlias `json:"aliases"`
	IPv4DHCP            bool             `json:"ipv4_dhcp"`
	IPv6Auto            bool             `json:"ipv6_auto"`
	Description         string           `json:"description"`
	MTU                 *int             `json:"mtu"`
	VLANParentInterface *string          `json:"vlan_parent_interface"`
	VLANTag             *int             `json:"vlan_tag"`
	VLANPCP             *int             `json:"vlan_pcp"`
	LAGProtocol         string           `json:"lag_protocol"`
	LAGPorts            []string         `json:"lag_ports"`
	BridgeMembers       []string         `json:"bridge_members"`
	EnableLearning      bool             `json:"enable_learning"`
}

type InterfaceState struct {
	Name                 string                `json:"name"`
	OriginalName         string                `json:"orig_name"`
	Description          string                `json:"description"`
	MTU                  int                   `json:"mtu"`
	Cloned               bool                  `json:"cloned"`
	Flags                []string              `json:"flags"`
	LinkState            string                `json:"link_state"`
	MediaType            string                `json:"media_type"`
	MediaSubtype         string                `json:"media_subtype"`
	ActiveMediaType      string                `json:"active_media_type"`
	ActiveMediaSubtype   string                `json:"active_media_subtype"`
	Capabilities         []any                 `json:"capabilities"`
	ND6Flags             []any                 `json:"nd6_flags"`
	SupportedMedia       []string              `json:"supported_media"`
	MediaOptions         []string              `json:"media_options"`
	LinkAddress          string                `json:"link_address"`
	PermanentLinkAddress string                `json:"permanent_link_address"`
	HardwareLinkAddress  string                `json:"hardware_link_address"`
	Aliases              []InterfaceStateAlias `json:"aliases"`
	Protocol             *string               `json:"protocol"`
	Ports                []InterfaceStatePort  `json:"ports"`
	Parent               *string               `json:"parent"`
	Tag                  *int                  `json:"tag"`
	PCP                  *int                  `json:"pcp"`
}

type InterfaceStateAlias struct {
	Type      string `json:"type"`
	Address   string `json:"address"`
	Netmask   any    `json:"netmask"`
	Broadcast string `json:"broadcast"`
}
type InterfaceStatePort struct {
	Name  string   `json:"name"`
	Flags []string `json:"flags"`
}
type InterfaceAlias struct {
	Type    string `json:"type"`
	Address string `json:"address"`
	Netmask any    `json:"netmask"`
}

type NetworkConfigurationEntry struct {
	ID                  int                          `json:"id"`
	Hostname            string                       `json:"hostname"`
	Domain              string                       `json:"domain"`
	IPv4Gateway         string                       `json:"ipv4gateway"`
	IPv6Gateway         string                       `json:"ipv6gateway"`
	NameServer1         string                       `json:"nameserver1"`
	NameServer2         string                       `json:"nameserver2"`
	NameServer3         string                       `json:"nameserver3"`
	HTTPProxy           string                       `json:"httpproxy"`
	Hosts               []string                     `json:"hosts"`
	Domains             []string                     `json:"domains"`
	ServiceAnnouncement NetworkServiceAnnouncement   `json:"service_announcement"`
	Activity            NetworkConfigurationActivity `json:"activity"`
	HostnameLocal       string                       `json:"hostname_local"`
	HostnameB           string                       `json:"hostname_b"`
	HostnameVirtual     string                       `json:"hostname_virtual"`
}
type NetworkServiceAnnouncement struct {
	NetBIOS bool `json:"netbios"`
	MDNS    bool `json:"mdns"`
	WSD     bool `json:"wsd"`
}
type NetworkConfigurationActivity struct {
	Type       string   `json:"type"`
	Activities []string `json:"activities"`
}
type NetworkGeneralSummary struct {
	IPs           map[string]NetworkSummaryIP `json:"ips"`
	DefaultRoutes []string                    `json:"default_routes"`
	NameServers   []string                    `json:"nameservers"`
}
type NetworkSummaryIP struct {
	IPv4 []string `json:"IPV4"`
	IPv6 []string `json:"IPV6"`
}
type StaticRouteEntry struct {
	ID          int    `json:"id"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Description string `json:"description"`
}
type DNSQueryEntry struct {
	Nameserver string `json:"nameserver"`
}
type SystemRouteEntry struct {
	Network   string   `json:"network"`
	Netmask   string   `json:"netmask"`
	Gateway   string   `json:"gateway"`
	Interface string   `json:"interface"`
	Flags     []string `json:"flags"`
}

func (s InterfaceService) QueryEntries(ctx context.Context, filters []Filter, options QueryOptions) ([]InterfaceEntry, error) {
	return Query[InterfaceEntry](ctx, s.client, "interface.query", filters, options)
}
func (s StaticRouteService) QueryEntries(ctx context.Context, filters []Filter, options QueryOptions) ([]StaticRouteEntry, error) {
	return Query[StaticRouteEntry](ctx, s.client, "staticroute.query", filters, options)
}
func (s DNSService) QueryEntries(ctx context.Context, filters []Filter, options QueryOptions) ([]DNSQueryEntry, error) {
	return Query[DNSQueryEntry](ctx, s.client, "dns.query", filters, options)
}
func (s RouteService) SystemRouteEntries(ctx context.Context, filters []Filter, options QueryOptions) ([]SystemRouteEntry, error) {
	return Query[SystemRouteEntry](ctx, s.client, "route.system_routes", filters, options)
}
