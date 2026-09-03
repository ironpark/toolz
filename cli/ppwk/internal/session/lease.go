package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

const DefaultActivityTTL = 8 * time.Hour

// WorktreeBusyError 는 살아 있는 다른 세션이 이미 이 worktree 를 쥐고 있다는
// 뜻이다. board.ConflictError(CAS 경쟁)와는 다른 사건이므로 이름을 나눈다.
type WorktreeBusyError struct{ Lease model.Lease }

func (e *WorktreeBusyError) Error() string {
	return fmt.Sprintf("worktree %s is in use by %s (session %s)", e.Lease.Worktree, e.Lease.Agent, short(e.Lease.Session))
}

// Registry stores machine-local liveness records below the shared git directory.
type Registry struct {
	Dir      string
	Identity Identity
	Worktree string
	TTL      time.Duration
	Now      func() time.Time
	AlivePID func(pid int, starttime string) bool
}

func NewRegistry(commonDir, worktree string, identity Identity) *Registry {
	ttl := DefaultActivityTTL
	if raw := os.Getenv("PPWK_ACTIVITY_TTL"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			ttl = parsed
		}
	}
	return &Registry{Dir: filepath.Join(commonDir, "ppwk", "locks"), Identity: identity,
		Worktree: canonical(worktree), TTL: ttl, Now: time.Now, AlivePID: processAlive}
}

// Register atomically acquires the worktree for this session and updates activity.
// The advisory lock is held only while the JSON record is replaced.
func (r *Registry) Register(allowShared bool) (model.Lease, error) {
	return r.register(allowShared, nil)
}

// RegisterHook 은 Register 에 hook_pid 기록을 더한다 (§3.8 층 3).
//
// 훅의 부모는 구조적으로 도구 프로세스다. 그래서 프로세스 트리를 뒤지지 않고
// $PPID 하나만 본다 — 임의 위치에서 거슬러 올라가는 것(D10 기각)과는 다르다.
func (r *Registry) RegisterHook(pid int, allowShared bool) (model.Lease, error) {
	return r.register(allowShared, &pid)
}

func (r *Registry) register(allowShared bool, hookPID *int) (model.Lease, error) {
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return model.Lease{}, fmt.Errorf("잠금 디렉터리 생성: %w", err)
	}
	var lease model.Lease
	err := r.withLocked(r.worktreePath(), func(f *os.File) error {
		previous, readErr := readLeaseFile(f)
		same := readErr == nil && previous.Agent == r.Identity.Agent && previous.Session == r.Identity.Session
		if readErr == nil && previous.Session != r.Identity.Session && r.Alive(previous) && !allowShared {
			return &WorktreeBusyError{Lease: previous}
		}
		now := model.NewTimestamp(r.Now())
		since := now
		if same {
			since = previous.Since
		}
		lease = model.Lease{Agent: r.Identity.Agent, Session: r.Identity.Session, Worktree: r.Worktree,
			Since: since, LastActivity: now}
		switch {
		case hookPID != nil:
			lease.HookPID = hookPID
			if start := ProcessStarttime(*hookPID); start != "" {
				lease.HookStarttime = &start
			}
		case same:
			// 훅이 남긴 hook_pid 를 평범한 명령이 지우면 안 된다. 지우면
			// 첫 claim 한 번으로 즉시 감지가 8시간 임계값으로 되돌아간다.
			lease.HookPID, lease.HookStarttime = previous.HookPID, previous.HookStarttime
		}
		return writeLease(f, lease)
	})
	if err != nil {
		return model.Lease{}, err
	}
	// 에이전트 기록은 소유자별 회수를 위한 색인이다. 같은 신원의 동시 갱신은
	// 이 파일 자신의 flock 이 막는다.
	if err := r.withLocked(r.agentPath(lease.Agent), func(f *os.File) error {
		return writeLease(f, lease)
	}); err != nil {
		return model.Lease{}, err
	}
	return lease, nil
}

// withLocked 는 파일을 열고 배타 잠금 아래에서 fn 을 부른다.
//
// 열기·잠그기·풀기·닫기를 매번 손으로 적으면 언젠가 하나를 빠뜨린다. 그 실수는
// 리뷰에서 보이지 않으므로 순서를 여기 한 곳에 가둔다.
func (r *Registry) withLocked(path string, fn func(*os.File) error) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("잠금 파일 열기: %w", err)
	}
	defer f.Close()
	if err := flock(f); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn(f)
}

