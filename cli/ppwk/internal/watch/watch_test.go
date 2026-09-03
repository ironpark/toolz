package watch

import (
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// fakeLister 는 미리 정해둔 목록을 차례로 돌려준다.
type fakeLister struct {
	rounds [][]refstore.RefEntry
	calls  int
	prefix string
}

func (f *fakeLister) List(prefix string) ([]refstore.RefEntry, error) {
	f.prefix = prefix
	round := f.rounds[min(f.calls, len(f.rounds)-1)]
	f.calls++
	return round, nil
}

func hash(s string) plumbing.Hash { return plumbing.NewHash(s) }

const (
	oidA = "1111111111111111111111111111111111111111"
	oidB = "2222222222222222222222222222222222222222"
)

func entries(pairs ...string) []refstore.RefEntry {
	out := make([]refstore.RefEntry, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, refstore.RefEntry{Ref: pairs[i], Hash: hash(pairs[i+1])})
	}
	return out
}

// 빈 저장소에서 시작해도 첫 주기는 베이스라인이다.
//
// snapshot 을 nil 과 빈 map 으로 구분하지 않으면 여기서 깨진다 — 빈 저장소의
// 첫 주기가 곧바로 "비교" 로 취급되어 두 번째 주기의 생성이 유실된다.
func TestBaselineOnEmptyRepository(t *testing.T) {
	l := &fakeLister{rounds: [][]refstore.RefEntry{
		{},
		entries("refs/ppwk/issues/T001", oidA),
	}}
	p := &Poller{Lister: l}

	if events, err := p.Poll(); err != nil || len(events) != 0 {
		t.Fatalf("첫 주기 = %v, %v", events, err)
	}
	events, err := p.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != KindCreated {
		t.Fatalf("두 번째 주기 = %v", events)
	}
}

func TestDiffKinds(t *testing.T) {
	l := &fakeLister{rounds: [][]refstore.RefEntry{
		entries("refs/ppwk/issues/T001", oidA, "refs/ppwk/issues/T002", oidA),
		entries("refs/ppwk/issues/T002", oidB, "refs/ppwk/issues/T003", oidA),
	}}
	p := &Poller{Lister: l}
	if _, err := p.Poll(); err != nil {
		t.Fatal(err)
	}
	events, err := p.Poll()
	if err != nil {
		t.Fatal(err)
	}

	// ref 순으로 정렬된다. 순서가 흔들리면 출력을 비교할 수 없다.
	want := []Event{
		{Ref: "refs/ppwk/issues/T001", Old: oidA, Kind: KindDeleted},
		{Ref: "refs/ppwk/issues/T002", Old: oidA, New: oidB, Kind: KindUpdated},
		{Ref: "refs/ppwk/issues/T003", New: oidA, Kind: KindCreated},
	}
	if len(events) != len(want) {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events[%d] = %+v, want %+v", i, events[i], want[i])
		}
	}
}

// Prefix 를 주지 않으면 refs/ppwk/ 전체를 본다.
func TestDefaultPrefix(t *testing.T) {
	l := &fakeLister{rounds: [][]refstore.RefEntry{{}}}
	p := &Poller{Lister: l}
	if _, err := p.Poll(); err != nil {
		t.Fatal(err)
	}
	if l.prefix != refstore.Prefix {
		t.Fatalf("prefix = %q, want %q", l.prefix, refstore.Prefix)
	}
}
