package board

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// allStatuses 는 전이 매트릭스를 전부 덮기 위한 상태 목록이다.
var allStatuses = []model.Status{
	model.StatusOpen, model.StatusClaimed, model.StatusWorking,
	model.StatusBlocked, model.StatusDone, model.StatusCancelled,
}

var allActions = []Action{
	ActionClaim, ActionStart, ActionDone, ActionBlock,
	ActionUnblock, ActionRelease, ActionCancel,
}

// initBoard 는 초기화만 된 빈 보드다.
func initBoard(t *testing.T) *Board {
	t.Helper()
	b, _ := initBoardDir(t)
	return b
}

// initBoardDir 은 초기화된 보드와 그 경로를 함께 돌려준다. 자식 프로세스를
// 띄우는 테스트가 경로를 필요로 한다.
func initBoardDir(t *testing.T) (*Board, string) {
	t.Helper()
	b, dir := newBoard(t)
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	return b, dir
}

// issueIn 은 주어진 상태의 이슈 하나를 만든다.
//
// 상태를 직접 써넣지 않고 실제 전이를 거쳐 만든다. 그래야 도달 불가능한
// 조합을 테스트가 만들어내지 않는다.
func issueIn(t *testing.T, b *Board, status model.Status) *Issue {
	t.Helper()
	issue, err := b.Add(AddOptions{Title: "대상 " + string(status)})
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}

	var path []Action
	switch status {
	case model.StatusOpen:
	case model.StatusClaimed:
		path = []Action{ActionClaim}
	case model.StatusWorking:
		path = []Action{ActionStart}
	case model.StatusBlocked:
		path = []Action{ActionStart, ActionBlock}
	case model.StatusDone:
		path = []Action{ActionStart, ActionDone}
	case model.StatusCancelled:
		path = []Action{ActionCancel}
	default:
		t.Fatalf("알 수 없는 상태 %q", status)
	}

	for _, a := range path {
		if issue, err = b.Transition(a, issue.ID, TransitionOptions{}); err != nil {
			t.Fatalf("%s → %s 준비 중 %s 실패: %v", issue.Status, status, a, err)
		}
	}
	if issue.Status != status {
		t.Fatalf("준비 결과 = %q, want %q", issue.Status, status)
	}
	return issue
}

// T3.1 / T3.2 전이 매트릭스 전체 조합.
//
// 상태 6개 × 명령 7개 = 42가지. 조합이 작으므로 exhaustive 가 fuzz 보다 낫다 —
// 전부 덮으면서 결정적이다. 표에 없는 칸은 예외 없이 exit 3 이어야 한다.
func TestTransitionMatrix(t *testing.T) {
	// want 는 성공해야 하는 (상태, 명령) → 도착 상태다. 나머지는 전부 거부다.
	want := map[model.Status]map[Action]model.Status{
		model.StatusOpen: {
			ActionClaim:  model.StatusClaimed,
			ActionStart:  model.StatusWorking,
			ActionCancel: model.StatusCancelled,
		},
		model.StatusClaimed: {
			ActionStart:   model.StatusWorking,
			ActionRelease: model.StatusOpen,
			ActionCancel:  model.StatusCancelled,
		},
		model.StatusWorking: {
			ActionDone:   model.StatusDone,
			ActionBlock:  model.StatusBlocked,
			ActionCancel: model.StatusCancelled,
		},
		model.StatusBlocked: {
			ActionUnblock: model.StatusWorking,
			ActionCancel:  model.StatusCancelled,
		},
		model.StatusDone:      {},
		model.StatusCancelled: {},
	}

	for _, from := range allStatuses {
		for _, act := range allActions {
			name := fmt.Sprintf("%s/%s", from, act)
			t.Run(name, func(t *testing.T) {
				b := initBoard(t)
				issue := issueIn(t, b, from)

				got, err := b.Transition(act, issue.ID, TransitionOptions{})
				to, allowed := want[from][act]

				if !allowed {
					var transition *TransitionError
					if !errors.As(err, &transition) {
						t.Fatalf("Transition() = (%v, %v), want *TransitionError", got, err)
					}
					// 상태가 바뀌지 않아야 한다.
					after, err := b.Show(issue.ID)
					if err != nil {
						t.Fatalf("Show() = %v", err)
					}
					if after.Status != from {
						t.Fatalf("거부됐는데 상태가 %q 로 바뀌었습니다", after.Status)
					}
					return
				}

				if err != nil {
					t.Fatalf("Transition() = %v, want 성공 → %s", err, to)
				}
				if got.Status != to {
					t.Fatalf("status = %q, want %q", got.Status, to)
				}
			})
		}
	}
}

