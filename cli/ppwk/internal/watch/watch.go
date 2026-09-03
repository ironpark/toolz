// Package watch 는 ref 변경을 polling 으로 감지한다 (design §6.2).
//
// 파일 mtime 이나 inotify 를 쓰지 않는다. pack-refs 가 loose 파일을 없애고,
// reftable backend 에는 애초에 ref 별 파일이 없다 — 둘 다 조용히 깨지는
// 형태로 나타나므로 처음부터 ref 목록 자체를 비교한다.
package watch

import (
	"slices"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// Kind 는 변경의 종류다.
const (
	KindCreated = "created"
	KindUpdated = "updated"
	KindDeleted = "deleted"
)

// Event 는 ref 하나의 변경이다 (features §6).
type Event struct {
	Ref    string `json:"ref"`
	Old    string `json:"old,omitempty"`
	New    string `json:"new,omitempty"`
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status,omitempty"`
}

// Lister 는 ref 목록을 주는 것이다. refstore.RefStore 가 만족한다.
type Lister interface {
	List(prefix string) ([]refstore.RefEntry, error)
}

// Poller 는 주기마다 ref 목록을 찍어 이전 스냅샷과 비교한다.
type Poller struct {
	// Lister 는 조회 경로다.
	Lister Lister
	// Prefix 는 볼 범위다. 빈 문자열이면 refs/ppwk/ 전체다.
	Prefix string

	// snapshot 은 직전 상태다. nil 이면 아직 베이스라인이 없다는 뜻이다.
	//
	// 길이 0 인 map 과 nil 을 구분하는 것이 중요하다. 빈 저장소에서 시작해도
	// 첫 주기는 베이스라인이어야 하고, 두 번째 주기부터 보고해야 한다.
	snapshot map[string]plumbing.Hash
}

// Started 는 베이스라인이 잡혔는지다.
func (p *Poller) Started() bool { return p.snapshot != nil }

// Poll 은 한 주기를 돌고 변경을 돌려준다.
//
// 첫 호출은 베이스라인만 잡고 아무것도 보고하지 않는다. 그러지 않으면 watch 를
// 붙이는 순간 기존 이슈 전부가 created 로 쏟아진다 (features §6).
func (p *Poller) Poll() ([]Event, error) {
	prefix := p.Prefix
	if prefix == "" {
		prefix = refstore.Prefix
	}
	entries, err := p.Lister.List(prefix)
	if err != nil {
		return nil, err
	}

	current := make(map[string]plumbing.Hash, len(entries))
	for _, entry := range entries {
		current[entry.Ref] = entry.Hash
	}
	if p.snapshot == nil {
		p.snapshot = current
		return nil, nil
	}

	events := diff(p.snapshot, current)
	p.snapshot = current
	return events, nil
}

// diff 는 두 스냅샷을 비교한다.
//
// 한 주기 안의 A→B→A 는 변경 없음으로 보인다. 중간 상태 유실을 허용하는
// 대신 상태 비교만으로 성립하는 구조를 얻는다 — 이벤트 로그를 따로 두면
// 그것 자체가 유실될 수 있는 두 번째 진실이 된다.
func diff(old, current map[string]plumbing.Hash) []Event {
	var events []Event
	for ref, hash := range current {
		previous, existed := old[ref]
		switch {
		case !existed:
			events = append(events, Event{Ref: ref, New: hash.String(), Kind: KindCreated})
		case previous != hash:
			events = append(events, Event{Ref: ref, Old: previous.String(), New: hash.String(), Kind: KindUpdated})
		}
	}
	for ref, hash := range old {
		if _, still := current[ref]; !still {
			events = append(events, Event{Ref: ref, Old: hash.String(), Kind: KindDeleted})
		}
	}
	// map 순회는 순서가 없다. 정렬하지 않으면 같은 주기의 이벤트 순서가
	// 실행마다 달라져 출력을 비교할 수 없게 된다.
	slices.SortFunc(events, func(a, b Event) int {
		if a.Ref != b.Ref {
			if a.Ref < b.Ref {
				return -1
			}
			return 1
		}
		return 0
	})
	return events
}
