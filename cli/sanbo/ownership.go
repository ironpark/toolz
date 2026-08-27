package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	clusterHeartbeatInterval = 100 * time.Millisecond
	clusterLeaseDuration     = 750 * time.Millisecond
)

type ownershipRecord struct {
	relay   *Relay
	ownerID string
	target  string
}

func (record ownershipRecord) ownedBy(relay *Relay) bool {
	return record.relay == relay || (record.ownerID != "" && record.ownerID == relay.ownership.identity())
}

type ownershipCoordinator interface {
	identity() string
	lookup(serverID string) (ownershipRecord, bool, error)
	claim(serverID string, relay *Relay) (ownershipRecord, bool, error)
	release(serverID string, relay *Relay) error
	ownedServers() (map[string]bool, error)
	members() (int, error)
	close() error
}

// ownershipTable is the local/default backend. It is deliberately process-local;
// the file backend below is selected only when all host-cluster settings are
// present.
var ownershipTable = struct {
	sync.Mutex
	owners map[string]ownershipRecord
}{owners: make(map[string]ownershipRecord)}

type localOwnershipCoordinator struct{}

func (*localOwnershipCoordinator) identity() string { return "" }

func (*localOwnershipCoordinator) lookup(serverID string) (ownershipRecord, bool, error) {
	ownershipTable.Lock()
	defer ownershipTable.Unlock()
	record, ok := ownershipTable.owners[serverID]
	return record, ok, nil
}

func (*localOwnershipCoordinator) claim(serverID string, relay *Relay) (ownershipRecord, bool, error) {
	ownershipTable.Lock()
	defer ownershipTable.Unlock()
	if record, ok := ownershipTable.owners[serverID]; ok {
		return record, false, nil
	}
	record := ownershipRecord{relay: relay, target: relay.Config.OwnershipTarget}
	ownershipTable.owners[serverID] = record
	return record, true, nil
}

func (*localOwnershipCoordinator) release(serverID string, relay *Relay) error {
	ownershipTable.Lock()
	defer ownershipTable.Unlock()
	if record, ok := ownershipTable.owners[serverID]; ok && record.relay == relay {
		delete(ownershipTable.owners, serverID)
	}
	return nil
}

// ownedServers is only consulted by the cluster reconciler, which does not run
// for the in-process backend.
func (*localOwnershipCoordinator) ownedServers() (map[string]bool, error) { return nil, nil }
func (*localOwnershipCoordinator) members() (int, error)                  { return 1, nil }
func (*localOwnershipCoordinator) close() error                           { return nil }

var localOwnership ownershipCoordinator = &localOwnershipCoordinator{}

type clusterMember struct {
	ID        string `json:"id"`
	Heartbeat int64  `json:"heartbeat"`
}

type clusterOwner struct {
	ServerID string `json:"server_id"`
	MemberID string `json:"member_id"`
	Target   string `json:"target"`
}

// fileOwnershipCoordinator is a same-host lease registry. File locking makes
// claims atomic across independent OS processes, while heartbeat leases make
// membership and ownership recover after an ungraceful process exit. It is not
// a network discovery or cross-host clustering backend.
type fileOwnershipCoordinator struct {
	dir       string
	memberID  string
	target    string
	stopBeat  func()
	closed    atomic.Bool
	liveCount atomic.Int64
	lockMu    sync.Mutex
	lockFile  *os.File
	lastSweep time.Time
}

