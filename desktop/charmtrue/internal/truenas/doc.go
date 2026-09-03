// Package truenas provides the persistent JSON-RPC 2.0 WebSocket transport
// used by the TrueNAS 25.10 API.
//
// Callers should keep one authenticated Client for the lifetime of a managed
// system. The client reconnects, reauthenticates, and restores subscriptions;
// it also exposes typed query, job, notification, and method-risk helpers.
// Domain-specific services can be built on Client.Call as their API surface is
// introduced.
package truenas
