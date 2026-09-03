// Package board 는 보드의 도메인 로직이다 (design §14.8).
//
// RefStore 인터페이스에만 의존한다. 단계 1 시점에는 생성과 조회만 있고,
// 상태 전이·gate·next 는 뒤 단계에서 들어온다.
package board

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// Board 는 한 저장소의 보드다.
type Board struct {
	// store 는 ref 갱신 경로다. 인터페이스인 이유는 결함 주입 때문이다 —
	// 결정적 동시성 테스트가 여기에 지연과 실패를 끼워 넣는다 (D2.1).
	store refstore.RefStore
	// git 은 ref 밖의 git 접근(설정 등)이다. 결함 주입 대상이 아니다.
	git      *refstore.ExecRefStore
	repo     *git.Repository
	identity session.Identity
	// backoff 는 CAS 재시도 정책이다.
	backoff Backoff
	// root 는 저장소의 작업 트리 최상단이다. 에이전트 문서가 여기 놓인다.
	root string
}

// Open 은 path 의 보드를 연다. init 여부는 확인하지 않는다.
func Open(path string, ident session.Identity) (*Board, error) {
	store, err := refstore.NewExecRefStore(path)
	if err != nil {
		return nil, err
	}
	root, err := refstore.WorktreeRoot(path)
	if err != nil {
		return nil, err
	}
	return &Board{
		store:    store,
		git:      store,
		repo:     store.Repo(),
		identity: ident,
		root:     root,
		backoff:  DefaultBackoff(),
	}, nil
}

// Store 는 ref 저장소를 돌려준다.
func (b *Board) Store() refstore.RefStore { return b.store }

// WithStore 는 ref 저장소를 바꾼 사본을 돌려준다.
//
// 결함 주입 테스트(D2.1)를 위한 것이다. 원본 Board 는 그대로 둔다.
func (b *Board) WithStore(store refstore.RefStore) *Board {
	clone := *b
	clone.store = store
	return &clone
}

// WithBackoff 는 재시도 정책을 바꾼 사본을 돌려준다. 테스트가 대기를 없앤다.
func (b *Board) WithBackoff(backoff Backoff) *Board {
	clone := *b
	clone.backoff = backoff
	return &clone
}

// Identity 는 이 실행의 주체다.
func (b *Board) Identity() session.Identity { return b.identity }

// Root 는 작업 트리 최상단이다.
func (b *Board) Root() string { return b.root }

// Initialized 는 meta/schema 가 있는지로 init 여부를 판단한다.
func (b *Board) Initialized() (bool, error) {
	_, err := b.store.Get(refstore.Schema)
	switch {
	case err == nil:
		return true, nil
	case isNotFound(err):
		return false, nil
	default:
		return false, err
	}
}

// requireWritable 은 쓰기 전에 스키마 버전을 확인한다 (§9.4).
//
// 보드가 아는 것보다 높은 버전이면 읽기만 허용한다. 여러 에이전트가 섞인
// 버전으로 도는 상황에서 데이터를 깨뜨리지 않기 위함이다.
func (b *Board) requireWritable() error {
	version, err := b.SchemaVersion()
	if err != nil {
		return err
	}
	if version > model.SchemaVersion {
		return fmt.Errorf("%w: 보드 스키마 %d 는 이 CLI(%d)보다 높습니다. 읽기만 가능합니다 — 업그레이드하세요",
			ErrSchemaTooNew, version, model.SchemaVersion)
	}
	return nil
}