// T3.1b start 는 open 에서 working 으로 한 CAS 에 간다 (claim 겸함, D16).
func TestStartFromOpenIsSingleCAS(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusOpen)

	counting := &alwaysStore{inner: b.store}
	got, err := b.WithStore(counting).Transition(ActionStart, issue.ID, TransitionOptions{})
	if err != nil {
		t.Fatalf("Transition() = %v", err)
	}
	if counting.calls != 1 {
		t.Fatalf("CAS 호출 %d회, want 1 (claim 과 start 가 한 전이여야 합니다)", counting.calls)
	}
	if got.Status != model.StatusWorking || got.Owner != b.identity.Agent {
		t.Fatalf("status=%q owner=%q, want working/%s", got.Status, got.Owner, b.identity.Agent)
	}

	// history 에 claim 이벤트가 따로 생기면 안 된다.
	events, err := b.History(issue.ID, 0)
	if err != nil {
		t.Fatalf("History() = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("이벤트 %d개, want 2 (create, start): %v", len(events), events)
	}
}

// T3.1c block 이 --on 과 --message 를 각각·함께 받는다.
func TestBlockAcceptsOnAndMessage(t *testing.T) {
	tests := []struct {
		name string
		opts func(cause string) TransitionOptions
	}{
		{"on 만", func(c string) TransitionOptions { return TransitionOptions{On: c} }},
		{"message 만", func(string) TransitionOptions { return TransitionOptions{Message: "스키마 결정 대기"} }},
		{"둘 다", func(c string) TransitionOptions {
			return TransitionOptions{On: c, Message: "스키마 결정 대기"}
		}},
		{"둘 다 없음", func(string) TransitionOptions { return TransitionOptions{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := initBoard(t)
			cause := issueIn(t, b, model.StatusOpen)
			issue := issueIn(t, b, model.StatusWorking)

			opts := tt.opts(cause.ID)
			got, err := b.Transition(ActionBlock, issue.ID, opts)
			if err != nil {
				t.Fatalf("Transition() = %v", err)
			}
			if got.Status != model.StatusBlocked {
				t.Fatalf("status = %q", got.Status)
			}
			// T3.4 --on 이 있으면 대상 ID 가 기록돼야 한다.
			if opts.On != "" && !slices.Contains(got.DependsOn, opts.On) {
				t.Fatalf("depends_on = %v, %s 가 없습니다", got.DependsOn, opts.On)
			}
			// 사유는 subject 에만 붙고 제목은 오염되지 않아야 한다.
			events, err := b.History(issue.ID, 1)
			if err != nil {
				t.Fatalf("History() = %v", err)
			}
			if opts.Message != "" && !strings.Contains(events[0].Subject, opts.Message) {
				t.Fatalf("subject = %q, 사유가 없습니다", events[0].Subject)
			}
			entries, err := b.List(ListOptions{})
			if err != nil {
				t.Fatalf("List() = %v", err)
			}
			for _, e := range entries {
				if e.ID == issue.ID && e.Title != issue.Title {
					t.Fatalf("목록 제목 = %q, want %q — 사유가 제목으로 새어 나왔습니다", e.Title, issue.Title)
				}
			}
		})
	}
}

// T3.3 done 상태에서는 cancel 을 포함해 어떤 전이도 불가하다.
//
// 멱등 성공으로 처리하지 않는다. 실수를 숨기지 않기 위함이다 (features §3).
func TestDoneRejectsEverything(t *testing.T) {
	for _, act := range allActions {
		t.Run(string(act), func(t *testing.T) {
			b := initBoard(t)
			issue := issueIn(t, b, model.StatusDone)

			_, err := b.Transition(act, issue.ID, TransitionOptions{Force: true})
			var transition *TransitionError
			if !errors.As(err, &transition) {
				t.Fatalf("Transition(%s) = %v, want *TransitionError (--force 로도 불가)", act, err)
			}
		})
	}
}

// T3.5 다른 에이전트가 소유한 이슈는 start·done 이 거부된다.
func TestForeignOwnedIssueRejected(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)

	other := *b
	other.identity.Agent = "agent-b"
	other.identity.Session = "sess-b"

	_, err := other.Transition(ActionStart, issue.ID, TransitionOptions{})
	var transition *TransitionError
	if !errors.As(err, &transition) {
		t.Fatalf("start = %v, want *TransitionError", err)
	}
	if !strings.Contains(err.Error(), b.identity.Agent) {
		t.Fatalf("오류에 소유자가 없습니다: %v", err)
	}

	// working 인 이슈의 done 도 마찬가지다.
	working := issueIn(t, b, model.StatusWorking)
	if _, err := other.Transition(ActionDone, working.ID, TransitionOptions{}); !errors.As(err, &transition) {
		t.Fatalf("done = %v, want *TransitionError", err)
	}
}

