package board

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// 자식 프로세스 전환용 환경변수.
const (
	casChildDir   = "PPWK_TEST_MUTATE_DIR"
	casChildID    = "PPWK_TEST_MUTATE_ID"
	casChildAgent = "PPWK_TEST_MUTATE_AGENT"
)

// 자식 프로세스의 종료 코드. features §0.3 과 같은 뜻이다.
const (
	childOK         = 0
	childGeneral    = 1
	childTransition = 3
	childConflict   = 4
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(casChildDir); dir != "" {
		os.Exit(runMutateChild(dir))
	}
	if dir := os.Getenv(nextChildDir); dir != "" {
		os.Exit(runNextChild(dir))
	}
	if dir := os.Getenv(actionChildDir); dir != "" {
		os.Exit(runActionChild(dir))
	}
	os.Exit(m.Run())
}

// runMutateChild 는 자식 프로세스에서 전이 하나를 시도한다.
func runMutateChild(dir string) int {
	ident := session.Identity{
		Agent:   os.Getenv(casChildAgent),
		Session: session.NewNonce(),
	}
	b, err := Open(dir, ident)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return childGeneral
	}

	m := startMutation()
	m.ID = os.Getenv(casChildID)
	if _, err := b.Mutate(m); err != nil {
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
	return childOK
}

// T2.2 프로세스 16개가 동시에 같은 이슈를 전이하면 정확히 1개만 성공한다.
//
// goroutine 으로는 이 테스트가 의미를 잃는다. 한 프로세스 안이면 go-git 의
// CheckAndSetReference 로도 통과해버리기 때문이다 — 그러나 그것은 read-then-write
// 라 프로세스 경계를 넘으면 깨진다 (§14.2). 그래서 진짜 프로세스를 띄운다.
func TestSixteenProcessesRaceForOneIssue(t *testing.T) {
	if testing.Short() {
		t.Skip("프로세스 16개를 띄우므로 -short 에서는 건너뛴다")
	}
	ok, transition, conflict := raceOnce(t, 16)
	if ok != 1 {
		t.Fatalf("성공 %d개, want 1 (전이 %d, 경쟁 %d)", ok, transition, conflict)
	}
	if ok+transition+conflict != 16 {
		t.Fatalf("성공 %d + 전이 %d + 경쟁 %d != 16", ok, transition, conflict)
	}
}

// raceOnce 는 n개 프로세스를 같은 이슈에 붙이고 결과를 센다.
func raceOnce(t *testing.T, n int) (ok, transition, conflict int) {
	t.Helper()
	b, dir := newBoard(t)
	issue := seedIssue(t, b)

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() = %v", err)
	}

	start := make(chan struct{})
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(self)
			cmd.Env = append(os.Environ(),
				casChildDir+"="+dir,
				casChildID+"="+issue.ID,
				casChildAgent+"=agent-"+strconv.Itoa(i),
			)
			<-start
			err := cmd.Run()
			if err == nil {
				codes[i] = childOK
				return
			}
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				codes[i] = exit.ExitCode()
				return
			}
			t.Errorf("자식 실행 실패: %v", err)
			codes[i] = childGeneral
		}()
	}
	close(start)
	wg.Wait()

	for i, code := range codes {
		switch code {
		case childOK:
			ok++
		case childTransition:
			transition++
		case childConflict:
			conflict++
		default:
			t.Fatalf("자식 %d 가 예상 밖 코드 %d 로 끝났습니다", i, code)
		}
	}
	return ok, transition, conflict
}

// D2.2 T2.2 를 반복해 flake 가 없는지 본다 (R8).
//
// 반복마다 프로세스를 16개 띄우면 너무 비싸므로 4개로 줄이고 횟수를 늘린다.
// 보려는 성질은 "정확히 하나만 성공한다" 이고 그것은 N 에 의존하지 않는다.
func TestRaceStress(t *testing.T) {
	if testing.Short() {
		t.Skip("스트레스 테스트는 -short 에서는 건너뛴다")
	}
	iterations := 100
	if os.Getenv("PPWK_STRESS") == "" {
		// 기본 실행에서는 짧게 돈다. 전체 100회는 PPWK_STRESS=1 로 돌린다.
		iterations = 10
	}
	for i := range iterations {
		ok, transition, conflict := raceOnce(t, 4)
		if ok != 1 {
			t.Fatalf("%d회차: 성공 %d개, want 1 (전이 %d, 경쟁 %d)", i, ok, transition, conflict)
		}
	}
}
