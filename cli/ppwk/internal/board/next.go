package board

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// NextOptions 는 스케줄링 한 번의 입력이다 (features §4).
type NextOptions struct {
	// Plan 은 특정 plan 으로 후보를 제한한다.
	Plan string
	// Label 은 capability 필터다.
	Label string
	// Claim 은 고른 후보를 claim 까지 수행한다.
	Claim bool
	// DryRun 은 저장소를 전혀 변형하지 않는다. reap 도 건너뛴다.
	DryRun bool
	// MaxAttempts 는 claim 시도 상한이다. 후보가 수천 개여도 전부 시도하지 않는다.
	MaxAttempts int
}

// NextResult 는 next 한 번의 결과다.
type NextResult struct {
	// Candidates 는 정렬된 후보 전체다.
	Candidates []*Issue
	// Claimed 는 실제로 가져온 이슈다. 없으면 nil — 오류가 아니다.
	Claimed *Issue
	// Attempts 는 claim 을 시도한 횟수다.
	Attempts int
}

// Next 는 §7.2 의 스케줄링 알고리즘이다.
//
// reap 이 여기 안에 있는 것이 이 시스템의 유일한 자동 실행 지점이다. 별도
// 주기 실행 프로세스를 두지 않는 대신, 에이전트가 다음 일을 찾을 때마다
// 죽은 소유자의 이슈가 회수된다.
func (b *Board) Next(opts NextOptions) (*NextResult, error) {
	if !opts.DryRun {
		if _, err := b.Reap(ReapOptions{}); err != nil {
			return nil, err
		}
	}
	candidates, err := b.Candidates(opts)
	if err != nil {
		return nil, err
	}

	result := &NextResult{Candidates: candidates}
	if !opts.Claim || opts.DryRun {
		return result, nil
	}

	limit := max(opts.MaxAttempts, 1)
	for _, candidate := range candidates {
		if result.Attempts >= limit {
			break
		}
		result.Attempts++

		claimed, err := b.Transition(ActionClaim, candidate.ID, TransitionOptions{})
		if err == nil {
			result.Claimed = claimed
			return result, nil
		}
		// 경쟁에서 진 것은 재시도 신호가 아니라 다른 일을 찾을 신호다. 같은
		// 이슈를 다시 노리면 에이전트들이 한 줄로 서서 하나씩 통과하게 된다.
		// 다음 후보로 넘어가면 그 줄이 저절로 흩어진다 (§7.2).
		var conflict *ConflictError
		var transition *TransitionError
		if errors.As(err, &conflict) || errors.As(err, &transition) || errors.Is(err, ErrNotFound) {
			continue
		}
		return nil, err
	}
	// 전부 실패했어도 오류가 아니다. "지금 할 일이 없다" 와 같은 결과다.
	return result, nil
}

// Candidates 는 후보를 모아 정렬해 돌려준다. 저장소를 변형하지 않는다.
func (b *Board) Candidates(opts NextOptions) ([]*Issue, error) {
	entries, err := b.List(ListOptions{
		Status: []model.Status{model.StatusOpen},
		Plan:   opts.Plan,
	})
	if err != nil {
		return nil, err
	}

	loaded := make(map[string]*Issue, len(entries))
	open := make([]*Issue, 0, len(entries))
	for _, entry := range entries {
		issue, err := b.Show(entry.ID)
		if err != nil {
			// 손상된 이슈 하나가 스케줄링 전체를 멈추지 않는다. fsck 가 잡는다.
			continue
		}
		loaded[issue.ID] = issue
		open = append(open, issue)
	}

	sel := selector{
		label:        opts.Label,
		planPriority: b.planPriority(),
		status: func(id string) (model.Status, bool) {
			if issue, ok := loaded[id]; ok {
				if issue == nil {
					return "", false
				}
				return issue.Status, true
			}
			// Show 는 archive/ 도 본다. 완료된 이슈는 archive 로 옮겨지므로
			// issues/ 만 보면 "의존 대상이 사라짐" 으로 오판해 후속 작업이
			// 영원히 후보에서 빠진다 (T5.4).
			issue, err := b.Show(id)
			if err != nil {
				loaded[id] = nil
				return "", false
			}
			loaded[id] = issue
			return issue.Status, true
		},
	}
	return sel.pick(open), nil
}

// selector 는 후보 선정과 정렬 규칙이다.
//
// 저장소 접근을 함수로 받는다. 규칙 자체가 이 설계에서 가장 미묘한 부분이고
// (전순서, 순환 의존, archive 조회), 저장소 없이 대량으로 흔들어볼 수 있어야
// 한다 (F5.1, F5.2).
type selector struct {
	// status 는 의존 대상의 상태다. issues/ 와 archive/ 를 모두 본 결론이어야 한다.
	status func(id string) (model.Status, bool)
	// planPriority 는 소속 plan 의 우선순위다. plan 이 없으면 med 다.
	planPriority func(planID string) model.Priority
	// label 은 capability 필터다. 빈 문자열이면 거르지 않는다.
	label string
}

