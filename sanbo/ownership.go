package main

import "sync"

// ownershipTable is the process-local cluster registry. Multiple Relay
// instances use it exactly as separately addressed relay nodes use their
// distributed registry: server IDs have one owner and expose only an opaque
// reroute target to other nodes.
var ownershipTable = struct {
	sync.Mutex
	owners map[string]ownershipRecord
}{owners: make(map[string]ownershipRecord)}

type ownershipRecord struct {
	relay  *Relay
	target string
}

func lookupOwner(serverID string) (ownershipRecord, bool) {
	ownershipTable.Lock()
	defer ownershipTable.Unlock()
	record, ok := ownershipTable.owners[serverID]
	return record, ok
}

func claimOwner(serverID string, relay *Relay) ownershipRecord {
	ownershipTable.Lock()
	defer ownershipTable.Unlock()
	if record, ok := ownershipTable.owners[serverID]; ok {
		return record
	}
	record := ownershipRecord{relay: relay, target: relay.Config.OwnershipTarget}
	ownershipTable.owners[serverID] = record
	return record
}

func releaseOwner(serverID string, relay *Relay) {
	ownershipTable.Lock()
	defer ownershipTable.Unlock()
	if record, ok := ownershipTable.owners[serverID]; ok && record.relay == relay {
		delete(ownershipTable.owners, serverID)
	}
}
