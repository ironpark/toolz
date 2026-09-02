package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/toolz/desktop/charmtrue/internal/truenas"
)

const connectionTimeout = 15 * time.Second

// TrueNASService is the Wails backend boundary for TrueNAS API operations.
type TrueNASService struct {
	mu           sync.RWMutex
	profilesMu   sync.Mutex
	client       *truenas.Client
	endpoint     string
	system       SystemInfo
	profilesPath string
	credentials  credentialStore
}

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// SystemInfo contains the system.info fields needed by the initial UI.
type SystemInfo struct {
	Hostname       string  `json:"hostname"`
	Version        string  `json:"version"`
	Model          string  `json:"model"`
	Cores          int     `json:"cores"`
	PhysicalMemory uint64  `json:"physmem"`
	Uptime         string  `json:"uptime"`
	UptimeSeconds  float64 `json:"uptimeSeconds"`
}

// apiSystemInfo mirrors the snake_case field names returned by TrueNAS.
// SystemInfo remains a frontend-friendly DTO with camelCase JSON fields.
type apiSystemInfo struct {
	Hostname       string  `json:"hostname"`
	Version        string  `json:"version"`
	Model          string  `json:"model"`
	Cores          int     `json:"cores"`
	PhysicalMemory uint64  `json:"physmem"`
	Uptime         string  `json:"uptime"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
}

// ConnectionInfo describes the currently connected TrueNAS system.
type ConnectionInfo struct {
	Connected bool       `json:"connected"`
	Endpoint  string     `json:"endpoint"`
	System    SystemInfo `json:"system"`
}

// StorageOverview is the frontend-facing snapshot of pools, disks, datasets,
// and snapshots used by the storage management screens.
type StorageOverview struct {
	Pools          []StoragePool    `json:"pools"`
	Disks          []StorageDisk    `json:"disks"`
	Datasets       []StorageDataset `json:"datasets"`
	SnapshotCount  int              `json:"snapshotCount"`
	TotalSize      uint64           `json:"totalSize"`
	TotalAllocated uint64           `json:"totalAllocated"`
	TotalFree      uint64           `json:"totalFree"`
}

type StoragePool struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	GUID      string `json:"guid"`
	Status    string `json:"status"`
	Healthy   bool   `json:"healthy"`
	Size      uint64 `json:"size"`
	Allocated uint64 `json:"allocated"`
	Free      uint64 `json:"free"`
}
type StorageDisk struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	Type       string `json:"type"`
	Size       uint64 `json:"size"`
	Pool       string `json:"pool"`
}
type StorageDataset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Pool       string `json:"pool"`
	Type       string `json:"type"`
	Mountpoint string `json:"mountpoint"`
	Encrypted  bool   `json:"encrypted"`
	Locked     bool   `json:"locked"`
	Used       uint64 `json:"used"`
	Available  uint64 `json:"available"`
}

type apiStorageDataset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Pool       string `json:"pool"`
	Type       string `json:"type"`
	Mountpoint string `json:"mountpoint"`
	Encrypted  bool   `json:"encrypted"`
	Locked     bool   `json:"locked"`
	Used       struct {
		Parsed uint64 `json:"parsed"`
	} `json:"used"`
	Available struct {
		Parsed uint64 `json:"parsed"`
	} `json:"available"`
}

type SharingOverview struct {
	Shares     []ShareInfo     `json:"shares"`
	RsyncTasks []RsyncTaskInfo `json:"rsyncTasks"`
	SMBCount   int             `json:"smbCount"`
	NFSCount   int             `json:"nfsCount"`
}
type ShareInfo struct {
	ID                          int      `json:"id"`
	Protocol                    string   `json:"protocol"`
	Name                        string   `json:"name"`
	Path                        string   `json:"path"`
	Purpose                     string   `json:"purpose"`
	Comment                     string   `json:"comment"`
	Enabled                     bool     `json:"enabled"`
	ReadOnly                    bool     `json:"readOnly"`
	Locked                      bool     `json:"locked"`
	Browsable                   bool     `json:"browsable"`
	AccessBasedShareEnumeration bool     `json:"accessBasedShareEnumeration"`
	RecycleBin                  bool     `json:"recycleBin"`
	PathSuffix                  string   `json:"pathSuffix"`
	HostsAllow                  []string `json:"hostsAllow"`
	HostsDeny                   []string `json:"hostsDeny"`
	Home                        bool     `json:"home"`
	Networks                    []string `json:"networks"`
	Hosts                       []string `json:"hosts"`
	MapRootUser                 string   `json:"mapRootUser"`
	MapRootGroup                string   `json:"mapRootGroup"`
	MapAllUser                  string   `json:"mapAllUser"`
	MapAllGroup                 string   `json:"mapAllGroup"`
	Security                    []string `json:"security"`
	ExposeSnapshots             bool     `json:"exposeSnapshots"`
}
type RsyncTaskInfo struct {
	ID                  int      `json:"id"`
	Path                string   `json:"path"`
	User                string   `json:"user"`
	Mode                string   `json:"mode"`
	RemoteHost          string   `json:"remoteHost"`
	RemotePort          int      `json:"remotePort"`
	RemoteModule        string   `json:"remoteModule"`
	RemotePath          string   `json:"remotePath"`
	SSHCredentialID     int      `json:"sshCredentialId"`
	Destination         string   `json:"destination"`
	Direction           string   `json:"direction"`
	Description         string   `json:"description"`
	ScheduleMinute      string   `json:"scheduleMinute"`
	ScheduleHour        string   `json:"scheduleHour"`
	ScheduleDayOfMonth  string   `json:"scheduleDayOfMonth"`
	ScheduleMonth       string   `json:"scheduleMonth"`
	ScheduleDayOfWeek   string   `json:"scheduleDayOfWeek"`
	Recursive           bool     `json:"recursive"`
	Times               bool     `json:"times"`
	Compress            bool     `json:"compress"`
	Archive             bool     `json:"archive"`
	Delete              bool     `json:"delete"`
	Quiet               bool     `json:"quiet"`
	PreservePermissions bool     `json:"preservePermissions"`
	PreserveAttributes  bool     `json:"preserveAttributes"`
	DelayUpdates        bool     `json:"delayUpdates"`
	Extra               []string `json:"extra"`
	Enabled             bool     `json:"enabled"`
	ValidateRemotePath  bool     `json:"validateRemotePath"`
	SSHKeyScan          bool     `json:"sshKeyScan"`
}

type ShareMutation struct {
	ID                          int      `json:"id"`
	Protocol                    string   `json:"protocol"`
	Name                        string   `json:"name"`
	Path                        string   `json:"path"`
	Purpose                     string   `json:"purpose"`
	Comment                     string   `json:"comment"`
	Enabled                     bool     `json:"enabled"`
	ReadOnly                    bool     `json:"readOnly"`
	Browsable                   bool     `json:"browsable"`
	AccessBasedShareEnumeration bool     `json:"accessBasedShareEnumeration"`
	RecycleBin                  bool     `json:"recycleBin"`
	PathSuffix                  string   `json:"pathSuffix"`
	HostsAllow                  []string `json:"hostsAllow"`
	HostsDeny                   []string `json:"hostsDeny"`
	Home                        bool     `json:"home"`
	Networks                    []string `json:"networks"`
	Hosts                       []string `json:"hosts"`
	MapRootUser                 string   `json:"mapRootUser"`
	MapRootGroup                string   `json:"mapRootGroup"`
	MapAllUser                  string   `json:"mapAllUser"`
	MapAllGroup                 string   `json:"mapAllGroup"`
	Security                    []string `json:"security"`
	ExposeSnapshots             bool     `json:"exposeSnapshots"`
}

type RsyncTaskMutation struct {
	ID                  int      `json:"id"`
	Path                string   `json:"path"`
	User                string   `json:"user"`
	Mode                string   `json:"mode"`
	RemoteHost          string   `json:"remoteHost"`
	RemotePort          int      `json:"remotePort"`
	RemoteModule        string   `json:"remoteModule"`
	RemotePath          string   `json:"remotePath"`
	SSHCredentialID     int      `json:"sshCredentialId"`
	Direction           string   `json:"direction"`
	Description         string   `json:"description"`
	ScheduleMinute      string   `json:"scheduleMinute"`
	ScheduleHour        string   `json:"scheduleHour"`
	ScheduleDayOfMonth  string   `json:"scheduleDayOfMonth"`
	ScheduleMonth       string   `json:"scheduleMonth"`
	ScheduleDayOfWeek   string   `json:"scheduleDayOfWeek"`
	Recursive           bool     `json:"recursive"`
	Times               bool     `json:"times"`
	Compress            bool     `json:"compress"`
	Archive             bool     `json:"archive"`
	Delete              bool     `json:"delete"`
	Quiet               bool     `json:"quiet"`
	PreservePermissions bool     `json:"preservePermissions"`
	PreserveAttributes  bool     `json:"preserveAttributes"`
	DelayUpdates        bool     `json:"delayUpdates"`
	Extra               []string `json:"extra"`
	Enabled             bool     `json:"enabled"`
	ValidateRemotePath  bool     `json:"validateRemotePath"`
	SSHKeyScan          bool     `json:"sshKeyScan"`
}

type NetworkOverview struct {
	Interfaces       []NetworkInterfaceInfo `json:"interfaces"`
	Configuration    NetworkConfiguration   `json:"configuration"`
	Summary          NetworkSummary         `json:"summary"`
	StaticRoutes     []StaticRouteInfo      `json:"staticRoutes"`
	PendingChanges   bool                   `json:"pendingChanges"`
	CheckinRemaining int                    `json:"checkinRemaining"`
}
type NetworkInterfaceInfo struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Type           string             `json:"type"`
	Description    string             `json:"description"`
	LinkState      string             `json:"linkState"`
	MediaType      string             `json:"mediaType"`
	MediaSubtype   string             `json:"mediaSubtype"`
	MACAddress     string             `json:"macAddress"`
	MTU            int                `json:"mtu"`
	IPv4DHCP       bool               `json:"ipv4Dhcp"`
	IPv6Auto       bool               `json:"ipv6Auto"`
	Aliases        []NetworkAliasInfo `json:"aliases"`
	LAGProtocol    string             `json:"lagProtocol"`
	LAGPorts       []string           `json:"lagPorts"`
	BridgeMembers  []string           `json:"bridgeMembers"`
	VLANParent     string             `json:"vlanParent"`
	VLANTag        int                `json:"vlanTag"`
	VLANPriority   int                `json:"vlanPriority"`
	EnableLearning bool               `json:"enableLearning"`
}
type NetworkAliasInfo struct {
	Type    string `json:"type"`
	Address string `json:"address"`
	Netmask int    `json:"netmask"`
}
type NetworkConfiguration struct {
	Hostname        string   `json:"hostname"`
	Domain          string   `json:"domain"`
	IPv4Gateway     string   `json:"ipv4Gateway"`
	IPv6Gateway     string   `json:"ipv6Gateway"`
	NameServers     []string `json:"nameServers"`
	HTTPProxy       string   `json:"httpProxy"`
	Hosts           []string `json:"hosts"`
	SearchDomains   []string `json:"searchDomains"`
	AnnounceNetBIOS bool     `json:"announceNetbios"`
	AnnounceMDNS    bool     `json:"announceMdns"`
	AnnounceWSD     bool     `json:"announceWsd"`
}
type NetworkSummary struct {
	IPs           map[string]NetworkSummaryIPInfo `json:"ips"`
	DefaultRoutes []string                        `json:"defaultRoutes"`
	NameServers   []string                        `json:"nameServers"`
}
type NetworkSummaryIPInfo struct {
	IPv4 []string `json:"ipv4"`
	IPv6 []string `json:"ipv6"`
}
type StaticRouteInfo struct {
	ID          int    `json:"id"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Description string `json:"description"`
}
type NetworkConfigurationMutation struct {
	Hostname        string   `json:"hostname"`
	Domain          string   `json:"domain"`
	IPv4Gateway     string   `json:"ipv4Gateway"`
	IPv6Gateway     string   `json:"ipv6Gateway"`
	NameServers     []string `json:"nameServers"`
	HTTPProxy       string   `json:"httpProxy"`
	Hosts           []string `json:"hosts"`
	SearchDomains   []string `json:"searchDomains"`
	AnnounceNetBIOS bool     `json:"announceNetbios"`
	AnnounceMDNS    bool     `json:"announceMdns"`
	AnnounceWSD     bool     `json:"announceWsd"`
}
type NetworkInterfaceMutation struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Type           string                 `json:"type"`
	Description    string                 `json:"description"`
	IPv4DHCP       bool                   `json:"ipv4Dhcp"`
	IPv6Auto       bool                   `json:"ipv6Auto"`
	Aliases        []NetworkAliasMutation `json:"aliases"`
	MTU            int                    `json:"mtu"`
	LAGProtocol    string                 `json:"lagProtocol"`
	LAGPorts       []string               `json:"lagPorts"`
	BridgeMembers  []string               `json:"bridgeMembers"`
	VLANParent     string                 `json:"vlanParent"`
	VLANTag        int                    `json:"vlanTag"`
	VLANPriority   int                    `json:"vlanPriority"`
	EnableLearning bool                   `json:"enableLearning"`
}
type NetworkAliasMutation struct {
	Type    string `json:"type"`
	Address string `json:"address"`
	Netmask int    `json:"netmask"`
}
type StaticRouteMutation struct {
	ID          int    `json:"id"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Description string `json:"description"`
}

type SystemManagementOverview struct {
	System   SystemInfo          `json:"system"`
	State    string              `json:"state"`
	Services []SystemServiceInfo `json:"services"`
	Update   UpdateInfo          `json:"update"`
}
type SystemServiceInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Enabled bool   `json:"enabled"`
}
type UpdateInfo struct {
	Status    string `json:"status"`
	Available bool   `json:"available"`
	Version   string `json:"version"`
}
type IdentityOverview struct {
	Users        []UserInfo        `json:"users"`
	Groups       []GroupInfo       `json:"groups"`
	APIKeys      []APIKeyInfo      `json:"apiKeys"`
	ShellChoices map[string]string `json:"shellChoices"`
}
type UserInfo struct {
	ID                     int      `json:"id"`
	UID                    int      `json:"uid"`
	Username               string   `json:"username"`
	FullName               string   `json:"fullName"`
	Email                  string   `json:"email"`
	Home                   string   `json:"home"`
	Shell                  string   `json:"shell"`
	Local                  bool     `json:"local"`
	Builtin                bool     `json:"builtin"`
	Immutable              bool     `json:"immutable"`
	Locked                 bool     `json:"locked"`
	PasswordDisabled       bool     `json:"passwordDisabled"`
	SSHPasswordEnabled     bool     `json:"sshPasswordEnabled"`
	SSHPublicKey           string   `json:"sshPublicKey"`
	SMB                    bool     `json:"smb"`
	UserNSIDMap            string   `json:"usernsIdmap"`
	PrimaryGroupID         int      `json:"primaryGroupId"`
	Groups                 []int    `json:"groups"`
	SudoCommands           []string `json:"sudoCommands"`
	SudoCommandsNoPassword []string `json:"sudoCommandsNoPassword"`
}
type GroupInfo struct {
	ID        int    `json:"id"`
	GID       int    `json:"gid"`
	Name      string `json:"name"`
	Local     bool   `json:"local"`
	Builtin   bool   `json:"builtin"`
	UserCount int    `json:"userCount"`
	SMB       bool   `json:"smb"`
}
type APIKeyInfo struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Revoked   bool   `json:"revoked"`
	ExpiresAt string `json:"expiresAt"`
}

type UserMutation struct {
	ID                     int      `json:"id"`
	UID                    int      `json:"uid"`
	SetUID                 bool     `json:"setUid"`
	Username               string   `json:"username"`
	FullName               string   `json:"fullName"`
	Email                  string   `json:"email"`
	Home                   string   `json:"home"`
	Shell                  string   `json:"shell"`
	Password               string   `json:"password"`
	RandomPassword         bool     `json:"randomPassword"`
	SMB                    bool     `json:"smb"`
	Locked                 bool     `json:"locked"`
	PasswordDisabled       bool     `json:"passwordDisabled"`
	SSHPasswordEnabled     bool     `json:"sshPasswordEnabled"`
	SSHPublicKey           string   `json:"sshPublicKey"`
	GroupCreate            bool     `json:"groupCreate"`
	PrimaryGroupID         int      `json:"primaryGroupId"`
	Groups                 []int    `json:"groups"`
	HomeCreate             bool     `json:"homeCreate"`
	HomeMode               string   `json:"homeMode"`
	UserNSIDMap            string   `json:"usernsIdmap"`
	SudoCommands           []string `json:"sudoCommands"`
	SudoCommandsNoPassword []string `json:"sudoCommandsNoPassword"`
}
type UserMutationResult struct {
	ID       int    `json:"id"`
	Password string `json:"password"`
}
type GroupMutation struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	SMB  bool   `json:"smb"`
}
type APIKeyMutation struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	ExpiresAt string `json:"expiresAt"`
	Reset     bool   `json:"reset"`
}
type APIKeyMutationResult struct {
	ID  int    `json:"id"`
	Key string `json:"key"`
}

func (s *TrueNASService) SystemManagementOverview() (SystemManagementOverview, error) {
	client, err := s.connectedClient()
	if err != nil {
		return SystemManagementOverview{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	services, err := client.System().Services.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return SystemManagementOverview{}, fmt.Errorf("서비스 조회 실패: %w", err)
	}
	var state string
	if err = client.Call(ctx, "system.state", nil, &state); err != nil {
		return SystemManagementOverview{}, fmt.Errorf("시스템 상태 조회 실패: %w", err)
	}
	var updateRaw struct {
		Status     string `json:"status"`
		NewVersion string `json:"new_version"`
		Version    string `json:"version"`
	}
	_ = client.Call(ctx, "update.status", nil, &updateRaw)
	s.mu.RLock()
	info := s.system
	s.mu.RUnlock()
	result := SystemManagementOverview{System: info, State: state, Update: UpdateInfo{Status: updateRaw.Status, Available: updateRaw.NewVersion != "", Version: updateRaw.NewVersion}}
	if result.Update.Version == "" {
		result.Update.Version = updateRaw.Version
	}
	if result.Update.Status == "" {
		result.Update.Status = "UNKNOWN"
	}
	for _, x := range services {
		result.Services = append(result.Services, SystemServiceInfo{x.ID, x.Service, x.State, x.Enable})
	}
	return result, nil
}

func (s *TrueNASService) ControlSystemService(name, action string) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	switch action {
	case "start", "stop", "restart", "reload":
	default:
		return errors.New("지원하지 않는 서비스 작업입니다")
	}
	if name == "" {
		return errors.New("서비스 이름이 필요합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	var ok bool
	if err = client.Call(ctx, "service."+action, []any{name, map[string]any{}}, &ok); err != nil {
		return fmt.Errorf("서비스 %s 실패: %w", action, err)
	}
	if !ok {
		return errors.New("TrueNAS가 서비스 작업을 완료하지 못했습니다")
	}
	return nil
}

func (s *TrueNASService) PowerAction(action string) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	switch action {
	case "reboot":
		return client.Call(ctx, "system.reboot", []any{"CharmTrue에서 요청", map[string]any{}}, nil)
	case "shutdown":
		return client.Call(ctx, "system.shutdown", []any{"CharmTrue에서 요청", map[string]any{}}, nil)
	default:
		return errors.New("지원하지 않는 전원 작업입니다")
	}
}

func (s *TrueNASService) IdentityOverview() (IdentityOverview, error) {
	client, err := s.connectedClient()
	if err != nil {
		return IdentityOverview{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	users, err := client.Identity().Users.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return IdentityOverview{}, fmt.Errorf("사용자 조회 실패: %w", err)
	}
	groups, err := client.Identity().Groups.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return IdentityOverview{}, fmt.Errorf("그룹 조회 실패: %w", err)
	}
	keys, err := client.Identity().APIKeys.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return IdentityOverview{}, fmt.Errorf("API 키 조회 실패: %w", err)
	}
	r := IdentityOverview{ShellChoices: map[string]string{}}
	// Shell choices are auxiliary editor metadata. Older or restricted systems
	// may deny this call, so keep the user list usable when it is unavailable.
	_ = client.Call(ctx, "user.shell_choices", []any{[]int{}}, &r.ShellChoices)
	for _, x := range users {
		sshPublicKey := ""
		if x.SSHPublicKey != nil {
			sshPublicKey = *x.SSHPublicKey
		}
		r.Users = append(r.Users, UserInfo{
			ID: x.ID, UID: x.UID, Username: x.Username, FullName: x.FullName, Email: x.Email,
			Home: x.Home, Shell: x.Shell, Local: x.Local, Builtin: x.Builtin, Immutable: x.Immutable,
			Locked: x.Locked, PasswordDisabled: x.PasswordDisabled, SSHPasswordEnabled: x.SSHPasswordEnabled,
			SSHPublicKey: sshPublicKey, SMB: x.SMB, UserNSIDMap: formatUserNSIDMap(x.UserNSIDMap),
			PrimaryGroupID: x.Group.ID, Groups: x.Groups, SudoCommands: x.SudoCommands,
			SudoCommandsNoPassword: x.SudoCommandsNoPassword,
		})
	}
	for _, x := range groups {
		r.Groups = append(r.Groups, GroupInfo{x.ID, x.GID, x.Group, x.Local, x.Builtin, len(x.Users), x.SMB})
	}
	for _, x := range keys {
		expiresAt := ""
		if x.ExpiresAt != nil {
			expiresAt = *x.ExpiresAt
		}
		r.APIKeys = append(r.APIKeys, APIKeyInfo{x.ID, x.Name, x.Username, x.Revoked, expiresAt})
	}
	return r, nil
}

func (s *TrueNASService) SaveUser(input UserMutation) (UserMutationResult, error) {
	client, err := s.connectedClient()
	if err != nil {
		return UserMutationResult{}, err
	}
	input.Username = strings.TrimSpace(input.Username)
	input.FullName = strings.TrimSpace(input.FullName)
	input.Email = strings.TrimSpace(input.Email)
	input.Home = strings.TrimSpace(input.Home)
	input.Shell = strings.TrimSpace(input.Shell)
	input.HomeMode = strings.TrimSpace(input.HomeMode)
	input.SSHPublicKey = strings.TrimSpace(input.SSHPublicKey)
	if input.Username == "" || input.FullName == "" {
		return UserMutationResult{}, errors.New("사용자명과 표시 이름을 입력하세요")
	}
	if input.SMB && input.PasswordDisabled {
		return UserMutationResult{}, errors.New("SMB 사용자는 비밀번호를 비활성화할 수 없습니다")
	}
	if input.SSHPasswordEnabled && input.PasswordDisabled {
		return UserMutationResult{}, errors.New("SSH 비밀번호 로그인을 사용하려면 비밀번호 로그인을 활성화하세요")
	}
	if input.RandomPassword && input.Password != "" {
		return UserMutationResult{}, errors.New("비밀번호 직접 입력과 임의 비밀번호 생성 중 하나만 선택하세요")
	}
	if input.Home == "" {
		input.Home = "/var/empty"
	}
	if input.Shell == "" {
		input.Shell = "/usr/bin/zsh"
	}
	if input.HomeMode == "" {
		input.HomeMode = "700"
	}
	userNSIDMap, err := parseUserNSIDMap(input.UserNSIDMap)
	if err != nil {
		return UserMutationResult{}, err
	}
	data := map[string]any{
		"username": input.Username, "full_name": input.FullName, "email": nullableString(input.Email),
		"home": input.Home, "shell": input.Shell, "smb": input.SMB, "locked": input.Locked,
		"password_disabled": input.PasswordDisabled, "ssh_password_enabled": input.SSHPasswordEnabled,
		"sshpubkey": nullableString(input.SSHPublicKey), "userns_idmap": userNSIDMap,
		"group": nullablePositiveInt(input.PrimaryGroupID), "groups": cleanGroupIDs(input.Groups, input.PrimaryGroupID),
		"sudo_commands":          cleanStringList(input.SudoCommands),
		"sudo_commands_nopasswd": cleanStringList(input.SudoCommandsNoPassword),
		"home_create":            input.HomeCreate, "home_mode": input.HomeMode, "random_password": input.RandomPassword,
	}
	if input.Password != "" {
		data["password"] = input.Password
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	var result UserMutationResult
	if input.ID == 0 {
		if input.Password == "" && !input.RandomPassword && !input.PasswordDisabled {
			return UserMutationResult{}, errors.New("비밀번호를 입력하거나 임의 비밀번호 생성을 선택하세요")
		}
		if !input.GroupCreate && input.PrimaryGroupID < 1 {
			return UserMutationResult{}, errors.New("기본 그룹을 선택하거나 사용자명과 같은 그룹 자동 생성을 선택하세요")
		}
		if input.SetUID {
			if input.UID < 0 || input.UID > 90000000 {
				return UserMutationResult{}, errors.New("UID는 0에서 90000000 사이여야 합니다")
			}
			data["uid"] = input.UID
		}
		data["group_create"] = input.GroupCreate
		err = client.Call(ctx, "user.create", []any{data}, &result)
		return result, err
	}
	err = client.Call(ctx, "user.update", []any{input.ID, data}, &result)
	return result, err
}

func nullablePositiveInt(value int) any {
	if value < 1 {
		return nil
	}
	return value
}

func cleanStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func cleanGroupIDs(values []int, primaryGroupID int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value < 1 || value == primaryGroupID {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseUserNSIDMap(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.EqualFold(value, "DIRECT") {
		return "DIRECT", nil
	}
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil || id < 1 || id > 4294967294 {
		return nil, errors.New("컨테이너 UID 매핑은 DIRECT 또는 1~4294967294 사이 숫자여야 합니다")
	}
	return id, nil
}

func formatUserNSIDMap(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		return strconv.FormatUint(uint64(value), 10)
	case json.Number:
		return value.String()
	default:
		return fmt.Sprint(value)
	}
}

func (s *TrueNASService) SaveGroup(input GroupMutation) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return errors.New("그룹 이름을 입력하세요")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	data := map[string]any{"name": input.Name, "smb": input.SMB}
	if input.ID == 0 {
		return client.Call(ctx, "group.create", []any{data}, nil)
	}
	return client.Call(ctx, "group.update", []any{input.ID, data}, nil)
}

func (s *TrueNASService) SaveAPIKey(input APIKeyMutation) (APIKeyMutationResult, error) {
	client, err := s.connectedClient()
	if err != nil {
		return APIKeyMutationResult{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Username = strings.TrimSpace(input.Username)
	input.ExpiresAt = strings.TrimSpace(input.ExpiresAt)
	if input.Name == "" {
		return APIKeyMutationResult{}, errors.New("API 키 이름을 입력하세요")
	}
	data := map[string]any{"name": input.Name}
	if input.ExpiresAt == "" {
		data["expires_at"] = nil
	} else {
		data["expires_at"] = input.ExpiresAt
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	var result struct {
		ID  int    `json:"id"`
		Key string `json:"key"`
	}
	if input.ID == 0 {
		if input.Username == "" {
			return APIKeyMutationResult{}, errors.New("API 키 소유 사용자명을 입력하세요")
		}
		data["username"] = input.Username
		if err = client.Call(ctx, "api_key.create", []any{data}, &result); err != nil {
			return APIKeyMutationResult{}, err
		}
	} else {
		data["reset"] = input.Reset
		if err = client.Call(ctx, "api_key.update", []any{input.ID, data}, &result); err != nil {
			return APIKeyMutationResult{}, err
		}
	}
	return APIKeyMutationResult{result.ID, result.Key}, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *TrueNASService) DeleteIdentity(kind string, id int) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	if id < 1 {
		return errors.New("올바른 ID가 필요합니다")
	}
	var method string
	var params []any
	switch kind {
	case "user":
		method = "user.delete"
		params = []any{id, map[string]any{}}
	case "group":
		method = "group.delete"
		params = []any{id, map[string]any{"delete_users": false}}
	case "api_key":
		method = "api_key.delete"
		params = []any{id}
	default:
		return errors.New("지원하지 않는 계정 항목입니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, method, params, nil)
}

func (s *TrueNASService) connectedClient() (*truenas.Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.client == nil {
		return nil, errors.New("TrueNAS 시스템을 먼저 연결하세요")
	}
	return s.client, nil
}

// SharingOverview returns SMB, NFS and rsync definitions.
func (s *TrueNASService) SharingOverview() (SharingOverview, error) {
	client, err := s.connectedClient()
	if err != nil {
		return SharingOverview{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	smb, err := client.Sharing().SMBShares.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return SharingOverview{}, fmt.Errorf("SMB 공유 조회 실패: %w", err)
	}
	nfs, err := client.Sharing().NFSShares.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return SharingOverview{}, fmt.Errorf("NFS 공유 조회 실패: %w", err)
	}
	rsync, err := client.Sharing().RsyncTasks.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return SharingOverview{}, fmt.Errorf("Rsync 작업 조회 실패: %w", err)
	}
	result := SharingOverview{SMBCount: len(smb), NFSCount: len(nfs)}
	for _, x := range smb {
		result.Shares = append(result.Shares, ShareInfo{
			ID: x.ID, Protocol: "SMB", Name: x.Name, Path: x.Path, Purpose: x.Purpose,
			Comment: x.Comment, Enabled: x.Enabled, ReadOnly: x.ReadOnly || x.LegacyReadOnly,
			Locked: x.Locked, Browsable: x.Browsable, AccessBasedShareEnumeration: x.AccessBasedShareEnumeration,
			RecycleBin: x.RecycleBin, PathSuffix: optionalString(x.PathSuffix), HostsAllow: x.HostsAllow,
			HostsDeny: x.HostsDeny, Home: x.Home,
		})
	}
	for _, x := range nfs {
		path := x.Path
		if path == "" && len(x.LegacyPaths) > 0 {
			path = x.LegacyPaths[0]
		}
		result.Shares = append(result.Shares, ShareInfo{
			ID: x.ID, Protocol: "NFS", Name: path, Path: path, Comment: x.Comment, Enabled: x.Enabled,
			ReadOnly: x.ReadOnly, Networks: x.Networks, Hosts: x.Hosts,
			MapRootUser: optionalString(x.MapRootUser), MapRootGroup: optionalString(x.MapRootGroup),
			MapAllUser: optionalString(x.MapAllUser), MapAllGroup: optionalString(x.MapAllGroup),
			Security: x.Security, ExposeSnapshots: x.ExposeSnapshots,
		})
	}
	for _, x := range rsync {
		remotePort, sshCredentialID := 0, 0
		if x.RemotePort != nil {
			remotePort = *x.RemotePort
		}
		if x.SSHCredentials != nil {
			sshCredentialID = x.SSHCredentials.ID
		}
		destination := x.RemoteHost + ":" + x.RemoteModule
		if x.Mode == "SSH" {
			destination = x.RemoteHost + ":" + x.RemotePath
		}
		result.RsyncTasks = append(result.RsyncTasks, RsyncTaskInfo{
			ID: x.ID, Path: x.Path, User: x.User, Mode: x.Mode, RemoteHost: x.RemoteHost,
			RemotePort: remotePort, RemoteModule: x.RemoteModule, RemotePath: x.RemotePath,
			SSHCredentialID: sshCredentialID, Destination: destination, Direction: x.Direction,
			Description: x.Description, ScheduleMinute: x.Schedule.Minute, ScheduleHour: x.Schedule.Hour,
			ScheduleDayOfMonth: x.Schedule.DayOfMonth, ScheduleMonth: x.Schedule.Month,
			ScheduleDayOfWeek: x.Schedule.DayOfWeek, Recursive: x.Recursive, Times: x.Times,
			Compress: x.Compress, Archive: x.Archive, Delete: x.Delete, Quiet: x.Quiet,
			PreservePermissions: x.PreservePermissions, PreserveAttributes: x.PreserveAttributes,
			DelayUpdates: x.DelayUpdates, Extra: x.Extra, Enabled: x.Enabled,
			ValidateRemotePath: x.ValidateRemotePath, SSHKeyScan: x.SSHKeyScan,
		})
	}
	return result, nil
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *TrueNASService) SaveShare(input ShareMutation) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	if input.ID < 1 {
		return errors.New("올바른 공유 ID가 필요합니다")
	}
	input.Protocol = strings.ToUpper(strings.TrimSpace(input.Protocol))
	input.Name = strings.TrimSpace(input.Name)
	input.Path = strings.TrimSpace(input.Path)
	input.Comment = strings.TrimSpace(input.Comment)
	var method string
	var data map[string]any
	switch input.Protocol {
	case "SMB":
		if input.Name == "" || input.Path == "" {
			return errors.New("SMB 공유 이름과 경로를 입력하세요")
		}
		if input.Path != "EXTERNAL" && !strings.HasPrefix(input.Path, "/mnt/") {
			return errors.New("SMB 공유 경로는 /mnt/ 아래여야 합니다")
		}
		method = "sharing.smb.update"
		data = map[string]any{
			"name": input.Name, "path": input.Path, "purpose": input.Purpose, "comment": input.Comment,
			"enabled": input.Enabled, "readonly": input.ReadOnly, "browsable": input.Browsable,
			"access_based_share_enumeration": input.AccessBasedShareEnumeration,
			"recyclebin":                     input.RecycleBin, "path_suffix": nullableString(strings.TrimSpace(input.PathSuffix)),
			"hostsallow": cleanStringList(input.HostsAllow), "hostsdeny": cleanStringList(input.HostsDeny),
			"home": input.Home,
		}
	case "NFS":
		if input.Path == "" || !strings.HasPrefix(input.Path, "/mnt/") {
			return errors.New("NFS 공유 경로는 /mnt/ 아래여야 합니다")
		}
		method = "sharing.nfs.update"
		data = map[string]any{
			"path": input.Path, "comment": input.Comment, "networks": cleanStringList(input.Networks),
			"hosts": cleanStringList(input.Hosts), "ro": input.ReadOnly,
			"maproot_user":  nullableString(strings.TrimSpace(input.MapRootUser)),
			"maproot_group": nullableString(strings.TrimSpace(input.MapRootGroup)),
			"mapall_user":   nullableString(strings.TrimSpace(input.MapAllUser)),
			"mapall_group":  nullableString(strings.TrimSpace(input.MapAllGroup)),
			"security":      cleanStringList(input.Security), "enabled": input.Enabled,
			"expose_snapshots": input.ExposeSnapshots,
		}
	default:
		return errors.New("지원하지 않는 공유 프로토콜입니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	if err := client.Call(ctx, method, []any{input.ID, data}, nil); err != nil {
		return fmt.Errorf("%s 공유 수정 실패: %w", input.Protocol, err)
	}
	return nil
}

func (s *TrueNASService) SaveRsyncTask(input RsyncTaskMutation) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	input.Path = strings.TrimSpace(input.Path)
	input.User = strings.TrimSpace(input.User)
	input.Mode = strings.ToUpper(strings.TrimSpace(input.Mode))
	input.RemoteHost = strings.TrimSpace(input.RemoteHost)
	input.Direction = strings.ToUpper(strings.TrimSpace(input.Direction))
	if input.ID < 1 || input.Path == "" || input.User == "" || input.RemoteHost == "" {
		return errors.New("Rsync 작업의 경로, 실행 사용자와 원격 호스트를 입력하세요")
	}
	if input.Mode != "MODULE" && input.Mode != "SSH" {
		return errors.New("Rsync 전송 방식은 MODULE 또는 SSH여야 합니다")
	}
	if input.Direction != "PULL" && input.Direction != "PUSH" {
		return errors.New("Rsync 전송 방향은 PULL 또는 PUSH여야 합니다")
	}
	if input.Mode == "MODULE" && strings.TrimSpace(input.RemoteModule) == "" {
		return errors.New("MODULE 방식에는 원격 모듈 이름이 필요합니다")
	}
	if input.Mode == "SSH" && strings.TrimSpace(input.RemotePath) == "" {
		return errors.New("SSH 방식에는 원격 경로가 필요합니다")
	}
	data := map[string]any{
		"path": input.Path, "user": input.User, "mode": input.Mode,
		"remotehost": input.RemoteHost, "remoteport": nullablePositiveInt(input.RemotePort),
		"remotemodule": nullableString(strings.TrimSpace(input.RemoteModule)),
		"remotepath":   strings.TrimSpace(input.RemotePath), "ssh_credentials": nullablePositiveInt(input.SSHCredentialID),
		"direction": input.Direction, "desc": strings.TrimSpace(input.Description),
		"schedule": map[string]any{
			"minute": defaultString(input.ScheduleMinute, "00"), "hour": defaultString(input.ScheduleHour, "*"),
			"dom": defaultString(input.ScheduleDayOfMonth, "*"), "month": defaultString(input.ScheduleMonth, "*"),
			"dow": defaultString(input.ScheduleDayOfWeek, "*"),
		},
		"recursive": input.Recursive, "times": input.Times, "compress": input.Compress,
		"archive": input.Archive, "delete": input.Delete, "quiet": input.Quiet,
		"preserveperm": input.PreservePermissions, "preserveattr": input.PreserveAttributes,
		"delayupdates": input.DelayUpdates, "extra": cleanStringList(input.Extra), "enabled": input.Enabled,
		"validate_rpath": input.ValidateRemotePath, "ssh_keyscan": input.SSHKeyScan,
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	if err := client.Call(ctx, "rsynctask.update", []any{input.ID, data}, nil); err != nil {
		return fmt.Errorf("Rsync 작업 수정 실패: %w", err)
	}
	return nil
}

func defaultString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

// NetworkOverview returns the active network configuration together with any
// staged interface changes waiting to be committed or checked in.
func (s *TrueNASService) NetworkOverview() (NetworkOverview, error) {
	client, err := s.connectedClient()
	if err != nil {
		return NetworkOverview{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	interfaces, err := client.Network().Interfaces.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return NetworkOverview{}, fmt.Errorf("네트워크 인터페이스 조회 실패: %w", err)
	}
	staticRoutes, err := client.Network().StaticRoutes.QueryEntries(ctx, nil, truenas.QueryOptions{})
	if err != nil {
		return NetworkOverview{}, fmt.Errorf("정적 라우트 조회 실패: %w", err)
	}
	var configuration truenas.NetworkConfigurationEntry
	if err := client.Call(ctx, "network.configuration.config", nil, &configuration); err != nil {
		return NetworkOverview{}, fmt.Errorf("네트워크 전역 설정 조회 실패: %w", err)
	}
	var summary truenas.NetworkGeneralSummary
	if err := client.Call(ctx, "network.general.summary", nil, &summary); err != nil {
		return NetworkOverview{}, fmt.Errorf("네트워크 요약 조회 실패: %w", err)
	}
	var pending bool
	if err := client.Call(ctx, "interface.has_pending_changes", nil, &pending); err != nil {
		return NetworkOverview{}, fmt.Errorf("대기 중인 네트워크 변경 조회 실패: %w", err)
	}
	var waiting *int
	if err := client.Call(ctx, "interface.checkin_waiting", nil, &waiting); err != nil {
		return NetworkOverview{}, fmt.Errorf("네트워크 체크인 상태 조회 실패: %w", err)
	}

	result := NetworkOverview{
		Configuration: NetworkConfiguration{
			Hostname: configuration.Hostname, Domain: configuration.Domain,
			IPv4Gateway: configuration.IPv4Gateway, IPv6Gateway: configuration.IPv6Gateway,
			NameServers: cleanStringList([]string{configuration.NameServer1, configuration.NameServer2, configuration.NameServer3}),
			HTTPProxy:   configuration.HTTPProxy, Hosts: configuration.Hosts, SearchDomains: configuration.Domains,
			AnnounceNetBIOS: configuration.ServiceAnnouncement.NetBIOS,
			AnnounceMDNS:    configuration.ServiceAnnouncement.MDNS, AnnounceWSD: configuration.ServiceAnnouncement.WSD,
		},
		Summary:        NetworkSummary{IPs: make(map[string]NetworkSummaryIPInfo, len(summary.IPs)), DefaultRoutes: summary.DefaultRoutes, NameServers: summary.NameServers},
		PendingChanges: pending,
	}
	if waiting != nil {
		result.CheckinRemaining = *waiting
	}
	for name, addresses := range summary.IPs {
		result.Summary.IPs[name] = NetworkSummaryIPInfo{IPv4: addresses.IPv4, IPv6: addresses.IPv6}
	}
	for _, entry := range interfaces {
		mtu := entry.State.MTU
		if entry.MTU != nil {
			mtu = *entry.MTU
		}
		info := NetworkInterfaceInfo{
			ID: entry.ID, Name: entry.Name, Type: entry.Type, Description: entry.Description,
			LinkState: entry.State.LinkState, MediaType: entry.State.ActiveMediaType,
			MediaSubtype: entry.State.ActiveMediaSubtype, MACAddress: entry.State.HardwareLinkAddress,
			MTU: mtu, IPv4DHCP: entry.IPv4DHCP, IPv6Auto: entry.IPv6Auto,
			LAGProtocol: entry.LAGProtocol, LAGPorts: entry.LAGPorts, BridgeMembers: entry.BridgeMembers,
			VLANParent: optionalString(entry.VLANParentInterface), VLANTag: optionalInt(entry.VLANTag),
			VLANPriority: optionalInt(entry.VLANPCP), EnableLearning: entry.EnableLearning,
		}
		for _, alias := range entry.Aliases {
			info.Aliases = append(info.Aliases, NetworkAliasInfo{Type: alias.Type, Address: alias.Address, Netmask: networkNetmask(alias.Netmask)})
		}
		result.Interfaces = append(result.Interfaces, info)
	}
	for _, route := range staticRoutes {
		result.StaticRoutes = append(result.StaticRoutes, StaticRouteInfo{ID: route.ID, Destination: route.Destination, Gateway: route.Gateway, Description: route.Description})
	}
	return result, nil
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func networkNetmask(value any) int {
	switch value := value.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		parsed, _ := strconv.Atoi(value.String())
		return parsed
	case string:
		parsed, _ := strconv.Atoi(value)
		return parsed
	default:
		return 0
	}
}

func (s *TrueNASService) SaveNetworkConfiguration(input NetworkConfigurationMutation) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	input.Hostname = strings.TrimSpace(input.Hostname)
	input.Domain = strings.TrimSpace(input.Domain)
	if input.Hostname == "" {
		return errors.New("호스트 이름을 입력하세요")
	}
	nameServers := cleanStringList(input.NameServers)
	if len(nameServers) > 3 {
		return errors.New("DNS 서버는 최대 3개까지 지정할 수 있습니다")
	}
	for _, value := range append([]string{input.IPv4Gateway, input.IPv6Gateway}, nameServers...) {
		if value != "" {
			if _, err := netip.ParseAddr(strings.TrimSpace(value)); err != nil {
				return fmt.Errorf("올바르지 않은 IP 주소입니다: %s", value)
			}
		}
	}
	dns := append(nameServers, "", "")
	data := map[string]any{
		"hostname": input.Hostname, "domain": input.Domain,
		"ipv4gateway": strings.TrimSpace(input.IPv4Gateway), "ipv6gateway": strings.TrimSpace(input.IPv6Gateway),
		"nameserver1": dns[0], "nameserver2": dns[1], "nameserver3": dns[2],
		"httpproxy": strings.TrimSpace(input.HTTPProxy), "hosts": cleanStringList(input.Hosts),
		"domains":              cleanStringList(input.SearchDomains),
		"service_announcement": map[string]any{"netbios": input.AnnounceNetBIOS, "mdns": input.AnnounceMDNS, "wsd": input.AnnounceWSD},
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	if err := client.Call(ctx, "network.configuration.update", []any{data}, nil); err != nil {
		return fmt.Errorf("네트워크 전역 설정 수정 실패: %w", err)
	}
	return nil
}

func (s *TrueNASService) SaveNetworkInterface(input NetworkInterfaceMutation) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	input.ID = strings.TrimSpace(input.ID)
	input.Type = strings.ToUpper(strings.TrimSpace(input.Type))
	if input.ID == "" && input.Type != "BRIDGE" && input.Type != "LINK_AGGREGATION" && input.Type != "VLAN" {
		return errors.New("가상 인터페이스 유형을 선택하세요")
	}
	if input.MTU != 0 && (input.MTU < 68 || input.MTU > 9216) {
		return errors.New("MTU는 68에서 9216 사이여야 합니다")
	}
	aliases := make([]map[string]any, 0, len(input.Aliases))
	for _, alias := range input.Aliases {
		alias.Type = strings.ToUpper(strings.TrimSpace(alias.Type))
		alias.Address = strings.TrimSpace(alias.Address)
		address, parseErr := netip.ParseAddr(alias.Address)
		if parseErr != nil || (alias.Type != "INET" && alias.Type != "INET6") {
			return fmt.Errorf("올바르지 않은 인터페이스 주소입니다: %s", alias.Address)
		}
		maxMask := 32
		if address.Is6() {
			maxMask = 128
		}
		if alias.Netmask < 0 || alias.Netmask > maxMask {
			return fmt.Errorf("%s의 네트워크 마스크 범위가 올바르지 않습니다", alias.Address)
		}
		aliases = append(aliases, map[string]any{"type": alias.Type, "address": alias.Address, "netmask": alias.Netmask})
	}
	data := map[string]any{
		"description": strings.TrimSpace(input.Description), "ipv4_dhcp": input.IPv4DHCP,
		"ipv6_auto": input.IPv6Auto, "aliases": aliases,
	}
	if name := strings.TrimSpace(input.Name); name != "" && (input.ID == "" || name != input.ID) {
		data["name"] = name
	}
	if input.ID == "" {
		data["type"] = input.Type
	}
	if input.MTU > 0 {
		data["mtu"] = input.MTU
	} else {
		data["mtu"] = nil
	}
	if input.Type == "LINK_AGGREGATION" || input.LAGProtocol != "" {
		if input.LAGProtocol == "" || len(cleanStringList(input.LAGPorts)) == 0 {
			return errors.New("링크 집계 프로토콜과 구성 포트를 입력하세요")
		}
		data["lag_protocol"] = input.LAGProtocol
		data["lag_ports"] = cleanStringList(input.LAGPorts)
	}
	if input.Type == "BRIDGE" || len(input.BridgeMembers) > 0 {
		if len(cleanStringList(input.BridgeMembers)) == 0 {
			return errors.New("브리지 구성 인터페이스를 입력하세요")
		}
		data["bridge_members"] = cleanStringList(input.BridgeMembers)
		data["enable_learning"] = input.EnableLearning
	}
	if input.Type == "VLAN" || input.VLANParent != "" || input.VLANTag > 0 {
		if strings.TrimSpace(input.VLANParent) == "" || input.VLANTag < 1 || input.VLANTag > 4094 || input.VLANPriority < 0 || input.VLANPriority > 7 {
			return errors.New("VLAN 부모 인터페이스, 1~4094 태그와 0~7 우선순위를 확인하세요")
		}
		data["vlan_parent_interface"] = strings.TrimSpace(input.VLANParent)
		data["vlan_tag"] = input.VLANTag
		data["vlan_pcp"] = input.VLANPriority
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	method, params := "interface.create", []any{data}
	if input.ID != "" {
		method, params = "interface.update", []any{input.ID, data}
	}
	if err := client.Call(ctx, method, params, nil); err != nil {
		return fmt.Errorf("네트워크 인터페이스 수정 실패: %w", err)
	}
	return nil
}

func (s *TrueNASService) DeleteNetworkInterface(id string) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("인터페이스 ID가 필요합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	if err := client.Call(ctx, "interface.delete", []any{id}, nil); err != nil {
		return fmt.Errorf("가상 인터페이스 삭제 실패: %w", err)
	}
	return nil
}

func (s *TrueNASService) CommitNetworkChanges(checkinTimeout int) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	if checkinTimeout < 30 || checkinTimeout > 300 {
		return errors.New("체크인 제한 시간은 30초에서 300초 사이여야 합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, "interface.commit", []any{map[string]any{"rollback": true, "checkin_timeout": checkinTimeout}}, nil)
}

func (s *TrueNASService) CheckinNetworkChanges() error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, "interface.checkin", nil, nil)
}

func (s *TrueNASService) RollbackNetworkChanges() error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, "interface.rollback", nil, nil)
}

func (s *TrueNASService) SaveStaticRoute(input StaticRouteMutation) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	input.Destination = strings.TrimSpace(input.Destination)
	input.Gateway = strings.TrimSpace(input.Gateway)
	if _, err := netip.ParsePrefix(input.Destination); err != nil {
		return errors.New("목적지는 CIDR 형식으로 입력하세요")
	}
	if _, err := netip.ParseAddr(input.Gateway); err != nil {
		return errors.New("올바른 게이트웨이 IP 주소를 입력하세요")
	}
	data := map[string]any{"destination": input.Destination, "gateway": input.Gateway, "description": strings.TrimSpace(input.Description)}
	method, params := "staticroute.create", []any{data}
	if input.ID > 0 {
		method, params = "staticroute.update", []any{input.ID, data}
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	if err := client.Call(ctx, method, params, nil); err != nil {
		return fmt.Errorf("정적 라우트 저장 실패: %w", err)
	}
	return nil
}

func (s *TrueNASService) DeleteStaticRoute(id int) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	if id < 1 {
		return errors.New("올바른 정적 라우트 ID가 필요합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, "staticroute.delete", []any{id}, nil)
}

func (s *TrueNASService) SetShareEnabled(protocol string, id int, enabled bool) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	if id < 1 {
		return errors.New("올바른 공유 ID가 필요합니다")
	}
	var method string
	switch strings.ToUpper(protocol) {
	case "SMB":
		method = "sharing.smb.update"
	case "NFS":
		method = "sharing.nfs.update"
	default:
		return errors.New("지원하지 않는 공유 프로토콜입니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, method, []any{id, map[string]any{"enabled": enabled}}, nil)
}
func (s *TrueNASService) DeleteShare(protocol string, id int) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	var method string
	switch strings.ToUpper(protocol) {
	case "SMB":
		method = "sharing.smb.delete"
	case "NFS":
		method = "sharing.nfs.delete"
	default:
		return errors.New("지원하지 않는 공유 프로토콜입니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, method, []any{id}, nil)
}
func (s *TrueNASService) RunRsyncTask(id int) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return client.Call(ctx, "rsynctask.run", []any{id}, nil)
}

// StorageOverview fetches the storage collections in parallel at the Wails
// boundary. A failure in any collection fails the refresh so stale partial
// state is never presented as current.
func (s *TrueNASService) StorageOverview() (StorageOverview, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()
	if client == nil {
		return StorageOverview{}, errors.New("TrueNAS 시스템을 먼저 연결하세요")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	var result StorageOverview
	var datasets []apiStorageDataset
	var snapshots []struct {
		ID string `json:"id"`
	}
	type callResult struct{ err error }
	calls := make(chan callResult, 4)
	go func() {
		pools, err := client.Storage().Pools.QueryEntries(ctx, nil, truenas.QueryOptions{})
		if err == nil {
			for _, p := range pools {
				result.Pools = append(result.Pools, StoragePool{
					ID: p.ID, Name: p.Name, GUID: p.Guid, Status: p.Status,
					Healthy: p.Healthy, Size: p.Size, Allocated: p.Allocated, Free: p.Free,
				})
			}
		}
		calls <- callResult{err}
	}()
	go func() {
		disks, err := client.Storage().Disks.QueryEntries(ctx, nil, truenas.QueryOptions{})
		if err == nil {
			for _, d := range disks {
				var model, pool string
				var size uint64
				if d.Model != nil {
					model = *d.Model
				}
				if d.Pool != nil {
					pool = *d.Pool
				}
				if d.Size != nil {
					size = *d.Size
				}
				result.Disks = append(result.Disks, StorageDisk{d.Name, d.Identifier, model, d.Serial, d.Type, size, pool})
			}
		}
		calls <- callResult{err}
	}()
	go func() {
		err := client.Call(ctx, "pool.dataset.query", []any{[]any{}, truenas.QueryOptions{}}, &datasets)
		calls <- callResult{err}
	}()
	go func() {
		err := client.Call(ctx, "pool.snapshot.query", []any{[]any{}, truenas.QueryOptions{Count: true}}, &result.SnapshotCount)
		if err != nil {
			err = client.Call(ctx, "pool.snapshot.query", []any{[]any{}, truenas.QueryOptions{}}, &snapshots)
			result.SnapshotCount = len(snapshots)
		}
		calls <- callResult{err}
	}()
	for range 4 {
		if err := (<-calls).err; err != nil {
			return StorageOverview{}, fmt.Errorf("스토리지 정보 조회 실패: %w", err)
		}
	}
	for _, d := range datasets {
		result.Datasets = append(result.Datasets, StorageDataset{d.ID, d.Name, d.Pool, d.Type, d.Mountpoint, d.Encrypted, d.Locked, d.Used.Parsed, d.Available.Parsed})
	}
	result.TotalSize, result.TotalAllocated, result.TotalFree = summarizePoolCapacity(result.Pools)
	return result, nil
}

// summarizePoolCapacity prevents aliases or duplicate pool.query rows from
// inflating the appliance totals. A ZFS GUID identifies the physical pool even
// when TrueNAS exposes it through more than one logical name.
func summarizePoolCapacity(pools []StoragePool) (size, allocated, free uint64) {
	unique := make(map[string]StoragePool, len(pools))
	for _, pool := range pools {
		key := pool.GUID
		if key == "" {
			key = pool.Name
		}
		if key == "" {
			key = fmt.Sprintf("id:%d", pool.ID)
		}
		current, exists := unique[key]
		if !exists || pool.Size > current.Size || (pool.Size == current.Size && pool.Allocated > current.Allocated) {
			unique[key] = pool
		}
	}
	for _, pool := range unique {
		size += pool.Size
		allocated += pool.Allocated
		free += pool.Free
	}
	return size, allocated, free
}

func (s *TrueNASService) AppInfo() AppInfo {
	s.mu.RLock()
	connected := s.client != nil
	s.mu.RUnlock()

	status := "No TrueNAS system configured"
	if connected {
		status = "Connected"
	}
	return AppInfo{Name: "CharmTrue", Version: "0.1.0", Status: status}
}

// Connect authenticates to TrueNAS, verifies the session with system.info,
// then keeps the connection for subsequent service calls.
func (s *TrueNASService) Connect(endpoint, username, secret, authenticationMethod string, allowPrivateCertificate, saveServer, saveCredential bool) (ConnectionInfo, error) {
	if saveCredential && !saveServer {
		return ConnectionInfo{}, errors.New("로그인 정보를 저장하려면 서버 프로필 저장을 활성화하세요")
	}
	return s.connect(endpoint, username, secret, authenticationMethod, allowPrivateCertificate, saveServer, saveCredential)
}

// ConnectSavedServer signs in with a credential read entirely in the Go
// backend so a stored password or API key is never returned to the frontend.
func (s *TrueNASService) ConnectSavedServer(id string) (ConnectionInfo, error) {
	server, err := s.savedServer(id)
	if err != nil {
		return ConnectionInfo{}, err
	}
	if !server.CredentialStored {
		return ConnectionInfo{}, errors.New("이 서버에 저장된 로그인 정보가 없습니다")
	}
	secret, err := s.credentialStore().Get(credentialService, credentialAccount(server.ID, server.AuthenticationMethod))
	if err != nil {
		return ConnectionInfo{}, credentialReadError(err)
	}
	return s.connect(server.Endpoint, server.Username, secret, server.AuthenticationMethod, server.AllowPrivateCertificate, true, true)
}

func (s *TrueNASService) connect(endpoint, username, secret, authenticationMethod string, allowPrivateCertificate, saveServer, saveCredential bool) (ConnectionInfo, error) {
	endpoint = strings.TrimSpace(endpoint)
	username = strings.TrimSpace(username)
	authenticationMethod = strings.TrimSpace(authenticationMethod)
	if endpoint == "" {
		return ConnectionInfo{}, errors.New("TrueNAS 서버 주소를 입력하세요")
	}
	if secret == "" {
		return ConnectionInfo{}, errors.New("TrueNAS 인증 정보를 입력하세요")
	}
	if username == "" {
		return ConnectionInfo{}, errors.New("TrueNAS 사용자명을 입력하세요")
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	config := truenas.Config{
		Endpoint:           endpoint,
		Username:           username,
		InsecureSkipVerify: allowPrivateCertificate,
	}
	switch authenticationMethod {
	case "api_key":
		config.APIKey = strings.TrimSpace(secret)
		if config.APIKey == "" {
			return ConnectionInfo{}, errors.New("TrueNAS API 키를 입력하세요")
		}
	case "password":
		config.Password = secret
	default:
		return ConnectionInfo{}, errors.New("지원하지 않는 TrueNAS 인증 방식입니다")
	}

	client, err := truenas.Dial(ctx, config)
	if err != nil {
		if errors.Is(err, truenas.ErrOTPRequired) {
			return ConnectionInfo{}, errors.New("TrueNAS 계정에 2단계 인증이 설정되어 있습니다. OTP 로그인 지원이 필요합니다")
		}
		if errors.Is(err, truenas.ErrAuthenticationFailed) {
			return ConnectionInfo{}, errors.New("TrueNAS 인증 실패: 사용자명과 선택한 인증 정보를 확인하세요")
		}
		return ConnectionInfo{}, fmt.Errorf("TrueNAS 연결 실패: %w", err)
	}

	var apiSystem apiSystemInfo
	if err := client.Call(ctx, "system.info", nil, &apiSystem); err != nil {
		_ = client.Close()
		return ConnectionInfo{}, fmt.Errorf("TrueNAS 시스템 정보 조회 실패: %w", err)
	}
	system := SystemInfo{
		Hostname:       apiSystem.Hostname,
		Version:        apiSystem.Version,
		Model:          apiSystem.Model,
		Cores:          apiSystem.Cores,
		PhysicalMemory: apiSystem.PhysicalMemory,
		Uptime:         apiSystem.Uptime,
		UptimeSeconds:  apiSystem.UptimeSeconds,
	}
	if saveServer {
		if err := s.saveServerWithCredential(SavedServer{
			Name:                    system.Hostname,
			Endpoint:                endpoint,
			Username:                username,
			AuthenticationMethod:    authenticationMethod,
			AllowPrivateCertificate: allowPrivateCertificate,
		}, secret, saveCredential); err != nil {
			_ = client.Close()
			return ConnectionInfo{}, fmt.Errorf("서버 프로필 저장 실패: %w", err)
		}
	}

	s.mu.Lock()
	previous := s.client
	s.client = client
	s.endpoint = endpoint
	s.system = system
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}

	return ConnectionInfo{Connected: true, Endpoint: endpoint, System: system}, nil
}

// CurrentConnection returns the in-memory connection state without exposing
// credentials.
func (s *TrueNASService) CurrentConnection() ConnectionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ConnectionInfo{Connected: s.client != nil, Endpoint: s.endpoint, System: s.system}
}

// Disconnect closes and clears the current TrueNAS connection.
func (s *TrueNASService) Disconnect() error {
	s.mu.Lock()
	client := s.client
	s.client = nil
	s.endpoint = ""
	s.system = SystemInfo{}
	s.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Close()
}
