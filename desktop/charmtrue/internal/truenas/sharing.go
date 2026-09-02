package truenas

import (
	"context"
	"encoding/json"
)

type SharingService struct {
	FTP        FTPService
	NFS        NFSService
	RsyncTasks RsyncTaskService
	NFSShares  NFSShareService
	SMBShares  SMBShareService
	SMB        SMBService
	SSH        SSHService
}
type sharingCaller struct{ client *Client }
type SharingCall struct{ Params []any }
type FTPService struct{ sharingCaller }
type NFSService struct{ sharingCaller }
type RsyncTaskService struct{ sharingCaller }
type NFSShareService struct{ sharingCaller }
type SMBShareService struct{ sharingCaller }
type SMBService struct{ sharingCaller }
type SSHService struct{ sharingCaller }

func (c *Client) Sharing() SharingService {
	b := sharingCaller{c}
	return SharingService{FTPService{b}, NFSService{b}, RsyncTaskService{b}, NFSShareService{b}, SMBShareService{b}, SMBService{b}, SSHService{b}}
}
func sharingServiceCall(ctx context.Context, c sharingCaller, namespace, method string, r SharingCall) (json.RawMessage, error) {
	full := namespace + "." + method
	if _, ok := SharingMethodByName(full); !ok {
		return nil, &ValidationError{Field: "method", Message: "is not a TrueNAS 25.10 sharing method"}
	}
	var result json.RawMessage
	if err := c.client.Call(ctx, full, r.Params, &result); err != nil {
		return nil, err
	}
	return result, nil
}
func (s FTPService) Call(c context.Context, m string, r SharingCall) (json.RawMessage, error) {
	return sharingServiceCall(c, s.sharingCaller, "ftp", m, r)
}
func (s NFSService) Call(c context.Context, m string, r SharingCall) (json.RawMessage, error) {
	return sharingServiceCall(c, s.sharingCaller, "nfs", m, r)
}
func (s RsyncTaskService) Call(c context.Context, m string, r SharingCall) (json.RawMessage, error) {
	return sharingServiceCall(c, s.sharingCaller, "rsynctask", m, r)
}
func (s NFSShareService) Call(c context.Context, m string, r SharingCall) (json.RawMessage, error) {
	return sharingServiceCall(c, s.sharingCaller, "sharing.nfs", m, r)
}
func (s SMBShareService) Call(c context.Context, m string, r SharingCall) (json.RawMessage, error) {
	return sharingServiceCall(c, s.sharingCaller, "sharing.smb", m, r)
}
func (s SMBService) Call(c context.Context, m string, r SharingCall) (json.RawMessage, error) {
	return sharingServiceCall(c, s.sharingCaller, "smb", m, r)
}
func (s SSHService) Call(c context.Context, m string, r SharingCall) (json.RawMessage, error) {
	return sharingServiceCall(c, s.sharingCaller, "ssh", m, r)
}

type SMBShareEntry struct {
	ID                          int      `json:"id"`
	Name                        string   `json:"name"`
	Path                        string   `json:"path"`
	Purpose                     string   `json:"purpose"`
	Comment                     string   `json:"comment"`
	Enabled                     bool     `json:"enabled"`
	Locked                      bool     `json:"locked"`
	ReadOnly                    bool     `json:"readonly"`
	LegacyReadOnly              bool     `json:"ro"`
	Browsable                   bool     `json:"browsable"`
	AccessBasedShareEnumeration bool     `json:"access_based_share_enumeration"`
	RecycleBin                  bool     `json:"recyclebin"`
	PathSuffix                  *string  `json:"path_suffix"`
	HostsAllow                  []string `json:"hostsallow"`
	HostsDeny                   []string `json:"hostsdeny"`
	Home                        bool     `json:"home"`
}
type NFSShareEntry struct {
	ID              int      `json:"id"`
	Path            string   `json:"path"`
	LegacyPaths     []string `json:"paths"`
	Comment         string   `json:"comment"`
	Enabled         bool     `json:"enabled"`
	ReadOnly        bool     `json:"ro"`
	MapRootUser     *string  `json:"maproot_user"`
	MapRootGroup    *string  `json:"maproot_group"`
	MapAllUser      *string  `json:"mapall_user"`
	MapAllGroup     *string  `json:"mapall_group"`
	Networks        []string `json:"networks"`
	Hosts           []string `json:"hosts"`
	Security        []string `json:"security"`
	ExposeSnapshots bool     `json:"expose_snapshots"`
}
type RsyncTaskEntry struct {
	ID                  int                 `json:"id"`
	Path                string              `json:"path"`
	User                string              `json:"user"`
	Mode                string              `json:"mode"`
	RemoteHost          string              `json:"remotehost"`
	RemotePort          *int                `json:"remoteport"`
	RemoteModule        string              `json:"remotemodule"`
	RemotePath          string              `json:"remotepath"`
	SSHCredentials      *RsyncSSHCredential `json:"ssh_credentials"`
	Direction           string              `json:"direction"`
	Description         string              `json:"desc"`
	Schedule            RsyncSchedule       `json:"schedule"`
	Recursive           bool                `json:"recursive"`
	Times               bool                `json:"times"`
	Compress            bool                `json:"compress"`
	Archive             bool                `json:"archive"`
	Delete              bool                `json:"delete"`
	Quiet               bool                `json:"quiet"`
	PreservePermissions bool                `json:"preserveperm"`
	PreserveAttributes  bool                `json:"preserveattr"`
	DelayUpdates        bool                `json:"delayupdates"`
	Extra               []string            `json:"extra"`
	Enabled             bool                `json:"enabled"`
	ValidateRemotePath  bool                `json:"validate_rpath"`
	SSHKeyScan          bool                `json:"ssh_keyscan"`
}
type RsyncSSHCredential struct {
	ID int `json:"id"`
}
type RsyncSchedule struct {
	Minute     string `json:"minute"`
	Hour       string `json:"hour"`
	DayOfMonth string `json:"dom"`
	Month      string `json:"month"`
	DayOfWeek  string `json:"dow"`
}

func (s SMBShareService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]SMBShareEntry, error) {
	return Query[SMBShareEntry](ctx, s.client, "sharing.smb.query", f, o)
}
func (s NFSShareService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]NFSShareEntry, error) {
	return Query[NFSShareEntry](ctx, s.client, "sharing.nfs.query", f, o)
}
func (s RsyncTaskService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]RsyncTaskEntry, error) {
	return Query[RsyncTaskEntry](ctx, s.client, "rsynctask.query", f, o)
}

//go:generate go run ./cmd/gensharing -doc ../../docs/truenas-api-25.10/domains/sharing.md -out sharing_generated.go -checklist ../../docs/truenas-api-25.10/sharing-implementation.md
