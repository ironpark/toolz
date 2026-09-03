package board

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/watch"
)

// poll 은 한 주기를 돌고 살찌운 이벤트를 돌려준다.
func poll(t *testing.T, b *Board, p *watch.Poller) []watch.Event {
	t.Helper()
	var got []watch.Event
	if err := b.pollOnce(p, func(e watch.Event) error {
		got = append(got, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

// 첫 주기는 베이스라인만 잡는다. 기존 ref 를 created 로 쏟지 않는다.
func TestFirstPollIsBaselineOnly(t *testing.T) {
	b := initBoard(t)
	mustAdd(t, b, AddOptions{Title: "이미 있음"})
	mustAdd(t, b, AddOptions{Title: "이것도"})

	p := b.Poller(WatchOptions{})
	if events := poll(t, b, p); len(events) != 0 {
		t.Fatalf("첫 주기가 %d개를 쏟았습니다: %v", len(events), events)
	}
	if !p.Started() {
		t.Fatal("베이스라인이 잡히지 않았습니다")
	}
}

// T7.1 이슈 생성은 created 이벤트다.
func TestWatchReportsCreated(t *testing.T) {
	b := initBoard(t)
	p := b.Poller(WatchOptions{})
	poll(t, b, p)

	issue := mustAdd(t, b, AddOptions{Title: "새 이슈"})
	events := poll(t, b, p)
	if len(events) != 1 {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	got := events[0]
	if got.Kind != watch.KindCreated || got.ID != issue.ID || got.Status != "open" {
		t.Fatalf("event = %+v", got)
	}
	if got.Old != "" || got.New != issue.Commit.String() {
		t.Fatalf("old=%q new=%q, want new=%s", got.Old, got.New, issue.Commit)
	}
}

// T7.2 상태 변경은 updated 이벤트이고 old/new OID 를 담는다.
func TestWatchReportsUpdatedWithOIDs(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	p := b.Poller(WatchOptions{})
	poll(t, b, p)

	claimed, err := b.Transition(ActionClaim, issue.ID, TransitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events := poll(t, b, p)
	if len(events) != 1 {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	got := events[0]
	if got.Kind != watch.KindUpdated || got.Status != "claimed" {
		t.Fatalf("event = %+v", got)
	}
	if got.Old != issue.Commit.String() || got.New != claimed.Commit.String() {
		t.Fatalf("old=%s new=%s, want %s → %s", got.Old, got.New, issue.Commit, claimed.Commit)
	}
}

// T7.3 archive 이동은 deleted + created 로 보인다.
func TestWatchReportsArchiveMove(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	transitionAll(t, b, issue.ID, ActionStart)
	p := b.Poller(WatchOptions{})
	poll(t, b, p)

	transitionAll(t, b, issue.ID, ActionDone)
	events := poll(t, b, p)
	if len(events) != 2 {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	// 정렬은 ref 순이므로 archive/ 가 먼저다.
	if events[0].Ref != refstore.Archive+issue.ID || events[0].Kind != watch.KindCreated {
		t.Fatalf("events[0] = %+v", events[0])
	}
	if events[0].Status != "done" || events[0].ID != issue.ID {
		t.Fatalf("events[0] = %+v", events[0])
	}
	if events[1].Ref != refstore.Issues+issue.ID || events[1].Kind != watch.KindDeleted {
		t.Fatalf("events[1] = %+v", events[1])
	}
	if events[1].New != "" || events[1].ID != issue.ID {
		t.Fatalf("events[1] = %+v", events[1])
	}
}

// T7.4 변경이 없으면 이벤트도 없다.
func TestWatchReportsNothingWhenIdle(t *testing.T) {
	b := initBoard(t)
	mustAdd(t, b, AddOptions{Title: "대상"})
	p := b.Poller(WatchOptions{})
	poll(t, b, p)

	for range 3 {
		if events := poll(t, b, p); len(events) != 0 {
			t.Fatalf("변경이 없는데 %v", events)
		}
	}
}

// T7.5 pack-refs 뒤에도 정상 감지한다.
//
// mtime 이나 inotify 로 구현했다면 여기서 깨진다. pack-refs 는 loose 파일을
// 통째로 없애므로 파일 단위 감시가 조용히 무력해진다 (§6.2).
func TestWatchSurvivesPackRefs(t *testing.T) {
	b, dir := initBoardDir(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	p := b.Poller(WatchOptions{})
	poll(t, b, p)

	runGit(t, dir, "pack-refs", "--all")
	if loose := filepath.Join(dir, ".git", "refs", "ppwk", "issues", issue.ID); fileExists(loose) {
		t.Fatalf("pack-refs 후에도 loose 파일이 남아 있어 이 테스트가 무의미합니다: %s", loose)
	}

	// packed 상태에서 변경한다. 새 저장소 핸들로 보는 것이 아니라 같은
	// poller 가 계속 봐야 한다 — watch 는 오래 사는 프로세스다.
	if _, err := b.Transition(ActionClaim, issue.ID, TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	events := poll(t, b, p)
	if len(events) != 1 || events[0].Kind != watch.KindUpdated {
		t.Fatalf("이벤트 = %v", events)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// T7.6 한 주기에 여러 변경이 나면 전부 보고한다.
func TestWatchReportsAllChangesInOneCycle(t *testing.T) {
	b := initBoard(t)
	existing := mustAdd(t, b, AddOptions{Title: "이미 있음"})
	p := b.Poller(WatchOptions{})
	poll(t, b, p)

	created := mustAdd(t, b, AddOptions{Title: "새 이슈"})
	another := mustAdd(t, b, AddOptions{Title: "또 하나"})
	if _, err := b.Transition(ActionClaim, existing.ID, TransitionOptions{}); err != nil {
		t.Fatal(err)
	}

	events := poll(t, b, p)
	if len(events) != 3 {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	byID := map[string]watch.Event{}
	for _, e := range events {
		byID[e.ID] = e
	}
	for id, kind := range map[string]string{
		existing.ID: watch.KindUpdated, created.ID: watch.KindCreated, another.ID: watch.KindCreated,
	} {
		if byID[id].Kind != kind {
			t.Fatalf("%s = %+v, want %s", id, byID[id], kind)
		}
	}
}

// 한 주기 안의 A→B→A 는 변경 없음으로 보인다. 문서화된 동작이다.
func TestWatchCollapsesRoundTripWithinCycle(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	p := b.Poller(WatchOptions{})
	poll(t, b, p)

	// claim 했다가 release 하면 상태는 open 으로 돌아오지만 commit 은 앞으로
	// 나아간다. ref 는 바뀌었으므로 updated 하나가 나온다 — 이벤트 두 개가
	// 아니라 최종 상태 하나다.
	transitionAll(t, b, issue.ID, ActionClaim, ActionRelease)
	events := poll(t, b, p)
	if len(events) != 1 || events[0].Status != "open" {
		t.Fatalf("이벤트 = %v", events)
	}
}

// --filter 는 볼 범위를 좁힌다.
func TestWatchFilterNarrowsScope(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	transitionAll(t, b, issue.ID, ActionStart)
	p := b.Poller(WatchOptions{Filter: refstore.Archive})
	poll(t, b, p)

	// issues/ 안의 변경은 보이지 않는다.
	mustAdd(t, b, AddOptions{Title: "안 보임"})
	if events := poll(t, b, p); len(events) != 0 {
		t.Fatalf("필터 밖 변경이 보였습니다: %v", events)
	}
	transitionAll(t, b, issue.ID, ActionDone)
	events := poll(t, b, p)
	if len(events) != 1 || events[0].Kind != watch.KindCreated {
		t.Fatalf("이벤트 = %v", events)
	}
}

// ctx 취소는 오류가 아니라 정상 종료다 (SIGINT).
func TestWatchStopsOnContextCancel(t *testing.T) {
	b := initBoard(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- b.Watch(ctx, WatchOptions{Interval: time.Millisecond}, func(watch.Event) error { return nil })
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("취소 후에도 끝나지 않았습니다")
	}
}

// 조회 오류는 삼키지 않고 그대로 올린다 (저장소 삭제 등).
func TestWatchPropagatesListError(t *testing.T) {
	boom := errors.New("저장소가 사라졌습니다")
	p := &watch.Poller{Lister: failingLister{err: boom}}
	if _, err := p.Poll(); !errors.Is(err, boom) {
		t.Fatalf("Poll() = %v, want %v", err, boom)
	}
}

type failingLister struct{ err error }

func (l failingLister) List(string) ([]refstore.RefEntry, error) { return nil, l.err }

// §6.2 는 파일 감시를 금지한다. 코드가 그것을 다시 들이지 않는지 본다.
//
// 되돌리기 쉬운 종류의 결정이라 회귀 테스트로 못박는다. 여기서 걸리는 실수는
// 리뷰로는 잘 보이지 않고, 실패도 pack-refs 를 돌린 뒤에야 드러난다.
func TestNoFileWatching(t *testing.T) {
	banned := []string{"fsnotify", "inotify", "modtime", "kqueue", "fanotify"}
	for _, dir := range []string{"../watch", "."} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			source := strings.ToLower(stripComments(string(data)))
			for _, word := range banned {
				if strings.Contains(source, word) {
					t.Fatalf("%s 가 %q 를 씁니다 — §6.2 는 파일 감시를 금지합니다",
						filepath.Join(dir, name), word)
				}
			}
		}
	}
}

// stripComments 는 주석을 지운다. 금지어를 설명하는 주석이 스스로를 걸지
// 않게 하기 위한 것이다.
func stripComments(source string) string {
	var out strings.Builder
	for line := range strings.Lines(source) {
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx] + "\n"
		}
		out.WriteString(line)
	}
	return out.String()
}
