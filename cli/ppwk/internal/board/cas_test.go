package board

import (
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/ironpark/toolz/cli/ppwk/internal/faultstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// startMutation 은 단계 2 의 유일한 전이다.
//
// 진짜 전이 규칙은 단계 3 에서 들어온다. 여기서는 프로토콜만 시험하므로
// "open 일 때만 working 으로" 라는 규칙 하나면 충분하다 — 경쟁에서 밀린 쪽이
// 재시도가 아니라 규칙 위반으로 끝나는 경로를 만들어 주기 때문이다.
func startMutation() Mutation {
	return Mutation{
		Event: "start",
		Apply: func(issue *model.Issue) error {
			if issue.Status != model.StatusOpen {
				return &TransitionError{
					ID: issue.ID, From: issue.Status, To: model.StatusWorking,
					Reason: "open 상태에서만 시작할 수 있습니다",
				}
			}
			issue.Status = model.StatusWorking
			issue.Owner = "agent-a"
			return nil
		},
	}
}

// fastBackoff 는 대기 없이 도는 정책이다. 테스트가 실제로 자지 않게 한다.
func fastBackoff(cas, lock int) Backoff {
	slept := 0
	return Backoff{
		Base: time.Millisecond, Max: time.Millisecond,
		CASAttempts: cas, LockAttempts: lock,
		Sleep: func(time.Duration) { slept++ },
		Rand:  rand.New(rand.NewPCG(1, 2)),
	}
}

// seedIssue 는 초기화된 보드에 이슈 하나를 만든다.
func seedIssue(t *testing.T, b *Board) *Issue {
	t.Helper()
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	issue, err := b.Add(AddOptions{Title: "경쟁 대상"})
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}
	return issue
}

// T2.1 단일 프로세스 CAS 성공.
func TestMutateSucceeds(t *testing.T) {
	b, dir := newBoard(t)
	issue := seedIssue(t, b)

	m := startMutation()
	m.ID = issue.ID
	got, err := b.Mutate(m)
	if err != nil {
		t.Fatalf("Mutate() = %v", err)
	}
	if got.Status != model.StatusWorking {
		t.Fatalf("status = %q, want working", got.Status)
	}

	// ref 가 실제로 새 commit 을 가리켜야 한다.
	hash, err := b.store.Get(issue.Ref)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if hash != got.Commit {
		t.Fatalf("ref = %s, want %s", hash, got.Commit)
	}
	// 새 commit 의 parent 는 직전 상태여야 한다. history 가 이어진다 (§3.3).
	commit, err := object.GetCommit(b.repo.Storer, hash)
	if err != nil {
		t.Fatalf("GetCommit() = %v", err)
	}
	if len(commit.ParentHashes) != 1 || commit.ParentHashes[0] != issue.Commit {
		t.Fatalf("parents = %v, want [%s]", commit.ParentHashes, issue.Commit)
	}
	if got := runGit(t, dir, "log", "--format=%s", "-1", issue.Ref); got != "start: 경쟁 대상" {
		t.Fatalf("subject = %q", got)
	}
}

// T2.3 같은 tree·parent·시각이어도 세션이 다르면 OID 가 달라야 한다 (§4.3).
//
// content-addressed 이므로 OID 가 겹치면 양쪽 CAS 가 모두 "성공" 하고, 두
// 에이전트가 동시에 자기가 이슈를 가졌다고 믿는다.
//
// §4.3 은 방어를 세 겹으로 둔다 — trailer, issue.json 의 session 필드,
// committer email. 세 겹이 다 있으면 하나를 빼도 테스트가 통과해버려 회귀를
// 못 잡는다. 그래서 여기서는 나머지 둘을 고정하고 trailer 만 남긴다.
// issueTrailers 에서 Agent-Session 을 빼면 이 테스트가 반드시 실패한다.
func TestCommitOIDDiffersPerSession(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	// 문서·서명·시각·parent 를 전부 고정한다. 남는 변수는 trailer 뿐이다.
	doc := issue.Issue
	doc.Status = model.StatusWorking
	doc.Session = ""
	author := object.Signature{Name: "same", Email: "same@ppwk.local", When: doc.UpdatedAt.Time}

	write := func(session string) plumbing.Hash {
		t.Helper()
		hash, err := gitobj.Write(b.repo, gitobj.Commit{
			Doc:      doc,
			DocName:  gitobj.FileIssue,
			Subject:  "start: " + doc.Title,
			Trailers: issueTrailers(doc, session),
			Author:   author,
			Parent:   issue.Commit,
		})
		if err != nil {
			t.Fatalf("Write(%s) = %v", session, err)
		}
		return hash
	}

	a, c := write("session-a"), write("session-b")
	if a == c {
		t.Fatal("세션이 다른데 OID 가 같습니다 — Agent-Session trailer 가 빠졌습니다 (§4.3)")
	}
	// 테스트의 테스트: 세션이 같으면 OID 도 같아야 한다. 그래야 위 단언이
	// 세션 때문에 갈린 것이지 다른 잡음 때문이 아님이 확실해진다.
	if write("session-a") != a {
		t.Fatal("같은 입력인데 OID 가 다릅니다 — 이 테스트는 세션 차이를 증명하지 못합니다")
	}
}

