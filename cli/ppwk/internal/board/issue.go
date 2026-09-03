package board

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// maxIDAttempts 는 채번 재시도 상한이다.
const maxIDAttempts = 64

// AddOptions 는 이슈 생성 입력이다.
type AddOptions struct {
	Title     string
	Body      []byte
	Priority  model.Priority
	Labels    []string
	DependsOn []string
	Plan      string
	Phase     string
	Seq       int
	// SeqSet 은 --seq 가 명시됐는지다. 0 과 미지정을 구분한다.
	SeqSet bool
}

// Add 는 이슈를 만든다.
//
// 채번은 create-only CAS 로 한다. 별도 카운터 ref 를 두지 않는다 (§3.2).
func (b *Board) Add(opts AddOptions) (*Issue, error) {
	if err := b.requireWritable(); err != nil {
		return nil, err
	}

	// 제목에 개행이 있으면 첫 줄만 subject 로 쓰고 나머지는 본문으로 내린다.
	title, extra := splitTitle(opts.Title)
	if title == "" {
		return nil, errors.New("제목이 비어 있습니다")
	}
	body := opts.Body
	if extra != "" {
		body = append([]byte(extra), body...)
	}

	if opts.Priority == "" {
		opts.Priority = model.PriorityMed
	}

	now := model.Now()
	issue := model.Issue{
		Schema:    model.SchemaVersion,
		Title:     title,
		Status:    model.StatusOpen,
		Priority:  opts.Priority,
		Labels:    opts.Labels,
		Plan:      opts.Plan,
		Phase:     opts.Phase,
		Seq:       opts.Seq,
		DependsOn: opts.DependsOn,
		CreatedAt: now,
		UpdatedAt: now,
		UpdatedBy: b.identity.Agent,
	}

	next, err := b.nextIssueNumber()
	if err != nil {
		return nil, err
	}
	for attempt := range maxIDAttempts {
		issue.ID = formatIssueID(next + attempt)
		if err := refstore.ValidateID(issue.ID); err != nil {
			return nil, err
		}
		if err := issue.Validate(); err != nil {
			return nil, err
		}

		hash, err := b.writeIssueCommit(issue, body, "create", plumbing.ZeroHash)
		if err != nil {
			return nil, err
		}
		err = b.store.CAS(refstore.Issues+issue.ID, hash, plumbing.ZeroHash)
		if err == nil {
			return &Issue{Issue: issue, Body: body, Ref: refstore.Issues + issue.ID, Commit: hash}, nil
		}
		if !errors.Is(err, refstore.ErrCASConflict) {
			return nil, err
		}
		// 남이 그 번호를 먼저 가져갔다. 다음 번호로 간다.
	}
	return nil, fmt.Errorf("이슈 번호를 %d번 시도했지만 배정하지 못했습니다", maxIDAttempts)
}

// Issue 는 조회 결과 하나다.
type Issue struct {
	model.Issue
	Body   []byte
	Ref    string
	Commit plumbing.Hash
}

// Archived 는 archive/ 에 있는지다.
func (i Issue) Archived() bool {
	return strings.HasPrefix(i.Ref, refstore.Archive)
}

// ListOptions 는 조회 필터다. 단계 1 에서는 최소한만 쓴다.
type ListOptions struct {
	Status     []model.Status
	Priority   []model.Priority
	Owner      string
	Label      string
	Plan       string
	Phase      string
	Unassigned bool
	// Mine 은 이 세션이 쥔 것만이다.
	Mine bool
	// Archived 는 archive/ 만 본다. All 은 둘 다 본다.
	Archived bool
	All      bool
	Limit    int
}

// ListEntry 는 목록 한 줄이다.
//
// trailer 만 읽으므로 issue.json 을 열지 않는다. 상태를 commit message 에
// 복제해 둔 이유가 이것이다 (§3.3, §5.1).
type ListEntry struct {
	ID       string         `json:"id"`
	Status   model.Status   `json:"status"`
	Owner    string         `json:"owner,omitempty"`
	Priority model.Priority `json:"priority"`
	Plan     string         `json:"plan,omitempty"`
	Phase    string         `json:"phase,omitempty"`
	Seq      int            `json:"seq,omitempty"`
	Title    string         `json:"title"`
	Ref      string         `json:"ref"`
	Commit   string         `json:"commit"`
}

