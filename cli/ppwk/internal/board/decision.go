package board

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// DecideOptions 는 결정 기록 입력이다 (features §5.5).
type DecideOptions struct {
	Title        string
	Context      string
	Options      []string
	Chosen       string
	Consequences string
	Issues       []string
	Plan         string
	Supersedes   string
	// Body 는 긴 근거다. tree 의 body.md 가 된다.
	Body []byte
}

// Decision 은 조회 결과 하나다.
type Decision struct {
	model.Decision
	Body   []byte
	Ref    string
	Commit plumbing.Hash
}

// Decide 는 결정을 기록한다 (§3.9).
//
// 수정 명령이 없다. 여기가 결정 문서를 쓰는 유일한 지점이며, 한 번 쓰이면
// 그 ref 는 다시 갱신되지 않는다 — create-only CAS 하나로 끝난다.
func (b *Board) Decide(opts DecideOptions) (*Decision, error) {
	if err := b.requireWritable(); err != nil {
		return nil, err
	}
	title, extra := splitTitle(opts.Title)
	if title == "" {
		return nil, errors.New("제목이 비어 있습니다")
	}
	body := opts.Body
	if extra != "" {
		body = append([]byte(extra), body...)
	}

	// supersedes 대상은 CAS 루프 밖에서 한 번만 확인한다. 결정은 불변이므로
	// 다시 읽어도 답이 같다.
	if opts.Supersedes != "" {
		if _, err := b.ShowDecision(opts.Supersedes); err != nil {
			return nil, fmt.Errorf("대체 대상 %s: %w", opts.Supersedes, err)
		}
	}

	decision := model.Decision{
		Schema: model.SchemaVersion, Title: title,
		Context: opts.Context, Options: opts.Options, Chosen: opts.Chosen,
		Consequences: opts.Consequences, Issues: opts.Issues, Plan: opts.Plan,
		Supersedes: opts.Supersedes,
		DecidedBy:  b.identity.Agent, DecidedAt: model.Now(),
	}

	next, err := b.nextDecisionNumber()
	if err != nil {
		return nil, err
	}
	for attempt := range maxIDAttempts {
		decision.ID = formatDecisionID(next + attempt)
		if err := refstore.ValidateID(decision.ID); err != nil {
			return nil, err
		}
		if err := decision.Validate(); err != nil {
			return nil, err
		}

		hash, err := b.writeDecisionCommit(decision, body)
		if err != nil {
			return nil, err
		}
		ref := refstore.Decisions + decision.ID
		err = b.store.CAS(ref, hash, plumbing.ZeroHash)
		if err == nil {
			return &Decision{Decision: decision, Body: body, Ref: ref, Commit: hash}, nil
		}
		if !errors.Is(err, refstore.ErrCASConflict) {
			return nil, err
		}
		// 남이 그 번호를 먼저 가져갔다. 다음 번호로 간다.
	}
	return nil, fmt.Errorf("결정 번호를 %d번 시도했지만 배정하지 못했습니다", maxIDAttempts)
}

// ShowDecision 은 결정 하나를 읽는다.
func (b *Board) ShowDecision(id string) (*Decision, error) {
	if err := refstore.ValidateID(id); err != nil {
		return nil, err
	}
	ref := refstore.Decisions + id
	hash, err := b.store.Get(ref)
	if isNotFound(err) {
		return nil, fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var decision model.Decision
	body, _, _, err := gitobj.Read(b.repo, hash, gitobj.FileDecision, &decision)
	if err != nil {
		return nil, err
	}
	return &Decision{Decision: decision, Body: body, Ref: ref, Commit: hash}, nil
}

// DecisionEntry 는 목록 한 줄이다.
//
// trailer 만 읽으므로 decision.json 을 열지 않는다 (D5). SupersededBy 는
// 저장된 값이 아니라 이 목록 전체에서 계산한 것이다 — 역방향 엣지를 저장하면
// 두 ref 를 원자적으로 갱신해야 한다.
type DecisionEntry struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Supersedes   string   `json:"supersedes,omitempty"`
	Issues       []string `json:"issues,omitempty"`
	SupersededBy []string `json:"superseded_by,omitempty"`
	Ref          string   `json:"ref"`
	Commit       string   `json:"commit"`
}

// Superseded 는 이 결정이 다른 결정에 의해 대체됐는지다.
func (e DecisionEntry) Superseded() bool { return len(e.SupersededBy) > 0 }

// DecisionListOptions 는 결정 조회 필터다.
type DecisionListOptions struct {
	// All 은 superseded 된 것도 포함한다. 기본은 유효한 것만이다.
	All bool
	// Issue 는 이 이슈와 연결된 결정만 본다.
	Issue string
	// Plan 은 이 plan 과 연결된 결정만 본다.
	Plan string
	// Search 는 제목과 본문을 훑는다.
	Search string
}