// claim 경쟁에서 진 것은 exit 3 이지 4 가 아니다.
//
// CAS 에서 진 것은 4, 이미 남이 갖고 있는 걸 뒤늦게 안 것은 3 이다. 에이전트가
// 재시도 여부를 이 구분으로 판단한다.
func TestLosingClaimIsTransitionNotConflict(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)

	other := *b
	other.identity.Agent = "agent-b"
	_, err := other.Transition(ActionClaim, issue.ID, TransitionOptions{})

	var transition *TransitionError
	var conflict *ConflictError
	if errors.As(err, &conflict) {
		t.Fatalf("claim = %v, exit 4 가 아니라 3 이어야 합니다", err)
	}
	if !errors.As(err, &transition) {
		t.Fatalf("claim = %v, want *TransitionError", err)
	}
}

// T3.6 소유자가 아니면 release 는 --force 를 요구한다.
func TestReleaseRequiresForceForOthers(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)

	other := *b
	other.identity.Agent = "agent-b"

	var transition *TransitionError
	if _, err := other.Transition(ActionRelease, issue.ID, TransitionOptions{}); !errors.As(err, &transition) {
		t.Fatalf("release = %v, want *TransitionError", err)
	}
	if !strings.Contains(fmt.Sprint(transition), "--force") {
		t.Fatalf("오류가 --force 를 안내하지 않습니다: %v", transition)
	}

	got, err := other.Transition(ActionRelease, issue.ID, TransitionOptions{Force: true})
	if err != nil {
		t.Fatalf("release --force = %v", err)
	}
	if got.Status != model.StatusOpen || got.Owner != "" || got.Session != "" {
		t.Fatalf("status=%q owner=%q session=%q", got.Status, got.Owner, got.Session)
	}
}

// --force 는 멈춘 에이전트가 붙들고 있는 working 이슈도 회수한다 (§4.5).
func TestForceReleaseWorking(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusWorking)

	var transition *TransitionError
	if _, err := b.Transition(ActionRelease, issue.ID, TransitionOptions{}); !errors.As(err, &transition) {
		t.Fatalf("release = %v, working 은 --force 없이 반납되면 안 됩니다", err)
	}
	got, err := b.Transition(ActionRelease, issue.ID, TransitionOptions{Force: true})
	if err != nil {
		t.Fatalf("release --force = %v", err)
	}
	if got.Status != model.StatusOpen {
		t.Fatalf("status = %q", got.Status)
	}
}

// cancel 은 남은 owner 를 정리한다.
func TestCancelClearsOwner(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusWorking)

	got, err := b.Transition(ActionCancel, issue.ID, TransitionOptions{})
	if err != nil {
		t.Fatalf("cancel = %v", err)
	}
	if got.Owner != "" || got.Session != "" {
		t.Fatalf("owner=%q session=%q, 둘 다 비어야 합니다", got.Owner, got.Session)
	}
}

