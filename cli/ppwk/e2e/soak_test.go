package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// soakDuration 은 장기 실행 시나리오의 길이다.
//
// 명세의 24시간은 릴리스 전 항목이다 (§10.1). 기본은 짧게 돌려 회귀만 잡고,
// 진짜 길이는 환경변수로 연다. 짧게 돌려도 의미가 있는 이유는 fd 누수와
// 중복 배정이 시간이 아니라 반복 횟수에 비례해 드러나기 때문이다.
func soakDuration(t *testing.T) time.Duration {
	t.Helper()
	raw := os.Getenv("PPWK_E2E_SOAK")
	if raw == "" {
		if testing.Short() {
			t.Skip("soak 은 -short 에서 건너뜁니다")
		}
		return 15 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		t.Fatalf("PPWK_E2E_SOAK=%q: %v", raw, err)
	}
	return d
}

// E2E-29: 장기 무중단.
//
// 단위 테스트로는 안 잡히는 종류가 여기서 나온다 — 특히 세션 잠금 fd 누수와
// watch 의 스냅샷 메모리 증가.
func TestSoakNoLeaks(t *testing.T) {
	duration := soakDuration(t)
	f := newFixture(t)
	w := f.watch()

	const agents = 3
	var dirs []string
	for i := range agents {
		dirs = append(dirs, f.AddWorktree(fmt.Sprintf("w%d", i), fmt.Sprintf("br/%d", i)).Path)
	}

	deadline := time.Now().Add(duration)
	claims := map[string]string{}
	rounds := 0
	for time.Now().Before(deadline) {
		rounds++
		// producer: 에이전트 수만큼 이슈를 계속 만든다. 하나만 만들면 매
		// 회차에 첫 에이전트만 일감을 얻는다.
		for i := range agents {
			f.add(fmt.Sprintf("작업 %04d-%d", rounds, i))
		}

		for i := range agents {
			ident := Identity{
				Agent:   fmt.Sprintf("agent-%d", i),
				Session: fmt.Sprintf("session-%d", i),
			}
			r := f.RunAs(ident, dirs[i], "--json", "next", "--claim")
			id := claimedID(r.Stdout)
			if id == "" {
				continue
			}
			if prev, dup := claims[id]; dup {
				t.Fatalf("%s 가 %s 와 %s 에게 중복 배정됐습니다\n%s",
					id, prev, ident.Agent, f.issueTimeline(id))
			}
			claims[id] = ident.Agent
			f.RunAs(ident, dirs[i], "start", id)
			f.RunAs(ident, dirs[i], "done", id)
			delete(claims, id) // 끝난 것은 다시 나오지 않는다
			claims[id+"-done"] = ident.Agent
		}
	}
	if rounds < 3 {
		t.Fatalf("%d 회차밖에 돌지 못했습니다", rounds)
	}

	// 상시 실행 프로세스가 없다. watch 를 뺀 ppwk 프로세스는 남지 않는다.
	if n := countProcesses(t, binary); n > 1 {
		t.Fatalf("ppwk 프로세스가 %d개 남아 있습니다 — watch 하나뿐이어야 합니다", n)
	}
	// 기록이 회차에 비례해 늘지 않는다. 에이전트당 하나뿐이다.
	//
	// 이슈를 만들기만 하는 producer 는 여기 없다 — add 는 소유권을 만들지
	// 않으므로 세션을 등록하지 않는다.
	if leases := f.leases(); len(leases) != agents {
		t.Fatalf("%d 회차 뒤 잠금 기록 %d건, want %d (에이전트 수): %+v",
			rounds, len(leases), agents, leases)
	}
	// watch 가 살아 있고 여전히 감지한다.
	last := f.add("마지막")
	waitFor(t, 15*time.Second, "장시간 후에도 감지", func() bool {
		return w.seen("refs/ppwk/issues/"+last, "created")
	})

	f.expectGitFsckClean()
	f.MustRun("fsck")
	f.expectCleanTree()
	// loose ref 가 통제된다 — 최소한 보고는 된다.
	if check := f.doctorCheck("refs"); !strings.HasPrefix(fmt.Sprint(check["via"]), "loose ") {
		t.Fatalf("doctor 가 loose ref 를 보고하지 않습니다: %v", check)
	}
}

// countProcesses 는 이 바이너리로 뜬 프로세스 수다.
func countProcesses(t *testing.T, path string) int {
	t.Helper()
	out, err := exec.Command("ps", "-Ao", "command=").Output()
	if err != nil {
		t.Skipf("ps 를 쓸 수 없습니다: %v", err)
	}
	n := 0
	for _, line := range lines(string(out)) {
		if strings.HasPrefix(line, path) {
			n++
		}
	}
	return n
}

// E2E-30: 대량 이슈.
//
// 응답 시간이 실용 범위인지, 그리고 loose ref 증가가 보고되는지 본다.
func TestSoakManyIssues(t *testing.T) {
	count := 200
	if raw := os.Getenv("PPWK_E2E_ISSUES"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("PPWK_E2E_ISSUES=%q: %v", raw, err)
		}
		count = n
	} else if testing.Short() {
		t.Skip("대량 시나리오는 -short 에서 건너뜁니다")
	}

	f := newFixture(t)
	for i := range count {
		f.add(fmt.Sprintf("작업 %05d", i))
	}

	looseBefore := doctorLoose(t, f)
	if looseBefore < count {
		t.Fatalf("loose ref %d개, 이슈는 %d개입니다 — doctor 가 증가를 못 보고 있습니다",
			looseBefore, count)
	}

	listBefore := timeIt(func() { f.MustRun("list") })
	nextBefore := timeIt(func() { f.MustRun("next", "--dry-run") })

	f.Git("pack-refs", "--all")

	if after := doctorLoose(t, f); after >= looseBefore {
		t.Fatalf("pack-refs 후 loose ref 가 %d → %d 로 줄지 않았습니다", looseBefore, after)
	}
	// 기능은 그대로다. pack 된 ref 를 못 읽으면 여기서 드러난다.
	if got := len(f.listIDs()); got != count {
		t.Fatalf("pack-refs 후 목록이 %d건, want %d", got, count)
	}
	listAfter := timeIt(func() { f.MustRun("list") })
	nextAfter := timeIt(func() { f.MustRun("next", "--dry-run") })

	t.Logf("이슈 %d개 — list %s → %s, next %s → %s (pack-refs 전후)",
		count, listBefore, listAfter, nextBefore, nextAfter)

	// 실용 범위. 절대 수치는 기계마다 다르므로 넉넉히 잡는다. 여기서 잡으려는
	// 것은 미세한 차이가 아니라 이슈 수에 대해 초선형으로 터지는 구현이다.
	for _, tc := range []struct {
		name    string
		elapsed time.Duration
	}{{"list", listAfter}, {"next", nextAfter}} {
		if tc.elapsed > 10*time.Second {
			t.Fatalf("이슈 %d개에서 %s 가 %s 걸립니다", count, tc.name, tc.elapsed)
		}
	}
}

func doctorLoose(t *testing.T, f *Fixture) int {
	t.Helper()
	via := fmt.Sprint(f.doctorCheck("refs")["via"])
	n, err := strconv.Atoi(strings.TrimPrefix(via, "loose "))
	if err != nil {
		t.Fatalf("doctor 의 refs via = %q: %v", via, err)
	}
	return n
}

func timeIt(fn func()) time.Duration {
	start := time.Now()
	fn()
	return time.Since(start)
}
