package session

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

func TestRegisterIsExclusiveAndRefreshesActivity(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	r := NewRegistry(dir, filepath.Join(dir, "worktree"), Identity{Agent: "a", Session: "s1"})
	r.Now = func() time.Time { return now }
	first, err := r.Register(false)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	second, err := r.Register(false)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Since.Equal(first.Since.Time) || !second.LastActivity.After(first.LastActivity.Time) {
		t.Fatalf("refresh = %#v", second)
	}

	other := NewRegistry(dir, r.Worktree, Identity{Agent: "b", Session: "s2"})
	other.Now = r.Now
	if _, err := other.Register(false); err == nil {
		t.Fatal("live owner collision succeeded")
	} else {
		var conflict *WorktreeBusyError
		if !errors.As(err, &conflict) {
			t.Fatalf("error = %T %v", err, err)
		}
	}
	if _, err := other.Register(true); err != nil {
		t.Fatalf("allow shared: %v", err)
	}
}

func TestConcurrentRegisterExactlyOneWinner(t *testing.T) {
	dir := t.TempDir()
	worktree := filepath.Join(dir, "wt")
	const n = 16
	start := make(chan struct{})
	results := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r := NewRegistry(dir, worktree, Identity{Agent: string(rune('a' + i)), Session: string(rune('A' + i))})
			_, err := r.Register(false)
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d, want 1", wins)
	}
}

func TestAliveDecisionOrderAndTTL(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	r := &Registry{TTL: 8 * time.Hour, Now: func() time.Time { return now }, AlivePID: func(pid int, start string) bool { return pid == 7 && start == "same" }}
	base := model.Lease{Agent: "a", Session: "s", LastActivity: model.NewTimestamp(now.Add(-7 * time.Hour))}
	if !r.Alive(base) {
		t.Fatal("within TTL considered dead")
	}
	base.LastActivity = model.NewTimestamp(now.Add(-9 * time.Hour))
	if r.Alive(base) {
		t.Fatal("expired considered alive")
	}
	pid, start := 7, "same"
	base.HookPID = &pid
	base.HookStarttime = &start
	if !r.Alive(base) {
		t.Fatal("live hook lost to expired TTL")
	}
	start = "different"
	base.HookStarttime = &start
	base.LastActivity = model.NewTimestamp(now)
	if r.Alive(base) {
		t.Fatal("dead hook lost to fresh TTL")
	}
	if r.Alive(model.Lease{}) {
		t.Fatal("corrupt record alive")
	}
}

func TestCorruptRecordIsDead(t *testing.T) {
	r := NewRegistry(t.TempDir(), t.TempDir(), Identity{})
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.agentPath("a"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if leases := r.List(); len(leases) != 0 {
		t.Fatalf("corrupt record listed: %v", leases)
	}
}

func TestActivityTTLFromEnvironment(t *testing.T) {
	t.Setenv("PPWK_ACTIVITY_TTL", "90m")
	r := NewRegistry(t.TempDir(), t.TempDir(), Identity{})
	if r.TTL != 90*time.Minute {
		t.Fatalf("TTL=%s", r.TTL)
	}
}

func TestCanonicalWorktreeResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(dir, link, Identity{})
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if r.Worktree != want {
		t.Fatalf("worktree=%q want=%q", r.Worktree, want)
	}
}

// leaseAt 은 마지막 활동 시각만 다른 정상 기록을 만든다.
func leaseAt(when time.Time) model.Lease {
	return model.Lease{Agent: "a", Session: "s", LastActivity: model.NewTimestamp(when)}
}
