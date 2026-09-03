package board

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// 자식 프로세스를 전이 모드로 돌린다.
const (
	actionChildDir    = "PPWK_TEST_ACTION_DIR"
	actionChildID     = "PPWK_TEST_ACTION_ID"
	actionChildAction = "PPWK_TEST_ACTION"
)

// runActionChild 는 자식에서 전이 하나를 수행한다. done 이면 이동을 겸한다.
func runActionChild(dir string) int {
	b, err := OpenFor(dir, OpenOptions{
		Session:             session.Options{Worktree: dir},
		AllowSharedWorktree: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return childGeneral
	}
	_, err = b.Transition(Action(os.Getenv(actionChildAction)), os.Getenv(actionChildID), TransitionOptions{})
	if err == nil {
		return childOK
	}
	var transition *TransitionError
	var conflict *ConflictError
	switch {
	case errors.As(err, &transition):
		return childTransition
	case errors.As(err, &conflict):
		return childConflict
	}
	fmt.Fprintln(os.Stderr, err)
	return childGeneral
}

// T6.6 이동과 동시에 다른 프로세스가 같은 이슈를 바꾸려 하면 하나만 성공한다.
func TestArchiveRacesWithConcurrentTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("프로세스를 여러 개 띄우므로 -short 에서는 건너뛴다")
	}
	for range 10 {
		b, dir := initBoardDir(t)
		issue := mustAdd(t, b, AddOptions{Title: "대상"})
		transitionAll(t, b, issue.ID, ActionStart)

		// 둘 다 working 에서 출발한다. done 은 이동을 겸하고 block 은 아니다.
		codes := runChildren(t, b, dir, issue.ID, []Action{ActionDone, ActionBlock})
		ok := 0
		for _, c := range codes {
			if c == childOK {
				ok++
			}
		}
		if ok != 1 {
			t.Fatalf("성공 %d건, want 1 (codes=%v)", ok, codes)
		}
		// 어느 쪽이 이겼든 이슈는 정확히 한 곳에 있다.
		assertExactlyOnePrefix(t, b, issue.ID)
	}
}

// runChildren 은 액션마다 자식 프로세스를 띄우고 종료 코드를 모은다.
func runChildren(t *testing.T, b *Board, dir, id string, actions []Action) []int {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() = %v", err)
	}
	start := make(chan struct{})
	codes := make([]int, len(actions))
	var wg sync.WaitGroup
	for i, action := range actions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(self)
			cmd.Env = append(os.Environ(),
				actionChildDir+"="+dir,
				actionChildID+"="+id,
				actionChildAction+"="+string(action),
				// 소유자와 같은 신원으로 돈다. 다른 신원이면 소유권 규칙에서
				// 먼저 걸려 CAS 경쟁 자체가 일어나지 않는다.
				"PPWK_AGENT="+b.Identity().Agent,
				"PPWK_SESSION="+b.Identity().Session,
			)
			<-start
			codes[i] = exitCodeOf(t, cmd.Run())
		}()
	}
	close(start)
	wg.Wait()
	return codes
}

func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return childOK
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	t.Errorf("자식 실행 실패: %v", err)
	return childGeneral
}

// T6.2 이동 중 SIGKILL 을 맞아도 이슈가 양쪽에 있거나 양쪽에서 사라지지 않는다.
//
// 결함 주입판(TestArchiveMoveIsAtomic)은 CAS 경계에서 죽는 상황만 재현한다.
// 여기서는 진짜로 죽인다 — git 이 트랜잭션 중간에 죽었을 때 무엇을 남기는지는
// 우리 코드가 아니라 git 의 성질이고, 그 성질에 기대고 있음을 확인하는 것이
// 이 테스트의 목적이다.
func TestArchiveSurvivesSIGKILL(t *testing.T) {
	if testing.Short() {
		t.Skip("프로세스를 여러 개 띄우므로 -short 에서는 건너뛴다")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() = %v", err)
	}
	rng := rand.New(rand.NewPCG(20260903, 1))

	for i := range 40 {
		b, dir := initBoardDir(t)
		issue := mustAdd(t, b, AddOptions{Title: "대상"})
		transitionAll(t, b, issue.ID, ActionStart)

		cmd := exec.Command(self)
		cmd.Env = append(os.Environ(),
			actionChildDir+"="+dir,
			actionChildID+"="+issue.ID,
			actionChildAction+"="+string(ActionDone),
			"PPWK_AGENT="+b.Identity().Agent,
			"PPWK_SESSION="+b.Identity().Session,
		)
		if err := cmd.Start(); err != nil {
			t.Fatalf("%d회차 시작 실패: %v", i, err)
		}
		// 전이와 이동이 걸리는 구간 어딘가를 노린다.
		time.Sleep(time.Duration(rng.IntN(30000)) * time.Microsecond)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		assertExactlyOnePrefix(t, b, issue.ID)
	}
}

// assertExactlyOnePrefix 는 이슈가 issues/ 와 archive/ 중 정확히 한 곳에만
// 있는지 본다. 이것이 §4.4 가 지키는 불변식이다.
func assertExactlyOnePrefix(t *testing.T, b *Board, id string) {
	t.Helper()
	fresh, err := Open(b.Root(), b.Identity())
	if err != nil {
		t.Fatal(err)
	}
	inIssues := refExists(t, fresh, refstore.Issues+id)
	inArchive := refExists(t, fresh, refstore.Archive+id)
	if inIssues == inArchive {
		t.Fatalf("%s: issues/=%v archive/=%v — 정확히 한쪽에만 있어야 합니다", id, inIssues, inArchive)
	}
}