func (s selector) pick(issues []*Issue) []*Issue {
	out := make([]*Issue, 0, len(issues))
	for _, issue := range issues {
		if s.eligible(&issue.Issue) {
			out = append(out, issue)
		}
	}
	slices.SortFunc(out, func(a, b *Issue) int { return s.compare(&a.Issue, &b.Issue) })
	return out
}

// eligible 은 후보 자격을 본다 (§7.2 2~3단계).
//
// 의존은 직접 의존만 본다. 재귀로 따라가지 않으므로 순환이 있어도 멈추지
// 않는다 — 순환에 속한 이슈들은 서로가 done 이 아니라서 모두 후보에서 빠진다.
func (s selector) eligible(i *model.Issue) bool {
	if i.Status != model.StatusOpen {
		return false
	}
	// priority none 은 백로그다. 상태가 아니라 속성이므로 전이·회수 규칙에는
	// 예외가 생기지 않고, 후보 선정에서만 빠진다 (features §2).
	if i.Priority == model.PriorityNone {
		return false
	}
	if s.label != "" && !slices.Contains(i.Labels, s.label) {
		return false
	}
	for _, dep := range i.DependsOn {
		status, ok := s.status(dep)
		// 없는 의존 대상은 충족이 아니다. cancelled 도 마찬가지다 — 취소는
		// 일이 끝난 것이 아니다. 그래서 후속이 영원히 막힐 수 있고, 그것을
		// fsck 가 경고한다.
		if !ok || status != model.StatusDone {
			return false
		}
	}
	return true
}

// compare 는 §7.2 5단계의 정렬이다: plan priority DESC, seq ASC,
// priority DESC, created_at ASC.
//
// seq 가 priority 보다 앞선다. plan 안에서는 저자가 의도한 순서를 따라야 하고,
// priority 가 앞서면 high 인 task 가 계획 순서를 뛰어넘는다.
func (s selector) compare(a, b *model.Issue) int {
	if c := cmp.Compare(rank(s.planPriority(b.Plan)), rank(s.planPriority(a.Plan))); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Seq, b.Seq); c != 0 {
		return c
	}
	if c := cmp.Compare(rank(b.Priority), rank(a.Priority)); c != 0 {
		return c
	}
	if c := a.CreatedAt.Time.Compare(b.CreatedAt.Time); c != 0 {
		return c
	}
	// ID 로 마무리해 전순서를 만든다. 여기가 없으면 동순위 후보들의 순서가
	// 실행마다 달라져 재현이 불가능한 스케줄링이 된다 (F5.2).
	return cmp.Compare(a.ID, b.ID)
}

// rank 는 우선순위의 크기다. 모르는 값은 가장 낮게 본다 — 비교가 흔들리는
// 것보다 낫다.
func rank(p model.Priority) int {
	switch p {
	case model.PriorityHigh:
		return 3
	case model.PriorityMed:
		return 2
	case model.PriorityLow:
		return 1
	default:
		return 0
	}
}

// planPriority 는 plan 우선순위를 읽는 함수를 만든다. 호출당 한 번만 읽도록
// 캐시한다 — 같은 plan 에 속한 이슈가 많은 것이 정상이기 때문이다.
func (b *Board) planPriority() func(string) model.Priority {
	cache := map[string]model.Priority{}
	return func(id string) model.Priority {
		// plan 에 속하지 않은 이슈는 med 로 본다. 계획된 작업들 사이에
		// 자연스럽게 섞이게 하기 위함이다 (§7.2).
		if id == "" {
			return model.PriorityMed
		}
		if p, ok := cache[id]; ok {
			return p
		}
		p := model.PriorityMed
		if plan, err := b.ShowPlan(id); err == nil && plan.Priority.Valid() {
			p = plan.Priority
		}
		cache[id] = p
		return p
	}
}

// ShowPlan 은 plan 문서를 읽는다.
func (b *Board) ShowPlan(id string) (model.Plan, error) {
	hash, err := b.store.Get(refstore.Plans + id)
	if isNotFound(err) {
		return model.Plan{}, fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	if err != nil {
		return model.Plan{}, err
	}
	var plan model.Plan
	if _, _, _, err := gitobj.Read(b.repo, hash, gitobj.FilePlan, &plan); err != nil {
		return model.Plan{}, err
	}
	return plan, nil
}