// ListDecisions 는 결정 목록을 ID 순으로 돌려준다.
//
// 기본 경로는 for-each-ref 한 번이다. Plan 과 Search 는 trailer 에 없는 것을
// 보므로 (§3.9 는 Title, Supersedes, Issues 만 복제한다) 후보의 문서를 연다.
func (b *Board) ListDecisions(opts DecisionListOptions) ([]DecisionEntry, error) {
	refs, err := b.store.List(refstore.Decisions)
	if err != nil {
		return nil, err
	}
	entries := make([]DecisionEntry, 0, len(refs))
	for _, ref := range refs {
		entry, err := b.readDecisionEntry(ref.Ref, ref.Hash)
		if err != nil {
			// 손상된 결정 하나가 목록 전체를 죽이지 않는다. fsck 가 잡는다.
			continue
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, c DecisionEntry) int { return strings.Compare(a.ID, c.ID) })

	// 역방향 엣지는 여기서 만든다. 전체를 이미 읽었으므로 추가 비용이 없다.
	byID := make(map[string]int, len(entries))
	for i, entry := range entries {
		byID[entry.ID] = i
	}
	for _, entry := range entries {
		if at, ok := byID[entry.Supersedes]; ok && entry.Supersedes != "" {
			entries[at].SupersededBy = append(entries[at].SupersededBy, entry.ID)
		}
	}

	out := entries[:0]
	for _, entry := range entries {
		if !opts.All && entry.Superseded() {
			continue
		}
		if opts.Issue != "" && !slices.Contains(entry.Issues, opts.Issue) {
			continue
		}
		if opts.Plan != "" || opts.Search != "" {
			match, err := b.decisionMatchesDocument(entry, opts)
			if err != nil || !match {
				continue
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// decisionMatchesDocument 는 trailer 에 없는 조건을 문서를 열어 확인한다.
func (b *Board) decisionMatchesDocument(entry DecisionEntry, opts DecisionListOptions) (bool, error) {
	decision, err := b.ShowDecision(entry.ID)
	if err != nil {
		return false, err
	}
	if opts.Plan != "" && decision.Plan != opts.Plan {
		return false, nil
	}
	if opts.Search != "" {
		needle := strings.ToLower(opts.Search)
		haystack := strings.ToLower(strings.Join([]string{
			decision.Title, decision.Context, decision.Chosen,
			decision.Consequences, string(decision.Body),
		}, "\n"))
		if !strings.Contains(haystack, needle) {
			return false, nil
		}
	}
	return true, nil
}

// DecisionHistory 는 supersedes 체인을 최신에서 과거로 따라간다.
//
// 이미 superseded 된 결정을 다시 대체하는 것은 허용되므로 (분기) 체인은
// 나무가 될 수 있다. 여기서는 Supersedes 한 방향만 따라간다.
func (b *Board) DecisionHistory(id string) ([]*Decision, error) {
	var chain []*Decision
	seen := map[string]bool{}
	for current := id; current != ""; {
		if seen[current] {
			// 순환은 만들어질 수 없지만 (Decide 가 대상의 실재를 확인하고
			// 결정은 불변이다), 손상된 데이터에서 멈추지 않는 것이 중요하다.
			break
		}
		seen[current] = true
		decision, err := b.ShowDecision(current)
		if err != nil {
			if len(chain) > 0 && errors.Is(err, ErrNotFound) {
				// 사슬이 끊긴 것은 fsck 가 보고한다.
				break
			}
			return nil, err
		}
		chain = append(chain, decision)
		current = decision.Supersedes
	}
	return chain, nil
}

// DecisionsForIssue 는 이슈에 연결된 결정이다 (§3.9).
//
// 이슈 문서에는 결정 목록이 없다. 엣지가 결정 → 이슈 한 방향이므로 여기서
// 훑는다 — 이슈를 쓸 때마다 결정 ref 를 건드리지 않기 위한 대가다.
func (b *Board) DecisionsForIssue(id string) ([]DecisionEntry, error) {
	return b.ListDecisions(DecisionListOptions{Issue: id})
}

// readDecisionEntry 는 commit trailer 만 읽어 목록 한 줄을 만든다.
func (b *Board) readDecisionEntry(ref string, hash plumbing.Hash) (DecisionEntry, error) {
	commit, err := object.GetCommit(b.repo.Storer, hash)
	if err != nil {
		return DecisionEntry{}, err
	}
	subject, trailers := gitobj.ParseMessage(commit.Message)
	title := gitobj.TrailerValue(trailers, gitobj.KeyTitle)
	if title == "" {
		title = subject
	}
	return DecisionEntry{
		ID:         strings.TrimPrefix(ref, refstore.Decisions),
		Title:      title,
		Supersedes: gitobj.TrailerValue(trailers, gitobj.KeySupersedes),
		Issues:     splitList(gitobj.TrailerValue(trailers, gitobj.KeyIssues)),
		Ref:        ref,
		Commit:     hash.String(),
	}, nil
}

// splitList 는 ", " 로 이어 붙인 trailer 값을 되돌린다.
func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (b *Board) writeDecisionCommit(decision model.Decision, body []byte) (plumbing.Hash, error) {
	trailers := []gitobj.Trailer{{Key: gitobj.KeyTitle, Value: decision.Title}}
	if decision.Supersedes != "" {
		trailers = append(trailers, gitobj.Trailer{Key: gitobj.KeySupersedes, Value: decision.Supersedes})
	}
	if len(decision.Issues) > 0 {
		trailers = append(trailers, gitobj.Trailer{
			Key: gitobj.KeyIssues, Value: strings.Join(decision.Issues, ", "),
		})
	}
	if b.identity.Session != "" {
		trailers = append(trailers, gitobj.Trailer{Key: gitobj.KeyAgentSession, Value: b.identity.Session})
	}
	return gitobj.Write(b.repo, gitobj.Commit{
		Doc:      decision,
		DocName:  gitobj.FileDecision,
		Body:     body,
		Subject:  eventSubject("decide", decision.Title, ""),
		Trailers: trailers,
		Author:   b.signature(decision.DecidedAt),
		Parent:   plumbing.ZeroHash,
	})
}

func formatDecisionID(n int) string { return fmt.Sprintf("D%03d", n) }

func (b *Board) nextDecisionNumber() (int, error) {
	refs, err := b.store.List(refstore.Decisions)
	if err != nil {
		return 0, err
	}
	maximum := 0
	for _, ref := range refs {
		n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(ref.Ref, refstore.Decisions), "D"))
		if err == nil && n > maximum {
			maximum = n
		}
	}
	return maximum + 1, nil
}