// List 는 이슈 목록을 돌려준다.
//
// 설계 §5.1 은 for-each-ref 한 번을 상정했지만, go-git 으로 읽으면 fork 가
// 아예 없다. 읽는 대상은 같다 — commit 의 trailer 블록이다.
func (b *Board) List(opts ListOptions) ([]ListEntry, error) {
	prefixes := []string{refstore.Issues}
	switch {
	case opts.All:
		prefixes = []string{refstore.Issues, refstore.Archive}
	case opts.Archived:
		prefixes = []string{refstore.Archive}
	}

	var entries []ListEntry
	for _, prefix := range prefixes {
		refs, err := b.store.List(prefix)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			entry, err := b.readEntry(ref.Ref, ref.Hash)
			if err != nil {
				// 손상된 이슈 하나가 목록 전체를 죽이지 않는다.
				continue
			}
			if !matches(entry, opts) {
				continue
			}
			if opts.Mine && !b.ownedByThisSession(entry) {
				continue
			}
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	if opts.Limit > 0 && len(entries) > opts.Limit {
		entries = entries[:opts.Limit]
	}
	return entries, nil
}

// Show 는 이슈 하나를 읽는다. archive 에 있어도 찾는다.
func (b *Board) Show(id string) (*Issue, error) {
	if err := refstore.ValidateID(id); err != nil {
		return nil, err
	}
	for _, ref := range []string{refstore.Issues + id, refstore.Archive + id} {
		hash, err := b.store.Get(ref)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var issue model.Issue
		body, _, _, err := gitobj.Read(b.repo, hash, gitobj.FileIssue, &issue)
		if err != nil {
			return nil, err
		}
		return &Issue{Issue: issue, Body: body, Ref: ref, Commit: hash}, nil
	}
	return nil, fmt.Errorf("%s: %w", id, ErrNotFound)
}

// readEntry 는 commit 의 trailer 만 읽어 목록 한 줄을 만든다.
func (b *Board) readEntry(ref string, hash plumbing.Hash) (ListEntry, error) {
	commit, err := object.GetCommit(b.repo.Storer, hash)
	if err != nil {
		return ListEntry{}, err
	}
	subject, trailers := gitobj.ParseMessage(commit.Message)

	// 제목은 trailer 가 진실이다. 사유가 붙은 subject 를 파싱하면 사유가
	// 제목으로 새어 나온다. Title trailer 이전에 만들어진 commit 을 위해
	// subject 파싱을 남겨 두지만, 그것은 fallback 이다.
	title := gitobj.TrailerValue(trailers, gitobj.KeyTitle)
	if title == "" {
		title = subject
		if _, rest, found := strings.Cut(subject, ": "); found {
			title = rest
		}
	}

	seq, _ := strconv.Atoi(gitobj.TrailerValue(trailers, gitobj.KeySeq))
	return ListEntry{
		ID:       strings.TrimPrefix(strings.TrimPrefix(ref, refstore.Issues), refstore.Archive),
		Status:   model.Status(gitobj.TrailerValue(trailers, gitobj.KeyStatus)),
		Owner:    gitobj.TrailerValue(trailers, gitobj.KeyOwner),
		Priority: model.Priority(gitobj.TrailerValue(trailers, gitobj.KeyPriority)),
		Plan:     gitobj.TrailerValue(trailers, gitobj.KeyPlan),
		Phase:    gitobj.TrailerValue(trailers, gitobj.KeyPhase),
		Seq:      seq,
		Title:    title,
		Ref:      ref,
		Commit:   hash.String(),
	}, nil
}

// ownedByThisSession 은 owner 와 session 이 둘 다 이 실행의 것인지다.
//
// owner 는 trailer 에 있지만 session 은 없다. 그래서 소유자가 일치하는 것만
// issue.json 을 마저 읽는다 — 전체를 읽으면 목록 조회를 trailer 만으로
// 끝낸다는 §5.1 의 성질이 사라진다.
func (b *Board) ownedByThisSession(entry ListEntry) bool {
	if entry.Owner != b.identity.Agent {
		return false
	}
	issue, err := b.Show(entry.ID)
	if err != nil {
		return false
	}
	return issue.Session == b.identity.Session
}

// matches 는 필터를 적용한다.
func matches(e ListEntry, opts ListOptions) bool {
	if len(opts.Status) > 0 && !containsStatus(opts.Status, e.Status) {
		return false
	}
	if len(opts.Priority) > 0 && !containsPriority(opts.Priority, e.Priority) {
		return false
	}
	if opts.Owner != "" && e.Owner != opts.Owner {
		return false
	}
	if opts.Unassigned && e.Owner != "" {
		return false
	}
	if opts.Plan != "" && e.Plan != opts.Plan {
		return false
	}
	if opts.Phase != "" && e.Phase != opts.Phase {
		return false
	}
	return true
}

func containsStatus(list []model.Status, v model.Status) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func containsPriority(list []model.Priority, v model.Priority) bool {
	for _, p := range list {
		if p == v {
			return true
		}
	}
	return false
}

// splitTitle 은 제목의 첫 줄과 나머지를 나눈다.
func splitTitle(raw string) (title, rest string) {
	title, rest, found := strings.Cut(strings.TrimSpace(raw), "\n")
	title = strings.TrimSpace(title)
	if !found {
		return title, ""
	}
	rest = strings.TrimLeft(rest, "\n")
	if rest != "" && !strings.HasSuffix(rest, "\n") {
		rest += "\n"
	}
	return title, rest
}

// formatIssueID 는 sequential 전략의 ID 다 (§3.2).
func formatIssueID(n int) string {
	return fmt.Sprintf("T%03d", n)
}

// nextIssueNumber 는 현재 최대 번호 + 1 이다. archive 도 본다.
func (b *Board) nextIssueNumber() (int, error) {
	max := 0
	for _, prefix := range []string{refstore.Issues, refstore.Archive} {
		refs, err := b.store.List(prefix)
		if err != nil {
			return 0, err
		}
		for _, ref := range refs {
			id := strings.TrimPrefix(ref.Ref, prefix)
			n, err := strconv.Atoi(strings.TrimPrefix(id, "T"))
			if err != nil {
				continue
			}
			if n > max {
				max = n
			}
		}
	}
	return max + 1, nil
}
