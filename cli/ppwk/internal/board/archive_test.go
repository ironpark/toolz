package board

import (
	"errors"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"

	"github.com/ironpark/toolz/cli/ppwk/internal/faultstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// refExists 는 ref 하나가 있는지 본다.
func refExists(t *testing.T, b *Board, ref string) bool {
	t.Helper()
	_, err := b.Store().Get(ref)
	if err == nil {
		return true
	}
	if errors.Is(err, refstore.ErrRefNotFound) {
		return false
	}
	t.Fatalf("Get(%s) = %v", ref, err)
	return false
}

// T6.1 done 이면 archive 로 옮겨지고 issues/ 에서 사라진다.
func TestDoneMovesToArchive(t *testing.T) {
	for _, action := range []Action{ActionDone, ActionCancel} {
		t.Run(string(action), func(t *testing.T) {
			b := initBoard(t)
			issue := mustAdd(t, b, AddOptions{Title: "대상"})
			if action == ActionDone {
				transitionAll(t, b, issue.ID, ActionStart)
			}
			result, err := b.Transition(action, issue.ID, TransitionOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Archived() {
				t.Fatalf("전이 결과의 ref = %s", result.Ref)
			}
			if refExists(t, b, refstore.Issues+issue.ID) {
				t.Fatal("issues/ 에 남아 있습니다")
			}
			if !refExists(t, b, refstore.Archive+issue.ID) {
				t.Fatal("archive/ 에 없습니다")
			}
			// archive 이동 후에도 show 는 정상이다.
			shown, err := b.Show(issue.ID)
			if err != nil || !shown.Archived() {
				t.Fatalf("Show() = %v, %v", shown, err)
			}
		})
	}
}

// T6.2 이동은 원자적이다 — 양쪽에 동시에 있거나 양쪽에서 사라지지 않는다.
//
// 개별 update-ref 2회로 구현하면 이 테스트가 실패한다. 결함 주입은 CAS 를
// 세므로, 이동이 CAS 두 번이면 두 번째에서 중단되어 양쪽에 남는다. 하나의
// 트랜잭션이면 셀 CAS 자체가 없다 (§4.4).
func TestArchiveMoveIsAtomic(t *testing.T) {
	b := initBoard(t)
	issue := doneInIssues(t, b)

	// CAS 한 번 뒤에 죽는 상황에서 이동한다.
	dying := b.WithStore(faultstore.New(b.Store(), faultstore.Config{FailAfter: 1}))
	_, _ = dying.Archive(issue.ID)

	inIssues := refExists(t, b, refstore.Issues+issue.ID)
	inArchive := refExists(t, b, refstore.Archive+issue.ID)
	if inIssues == inArchive {
		t.Fatalf("issues/=%v archive/=%v — 정확히 한쪽에만 있어야 합니다", inIssues, inArchive)
	}
}

// 이동 실패가 전이 실패로 번지지 않는다.
//
// 이슈는 이미 done 이고 그 사실은 issues/ 에 정확히 기록돼 있다. 여기서
// 오류를 올리면 에이전트가 done 을 다시 시도하고, 그것은 exit 3 이 된다.
func TestArchiveFailureDoesNotFailTransition(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	transitionAll(t, b, issue.ID, ActionStart)

	blocked := b.WithStore(faultstore.New(b.Store(), faultstore.Config{
		TransactionErr: refstore.ErrLockBusy,
	}))
	result, err := blocked.Transition(ActionDone, issue.ID, TransitionOptions{})
	if err != nil {
		t.Fatalf("이동 실패가 전이 실패로 번졌습니다: %v", err)
	}
	if result.Status != model.StatusDone {
		t.Fatalf("상태 = %s, want done", result.Status)
	}
	if !refExists(t, b, refstore.Issues+issue.ID) {
		t.Fatal("이동에 실패했는데 issues/ 에서 사라졌습니다")
	}
}

// doneInIssues 는 done 이면서 issues/ 에 남아 있는 이슈를 만든다.
//
// Mutate 를 직접 쓴다. Transition 은 이동을 겸하므로, 이동 자체를 시험하려면
// 그 앞 상태를 만들 다른 길이 필요하다.
func doneInIssues(t *testing.T, b *Board) *Issue {
	t.Helper()
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	result, err := b.Mutate(Mutation{ID: issue.ID, Event: "done", Apply: func(i *model.Issue) error {
		i.Status = model.StatusDone
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// T6.3 archive 된 이슈의 history 가 보존된다.
func TestArchivedHistoryIsPreserved(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	transitionAll(t, b, issue.ID, ActionStart, ActionDone)

	events, err := b.History(issue.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	for i, want := range []string{"done", "start", "create"} {
		if got := events[i].Subject; len(got) < len(want) || got[:len(want)] != want {
			t.Fatalf("events[%d].Subject = %q, want %s 로 시작", i, got, want)
		}
	}
}

// T6.4 list 는 archive 를 제외하고, --archived 는 archive 만, --all 은 둘 다 본다.
func TestListArchiveScopes(t *testing.T) {
	b := initBoard(t)
	live := mustAdd(t, b, AddOptions{Title: "살아있음"})
	gone := mustAdd(t, b, AddOptions{Title: "끝남"})
	transitionAll(t, b, gone.ID, ActionStart, ActionDone)

	for _, tc := range []struct {
		name string
		opts ListOptions
		want []string
	}{
		{"기본", ListOptions{}, []string{live.ID}},
		{"archived", ListOptions{Archived: true}, []string{gone.ID}},
		{"all", ListOptions{All: true}, []string{live.ID, gone.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := b.List(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(entries))
			for _, e := range entries {
				got = append(got, e.ID)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("= %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// archive 에 같은 ID 가 이미 있으면 덮어쓰지 않고 거부한다.
func TestArchiveRefusesToOverwrite(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	transitionAll(t, b, issue.ID, ActionStart, ActionDone)

	// archive 에 있는 그대로 두고, issues/ 에 같은 ID 를 되살린다.
	hash, err := b.Store().Get(refstore.Archive + issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Store().CAS(refstore.Issues+issue.ID, hash, plumbing.ZeroHash); err != nil {
		t.Fatal(err)
	}

	if _, err := b.Archive(issue.ID); !errors.Is(err, ErrAlreadyArchived) {
		t.Fatalf("Archive() = %v, want ErrAlreadyArchived", err)
	}
	if !refExists(t, b, refstore.Issues+issue.ID) {
		t.Fatal("거부했는데 issues/ 에서 사라졌습니다")
	}
}

// archive 된 이슈는 되살릴 수 없다. v1 은 명시적 오류다.
func TestArchivedIssueRejectsTransitions(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	transitionAll(t, b, issue.ID, ActionStart, ActionDone)

	_, err := b.Transition(ActionClaim, issue.ID, TransitionOptions{Force: true})
	var transition *TransitionError
	if !errors.As(err, &transition) {
		t.Fatalf("Transition() = %v, want *TransitionError", err)
	}
}

// 종료 상태가 아니면 옮기지 않는다.
func TestArchiveRejectsLiveIssue(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	if _, err := b.Archive(issue.ID); !errors.Is(err, ErrNotTerminal) {
		t.Fatalf("Archive() = %v, want ErrNotTerminal", err)
	}
}

// --sweep 은 자동 이동이 실패해 남은 잔재를 걷는다.
func TestArchiveSweepCollectsLeftovers(t *testing.T) {
	b := initBoard(t)
	var stuck []string
	for range 2 {
		issue := mustAdd(t, b, AddOptions{Title: "대상"})
		transitionAll(t, b, issue.ID, ActionStart)
		blocked := b.WithStore(faultstore.New(b.Store(), faultstore.Config{
			TransactionErr: refstore.ErrLockBusy,
		}))
		if _, err := blocked.Transition(ActionDone, issue.ID, TransitionOptions{}); err != nil {
			t.Fatal(err)
		}
		stuck = append(stuck, issue.ID)
	}
	live := mustAdd(t, b, AddOptions{Title: "살아있음"})

	moved, err := b.ArchiveSweep()
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != len(stuck) {
		t.Fatalf("이동 %d건, want %d", len(moved), len(stuck))
	}
	for _, id := range stuck {
		if refExists(t, b, refstore.Issues+id) {
			t.Fatalf("%s 가 issues/ 에 남았습니다", id)
		}
	}
	if !refExists(t, b, refstore.Issues+live.ID) {
		t.Fatal("살아있는 이슈를 옮겼습니다")
	}
	if entries, err := b.List(ListOptions{Status: []model.Status{model.StatusDone}}); err != nil || len(entries) != 0 {
		t.Fatalf("done 이 issues/ 에 남았습니다: %v %v", entries, err)
	}
}
