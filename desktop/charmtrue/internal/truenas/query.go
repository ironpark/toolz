package truenas

import "context"

// Filter is one TrueNAS query predicate, for example ["name", "=", "tank"].
// Logical expressions can be represented directly with []any.
type Filter []any

// QueryOptions mirrors the common options accepted by *.query methods.
type QueryOptions struct {
	Count           bool     `json:"count,omitempty"`
	Get             bool     `json:"get,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Offset          int      `json:"offset,omitempty"`
	Select          []string `json:"select,omitempty"`
	OrderBy         []string `json:"order_by,omitempty"`
	ForceSQLFilters bool     `json:"force_sql_filters,omitempty"`
	Extra           any      `json:"extra,omitempty"`
}

// Query invokes a typed *.query method using the standard [filters, options]
// positional tuple. Use Client.Call for get/count query result shapes.
func Query[T any](ctx context.Context, client *Client, method string, filters []Filter, options QueryOptions) ([]T, error) {
	if client == nil {
		return nil, &ValidationError{Field: "client", Message: "is required"}
	}
	if filters == nil {
		filters = []Filter{}
	}
	params := []any{filters, options}
	var result []T
	if err := client.Call(ctx, method, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}
