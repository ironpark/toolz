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
	b, dir := newBoard(t)
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	for i := range candidates {
		mustAdd(t, b, AddOptions{Title: fmt.Sprintf("일감 %d", i)})
	}

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
	return claimed, min(processes, candidates)
}