func newOwnershipCoordinator(config Config) (ownershipCoordinator, error) {
	if config.ClusterQuery == "" || config.NodeName == "" || config.Cookie == "" {
		return localOwnership, nil
	}

	clusterKey := sha256.Sum256([]byte(config.ClusterQuery + "\x00" + config.Cookie))
	dir := filepath.Join(os.TempDir(), "sanbo-clusters-v1", hex.EncodeToString(clusterKey[:]))
	if err := os.MkdirAll(filepath.Join(dir, "members"), 0o700); err != nil {
		return nil, fmt.Errorf("create cluster member store: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "owners"), 0o700); err != nil {
		return nil, fmt.Errorf("create cluster owner store: %w", err)
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, fmt.Errorf("create cluster member identity: %w", err)
	}
	lockFile, err := os.OpenFile(filepath.Join(dir, "cluster.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open cluster lock: %w", err)
	}
	memberID := config.NodeName + ":" + hex.EncodeToString(token)
	coordinator := &fileOwnershipCoordinator{
		dir:      dir,
		memberID: memberID,
		target:   config.OwnershipTarget,
		lockFile: lockFile,
	}
	if err := coordinator.heartbeat(); err != nil {
		lockFile.Close()
		return nil, err
	}
	coordinator.stopBeat = startTicker(clusterHeartbeatInterval, func() { _ = coordinator.heartbeat() })
	return coordinator, nil
}

func digestName(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (coordinator *fileOwnershipCoordinator) identity() string { return coordinator.memberID }

func (coordinator *fileOwnershipCoordinator) heartbeat() error {
	return coordinator.withLock(func() error {
		now := time.Now()
		member := clusterMember{ID: coordinator.memberID, Heartbeat: now.UnixNano()}
		if err := writeJSONFile(coordinator.memberFile(coordinator.memberID), member); err != nil {
			return err
		}
		if now.Sub(coordinator.lastSweep) >= clusterLeaseDuration {
			if _, err := coordinator.sweepMembersLocked(now); err != nil {
				return err
			}
			coordinator.lastSweep = now
		}
		return nil
	})
}

func (coordinator *fileOwnershipCoordinator) lookup(serverID string) (ownershipRecord, bool, error) {
	var result ownershipRecord
	var found bool
	err := coordinator.withLock(func() error {
		owner, ok, err := coordinator.liveOwnerLocked(serverID)
		if err != nil || !ok {
			return err
		}
		result = ownershipRecord{ownerID: owner.MemberID, target: owner.Target}
		found = true
		return nil
	})
	return result, found, err
}

func (coordinator *fileOwnershipCoordinator) claim(serverID string, _ *Relay) (ownershipRecord, bool, error) {
	var result ownershipRecord
	var acquired bool
	err := coordinator.withLock(func() error {
		owner, ok, err := coordinator.liveOwnerLocked(serverID)
		if err != nil {
			return err
		}
		if ok {
			result = ownershipRecord{ownerID: owner.MemberID, target: owner.Target}
			return nil
		}
		owner = clusterOwner{ServerID: serverID, MemberID: coordinator.memberID, Target: coordinator.target}
		if err := writeJSONFile(coordinator.ownerFile(serverID), owner); err != nil {
			return err
		}
		result = ownershipRecord{ownerID: coordinator.memberID, target: coordinator.target}
		acquired = true
		return nil
	})
	return result, acquired, err
}

func (coordinator *fileOwnershipCoordinator) release(serverID string, _ *Relay) error {
	return coordinator.withLock(func() error {
		owner, ok, err := coordinator.readOwnerLocked(serverID)
		if err != nil || !ok || owner.MemberID != coordinator.memberID {
			return err
		}
		err = os.Remove(coordinator.ownerFile(serverID))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

// members reports the membership observed by the most recent heartbeat, so
// readiness probes and connection admission never touch the cluster lock.
func (coordinator *fileOwnershipCoordinator) members() (int, error) {
	return int(coordinator.liveCount.Load()), nil
}

// ownedServers answers the reconciler's question for every session in one
// locked pass instead of one lookup per session.
func (coordinator *fileOwnershipCoordinator) ownedServers() (map[string]bool, error) {
	owned := make(map[string]bool)
	err := coordinator.withLock(func() error {
		return coordinator.eachOwnedLocked(func(_ string, owner clusterOwner) {
			owned[owner.ServerID] = true
		})
	})
	return owned, err
}

func (coordinator *fileOwnershipCoordinator) close() error {
	if !coordinator.closed.CompareAndSwap(false, true) {
		return nil
	}
	coordinator.stopBeat()
	defer coordinator.lockFile.Close()
	return coordinator.withLock(func() error {
		if err := os.Remove(coordinator.memberFile(coordinator.memberID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return coordinator.eachOwnedLocked(func(path string, _ clusterOwner) {
			_ = os.Remove(path)
		})
	})
}

func (coordinator *fileOwnershipCoordinator) ownerFile(serverID string) string {
	return filepath.Join(coordinator.dir, "owners", digestName(serverID)+".json")
}

func (coordinator *fileOwnershipCoordinator) memberFile(memberID string) string {
	return filepath.Join(coordinator.dir, "members", digestName(memberID)+".json")
}

func (coordinator *fileOwnershipCoordinator) readOwnerLocked(serverID string) (clusterOwner, bool, error) {
	var owner clusterOwner
	path := coordinator.ownerFile(serverID)
	err := readJSONFile(path, &owner)
	if errors.Is(err, os.ErrNotExist) {
		return clusterOwner{}, false, nil
	}
	if err != nil {
		if invalidJSONRecord(err) {
			_ = os.Remove(path)
			return clusterOwner{}, false, nil
		}
		return clusterOwner{}, false, err
	}
	if owner.ServerID != serverID {
		_ = os.Remove(path)
		return clusterOwner{}, false, nil
	}
	return owner, true, nil
}

// eachOwnedLocked visits every owner record held by this member.
func (coordinator *fileOwnershipCoordinator) eachOwnedLocked(visit func(path string, owner clusterOwner)) error {
	entries, err := os.ReadDir(filepath.Join(coordinator.dir, "owners"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(coordinator.dir, "owners", entry.Name())
		var owner clusterOwner
		if err := readJSONFile(path, &owner); err != nil {
			if errors.Is(err, os.ErrNotExist) || invalidJSONRecord(err) {
				if invalidJSONRecord(err) {
					_ = os.Remove(path)
				}
				continue
			}
			return err
		}
		if owner.MemberID == coordinator.memberID {
			visit(path, owner)
		}
	}
	return nil
}

// liveOwnerLocked returns the current owner of serverID, evicting the record
// first if the owning member's lease has expired. This is the single place the
// lease-expiry rule is applied.
func (coordinator *fileOwnershipCoordinator) liveOwnerLocked(serverID string) (clusterOwner, bool, error) {
	owner, ok, err := coordinator.readOwnerLocked(serverID)
	if err != nil || !ok {
		return clusterOwner{}, false, err
	}
	alive, err := coordinator.memberAliveLocked(owner.MemberID, time.Now())
	if err != nil {
		return clusterOwner{}, false, err
	}
	if alive {
		return owner, true, nil
	}
	if err := os.Remove(coordinator.ownerFile(serverID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return clusterOwner{}, false, err
	}
	return clusterOwner{}, false, nil
}

// memberAliveLocked reads the one member file it needs. The heartbeat sweep is
// what garbage-collects expired members, so admission does not pay for a full
// directory scan per connection.
func (coordinator *fileOwnershipCoordinator) memberAliveLocked(memberID string, now time.Time) (bool, error) {
	var member clusterMember
	err := readJSONFile(coordinator.memberFile(memberID), &member)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		if invalidJSONRecord(err) {
			_ = os.Remove(coordinator.memberFile(memberID))
			return false, nil
		}
		return false, err
	}
	return member.ID == memberID && now.Sub(time.Unix(0, member.Heartbeat)) <= clusterLeaseDuration, nil
}

// sweepMembersLocked reads every member file, drops expired ones, and publishes
// the resulting count for lock-free readiness checks.
func (coordinator *fileOwnershipCoordinator) sweepMembersLocked(now time.Time) (int, error) {
	entries, err := os.ReadDir(filepath.Join(coordinator.dir, "members"))
	if err != nil {
		return 0, err
	}
	live := 0
	for _, entry := range entries {
		path := filepath.Join(coordinator.dir, "members", entry.Name())
		var member clusterMember
		if err := readJSONFile(path, &member); err != nil {
			if errors.Is(err, os.ErrNotExist) || invalidJSONRecord(err) {
				if invalidJSONRecord(err) {
					_ = os.Remove(path)
				}
				continue
			}
			return 0, err
		}
		if member.ID == "" || now.Sub(time.Unix(0, member.Heartbeat)) > clusterLeaseDuration {
			_ = os.Remove(path)
			continue
		}
		live++
	}
	coordinator.liveCount.Store(int64(live))
	return live, nil
}

func (coordinator *fileOwnershipCoordinator) withLock(action func() error) error {
	coordinator.lockMu.Lock()
	defer coordinator.lockMu.Unlock()
	fd := int(coordinator.lockFile.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }()
	return action()
}

func writeJSONFile(path string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func readJSONFile(path string, value any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, value)
}

func invalidJSONRecord(err error) bool {
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	return errors.As(err, &syntaxError) || errors.As(err, &typeError)
}
