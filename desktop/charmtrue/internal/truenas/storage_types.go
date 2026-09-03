package truenas

import "encoding/json"

// The following stable entry types cover the fields shared by the 25.10.5
// schemas. RawProperties preserves vendor additions without discarding data.
type DiskEntry struct {
	Identifier    string                     `json:"identifier"`
	Name          string                     `json:"name"`
	Serial        string                     `json:"serial"`
	Model         *string                    `json:"model"`
	Size          *uint64                    `json:"size"`
	Type          string                     `json:"type"`
	Pool          *string                    `json:"pool"`
	RotationRate  *int                       `json:"rotationrate"`
	Temperature   *int                       `json:"temperature"`
	RawProperties map[string]json.RawMessage `json:"-"`
}

type PoolEntry struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	Guid      string       `json:"guid"`
	Status    string       `json:"status"`
	Healthy   bool         `json:"healthy"`
	Path      string       `json:"path"`
	Size      uint64       `json:"size"`
	Allocated uint64       `json:"allocated"`
	Free      uint64       `json:"free"`
	Topology  PoolTopology `json:"topology"`
}

type PoolTopology struct {
	Data    []PoolVDev `json:"data,omitempty"`
	Log     []PoolVDev `json:"log,omitempty"`
	Cache   []PoolVDev `json:"cache,omitempty"`
	Spare   []PoolVDev `json:"spare,omitempty"`
	Special []PoolVDev `json:"special,omitempty"`
	Dedup   []PoolVDev `json:"dedup,omitempty"`
}
type PoolVDev struct {
	Name, Type, Status, Guid string
	Stats                    map[string]json.RawMessage `json:"stats,omitempty"`
	Children                 []PoolVDev                 `json:"children,omitempty"`
}

type DatasetEntry struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name"`
	Pool       string                     `json:"pool"`
	Type       string                     `json:"type"`
	Mountpoint *string                    `json:"mountpoint"`
	Encrypted  bool                       `json:"encrypted"`
	Locked     bool                       `json:"locked"`
	Children   []DatasetEntry             `json:"children"`
	Properties map[string]DatasetProperty `json:"-"`
}
type DatasetProperty struct {
	Value    any    `json:"value"`
	RawValue any    `json:"rawvalue"`
	Source   string `json:"source"`
	Parsed   any    `json:"parsed"`
}
type SnapshotEntry struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Dataset      string           `json:"dataset"`
	SnapshotName string           `json:"snapshot_name"`
	Used         uint64           `json:"used"`
	Referenced   uint64           `json:"referenced"`
	Holds        map[string]int64 `json:"holds"`
}
type ScrubEntry struct {
	ID          int            `json:"id"`
	Pool        int            `json:"pool"`
	Threshold   int            `json:"threshold"`
	Description string         `json:"description"`
	Schedule    map[string]any `json:"schedule"`
	Enabled     bool           `json:"enabled"`
}
type SnapshotTaskEntry struct {
	ID            int    `json:"id"`
	Dataset       string `json:"dataset"`
	Recursive     bool   `json:"recursive"`
	Enabled       bool   `json:"enabled"`
	LifetimeValue int    `json:"lifetime_value"`
	LifetimeUnit  string `json:"lifetime_unit"`
	NamingSchema  string `json:"naming_schema"`
}
type EnclosureEntry struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Model      string             `json:"model"`
	Controller string             `json:"controller"`
	Elements   []EnclosureElement `json:"elements"`
}
type EnclosureElement struct {
	Slot   int     `json:"slot"`
	Dev    *string `json:"dev"`
	Status string  `json:"status"`
	Value  string  `json:"value"`
}
type FilesystemEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	RealPath string `json:"realpath"`
	Type     string `json:"type"`
	Size     uint64 `json:"size"`
	Mode     int    `json:"mode"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	ACL      bool   `json:"acl"`
}
type ACLTemplateEntry struct {
	ID      int        `json:"id"`
	Name    string     `json:"name"`
	Comment string     `json:"comment"`
	ACLType string     `json:"acltype"`
	ACL     []ACLEntry `json:"acl"`
}
type ACLEntry struct {
	Tag   string          `json:"tag"`
	Type  string          `json:"type"`
	ID    *int            `json:"id"`
	Who   *string         `json:"who"`
	Perms map[string]bool `json:"perms"`
	Flags map[string]bool `json:"flags"`
}
type ResilverEntry struct {
	Begin   string `json:"begin"`
	End     string `json:"end"`
	Enabled bool   `json:"enabled"`
	Weekday []int  `json:"weekday"`
}
type ZFSResourceEntry struct {
	ID         string                     `json:"id"`
	Type       string                     `json:"type"`
	Properties map[string]DatasetProperty `json:"properties"`
}
