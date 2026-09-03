package session

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// T4.8/T4.9 — 살아 있는 다른 세션이 쥔 worktree 는 거부하고,
// allowShared 는 그 거부를 우회한다. board 계층 확인은 T4.8 쪽에 있다.
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

// T4.1c — 잠금 파일 read-modify-write 가 원자적이다.
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

// T4.15/T4.16 — 생존 판정 5단계 순서와 last_activity 임계값 (§3.6 표).
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

// T4.17 — 손상된 JSON 기록은 사망으로 보고 panic 하지 않는다.
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

// T4.13 — PPWK_ACTIVITY_TTL 로 임계값을 조정할 수 있다.
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

// T4.2 — flock 은 JSON 기록을 바꾸는 순간에만 잡힌다 (§3.6).
//
// 세션 수명 동안 잠금을 쥐면 멈춘 프로세스가 worktree 를 영구히 붙잡는다
// (T4.1b 가 막는 것과 같은 실패다). 양쪽을 다 본다 — 갱신 중에는 실제로
// 배타이고, 반환된 뒤에는 남아 있지 않다.
func TestLockHeldOnlyDuringUpdate(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir, filepath.Join(dir, "wt"), Identity{Agent: "a", Session: "s1"})
	if _, err := r.Register(false); err != nil {
		t.Fatal(err)
	}

	// 1. 반환된 뒤에는 잠금이 남아 있지 않다.
	f, err := os.OpenFile(r.worktreePath(), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("Register 가 반환된 뒤에도 잠금이 남아 있다: %v", err)
	}

	// 2. 그 구간에서는 배타다. 잠금을 쥔 채로는 Register 가 진행하지 못한다.
	done := make(chan error, 1)
	go func() { _, err := r.Register(false); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("잠금을 쥐고 있는데 Register 가 끝났다: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("잠금을 푼 뒤 Register = %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("잠금을 풀었는데 Register 가 끝나지 않았다")
	}
}
