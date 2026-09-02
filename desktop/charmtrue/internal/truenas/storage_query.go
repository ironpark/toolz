package truenas

import "context"

func storageQueryEntries[T any](ctx context.Context, c *Client, method string, filters []Filter, options QueryOptions) ([]T, error) {
	options.Get, options.Count = false, false
	return Query[T](ctx, c, method, filters, options)
}
func storageQueryOne[T any](ctx context.Context, c *Client, method string, filters []Filter, options QueryOptions) (T, error) {
	var result T
	options.Get, options.Count = true, false
	if filters == nil {
		filters = []Filter{}
	}
	err := c.Call(ctx, method, []any{filters, options}, &result)
	return result, err
}
func storageQueryCount(ctx context.Context, c *Client, method string, filters []Filter, options QueryOptions) (int, error) {
	options.Get, options.Count = false, true
	if filters == nil {
		filters = []Filter{}
	}
	var result int
	err := c.Call(ctx, method, []any{filters, options}, &result)
	return result, err
}

func (s DiskService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]DiskEntry, error) {
	return storageQueryEntries[DiskEntry](ctx, s.client, "disk.query", f, o)
}
func (s DiskService) GetEntry(ctx context.Context, f []Filter, o QueryOptions) (DiskEntry, error) {
	return storageQueryOne[DiskEntry](ctx, s.client, "disk.query", f, o)
}
func (s DiskService) Count(ctx context.Context, f []Filter, o QueryOptions) (int, error) {
	return storageQueryCount(ctx, s.client, "disk.query", f, o)
}
func (s EnclosureService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]EnclosureEntry, error) {
	return storageQueryEntries[EnclosureEntry](ctx, s.client, "enclosure2.query", f, o)
}
func (s PoolService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]PoolEntry, error) {
	return storageQueryEntries[PoolEntry](ctx, s.client, "pool.query", f, o)
}
func (s PoolService) GetEntry(ctx context.Context, f []Filter, o QueryOptions) (PoolEntry, error) {
	return storageQueryOne[PoolEntry](ctx, s.client, "pool.query", f, o)
}
func (s PoolService) Count(ctx context.Context, f []Filter, o QueryOptions) (int, error) {
	return storageQueryCount(ctx, s.client, "pool.query", f, o)
}
func (s DatasetService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]DatasetEntry, error) {
	return storageQueryEntries[DatasetEntry](ctx, s.client, "pool.dataset.query", f, o)
}
func (s DatasetService) GetEntry(ctx context.Context, f []Filter, o QueryOptions) (DatasetEntry, error) {
	return storageQueryOne[DatasetEntry](ctx, s.client, "pool.dataset.query", f, o)
}
func (s DatasetService) Count(ctx context.Context, f []Filter, o QueryOptions) (int, error) {
	return storageQueryCount(ctx, s.client, "pool.dataset.query", f, o)
}
func (s ScrubService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]ScrubEntry, error) {
	return storageQueryEntries[ScrubEntry](ctx, s.client, "pool.scrub.query", f, o)
}
func (s SnapshotService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]SnapshotEntry, error) {
	return storageQueryEntries[SnapshotEntry](ctx, s.client, "pool.snapshot.query", f, o)
}
func (s SnapshotService) GetEntry(ctx context.Context, f []Filter, o QueryOptions) (SnapshotEntry, error) {
	return storageQueryOne[SnapshotEntry](ctx, s.client, "pool.snapshot.query", f, o)
}
func (s SnapshotService) Count(ctx context.Context, f []Filter, o QueryOptions) (int, error) {
	return storageQueryCount(ctx, s.client, "pool.snapshot.query", f, o)
}
func (s SnapshotTaskService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]SnapshotTaskEntry, error) {
	return storageQueryEntries[SnapshotTaskEntry](ctx, s.client, "pool.snapshottask.query", f, o)
}
func (s ACLTemplateService) QueryEntries(ctx context.Context, f []Filter, o QueryOptions) ([]ACLTemplateEntry, error) {
	return storageQueryEntries[ACLTemplateEntry](ctx, s.client, "filesystem.acltemplate.query", f, o)
}
