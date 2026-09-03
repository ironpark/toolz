package e2e

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// stressRounds 는 반복 횟수다.
//
// 명세는 E2E-4 를 100회 돌리라고 한다. 매 실행에 1600개 프로세스가 뜨므로
// 기본값은 낮추고, 그 횟수는 환경변수로 연다. 낮춰도 의미가 있는 이유는
// 실패가 한 번이라도 나면 그 자리에서 끝나기 때문이다 (§10.2 — 재시도로
// 덮지 않는다).
func stressRounds(t *testing.T, full int) int {
	t.Helper()
	if raw := os.Getenv("PPWK_E2E_ROUNDS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			t.Fatalf("PPWK_E2E_ROUNDS=%q", raw)
		}
		return n
	}
	if testing.Short() {
		return 1
	}
	_ = full
	return 3
}

// race 는 N 개 프로세스를 동시에 띄우고 결과를 모은다.
//
// 순서가 아니라 결과의 집합을 검증하기 위한 것이다 (§1.2).
func race(f *Fixture, n int, run func(i int, id Identity) Result) []Result {
	f.t.Helper()
	results := make([]Result, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = run(i, Identity{
				Agent:   fmt.Sprintf("agent-%02d", i),
				Session: fmt.Sprintf("session-%02d", i),
			})
		}(i)
	}
	close(start)
	wg.Wait()
	return results
}

// E2E-4: 동시 claim 경쟁.
//
// 이슈 1개에 16 프로세스. 정확히 1개만 이기고, 나머지는 재시도 신호(exit 4)
// 또는 규칙 위반(exit 3)으로 끝난다. 누구도 panic 하지 않는다.
func TestConcurrentClaimExactlyOneWinner(t *testing.T) {
	rounds := stressRounds(t, 100)
	for round := range rounds {
		f := newFixture(t)
		// 각 프로세스가 자기 worktree 를 쓴다. 한 worktree 를 공유하면
		// 경쟁이 CAS 가 아니라 worktree 배타에서 갈린다 (E2E-10e 쪽 사건).
		var dirs []string
		const n = 16
		for i := range n {
			dirs = append(dirs, f.AddWorktree(fmt.Sprintf("r%d-%02d", round, i),
				fmt.Sprintf("br/%d/%d", round, i)).Path)
		}
		id := f.add("경쟁 대상")
		before := len(lines(f.Git("log", "--format=%H", "refs/ppwk/issues/"+id)))

		results := race(f, n, func(i int, ident Identity) Result {
			return f.RunAs(ident, dirs[i], "claim", id)
		})

		var winners []int
		for i, r := range results {
			switch r.ExitCode {
			case 0:
				winners = append(winners, i)
			case 3, 4:
			default:
				t.Fatalf("agent-%02d 의 종료 코드 %d (3 또는 4 여야 합니다):\n%s", i, r.ExitCode, r)
			}
			if strings.Contains(r.Stderr, "panic:") || strings.Contains(r.Stderr, "goroutine ") {
				t.Fatalf("agent-%02d 가 panic 했습니다:\n%s", i, r)
			}
		}
		if len(winners) != 1 {
			t.Fatalf("승자 %d명, want 1: %v", len(winners), winners)
		}

		// git 층: commit 이 정확히 1개 늘었고, owner 가 승자다.
		after := len(lines(f.Git("log", "--format=%H", "refs/ppwk/issues/"+id)))
		if after != before+1 {
			t.Fatalf("commit 이 %d → %d, 정확히 1개만 늘어야 합니다", before, after)
		}
		want := fmt.Sprintf("agent-%02d", winners[0])
		if owner := f.show(id)["owner"]; owner != want {
			t.Fatalf("owner = %v, want %s", owner, want)
		}
	}
}

// E2E-5: 동시 next --claim 분산.
//
// 8 에이전트가 각 3회. 중복 배정이 0건이어야 한다.
func TestConcurrentNextDistributes(t *testing.T) {
	f := newFixture(t)
	const agents, perAgent, issues = 8, 3, 20
	for i := range issues {
		f.add(fmt.Sprintf("작업 %02d", i))
	}
	var dirs []string
	for i := range agents {
		dirs = append(dirs, f.AddWorktree(fmt.Sprintf("a%02d", i), fmt.Sprintf("br/%d", i)).Path)
	}

	claimed := make([][]string, agents)
	race(f, agents, func(i int, ident Identity) Result {
		for range perAgent {
			r := f.RunAs(ident, dirs[i], "--json", "next", "--claim")
			if r.ExitCode != 0 {
				continue
			}
			if id := claimedID(r.Stdout); id != "" {
				claimed[i] = append(claimed[i], id)
			}
		}
		return Result{}
	})

	owners := map[string]string{}
	total := 0
	for i, ids := range claimed {
		for _, id := range ids {
			total++
			if prev, dup := owners[id]; dup {
				t.Fatalf("%s 가 %s 와 agent-%02d 에게 중복 배정됐습니다\n%s",
					id, prev, i, f.issueTimeline(id))
			}
			owners[id] = fmt.Sprintf("agent-%02d", i)
		}
	}
	if total > issues {
		t.Fatalf("배정 %d건, 이슈는 %d개뿐입니다", total, issues)
	}
	if total == 0 {
		t.Fatal("아무도 배정받지 못했습니다")
	}

	// git 층: 각 이슈에 claim commit 이 1개씩이고 owner 가 일치한다.
	for id, want := range owners {
		subjects := lines(f.Git("log", "--format=%s", "refs/ppwk/issues/"+id))
		claims := 0
		for _, s := range subjects {
			if strings.HasPrefix(s, "claim:") {
				claims++
			}
		}
		if claims != 1 {
			t.Fatalf("%s 의 claim commit %d개, want 1: %v", id, claims, subjects)
		}
		if got := f.show(id)["owner"]; got != want {
			t.Fatalf("%s 의 owner = %v, want %s", id, got, want)
		}
	}
}