// Alive applies the five-level decision order from design section 3.6.
func (r *Registry) Alive(lease model.Lease) bool {
	if lease.Agent == "" || lease.Session == "" || lease.LastActivity.Time.IsZero() {
		return false
	}
	if lease.HookPID != nil {
		if r.AlivePID == nil {
			return false
		}
		start := ""
		if lease.HookStarttime != nil {
			start = *lease.HookStarttime
		}
		return r.AlivePID(*lease.HookPID, start)
	}
	now := r.Now()
	if lease.LastActivity.After(now) {
		return true
	}
	return now.Sub(lease.LastActivity.Time) <= r.TTL
}

// LookupWorktree 는 이 worktree 의 현재 점유 기록을 읽는다.
//
// 조회 전용이다 — doctor 같은 읽기 명령이 last_activity 를 건드리면 안 된다.
func (r *Registry) LookupWorktree() (model.Lease, bool) {
	return readLeaseAt(r.worktreePath())
}

// ProbeLock 은 잠금 디렉터리에서 flock 이 실제로 동작하는지 본다.
//
// flock 은 로컬 파일시스템을 전제한다. NFS 등에서는 조용히 무력해지므로
// 판정을 추측이 아니라 실제 시도로 한다 (design §719).
func (r *Registry) ProbeLock() error {
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return fmt.Errorf("잠금 디렉터리 생성: %w", err)
	}
	path := filepath.Join(r.Dir, "flock-probe")
	defer os.Remove(path)
	return r.withLocked(path, func(*os.File) error { return nil })
}

// List 는 잠금 디렉터리의 에이전트 기록을 전부 읽는다.
func (r *Registry) List() []model.Lease {
	entries, _ := filepath.Glob(filepath.Join(r.Dir, "agent-*.lock"))
	out := make([]model.Lease, 0, len(entries))
	for _, p := range entries {
		if l, ok := readLeaseAt(p); ok {
			out = append(out, l)
		}
	}
	return out
}

// readLeaseAt 은 경로에서 기록 하나를 읽는다. 깨진 기록은 없는 것으로 본다 —
// "건너뛴다" 의 정의가 두 벌 생기지 않게 한 곳에 둔다.
func readLeaseAt(path string) (model.Lease, bool) {
	f, err := os.Open(path)
	if err != nil {
		return model.Lease{}, false
	}
	defer f.Close()
	l, err := decodeLease(f)
	return l, err == nil
}

// flockTimeout 은 잠금을 기다리는 상한이다.
//
// 이 잠금은 기록 하나를 읽고 쓰는 동안만 잡히므로 (§3.6) 정상적으로는
// 마이크로초 단위다. 그래도 넉넉히 두는 이유는, 프로세스 수십 개가 동시에
// git 을 fork 하는 순간의 스케줄링 지연 때문이다 — 거기서 포기하면
// 사용자에게는 아무 이유 없이 실패한 claim 으로 보인다. 잠금을 쥔
// 프로세스가 죽으면 커널이 즉시 풀어 주므로 오래 기다려도 매달리지 않는다.
const flockTimeout = 5 * time.Second

func flock(f *os.File) error {
	deadline := time.Now().Add(flockTimeout)
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return nil
		} else if !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("flock: %w", err)
		}
		if time.Now().After(deadline) {
			return errors.New("flock timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func readLeaseFile(f *os.File) (model.Lease, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return model.Lease{}, err
	}
	return decodeLease(f)
}
func decodeLease(rd io.Reader) (model.Lease, error) {
	var l model.Lease
	err := json.NewDecoder(rd).Decode(&l)
	return l, err
}
func writeLease(f *os.File, l model.Lease) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err = f.Truncate(0); err != nil {
		return err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	return f.Sync()
}
func canonical(p string) string {
	a, err := filepath.Abs(p)
	if err == nil {
		p = a
	}
	if e, err := filepath.EvalSymlinks(p); err == nil {
		p = e
	}
	return filepath.Clean(p)
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:16]) }
func (r *Registry) worktreePath() string {
	return filepath.Join(r.Dir, "worktree-"+digest(r.Worktree)+".lock")
}
func (r *Registry) agentPath(a string) string {
	return filepath.Join(r.Dir, "agent-"+digest(a)+".lock")
}
func short(s string) string {
	if len(s) > 8 {
		return s[:8] + "..."
	}
	return s
}

// ProcessStarttime 은 pid 의 시작 시각이다. 못 읽으면 빈 문자열이다.
//
// pid 재사용을 걸러내기 위한 값이다. 이름이나 트리는 보지 않는다 (D10).
func ProcessStarttime(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func processAlive(pid int, starttime string) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if starttime == "" {
		return true
	}
	// Only the process start time is queried. Process names and process-tree
	// scanning are deliberately excluded (D10); the value detects PID reuse.
	return ProcessStarttime(pid) == starttime
}
