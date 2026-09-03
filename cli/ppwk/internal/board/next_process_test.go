package board

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// nextChildDir 은 자식 프로세스를 next --claim 모드로 돌린다.
const nextChildDir = "PPWK_TEST_NEXT_DIR"

// runNextChild 는 자식에서 next --claim 을 한 번 수행하고 가져온 ID 를 낸다.
//
// 아무것도 못 가져와도 성공이다 — 그것이 "할 일 없음" 의 정상 결과다.
func runNextChild(dir string) int {
	b, err := OpenFor(dir, OpenOptions{
		Session:             session.Options{Worktree: dir},
		AllowSharedWorktree: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return childGeneral
	}
	result, err := b.Next(NextOptions{Claim: true, MaxAttempts: 5})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return childGeneral
	}
	if result.Claimed != nil {
		fmt.Println(result.Claimed.ID)
	}
	return childOK
}

// T5.5 프로세스 16개가 동시에 next --claim 해도 중복 배정이 없다.
//
// goroutine 으로는 의미가 없다. CAS 가 프로세스 경계를 넘어 동작하는지가
// 이 테스트의 전부다 (§14.2).
func TestSixteenProcessesClaimWithoutDuplicates(t *testing.T) {
	if testing.Short() {
		t.Skip("프로세스 16개를 띄우므로 -short 에서는 건너뛴다")
	}
	// 중복 없음이 claimRace 안에서 검사된다. 여기서는 배정이 실제로
	// 일어났는지만 본다 — 후보와 프로세스가 같은 수여도 --max-attempts 상한
	// 때문에 전원이 일감을 받는 것은 아니고, 그것이 의도된 동작이다.
	claimed, _ := claimRace(t, 16, 16)
	if len(claimed) == 0 {
		t.Fatal("아무도 배정받지 못했습니다")
	}
}

// T5.6 후보보다 프로세스가 많아도 후보 수만큼만 배정된다.
func TestMoreProcessesThanCandidates(t *testing.T) {
	if testing.Short() {
		t.Skip("프로세스를 여러 개 띄우므로 -short 에서는 건너뛴다")
	}
	claimed, candidates := claimRace(t, 8, 3)
	if len(claimed) != candidates {
		t.Fatalf("배정 %d건, want %d: %v", len(claimed), candidates, claimed)
	}
}

// D5.1 T5.5 를 반복해 중복 배정이 정말 0인지 본다.
//
// 16개를 100번 띄우면 너무 비싸므로 프로세스를 줄이고 횟수를 늘린다. 보려는
// 성질은 "한 이슈가 두 번 배정되지 않는다" 이고 그것은 N 에 의존하지 않는다.
func TestClaimRaceStress(t *testing.T) {
	if testing.Short() {
		t.Skip("스트레스 테스트는 -short 에서는 건너뛴다")
	}
	iterations := 100
	processes, candidates := 2, 1
	if os.Getenv("PPWK_STRESS") == "" {
		iterations = 10
	}
	for i := range iterations {
		claimed, want := claimRace(t, processes, candidates)
		if len(claimed) != want {
			t.Fatalf("%d회차: 배정 %d건, want %d: %v", i, len(claimed), want, claimed)
		}
	}
}

// claimRace 는 후보 candidates 개를 두고 processes 개 프로세스를 동시에 붙인다.
// 돌려주는 집합은 중복 없이 배정된 ID 들이다.
func claimRace(t *testing.T, processes, candidates int) (map[string]bool, int) {
	t.Helper()
	b, dir := initBoardDir(t)
	for i := range candidates {
		mustAdd(t, b, AddOptions{Title: fmt.Sprintf("일감 %d", i)})
	}
	claimed, _ := claimRaceIn(t, b, dir, processes)
	return claimed, min(processes, candidates)
}

// claimRaceIn 은 이미 준비된 보드에 프로세스 n개를 동시에 붙인다.
func claimRaceIn(t *testing.T, b *Board, dir string, processes int) (map[string]bool, int) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() = %v", err)
	}

	start := make(chan struct{})
	outputs := make([]string, processes)
	var wg sync.WaitGroup
	for i := range processes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(self)
			cmd.Env = append(os.Environ(),
				nextChildDir+"="+dir,
				"PPWK_AGENT=agent-"+strconv.Itoa(i),
			)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			<-start
			out, err := cmd.Output()
			if err != nil {
				var exit *exec.ExitError
				if errors.As(err, &exit) {
					t.Errorf("자식 %d 가 코드 %d 로 끝났습니다: %s", i, exit.ExitCode(), stderr.String())
					return
				}
				t.Errorf("자식 %d 실행 실패: %v", i, err)
				return
			}
			outputs[i] = strings.TrimSpace(string(out))
		}()
	}
	close(start)
	wg.Wait()

	claimed := map[string]bool{}
	for i, id := range outputs {
		if id == "" {
			continue
		}
		if claimed[id] {
			t.Fatalf("%s 가 두 번 배정됐습니다 (자식 %d)", id, i)
		}
		claimed[id] = true
	}
	return claimed, len(claimed)
}

