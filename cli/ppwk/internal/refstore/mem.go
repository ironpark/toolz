package refstore

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/go-git/go-git/v6/plumbing"
)

// MemRefStore 는 테스트용 구현이다.
//
// 진짜 mutex 로 원자성을 보장한다. 프로세스 간 경쟁은 재현하지 못하므로
// ExecRefStore 를 대체하지 않는다 — 도메인 테스트를 빠르게 돌리는 용도다.
type MemRefStore struct {
	mu   sync.Mutex
	refs map[string]plumbing.Hash
}

var _ RefStore = (*MemRefStore)(nil)

// NewMemRefStore 는 빈 저장소를 만든다.
func NewMemRefStore() *MemRefStore {
	return &MemRefStore{refs: make(map[string]plumbing.Hash)}
}

// Get 은 ref 가 가리키는 해시를 돌려준다.
func (s *MemRefStore) Get(ref string) (plumbing.Hash, error) {
	if err := ValidateRefName(ref); err != nil {
		return plumbing.ZeroHash, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	h, ok := s.refs[ref]
	if !ok {
		return plumbing.ZeroHash, fmt.Errorf("%s: %w", ref, ErrRefNotFound)
	}
	return h, nil
}

// List 는 prefix 로 시작하는 ref 를 이름 순으로 돌려준다.
func (s *MemRefStore) List(prefix string) ([]RefEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var entries []RefEntry
	for ref, h := range s.refs {
		if strings.HasPrefix(ref, prefix) {
			entries = append(entries, RefEntry{Ref: ref, Hash: h})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Ref < entries[j].Ref })
	return entries, nil
}

// CAS 는 ref 가 old 일 때만 new 로 바꾼다.
func (s *MemRefStore) CAS(ref string, new, old plumbing.Hash) error {
	if err := ValidateRefName(ref); err != nil {
		return err
	}
	if new.IsZero() && old.IsZero() {
		return fmt.Errorf("%s: new 와 old 가 모두 비어 있습니다", ref)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.check(ref, old); err != nil {
		return err
	}
	s.apply(ref, new)
	return nil
}

// Transaction 은 전부 검사한 뒤 전부 적용한다.
func (s *MemRefStore) Transaction(ops []RefOp) error {
	if len(ops) == 0 {
		return nil
	}
	for _, op := range ops {
		if err := ValidateRefName(op.Ref); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// 하나라도 어긋나면 아무것도 바꾸지 않는다.
	for _, op := range ops {
		switch op.Kind {
		case OpCreate:
			if err := s.check(op.Ref, plumbing.ZeroHash); err != nil {
				return err
			}
		case OpUpdate, OpDelete:
			if err := s.check(op.Ref, op.Old); err != nil {
				return err
			}
		default:
			return fmt.Errorf("알 수 없는 연산: %d", op.Kind)
		}
	}
	for _, op := range ops {
		if op.Kind == OpDelete {
			s.apply(op.Ref, plumbing.ZeroHash)
			continue
		}
		s.apply(op.Ref, op.New)
	}
	return nil
}

// check 는 현재 값이 old 와 같은지 본다. 호출자가 mu 를 쥐고 있어야 한다.
func (s *MemRefStore) check(ref string, old plumbing.Hash) error {
	current, exists := s.refs[ref]
	switch {
	case old.IsZero() && exists:
		return fmt.Errorf("%s: %w", ref, ErrCASConflict)
	case !old.IsZero() && !exists:
		return fmt.Errorf("%s: %w", ref, ErrCASConflict)
	case !old.IsZero() && current != old:
		return fmt.Errorf("%s: %w", ref, ErrCASConflict)
	}
	return nil
}

// apply 는 값을 쓴다. 호출자가 mu 를 쥐고 있어야 한다.
func (s *MemRefStore) apply(ref string, new plumbing.Hash) {
	if new.IsZero() {
		delete(s.refs, ref)
		return
	}
	s.refs[ref] = new
}