// block 의 대상 검사 — 자기 자신, 없는 이슈, 순환.
func TestBlockTargetValidation(t *testing.T) {
	t.Run("자기 자신", func(t *testing.T) {
		b := initBoard(t)
		issue := issueIn(t, b, model.StatusWorking)
		var transition *TransitionError
		if _, err := b.Transition(ActionBlock, issue.ID, TransitionOptions{On: issue.ID}); !errors.As(err, &transition) {
			t.Fatalf("block --on self = %v, want *TransitionError", err)
		}
	})

	t.Run("없는 이슈", func(t *testing.T) {
		b := initBoard(t)
		issue := issueIn(t, b, model.StatusWorking)
		if _, err := b.Transition(ActionBlock, issue.ID, TransitionOptions{On: "T999"}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("block --on T999 = %v, want ErrNotFound", err)
		}
	})

	t.Run("순환", func(t *testing.T) {
		b := initBoard(t)
		a := issueIn(t, b, model.StatusWorking)
		c := issueIn(t, b, model.StatusWorking)

		// A 를 C 에 막는다.
		if _, err := b.Transition(ActionBlock, a.ID, TransitionOptions{On: c.ID}); err != nil {
			t.Fatalf("A block on C = %v", err)
		}
		// 이제 C 를 A 에 막으면 순환이다.
		var transition *TransitionError
		if _, err := b.Transition(ActionBlock, c.ID, TransitionOptions{On: a.ID}); !errors.As(err, &transition) {
			t.Fatalf("C block on A = %v, 순환이므로 거부돼야 합니다", err)
		}
	})
}

// 아무도 claim 하지 않은 이슈는 open 으로 남는다. 방치 자체가 정상이다.
func TestUnclaimedIssueStaysOpen(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusOpen)

	got, err := b.Show(issue.ID)
	if err != nil {
		t.Fatalf("Show() = %v", err)
	}
	if got.Status != model.StatusOpen || got.Owner != "" {
		t.Fatalf("status=%q owner=%q", got.Status, got.Owner)
	}
}

// --retry 는 기본 0 이고, 지정하면 그만큼 더 시도한다.
func TestRetryOption(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusOpen)

	always := &alwaysStore{inner: b.store, err: refstore.ErrCASConflict}
	sub := b.WithStore(always)

	if _, err := sub.Transition(ActionClaim, issue.ID, TransitionOptions{}); err == nil {
		t.Fatal("경쟁 실패인데 성공했습니다")
	}
	if always.calls != 1 {
		t.Fatalf("기본 CAS 시도 %d회, want 1 (--retry 기본 0)", always.calls)
	}

	always.calls = 0
	noWait := b.backoff
	noWait.Base = 0
	if _, err := b.WithBackoff(noWait).WithStore(always).
		Transition(ActionClaim, issue.ID, TransitionOptions{Retry: 3}); err == nil {
		t.Fatal("경쟁 실패인데 성공했습니다")
	}
	if always.calls != 4 {
		t.Fatalf("--retry 3 일 때 CAS 시도 %d회, want 4", always.calls)
	}
}

// T3.7 history 가 이벤트 순서대로 나온다.
func TestHistoryOrder(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusOpen)

	for _, act := range []Action{ActionClaim, ActionStart, ActionDone} {
		if _, err := b.Transition(act, issue.ID, TransitionOptions{}); err != nil {
			t.Fatalf("%s = %v", act, err)
		}
	}

	events, err := b.History(issue.ID, 0)
	if err != nil {
		t.Fatalf("History() = %v", err)
	}
	want := []string{"done", "start", "claim", "create"}
	if len(events) != len(want) {
		t.Fatalf("이벤트 %d개, want %d: %v", len(events), len(want), events)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(events[i].Subject, prefix+":") {
			t.Fatalf("events[%d].Subject = %q, want %q 로 시작", i, events[i].Subject, prefix)
		}
	}

	// -n 은 최신 것부터 자른다.
	short, err := b.History(issue.ID, 2)
	if err != nil {
		t.Fatalf("History(2) = %v", err)
	}
	if len(short) != 2 || !strings.HasPrefix(short[0].Subject, "done:") {
		t.Fatalf("History(2) = %v", short)
	}
}

// release --mine 은 claimed 만 반납한다. working 은 건드리지 않는다 (D15).
func TestReleaseMineOnlyClaimed(t *testing.T) {
	b := initBoard(t)
	claimed := issueIn(t, b, model.StatusClaimed)
	working := issueIn(t, b, model.StatusWorking)

	released, err := b.ReleaseMine(TransitionOptions{})
	if err != nil {
		t.Fatalf("ReleaseMine() = %v", err)
	}
	if len(released) != 1 || released[0].ID != claimed.ID {
		t.Fatalf("반납 = %v, want [%s]", released, claimed.ID)
	}

	after, err := b.Show(working.ID)
	if err != nil {
		t.Fatalf("Show() = %v", err)
	}
	if after.Status != model.StatusWorking {
		t.Fatalf("working 이슈가 %q 로 바뀌었습니다", after.Status)
	}
}
