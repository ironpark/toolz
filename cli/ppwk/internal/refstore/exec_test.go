package refstore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
)

// casChildEnv 가 설정되면 테스트 바이너리는 CAS 를 한 번 시도하고 끝난다.
//
// T0.7 은 goroutine 이 아니라 실제 프로세스여야 한다. goroutine 으로 하면
// go-git 의 CheckAndSetReference 로도 통과해버려 테스트가 의미를 잃는다 (§14.2).
const (
	casChildEnv  = "PPWK_TEST_CAS_CHILD"
	casChildRef  = "PPWK_TEST_CAS_REF"
	casChildHash = "PPWK_TEST_CAS_HASH"
)

// 자식 프로세스의 종료 코드.
const (
	childWon      = 0 // CAS 성공
	childLost     = 3 // ErrCASConflict 또는 ErrLockBusy
	childBroken   = 4 // 그 밖의 오류
	childSetupErr = 5 // 저장소를 열지 못함
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(casChildEnv); dir != "" {
		os.Exit(runCASChild(dir))
	}
	os.Exit(m.Run())
}

// runCASChild 는 CAS 를 한 번 시도하고 결과를 종료 코드로 알린다.
func runCASChild(dir string) int {
	store, err := NewExecRefStore(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return childSetupErr
	}
	err = store.CAS(os.Getenv(casChildRef), plumbing.NewHash(os.Getenv(casChildHash)), plumbing.ZeroHash)
	switch {
	case err == nil:
		return childWon
	case errors.Is(err, ErrCASConflict), errors.Is(err, ErrLockBusy):
		return childLost
	default:
		fmt.Fprintln(os.Stderr, err)
		return childBroken
	}
}

// T0.7 프로세스 16개가 같은 ref 에 동시 CAS → 정확히 1개만 성공
func TestConcurrentCASAcrossProcesses(t *testing.T) {
	const workers = 16

	dir := newTestRepo(t)
	oid := makeCommits(t, dir, 1)

	children := make([]*exec.Cmd, 0, workers)
	for range workers {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(),
			casChildEnv+"="+dir,
			casChildRef+"="+testRef,
			casChildHash+"="+oid[0].String(),
		)
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("자식 프로세스 시작 실패: %v", err)
		}
		children = append(children, cmd)
	}

	var won, lost int
	for _, cmd := range children {
		err := cmd.Wait()
		code := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else if err != nil {
			t.Fatalf("자식 프로세스 대기 실패: %v", err)
		}
		switch code {
		case childWon:
			won++
		case childLost:
			lost++
		default:
			t.Fatalf("자식이 예상 밖 코드 %d 로 종료", code)
		}
	}

	if won != 1 {
		t.Fatalf("성공 %d개, want 1", won)
	}
	if lost != workers-1 {
		t.Fatalf("실패 %d개, want %d", lost, workers-1)
	}
}

// T0.11 linked worktree 3개에서 각각 List → 동일한 결과
// T0.12 linked worktree 에서도 공유 ref 가 보인다 (commondir 회귀)
func TestListAcrossLinkedWorktrees(t *testing.T) {
	main := newTestRepo(t)
	oid := makeCommits(t, main, 2)

	store, err := NewExecRefStore(main)
	if err != nil {
		t.Fatalf("NewExecRefStore() = %v", err)
	}
	if err := store.CAS("refs/ppwk/issues/T001", oid[0], plumbing.ZeroHash); err != nil {
		t.Fatalf("CAS() = %v", err)
	}
	if err := store.CAS("refs/ppwk/issues/T002", oid[1], plumbing.ZeroHash); err != nil {
		t.Fatalf("CAS() = %v", err)
	}
	want, err := store.List(Issues)
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if len(want) != 2 {
		t.Fatalf("List() = %v, want 2개", want)
	}

	// worktree add 는 커밋된 브랜치를 요구하므로 실제 커밋을 하나 만든다.
	runGit(t, main, "commit", "--allow-empty", "--quiet", "-m", "base")

	base := t.TempDir()
	for i := range 3 {
		wt := filepath.Join(base, fmt.Sprintf("wt%d", i))
		runGit(t, main, "worktree", "add", "--quiet", "-b", fmt.Sprintf("b%d", i), wt)

		linked, err := NewExecRefStore(wt)
		if err != nil {
			t.Fatalf("worktree %d: NewExecRefStore() = %v", i, err)
		}
		got, err := linked.List(Issues)
		if err != nil {
			t.Fatalf("worktree %d: List() = %v", i, err)
		}
		if len(got) != len(want) {
			t.Fatalf("worktree %d: List() = %v, want %v — commondir 가 반영되지 않음", i, got, want)
		}
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("worktree %d: List()[%d] = %v, want %v", i, j, got[j], want[j])
			}
		}
	}
}