// 실제 전이 경로에서도 두 에이전트의 commit 은 갈려야 한다.
//
// 위 테스트가 trailer 한 겹을 보는 것이라면, 이것은 Board 를 통과하는 전체
// 경로를 본다. 어느 겹이 남아 있든 결과는 달라야 한다.
func TestConcurrentAgentsProduceDifferentOIDs(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	build := func(agent, sess string) plumbing.Hash {
		t.Helper()
		clone := *b
		clone.identity.Agent = agent
		clone.identity.Session = sess
		doc := issue.Issue
		doc.Status = model.StatusWorking
		doc.Session = sess
		hash, err := clone.writeIssueCommit(doc, nil, "start", issue.Commit)
		if err != nil {
			t.Fatalf("writeIssueCommit(%s) = %v", agent, err)
		}
		return hash
	}

	if a, c := build("agent-a", "s1"), build("agent-b", "s2"); a == c {
		t.Fatal("두 에이전트의 commit OID 가 같습니다 (§4.3)")
	}
}

// T2.4 lock 실패를 주입하면 backoff 후 최종 성공한다.
func TestMutateRetriesOnLockBusy(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	faulty := faultstore.New(b.store, faultstore.Config{Seed: 7, LockBusyRate: 0.8})
	sub := b.WithStore(faulty).WithBackoff(fastBackoff(4, 40))

	m := startMutation()
	m.ID = issue.ID
	if _, err := sub.Mutate(m); err != nil {
		t.Fatalf("Mutate() = %v", err)
	}
	if faulty.LockHits() == 0 {
		t.Fatal("lock 실패가 한 번도 주입되지 않아 재시도 경로를 지나지 않았습니다")
	}
}

// T2.5 CAS 실패는 같은 commit 재시도가 아니라 상태 재읽기 경로로 간다.
//
// lock 실패와 CAS 실패를 뭉개면 이 차이가 사라진다 (§4.2). lock 실패는 상태가
// 그대로라 만들어 둔 commit 이 유효하지만, CAS 실패는 남이 바꿨다는 뜻이라
// 반드시 다시 읽어야 한다.
func TestCASConflictRereadsButLockBusyDoesNot(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	withFirstError := func(err error) *Board {
		return b.WithStore(&scriptedStore{inner: b.store, errs: []error{err}}).
			WithBackoff(fastBackoff(4, 4))
	}

	// CAS 실패 → Apply 가 두 번 불린다 (다시 읽었다는 뜻).
	var applies int
	m := Mutation{ID: issue.ID, Event: "start", Apply: func(i *model.Issue) error {
		applies++
		i.Status = model.StatusWorking
		return nil
	}}
	if _, err := withFirstError(refstore.ErrCASConflict).Mutate(m); err != nil {
		t.Fatalf("Mutate() = %v", err)
	}
	if applies != 2 {
		t.Fatalf("CAS 실패 후 Apply 호출 = %d, want 2 (재읽기)", applies)
	}

	// lock 실패 → Apply 는 한 번뿐이다 (같은 commit 으로 재시도).
	b2, _ := newBoard(t)
	issue2 := seedIssue(t, b2)
	applies = 0
	m.ID = issue2.ID
	store := &scriptedStore{inner: b2.store, errs: []error{refstore.ErrLockBusy}}
	if _, err := b2.WithStore(store).WithBackoff(fastBackoff(4, 4)).Mutate(m); err != nil {
		t.Fatalf("Mutate() = %v", err)
	}
	if applies != 1 {
		t.Fatalf("lock 실패 후 Apply 호출 = %d, want 1 (재계산 없음)", applies)
	}
}

