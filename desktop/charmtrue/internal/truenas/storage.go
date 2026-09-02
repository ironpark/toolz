package truenas

import (
	"context"
	"encoding/json"
)

// StorageService groups the TrueNAS 25.10 storage namespaces. Services are
// lightweight handles and are safe to retain for the lifetime of Client.
type StorageService struct {
	Devices       DeviceService
	Disks         DiskService
	Enclosures    EnclosureService
	Filesystems   FilesystemService
	ACLTemplates  ACLTemplateService
	Pools         PoolService
	Datasets      DatasetService
	Resilver      ResilverService
	Scrubs        ScrubService
	Snapshots     SnapshotService
	SnapshotTasks SnapshotTaskService
	ZFSResources  ZFSResourceService
}

type storageCaller struct{ client *Client }

func (c storageCaller) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	var result json.RawMessage
	if err := c.client.Call(ctx, method, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Storage returns the storage-domain API rooted at this client.
func (c *Client) Storage() StorageService {
	b := storageCaller{client: c}
	return StorageService{DeviceService{b}, DiskService{b}, EnclosureService{b}, FilesystemService{b}, ACLTemplateService{b}, PoolService{b}, DatasetService{b}, ResilverService{b}, ScrubService{b}, SnapshotService{b}, SnapshotTaskService{b}, ZFSResourceService{b}}
}

// StorageCall is the lossless request shape used by endpoints whose official
// schema intentionally permits heterogeneous tuples or free-form objects.
type StorageCall struct{ Params []any }

type DeviceService struct{ storageCaller }
type DiskService struct{ storageCaller }
type EnclosureService struct{ storageCaller }
type FilesystemService struct{ storageCaller }
type ACLTemplateService struct{ storageCaller }
type PoolService struct{ storageCaller }
type DatasetService struct{ storageCaller }
type ResilverService struct{ storageCaller }
type ScrubService struct{ storageCaller }
type SnapshotService struct{ storageCaller }
type SnapshotTaskService struct{ storageCaller }
type ZFSResourceService struct{ storageCaller }

// Call invokes a method belonging to the service after validating it against
// the pinned storage manifest. Prefer generated named helpers for common CRUD.
func storageServiceCall(ctx context.Context, c storageCaller, namespace, method string, request StorageCall) (json.RawMessage, error) {
	full := namespace + "." + method
	meta, ok := StorageMethodByName(full)
	if !ok {
		return nil, &ValidationError{Field: "method", Message: "is not a TrueNAS 25.10 storage method"}
	}
	_ = meta
	return c.call(ctx, full, request.Params)
}

func (s DeviceService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "device", method, r)
}
func (s DiskService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "disk", method, r)
}
func (s EnclosureService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	if method == "label.set" {
		return storageServiceCall(ctx, s.storageCaller, "enclosure", method, r)
	}
	return storageServiceCall(ctx, s.storageCaller, "enclosure2", method, r)
}
func (s FilesystemService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "filesystem", method, r)
}
func (s ACLTemplateService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "filesystem.acltemplate", method, r)
}
func (s PoolService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "pool", method, r)
}
func (s DatasetService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "pool.dataset", method, r)
}
func (s ResilverService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "pool.resilver", method, r)
}
func (s ScrubService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "pool.scrub", method, r)
}
func (s SnapshotService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "pool.snapshot", method, r)
}
func (s SnapshotTaskService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "pool.snapshottask", method, r)
}
func (s ZFSResourceService) Call(ctx context.Context, method string, r StorageCall) (json.RawMessage, error) {
	return storageServiceCall(ctx, s.storageCaller, "zfs.resource", method, r)
}

// DecodeStorageResult decodes a lossless storage response into its generated
// or application-specific type.
func DecodeStorageResult[T any](raw json.RawMessage) (T, error) {
	var result T
	err := json.Unmarshal(raw, &result)
	return result, err
}
