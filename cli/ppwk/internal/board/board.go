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
	store    *refstore.ExecRefStore
	repo     *git.Repository
	identity session.Identity
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
		repo:     store.Repo(),
		identity: ident,
		root:     root,
	}, nil
}

// Store 는 ref 저장소를 돌려준다.
func (b *Board) Store() *refstore.ExecRefStore { return b.store }

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
		return fmt.Errorf("보드 스키마 %d 는 이 CLI(%d)보다 높습니다. 읽기만 가능합니다 — 업그레이드하세요",
			version, model.SchemaVersion)
	}
	return nil
}