// T2.6 재시도 상한을 넘기면 ConflictError 다 (§7.3, exit 4).
func TestMutateGivesUpAfterCASAttempts(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	always := &alwaysStore{inner: b.store, err: refstore.ErrCASConflict}
	m := startMutation()
	m.ID = issue.ID

	_, err := b.WithStore(always).WithBackoff(fastBackoff(3, 2)).Mutate(m)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Mutate() = %v, want *ConflictError", err)
	}
	if conflict.Attempts != 3 {
		t.Fatalf("Attempts = %d, want 3", conflict.Attempts)
	}
	// lock 실패로 상한에 닿는 경로도 같은 오류여야 한다. 둘 다 "지금은 안 된다,
	// 다시 해봐라" 이므로 호출하는 쪽이 구분할 이유가 없다.
	always.err = refstore.ErrLockBusy
	if _, err := b.WithStore(always).WithBackoff(fastBackoff(2, 2)).Mutate(m); !errors.As(err, &conflict) {
		t.Fatalf("lock 상한 초과 = %v, want *ConflictError", err)
	}
}

// T2.7 전이 규칙 위반은 재시도하지 않는다 (exit 3).
func TestTransitionViolationDoesNotRetry(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	counting := &alwaysStore{inner: b.store}
	m := startMutation()
	m.ID = issue.ID
	// 이미 working 으로 만들어 둔다.
	if _, err := b.Mutate(m); err != nil {
		t.Fatalf("준비 Mutate() = %v", err)
	}

	_, err := b.WithStore(counting).WithBackoff(fastBackoff(5, 5)).Mutate(m)
	var transition *TransitionError
	if !errors.As(err, &transition) {
		t.Fatalf("Mutate() = %v, want *TransitionError", err)
	}
	if counting.calls != 0 {
		t.Fatalf("규칙 위반인데 CAS 를 %d번 시도했습니다", counting.calls)
	}
}

// T2.8 4단계와 5단계 사이에 외부에서 ref 가 바뀌면 CAS 실패로 검출된다.
func TestExternalChangeBetweenBuildAndCAS(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	// 첫 CAS 직전에 다른 에이전트가 끼어든다.
	var once bool
	hook := func(ref string, _, _ plumbing.Hash) {
		if once {
			return
		}
		once = true
		other := *b
		other.identity.Agent = "agent-b"
		other.identity.Session = "sess-2"
		m := startMutation()
		m.ID = issue.ID
		if _, err := other.Mutate(m); err != nil {
			t.Errorf("끼어드는 Mutate() = %v", err)
		}
	}

	faulty := faultstore.New(b.store, faultstore.Config{Seed: 3, Hook: hook})
	m := startMutation()
	m.ID = issue.ID

	_, err := b.WithStore(faulty).WithBackoff(fastBackoff(4, 4)).Mutate(m)
	var transition *TransitionError
	if !errors.As(err, &transition) {
		t.Fatalf("Mutate() = %v, want *TransitionError (재읽기 후 규칙 위반)", err)
	}
	if !once {
		t.Fatal("hook 이 불리지 않았습니다")
	}
}

// ref 가 CAS 직전에 삭제되면 재읽기에서 "없음" 이 된다 (exit 5).
func TestRefDeletedBeforeCAS(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	var once bool
	hook := func(ref string, _, _ plumbing.Hash) {
		if once {
			return
		}
		once = true
		hash, err := b.store.Get(ref)
		if err != nil {
			t.Errorf("Get() = %v", err)
			return
		}
		if err := b.store.CAS(ref, plumbing.ZeroHash, hash); err != nil {
			t.Errorf("삭제 CAS = %v", err)
		}
	}

	faulty := faultstore.New(b.store, faultstore.Config{Seed: 4, Hook: hook})
	m := startMutation()
	m.ID = issue.ID

	_, err := b.WithStore(faulty).WithBackoff(fastBackoff(4, 4)).Mutate(m)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Mutate() = %v, want ErrNotFound", err)
	}
}

