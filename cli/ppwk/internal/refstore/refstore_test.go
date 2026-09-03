package refstore

import (
	"errors"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
)

// factory 는 빈 저장소와 그 저장소에서 쓸 수 있는 OID 들을 만든다.
type factory struct {
	name string
	make func(t *testing.T) (RefStore, []plumbing.Hash)
}

// factories 는 같은 스위트를 돌릴 구현 목록이다.
//
// MemRefStore 와 ExecRefStore 가 동일한 테스트를 통과해야 한다 (단계 0 Exit criteria).
func factories() []factory {
	return []factory{
		{
			name: "mem",
			make: func(t *testing.T) (RefStore, []plumbing.Hash) {
				return NewMemRefStore(), fakeHashes(4)
			},
		},
		{
			name: "exec",
			make: func(t *testing.T) (RefStore, []plumbing.Hash) {
				dir := newTestRepo(t)
				store, err := NewExecRefStore(dir)
				if err != nil {
					t.Fatalf("NewExecRefStore() = %v", err)
				}
				return store, makeCommits(t, dir, 4)
			},
		},
	}
}

// forEachStore 는 모든 구현에 같은 테스트 본문을 돌린다.
func forEachStore(t *testing.T, body func(t *testing.T, s RefStore, oid []plumbing.Hash)) {
	t.Helper()
	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) {
			s, oid := f.make(t)
			body(t, s, oid)
		})
	}
}

const testRef = "refs/ppwk/issues/T001"

// T0.1 빈 ref 에 CAS(new, ZeroHash) → 생성 성공
func TestCASCreate(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		if err := s.CAS(testRef, oid[0], plumbing.ZeroHash); err != nil {
			t.Fatalf("CAS() = %v, want nil", err)
		}
		got, err := s.Get(testRef)
		if err != nil {
			t.Fatalf("Get() = %v", err)
		}
		if got != oid[0] {
			t.Fatalf("Get() = %s, want %s", got, oid[0])
		}
	})
}

// T0.2 같은 CAS 를 두 번 → 두 번째는 ErrCASConflict
func TestCASCreateTwice(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		if err := s.CAS(testRef, oid[0], plumbing.ZeroHash); err != nil {
			t.Fatalf("첫 CAS() = %v, want nil", err)
		}
		err := s.CAS(testRef, oid[1], plumbing.ZeroHash)
		if !errors.Is(err, ErrCASConflict) {
			t.Fatalf("두 번째 CAS() = %v, want ErrCASConflict", err)
		}
	})
}

// T0.3 old 가 현재 값과 다름 → ErrCASConflict
func TestCASStaleOld(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		if err := s.CAS(testRef, oid[0], plumbing.ZeroHash); err != nil {
			t.Fatalf("CAS() = %v", err)
		}
		err := s.CAS(testRef, oid[2], oid[1])
		if !errors.Is(err, ErrCASConflict) {
			t.Fatalf("CAS() = %v, want ErrCASConflict", err)
		}
		// 값이 바뀌지 않았어야 한다.
		got, err := s.Get(testRef)
		if err != nil {
			t.Fatalf("Get() = %v", err)
		}
		if got != oid[0] {
			t.Fatalf("Get() = %s, want %s", got, oid[0])
		}
	})
}

// T0.4 존재하지 않는 ref 를 Get → ErrRefNotFound
func TestGetMissing(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		_, err := s.Get("refs/ppwk/issues/T404")
		if !errors.Is(err, ErrRefNotFound) {
			t.Fatalf("Get() = %v, want ErrRefNotFound", err)
		}
	})
}

// CAS 로 ref 를 지운다.
func TestCASDelete(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		if err := s.CAS(testRef, oid[0], plumbing.ZeroHash); err != nil {
			t.Fatalf("CAS() = %v", err)
		}
		if err := s.CAS(testRef, plumbing.ZeroHash, oid[0]); err != nil {
			t.Fatalf("삭제 CAS() = %v, want nil", err)
		}
		if _, err := s.Get(testRef); !errors.Is(err, ErrRefNotFound) {
			t.Fatalf("Get() = %v, want ErrRefNotFound", err)
		}
	})
}

