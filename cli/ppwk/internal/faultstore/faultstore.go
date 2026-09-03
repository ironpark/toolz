// Package faultstore 는 RefStore 에 결함을 주입한다 (implementation D2.1).
//
// CAS 동시성은 fuzz 로 잡히지 않는다. fuzz 는 입력을 흔드는 도구지 스케줄링을
// 흔드는 도구가 아니다. 대신 여기서 지연과 실패를 seed 로 재현 가능하게
// 주입해, 실제 인터리빙을 기다리지 않고 경쟁 경로를 결정적으로 밟는다.
//
// RefStore 를 단계 0 에서 인터페이스로 뽑아둔 것이 여기서 값을 한다.
package faultstore

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// Config 는 주입할 결함이다.
type Config struct {
	// Seed 는 난수 시드다. 같은 seed 는 같은 결함 순서를 낸다.
	Seed uint64
	// LockBusyRate 는 CAS 가 ErrLockBusy 를 내는 확률이다 [0,1].
	LockBusyRate float64
	// BeforeCAS 는 CAS 직전에 넣는 지연이다. 경쟁 창을 넓힌다.
	BeforeCAS time.Duration
	// Hook 은 CAS 직전에 불린다. 4단계와 5단계 사이에 외부에서 ref 를 바꾸는
	// 상황을 재현할 때 쓴다 (T2.8).
	Hook func(ref string, new, old plumbing.Hash)
	// FailAfter 는 이 횟수만큼 CAS 를 성공시킨 뒤 Abort 를 낸다.
	//
	// 0 이면 쓰지 않는다. 객체 생성은 끝났는데 CAS 직전에 프로세스가 죽는
	// 상황을 흉내 낸다 — ref 가 안 바뀌었음을 확인하기 위한 것이다.
	FailAfter int
	// Abort 는 FailAfter 도달 시 낼 오류다. nil 이면 ErrAborted.
	Abort error
	// TransactionErr 는 Transaction 이 낼 오류다. nil 이면 그대로 위임한다.
	//
	// 다중 ref 이동이 실패했을 때 호출자가 무엇을 하는지 보기 위한 것이다
	// (T6.1 계열) — 이동 실패가 전이 실패로 번지면 안 된다.
	TransactionErr error
}

// Store 는 결함을 주입하는 RefStore 래퍼다. 동시 사용에 안전하다.
type Store struct {
	inner refstore.RefStore
	cfg   Config

	mu       sync.Mutex
	rand     *rand.Rand
	casCount int
	lockHits int
	txCount  int
}

var _ refstore.RefStore = (*Store)(nil)

// New 는 inner 를 감싼 결함 주입 저장소를 만든다.
func New(inner refstore.RefStore, cfg Config) *Store {
	return &Store{
		inner: inner,
		cfg:   cfg,
		rand:  rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15)),
	}
}

// Get 은 그대로 위임한다.
func (s *Store) Get(ref string) (plumbing.Hash, error) { return s.inner.Get(ref) }

// List 는 그대로 위임한다.
func (s *Store) List(prefix string) ([]refstore.RefEntry, error) { return s.inner.List(prefix) }

// Transaction 은 설정된 오류가 있으면 그것을 내고, 없으면 위임한다.
func (s *Store) Transaction(ops []refstore.RefOp) error {
	s.mu.Lock()
	s.txCount++
	err := s.cfg.TransactionErr
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.inner.Transaction(ops)
}

// TransactionCalls 는 Transaction 호출 횟수다.
func (s *Store) TransactionCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.txCount
}

// CAS 는 설정된 결함을 먼저 적용한 뒤 위임한다.
func (s *Store) CAS(ref string, new, old plumbing.Hash) error {
	if injected := s.injectBefore(); injected != nil {
		return injected
	}
	if s.cfg.BeforeCAS > 0 {
		time.Sleep(s.cfg.BeforeCAS)
	}
	if s.cfg.Hook != nil {
		s.cfg.Hook(ref, new, old)
	}
	return s.inner.CAS(ref, new, old)
}

// injectBefore 는 CAS 전에 낼 오류를 결정한다. nil 이면 통과다.
func (s *Store) injectBefore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.FailAfter > 0 && s.casCount >= s.cfg.FailAfter {
		s.casCount++
		if s.cfg.Abort != nil {
			return s.cfg.Abort
		}
		return ErrAborted
	}
	if s.cfg.LockBusyRate > 0 && s.rand.Float64() < s.cfg.LockBusyRate {
		s.lockHits++
		return refstore.ErrLockBusy
	}
	s.casCount++
	return nil
}

// LockHits 는 지금까지 주입한 ErrLockBusy 횟수다.
func (s *Store) LockHits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lockHits
}

// CASCalls 는 실제로 위임된 CAS 횟수다.
func (s *Store) CASCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.casCount
}
