// Package refstore 는 ref 를 원자적으로 바꾸는 방법 하나만 다룬다 (design §14.5).
//
// 이슈도 상태도 JSON 도 여기 없다. 도메인 로직은 이 인터페이스에만 의존하며,
// go-git 타입에 직접 결합되지 않는다.
package refstore

import "github.com/go-git/go-git/v6/plumbing"

// RefStore 는 ref 저장소다. 호출자는 어느 구현인지 알 필요가 없다.
type RefStore interface {
	// Get 은 ref 가 가리키는 해시를 돌려준다. 없으면 ErrRefNotFound.
	Get(ref string) (plumbing.Hash, error)
	// CAS 는 ref 가 old 일 때만 new 로 바꾼다.
	//
	// old 가 ZeroHash 면 생성(create-only), new 가 ZeroHash 면 삭제다.
	// 경쟁에서 밀리면 ErrCASConflict, 잠금을 못 잡으면 ErrLockBusy.
	CAS(ref string, new, old plumbing.Hash) error
	// Transaction 은 ops 를 전부 적용하거나 전부 적용하지 않는다.
	Transaction(ops []RefOp) error
	// List 는 prefix 로 시작하는 ref 를 돌려준다.
	List(prefix string) ([]RefEntry, error)
}

// RefOpKind 는 트랜잭션 안 한 연산의 종류다.
type RefOpKind int

const (
	// OpUpdate 는 Old 에서 New 로 바꾼다.
	OpUpdate RefOpKind = iota
	// OpCreate 는 ref 가 없을 때만 만든다.
	OpCreate
	// OpDelete 는 Old 일 때만 지운다.
	OpDelete
)

func (k RefOpKind) String() string {
	switch k {
	case OpUpdate:
		return "update"
	case OpCreate:
		return "create"
	case OpDelete:
		return "delete"
	}
	return "unknown"
}

// RefOp 는 트랜잭션의 한 연산이다.
type RefOp struct {
	Kind RefOpKind
	Ref  string
	New  plumbing.Hash
	Old  plumbing.Hash
}

// RefEntry 는 List 결과 한 줄이다.
type RefEntry struct {
	Ref  string
	Hash plumbing.Hash
}