// T0.5 Transaction 3개 op 전부 성공
func TestTransactionAllSucceed(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		// archive 로 옮기는 형태 — create 둘, 그리고 update 하나.
		if err := s.CAS("refs/ppwk/issues/T001", oid[0], plumbing.ZeroHash); err != nil {
			t.Fatalf("준비 CAS() = %v", err)
		}
		ops := []RefOp{
			{Kind: OpCreate, Ref: "refs/ppwk/archive/T001", New: oid[1]},
			{Kind: OpDelete, Ref: "refs/ppwk/issues/T001", Old: oid[0]},
			{Kind: OpCreate, Ref: "refs/ppwk/issues/T002", New: oid[2]},
		}
		if err := s.Transaction(ops); err != nil {
			t.Fatalf("Transaction() = %v, want nil", err)
		}
		if _, err := s.Get("refs/ppwk/issues/T001"); !errors.Is(err, ErrRefNotFound) {
			t.Fatalf("T001 이 남아 있습니다: %v", err)
		}
		if got, err := s.Get("refs/ppwk/archive/T001"); err != nil || got != oid[1] {
			t.Fatalf("archive/T001 = %s, %v", got, err)
		}
		if got, err := s.Get("refs/ppwk/issues/T002"); err != nil || got != oid[2] {
			t.Fatalf("T002 = %s, %v", got, err)
		}
	})
}

// T0.6 Transaction 중 하나가 CAS 위반 → 전부 롤백
func TestTransactionRollback(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		if err := s.CAS("refs/ppwk/issues/T001", oid[0], plumbing.ZeroHash); err != nil {
			t.Fatalf("준비 CAS() = %v", err)
		}
		ops := []RefOp{
			{Kind: OpCreate, Ref: "refs/ppwk/issues/T002", New: oid[1]},
			// 현재 값은 oid[0] 이므로 이 op 는 반드시 실패한다.
			{Kind: OpUpdate, Ref: "refs/ppwk/issues/T001", New: oid[2], Old: oid[3]},
		}
		if err := s.Transaction(ops); err == nil {
			t.Fatal("Transaction() = nil, want error")
		}
		// 앞의 op 도 적용되지 않았어야 한다.
		if _, err := s.Get("refs/ppwk/issues/T002"); !errors.Is(err, ErrRefNotFound) {
			t.Fatalf("T002 가 생겼습니다 — 롤백되지 않음: %v", err)
		}
		if got, _ := s.Get("refs/ppwk/issues/T001"); got != oid[0] {
			t.Fatalf("T001 = %s, want %s — 롤백되지 않음", got, oid[0])
		}
	})
}

// List 는 prefix 로 거른다.
func TestListPrefix(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		if err := s.CAS("refs/ppwk/issues/T001", oid[0], plumbing.ZeroHash); err != nil {
			t.Fatalf("CAS() = %v", err)
		}
		if err := s.CAS("refs/ppwk/archive/T002", oid[1], plumbing.ZeroHash); err != nil {
			t.Fatalf("CAS() = %v", err)
		}
		entries, err := s.List(Issues)
		if err != nil {
			t.Fatalf("List() = %v", err)
		}
		if len(entries) != 1 || entries[0].Ref != "refs/ppwk/issues/T001" {
			t.Fatalf("List(%q) = %v, want T001 하나", Issues, entries)
		}
		all, err := s.List(Prefix)
		if err != nil {
			t.Fatalf("List() = %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("List(%q) = %v, want 2개", Prefix, all)
		}
	})
}

// 잘못된 ref 이름은 저장소에 닿기 전에 거부된다.
func TestRejectsBadRefName(t *testing.T) {
	forEachStore(t, func(t *testing.T, s RefStore, oid []plumbing.Hash) {
		if err := s.CAS("refs/ppwk/issues/../evil", oid[0], plumbing.ZeroHash); err == nil {
			t.Fatal("CAS() = nil, want error")
		}
	})
}