// linked worktree 에서 CAS 한 결과가 본 저장소에도 보인다.
func TestCASFromLinkedWorktree(t *testing.T) {
	main := newTestRepo(t)
	oid := makeCommits(t, main, 1)
	runGit(t, main, "commit", "--allow-empty", "--quiet", "-m", "base")

	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, main, "worktree", "add", "--quiet", "-b", "side", wt)

	linked, err := NewExecRefStore(wt)
	if err != nil {
		t.Fatalf("NewExecRefStore() = %v", err)
	}
	if err := linked.CAS(testRef, oid[0], plumbing.ZeroHash); err != nil {
		t.Fatalf("CAS() = %v", err)
	}

	store, err := NewExecRefStore(main)
	if err != nil {
		t.Fatalf("NewExecRefStore() = %v", err)
	}
	got, err := store.Get(testRef)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if got != oid[0] {
		t.Fatalf("Get() = %s, want %s", got, oid[0])
	}
}

// packed-refs 로 packing 된 뒤에도 Get/List/CAS 가 정상 동작한다.
func TestPackedRefs(t *testing.T) {
	dir := newTestRepo(t)
	oid := makeCommits(t, dir, 2)

	store, err := NewExecRefStore(dir)
	if err != nil {
		t.Fatalf("NewExecRefStore() = %v", err)
	}
	if err := store.CAS(testRef, oid[0], plumbing.ZeroHash); err != nil {
		t.Fatalf("CAS() = %v", err)
	}
	runGit(t, dir, "pack-refs", "--all")

	// packing 후에도 열려 있던 저장소가 값을 본다.
	fresh, err := NewExecRefStore(dir)
	if err != nil {
		t.Fatalf("NewExecRefStore() = %v", err)
	}
	if got, err := fresh.Get(testRef); err != nil || got != oid[0] {
		t.Fatalf("Get() = %s, %v", got, err)
	}
	entries, err := fresh.List(Issues)
	if err != nil || len(entries) != 1 {
		t.Fatalf("List() = %v, %v", entries, err)
	}
	// packed ref 에 대한 CAS 도 update-ref 가 알아서 처리한다.
	if err := fresh.CAS(testRef, oid[1], oid[0]); err != nil {
		t.Fatalf("packed ref CAS() = %v", err)
	}
}

// git 환경 검증은 시작 시점에 이뤄진다.
func TestNewExecRefStoreRejectsBadDir(t *testing.T) {
	if _, err := NewExecRefStore(filepath.Join(t.TempDir(), "없는경로")); err == nil {
		t.Fatal("NewExecRefStore() = nil, want error")
	}
}

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		in           string
		major, minor int
		wantErr      bool
	}{
		{in: "git version 2.43.0\n", major: 2, minor: 43},
		{in: "git version 2.28.0.windows.1\n", major: 2, minor: 28},
		{in: "git version 2.39.5 (Apple Git-154)\n", major: 2, minor: 39},
		{in: "garbage", wantErr: true},
		{in: "git version x.y", wantErr: true},
	}
	for _, tt := range tests {
		major, minor, err := parseGitVersion(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("parseGitVersion(%q) = nil, want error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseGitVersion(%q) = %v", tt.in, err)
		}
		if major != tt.major || minor != tt.minor {
			t.Fatalf("parseGitVersion(%q) = %d.%d, want %d.%d", tt.in, major, minor, tt.major, tt.minor)
		}
	}
}