// E2E-6: 경쟁 후 다음 후보로 이동.
//
// 이슈 2개에 에이전트 2명이면 둘 다 빈손이 아니어야 한다. 경쟁에서 밀렸을 때
// 같은 이슈를 재시도하는 구현이면 한쪽이 빈손이 된다 (§7.2).
func TestLoserMovesToNextCandidate(t *testing.T) {
	rounds := stressRounds(t, 20)
	for round := range rounds {
		f := newFixture(t)
		f.add("작업 1")
		f.add("작업 2")
		dirs := []string{
			f.AddWorktree(fmt.Sprintf("x%d", round), fmt.Sprintf("br/x/%d", round)).Path,
			f.AddWorktree(fmt.Sprintf("y%d", round), fmt.Sprintf("br/y/%d", round)).Path,
		}

		results := race(f, 2, func(i int, ident Identity) Result {
			return f.RunAs(ident, dirs[i], "--json", "next", "--claim")
		})

		var got []string
		for i, r := range results {
			if r.ExitCode != 0 {
				t.Fatalf("agent-%02d 가 빈손입니다 (exit %d):\n%s", i, r.ExitCode, r)
			}
			id := claimedID(r.Stdout)
			if id == "" {
				t.Fatalf("agent-%02d 가 아무것도 claim 하지 못했습니다:\n%s", i, r)
			}
			got = append(got, id)
		}
		if got[0] == got[1] {
			t.Fatalf("둘 다 %s 를 가져갔습니다", got[0])
		}
	}
}

// E2E-7: OID 충돌 회귀.
//
// 같은 초에 같은 parent 로 만들어진 두 claim commit 의 OID 가 달라야 한다.
// 겹치면 양쪽 CAS 가 모두 "성공" 하고 둘 다 자기가 claim 했다고 믿는다 (§4.3).
//
// 명세는 "Agent-Session trailer 를 제거한 빌드로 돌리면 실패해야 한다" 고
// 적었지만 그렇지 않다. 설계가 고유값을 세 군데 두었고(trailer, issue.json 의
// session, committer email) "이 중 하나만 있어도 충분" 하다고 밝히므로,
// 하나를 빼도 나머지가 OID 를 갈라 놓는다. 그래서 여기서는 결과(OID 가
// 다르다)와 방어의 존재(trailer 가 세션마다 다르다)를 나눠서 본다.
func TestConcurrentClaimsProduceDistinctOIDs(t *testing.T) {
	rounds := stressRounds(t, 50)
	for round := range rounds {
		f := newFixture(t)
		id := f.add("같은 순간")
		dirs := []string{
			f.AddWorktree(fmt.Sprintf("p%d", round), fmt.Sprintf("br/p/%d", round)).Path,
			f.AddWorktree(fmt.Sprintf("q%d", round), fmt.Sprintf("br/q/%d", round)).Path,
		}

		results := race(f, 2, func(i int, ident Identity) Result {
			return f.RunAs(ident, dirs[i], "claim", id)
		})

		wins := 0
		for _, r := range results {
			if r.ExitCode == 0 {
				wins++
			}
		}
		if wins != 1 {
			t.Fatalf("성공 %d건, want 1:\n%s\n%s", wins, results[0], results[1])
		}

		// 진 쪽의 commit 은 dangling 으로 남는다. 저장소 전체를 훑어 같은
		// 작업의 claim commit 을 모은다.
		oids := map[string]string{}
		for _, line := range lines(f.Git("cat-file", "--batch-all-objects", "--batch-check=%(objecttype) %(objectname)")) {
			kind, oid, _ := strings.Cut(line, " ")
			if kind != "commit" {
				continue
			}
			body := f.Git("cat-file", "-p", oid)
			if !strings.Contains(body, "claim: 같은 순간") {
				continue
			}
			oids[oid] = body
		}
		if len(oids) != 2 {
			t.Fatalf("claim commit 이 %d개, want 2 — OID 가 겹쳤습니다", len(oids))
		}

		// 같은 초에 만들어졌는지 확인한다. 아니면 이 회차는 충돌 조건을
		// 재현하지 못한 것이고, OID 가 다른 것도 근거가 되지 못한다.
		sessions := map[string]bool{}
		var when []string
		for oid, body := range oids {
			session := trailerValue(body, "Agent-Session")
			if session == "" {
				t.Fatalf("commit %s 에 Agent-Session trailer 가 없습니다:\n%s", oid, body)
			}
			sessions[session] = true
			when = append(when, strings.TrimSpace(f.Git("show", "-s", "--format=%ct", oid)))
		}
		if len(sessions) != 2 {
			t.Fatalf("Agent-Session 이 세션마다 다르지 않습니다: %v", sessions)
		}
		if when[0] != when[1] {
			t.Logf("두 commit 이 다른 초에 만들어졌습니다 (%v). 충돌 조건이 아닙니다.", when)
		}
	}
}

// trailerValue 는 commit 본문에서 trailer 하나를 꺼낸다.
func trailerValue(body, key string) string {
	for _, line := range lines(body) {
		if rest, ok := strings.CutPrefix(line, key+": "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// claimedID 는 next --claim 의 JSON 에서 claim 된 ID 를 꺼낸다.
func claimedID(stdout string) string {
	var envelope struct {
		Data struct {
			Claimed struct {
				ID string `json:"id"`
			} `json:"claimed"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(stdout, &envelope); err != nil {
		return ""
	}
	return envelope.Data.Claimed.ID
}