// T9.6 같은 plan 의 task 를 N개 프로세스가 동시에 claim 해도 중복이 없고,
// **plan ref 는 한 번도 쓰이지 않는다.**
//
// 뒤쪽이 §3.7.1 의 회귀 테스트다. plan 에 진행률이나 현재 phase 같은 상태를
// 넣는 순간 task 상태가 바뀔 때마다 plan ref 를 갱신해야 하고, 이슈별로 ref 를
// 나눠 경쟁을 분산시킨 이득이 통째로 사라진다. 그 회귀는 성능 문제로만
// 나타나서 테스트 없이는 눈치채기 어렵다.
func TestConcurrentClaimNeverWritesPlanRef(t *testing.T) {
	if testing.Short() {
		t.Skip("프로세스를 여러 개 띄우므로 -short 에서는 건너뛴다")
	}
	b, dir := initBoardDir(t)
	plan := makePlan(t, b, "계획", model.PriorityHigh,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone})
	for i := range 6 {
		task(t, b, plan.ID, "p1", 10*(i+1))
	}

	before, err := b.Store().Get(refstore.Plans + plan.ID)
	if err != nil {
		t.Fatal(err)
	}

	claimed, _ := claimRaceIn(t, b, dir, 8)
	if len(claimed) == 0 {
		t.Fatal("아무도 배정받지 못했습니다")
	}

	fresh, err := Open(dir, b.Identity())
	if err != nil {
		t.Fatal(err)
	}
	after, err := fresh.Store().Get(refstore.Plans + plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("plan ref 가 %s → %s 로 바뀌었습니다 — plan 은 진행 상태를 갖지 않아야 합니다 (§3.7.1)",
			before, after)
	}
}

// 자식 프로세스를 decide 모드로 돌린다.
const (
	decideChildDir   = "PPWK_TEST_DECIDE_DIR"
	decideChildTitle = "PPWK_TEST_DECIDE_TITLE"
)

func runDecideChild(dir string) int {
	b, err := OpenFor(dir, OpenOptions{
		Session:             session.Options{Worktree: dir},
		AllowSharedWorktree: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return childGeneral
	}
	decision, err := b.Decide(DecideOptions{Title: os.Getenv(decideChildTitle)})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return childGeneral
	}
	fmt.Println(decision.ID)
	return childOK
}

// T12.2 같은 결정을 N개 프로세스가 동시에 만들어도 ID 가 겹치지 않고 전부
// 성공한다.
//
// 이슈 채번(§3.2)과 같은 create-only CAS 다. 다른 점은 여기서 아무도 지지
// 않는다는 것이다 — 결정은 서로 경쟁하는 자원이 아니라 각자 하나씩 쌓이는
// 기록이므로, 번호가 밀리면 다음 번호로 가면 된다.
func TestConcurrentDecideAllSucceed(t *testing.T) {
	if testing.Short() {
		t.Skip("프로세스를 여러 개 띄우므로 -short 에서는 건너뛴다")
	}
	const n = 8
	b, dir := initBoardDir(t)

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() = %v", err)
	}
	start := make(chan struct{})
	outputs := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(self)
			cmd.Env = append(os.Environ(),
				decideChildDir+"="+dir,
				// 제목까지 같게 둔다. 같은 내용이면 같은 OID 가 되어 두 CAS 가
				// 모두 성공하는 §4.3 의 함정을 여기서도 밟는다.
				decideChildTitle+"=같은 제목",
				"PPWK_AGENT=agent-"+strconv.Itoa(i),
			)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			<-start
			out, err := cmd.Output()
			if err != nil {
				t.Errorf("자식 %d: %v\n%s", i, err, stderr.String())
				return
			}
			outputs[i] = strings.TrimSpace(string(out))
		}()
	}
	close(start)
	wg.Wait()

	seen := map[string]bool{}
	for i, id := range outputs {
		if id == "" {
			t.Fatalf("자식 %d 가 ID 를 내지 않았습니다", i)
		}
		if seen[id] {
			t.Fatalf("%s 가 두 번 배정됐습니다", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("ID %d개, want %d", len(seen), n)
	}

	fresh, err := Open(dir, b.Identity())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fresh.ListDecisions(DecisionListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != n {
		t.Fatalf("결정 %d개, want %d", len(entries), n)
	}
}
