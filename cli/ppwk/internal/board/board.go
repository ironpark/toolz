// Package board 는 보드의 도메인 로직이다 (design §14.8).
//
// RefStore 인터페이스에만 의존한다. 단계 1 시점에는 생성과 조회만 있고,
// 상태 전이·gate·next 는 뒤 단계에서 들어온다.
package board

import (
	"fmt"
	"time"

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
	// leases 는 machine-local 생존 기록이다 (D13).
	leases *session.Registry
	// leaseSnapshot 은 생존 판정에 쓸 기록 전체를 한 번에 읽는다. 필드로 둔
	// 이유는 reap 이 소유자 수·이슈 수와 무관하게 한 번만 읽는지 테스트가
	// 셀 수 있어야 하기 때문이다 (T4.18).
	leaseSnapshot func() []model.Lease
	// allowSharedWorktree 는 Open 에서 한 번 정해진다. 전이마다 설정을
	// 다시 읽지 않도록, 이것은 연산이 아니라 이 프로세스의 성질로 둔다.
	allowSharedWorktree bool
}

// OpenOptions 는 보드를 열면서 함께 정하는 것들이다.
type OpenOptions struct {
	// Session 은 신원 결정 입력이다. git config 단계는 여기서 채워 준다.
	Session session.Options
	// AllowSharedWorktree 는 worktree 배타 확보를 건너뛴다. 플래그로 켜지지
	// 않았다면 ppwk.allowSharedWorktree 설정을 본다.
	AllowSharedWorktree bool
}

// RegisterHookSession 은 SessionStart 훅이 세션을 등록하며 hook_pid 를 남긴다
// (§3.8 층 3).
//
// 이 값이 있으면 생존 판정이 last_activity 임계값(8시간)까지 내려가지 않고
// 2단계에서 끝난다 — 감지가 즉시가 된다 (D11).
func (b *Board) RegisterHookSession(pid int) error {
	_, err := b.leases.RegisterHook(pid, b.allowSharedWorktree)
	return err
}

// OpenFor 는 신원까지 함께 결정해 보드를 연다.
//
// §0.2 의 마지막 단계인 git config 는 저장소를 연 뒤에야 읽을 수 있다. 그래서
// 결정 순서 전체를 Resolve 안에 두고, 여기서는 그 마지막 단계를 넘겨주기만
// 한다 — 순서를 두 곳에 나눠 적으면 반드시 어긋난다.
func OpenFor(path string, opts OpenOptions) (*Board, error) {
	store, root, err := openRepo(path)
	if err != nil {
		return nil, err
	}
	sopts := opts.Session
	sopts.GitConfig = func() string {
		v, _ := store.ConfigGet("ppwk.agent")
		return v
	}
	b := makeBoard(store, root, session.Resolve(sopts))
	b.allowSharedWorktree = opts.AllowSharedWorktree
	if !b.allowSharedWorktree {
		shared, err := store.ConfigBool("ppwk.allowSharedWorktree")
		if err != nil {
			return nil, err
		}
		b.allowSharedWorktree = shared
	}
	return b, nil
}

// Open 은 이미 정해진 신원으로 path 의 보드를 연다. init 여부는 확인하지 않는다.
func Open(path string, ident session.Identity) (*Board, error) {
	store, root, err := openRepo(path)
	if err != nil {
		return nil, err
	}
	return makeBoard(store, root, ident), nil
}

func openRepo(path string) (*refstore.ExecRefStore, string, error) {
	store, err := refstore.NewExecRefStore(path)
	if err != nil {
		return nil, "", err
	}
	root, err := refstore.WorktreeRoot(path)
	if err != nil {
		return nil, "", err
	}
	return store, root, nil
}

func makeBoard(store *refstore.ExecRefStore, root string, ident session.Identity) *Board {
	b := &Board{
		store:    store,
		git:      store,
		repo:     store.Repo(),
		identity: ident,
		root:     root,
		backoff:  DefaultBackoff(),
		leases:   session.NewRegistry(store.CommonDir(), root, ident),
	}
	b.leaseSnapshot = b.leases.List
	return b
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

// RegisterSession 은 쓰기 전에 하는 암묵적 세션 등록이다 (§3.6).
func (b *Board) RegisterSession() error {
	_, err := b.leases.Register(b.allowSharedWorktree)
	return err
}

// WorktreeLease 는 이 worktree 를 누가 쥐고 있는지다. 쓰지 않는다.
func (b *Board) WorktreeLease() (model.Lease, bool) { return b.leases.LookupWorktree() }

// LeaseAlive 는 기록의 생존 판정이다 (§3.6).
func (b *Board) LeaseAlive(lease model.Lease) bool { return b.leases.Alive(lease) }

// ProbeLock 은 flock 이 이 파일시스템에서 동작하는지 확인한다.
func (b *Board) ProbeLock() error { return b.leases.ProbeLock() }

// ActivityTTL 은 마지막 활동을 죽음으로 볼 때까지의 시간이다.
func (b *Board) ActivityTTL() time.Duration { return b.leases.TTL }

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