// 객체는 만들어졌지만 CAS 직전에 죽으면 ref 는 그대로여야 한다.
//
// 부분 상태가 생기지 않는 것이 이 설계의 강점이다 (§4.1). 명시적으로 확인한다.
func TestAbortBeforeCASLeavesRefUnchanged(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	abort := &alwaysStore{inner: b.store, err: faultstore.ErrAborted}
	m := startMutation()
	m.ID = issue.ID
	if _, err := b.WithStore(abort).WithBackoff(fastBackoff(3, 3)).Mutate(m); !errors.Is(err, faultstore.ErrAborted) {
		t.Fatalf("Mutate() = %v, want ErrAborted", err)
	}

	hash, err := b.store.Get(issue.Ref)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if hash != issue.Commit {
		t.Fatalf("ref 가 %s 로 바뀌었습니다. 원래 %s 여야 합니다", hash, issue.Commit)
	}
}

// T2.9 backoff 에 jitter 가 있어 N개가 같은 순간에 재시도하지 않는다.
func TestBackoffHasJitter(t *testing.T) {
	b := Backoff{Base: 10 * time.Millisecond, Max: time.Second}

	seen := map[time.Duration]int{}
	const samples = 200
	for range samples {
		seen[b.Duration(3)]++
	}
	// jitter 가 없으면 값이 하나뿐이다.
	if len(seen) < samples/4 {
		t.Fatalf("서로 다른 대기 시간이 %d개뿐입니다 — jitter 가 없습니다", len(seen))
	}
	// 상한을 넘지 않아야 한다.
	for d := range seen {
		if d < 0 || d >= b.Max {
			t.Fatalf("대기 %v 가 [0, %v) 밖입니다", d, b.Max)
		}
	}
	// Max 는 실제로 상한 역할을 해야 한다. 지수만 있으면 무한히 커진다.
	capped := Backoff{Base: time.Millisecond, Max: 4 * time.Millisecond}
	if got := capped.Duration(30); got >= 4*time.Millisecond {
		t.Fatalf("Duration(30) = %v, Max 를 넘었습니다", got)
	}
}

// 시계가 뒤로 가도 동작에 영향이 없어야 한다.
func TestClockGoingBackwards(t *testing.T) {
	b, _ := newBoard(t)
	issue := seedIssue(t, b)

	m := startMutation()
	m.ID = issue.ID
	// updated_at 을 미래로 밀어 둔 상태에서 전이한다. CAS 는 시각이 아니라
	// OID 를 보므로 역행해도 무관하다.
	future := model.Timestamp{Time: issue.UpdatedAt.Add(time.Hour)}
	forced := issue.Issue
	forced.UpdatedAt = future
	hash, err := b.writeIssueCommit(forced, nil, "touch", issue.Commit)
	if err != nil {
		t.Fatalf("writeIssueCommit() = %v", err)
	}
	if err := b.store.CAS(issue.Ref, hash, issue.Commit); err != nil {
		t.Fatalf("CAS() = %v", err)
	}

	got, err := b.Mutate(m)
	if err != nil {
		t.Fatalf("Mutate() = %v", err)
	}
	if got.Status != model.StatusWorking {
		t.Fatalf("status = %q", got.Status)
	}
}

// scriptedStore 는 앞선 몇 번의 CAS 만 정해진 오류로 실패시킨다.
type scriptedStore struct {
	inner refstore.RefStore
	errs  []error
	calls int
}

func (s *scriptedStore) Get(ref string) (plumbing.Hash, error) { return s.inner.Get(ref) }
func (s *scriptedStore) List(p string) ([]refstore.RefEntry, error) {
	return s.inner.List(p)
}
func (s *scriptedStore) Transaction(ops []refstore.RefOp) error { return s.inner.Transaction(ops) }
func (s *scriptedStore) CAS(ref string, new, old plumbing.Hash) error {
	if s.calls < len(s.errs) {
		s.calls++
		return s.errs[s.calls-1]
	}
	s.calls++
	return s.inner.CAS(ref, new, old)
}

// alwaysStore 는 CAS 를 항상 같은 오류로 실패시킨다. err 이 nil 이면 위임한다.
type alwaysStore struct {
	inner refstore.RefStore
	err   error
	calls int
}

func (s *alwaysStore) Get(ref string) (plumbing.Hash, error) { return s.inner.Get(ref) }
func (s *alwaysStore) List(p string) ([]refstore.RefEntry, error) {
	return s.inner.List(p)
}
func (s *alwaysStore) Transaction(ops []refstore.RefOp) error { return s.inner.Transaction(ops) }
func (s *alwaysStore) CAS(ref string, new, old plumbing.Hash) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	return s.inner.CAS(ref, new, old)
}
