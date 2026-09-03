package board

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// ErrPhaseInUse 는 소속 task 가 있는 phase 를 지우려 했다는 뜻이다.
var ErrPhaseInUse = errors.New("소속 task 가 있는 phase 는 제거할 수 없습니다")

// PlanAddOptions 는 plan 생성 입력이다.
type PlanAddOptions struct {
	Title    string
	Priority model.Priority
	// ID 는 직접 지정한 plan ID 다. 비어 있으면 채번한다.
	ID string
}

// AddPlan 은 plan 을 만든다.
//
// 채번은 이슈와 같은 create-only CAS 다 (§3.2). plan 은 구조만 담고 진행
// 상태를 담지 않으므로 (§3.7.1), 여기서 쓰는 것이 사실상 마지막 쓰기다.
func (b *Board) AddPlan(opts PlanAddOptions) (*Plan, error) {
	if err := b.requireWritable(); err != nil {
		return nil, err
	}
	title, _ := splitTitle(opts.Title)
	if title == "" {
		return nil, errors.New("제목이 비어 있습니다")
	}
	if opts.Priority == "" {
		opts.Priority = model.PriorityMed
	}

	now := model.Now()
	plan := model.Plan{
		Schema: model.SchemaVersion, Title: title, Status: model.PlanActive,
		Priority: opts.Priority, Phases: []model.Phase{}, AdvancedPhases: []string{},
		CreatedAt: now, UpdatedAt: now,
	}

	if opts.ID != "" {
		plan.ID = opts.ID
		return b.createPlan(plan)
	}
	next, err := b.nextPlanNumber()
	if err != nil {
		return nil, err
	}
	for attempt := range maxIDAttempts {
		plan.ID = formatPlanID(next + attempt)
		created, err := b.createPlan(plan)
		if err == nil {
			return created, nil
		}
		if !errors.Is(err, refstore.ErrCASConflict) {
			return nil, err
		}
		// 남이 그 번호를 먼저 가져갔다. 다음 번호로 간다.
	}
	return nil, fmt.Errorf("plan 번호를 %d번 시도했지만 배정하지 못했습니다", maxIDAttempts)
}

func (b *Board) createPlan(plan model.Plan) (*Plan, error) {
	if err := refstore.ValidateID(plan.ID); err != nil {
		return nil, err
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	hash, err := b.writePlanCommit(plan, "plan-new", plumbing.ZeroHash)
	if err != nil {
		return nil, err
	}
	if err := b.store.CAS(refstore.Plans+plan.ID, hash, plumbing.ZeroHash); err != nil {
		return nil, err
	}
	return &Plan{Plan: plan, Ref: refstore.Plans + plan.ID, Commit: hash}, nil
}

// Plan 은 조회 결과 하나다.
type Plan struct {
	model.Plan
	Ref    string
	Commit plumbing.Hash
}

// ShowPlan 은 plan 문서를 읽는다.
func (b *Board) ShowPlan(id string) (*Plan, error) {
	if err := refstore.ValidateID(id); err != nil {
		return nil, err
	}
	ref := refstore.Plans + id
	hash, err := b.store.Get(ref)
	if isNotFound(err) {
		return nil, fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var plan model.Plan
	if _, _, _, err := gitobj.Read(b.repo, hash, gitobj.FilePlan, &plan); err != nil {
		return nil, err
	}
	return &Plan{Plan: plan, Ref: ref, Commit: hash}, nil
}

// ListPlans 는 plan 목록을 ID 순으로 돌려준다.
func (b *Board) ListPlans(status model.PlanStatus) ([]*Plan, error) {
	refs, err := b.store.List(refstore.Plans)
	if err != nil {
		return nil, err
	}
	plans := make([]*Plan, 0, len(refs))
	for _, ref := range refs {
		plan, err := b.ShowPlan(strings.TrimPrefix(ref.Ref, refstore.Plans))
		if err != nil {
			// 손상된 plan 하나가 목록 전체를 죽이지 않는다. fsck 가 잡는다.
			continue
		}
		if status != "" && plan.Status != status {
			continue
		}
		plans = append(plans, plan)
	}
	slices.SortFunc(plans, func(a, c *Plan) int { return strings.Compare(a.ID, c.ID) })
	return plans, nil
}

// MutatePlan 은 plan 문서에 대한 §4.1 CAS 루프다.
//
// 이슈의 Mutate 와 같은 구조다. 다른 함수인 이유는 문서 종류와 trailer 가
// 다르기 때문이며, 경쟁 처리 규칙은 동일하다.
func (b *Board) MutatePlan(id, event string, apply func(*model.Plan) error) (*Plan, error) {
	if err := refstore.ValidateID(id); err != nil {
		return nil, err
	}
	if err := b.requireWritable(); err != nil {
		return nil, err
	}
	if err := b.RegisterSession(); err != nil {
		return nil, err
	}

	ref := refstore.Plans + id
	attempts := max(b.backoff.CASAttempts, 1)
	var lastErr error
	for attempt := range attempts {
		old, err := b.store.Get(ref)
		if isNotFound(err) {
			return nil, fmt.Errorf("%s: %w", id, ErrNotFound)
		}
		if err != nil {
			return nil, err
		}
		var plan model.Plan
		if _, _, _, err := gitobj.Read(b.repo, old, gitobj.FilePlan, &plan); err != nil {
			return nil, err
		}
		if err := apply(&plan); err != nil {
			return nil, err
		}
		plan.UpdatedAt = model.Now()
		if err := plan.Validate(); err != nil {
			return nil, err
		}

		hash, err := b.writePlanCommit(plan, event, old)
		if err != nil {
			return nil, err
		}
		err = b.casWithLockRetry(ref, hash, old)
		switch {
		case err == nil:
			return &Plan{Plan: plan, Ref: ref, Commit: hash}, nil
		case errors.Is(err, refstore.ErrCASConflict), errors.Is(err, refstore.ErrLockBusy):
			lastErr = err
			b.backoff.Wait(attempt)
		default:
			return nil, err
		}
	}
	return nil, &ConflictError{ID: id, Attempts: attempts, Cause: lastErr}
}

// PhaseAddOptions 는 phase 추가 입력이다.
type PhaseAddOptions struct {
	Title string
	Gate  model.Gate
	ID    string
	// Before/After 는 삽입 위치다. 둘 다 비어 있으면 맨 뒤다.
	Before string
	After  string
}

// AddPhase 는 plan 에 phase 를 넣는다.
func (b *Board) AddPhase(planID string, opts PhaseAddOptions) (*Plan, error) {
	if opts.Before != "" && opts.After != "" {
		return nil, errors.New("--before 와 --after 는 함께 쓸 수 없습니다")
	}
	title, _ := splitTitle(opts.Title)
	if title == "" {
		return nil, errors.New("제목이 비어 있습니다")
	}
	if opts.Gate == "" {
		opts.Gate = model.GateAllDone
	}
	return b.MutatePlan(planID, "phase-add", func(plan *model.Plan) error {
		phase := model.Phase{ID: opts.ID, Title: title, Gate: opts.Gate}
		if phase.ID == "" {
			phase.ID = nextPhaseID(*plan)
		}
		if _, exists := plan.Phase(phase.ID); exists {
			return fmt.Errorf("phase %s 가 이미 있습니다", phase.ID)
		}
		index := len(plan.Phases)
		if anchor := cmpOr(opts.Before, opts.After); anchor != "" {
			at := slices.IndexFunc(plan.Phases, func(p model.Phase) bool { return p.ID == anchor })
			if at < 0 {
				return fmt.Errorf("%s: 없는 phase 입니다", anchor)
			}
			index = at
			if opts.After != "" {
				index = at + 1
			}
		}
		plan.Phases = slices.Insert(slices.Clone(plan.Phases), index, phase)
		return nil
	})
}

// PhaseEditOptions 는 phase 수정 입력이다.
type PhaseEditOptions struct {
	Title string
	Gate  model.Gate
}

// EditPhase 는 phase 의 제목과 gate 를 바꾼다.
func (b *Board) EditPhase(planID, phaseID string, opts PhaseEditOptions) (*Plan, error) {
	if opts.Title == "" && opts.Gate == "" {
		return nil, errors.New("바꿀 것이 없습니다")
	}
	return b.MutatePlan(planID, "phase-edit", func(plan *model.Plan) error {
		at := slices.IndexFunc(plan.Phases, func(p model.Phase) bool { return p.ID == phaseID })
		if at < 0 {
			return fmt.Errorf("%s: 없는 phase 입니다", phaseID)
		}
		plan.Phases = slices.Clone(plan.Phases)
		if opts.Title != "" {
			plan.Phases[at].Title = opts.Title
		}
		if opts.Gate != "" {
			plan.Phases[at].Gate = opts.Gate
		}
		return nil
	})
}

// RemovePhase 는 phase 를 지운다. 소속 task 가 있으면 거부한다.
//
// 지우면 그 task 들이 고아가 된다. phase 필드는 id 참조라 자동으로 따라가지
// 않으므로, 사람이 먼저 옮기게 한다.
func (b *Board) RemovePhase(planID, phaseID string) (*Plan, error) {
	entries, err := b.List(ListOptions{All: true, Plan: planID, Phase: phaseID})
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return nil, fmt.Errorf("%s/%s: %w (%d개)", planID, phaseID, ErrPhaseInUse, len(entries))
	}
	return b.MutatePlan(planID, "phase-remove", func(plan *model.Plan) error {
		at := slices.IndexFunc(plan.Phases, func(p model.Phase) bool { return p.ID == phaseID })
		if at < 0 {
			return fmt.Errorf("%s: 없는 phase 입니다", phaseID)
		}
		plan.Phases = slices.Delete(slices.Clone(plan.Phases), at, at+1)
		// advanced_phases 에 남으면 Validate 가 막는다. 함께 지운다.
		plan.AdvancedPhases = slices.DeleteFunc(slices.Clone(plan.AdvancedPhases),
			func(id string) bool { return id == phaseID })
		return nil
	})
}

// AdvancePhase 는 manual gate 를 사람이 연다 (§3.7.5).
//
// plan ref 를 쓰는 몇 안 되는 경우다. 사람의 판단이므로 경쟁이 없다.
func (b *Board) AdvancePhase(planID, phaseID string) (*Plan, error) {
	return b.MutatePlan(planID, "phase-advance", func(plan *model.Plan) error {
		if _, ok := plan.Phase(phaseID); !ok {
			return fmt.Errorf("%s: 없는 phase 입니다", phaseID)
		}
		if slices.Contains(plan.AdvancedPhases, phaseID) {
			return nil
		}
		plan.AdvancedPhases = append(slices.Clone(plan.AdvancedPhases), phaseID)
		return nil
	})
}

// SetPlanStatus 는 plan 을 닫거나 취소한다 (§3.7.6).
func (b *Board) SetPlanStatus(planID string, status model.PlanStatus) (*Plan, error) {
	if !status.Valid() {
		return nil, fmt.Errorf("알 수 없는 plan 상태입니다: %q", status)
	}
	return b.MutatePlan(planID, "plan-"+string(status), func(plan *model.Plan) error {
		if plan.Status == status {
			return fmt.Errorf("이미 %s 입니다", status)
		}
		plan.Status = status
		return nil
	})
}

// PlanEditOptions 는 plan 메타데이터 수정 입력이다.
type PlanEditOptions struct {
	Title    string
	Priority model.Priority
}

// EditPlan 은 제목과 우선순위를 바꾼다.
func (b *Board) EditPlan(planID string, opts PlanEditOptions) (*Plan, error) {
	if opts.Title == "" && opts.Priority == "" {
		return nil, errors.New("바꿀 것이 없습니다")
	}
	return b.MutatePlan(planID, "plan-edit", func(plan *model.Plan) error {
		if opts.Title != "" {
			plan.Title = opts.Title
		}
		if opts.Priority != "" {
			plan.Priority = opts.Priority
		}
		return nil
	})
}

// writePlanCommit 은 plan 상태 commit 하나를 만든다 (§3.7.4).
func (b *Board) writePlanCommit(plan model.Plan, event string, parent plumbing.Hash) (plumbing.Hash, error) {
	return gitobj.Write(b.repo, gitobj.Commit{
		Doc:     plan,
		DocName: gitobj.FilePlan,
		Subject: eventSubject(event, plan.Title, ""),
		Trailers: []gitobj.Trailer{
			{Key: gitobj.KeyTitle, Value: plan.Title},
			{Key: gitobj.KeyStatus, Value: string(plan.Status)},
			{Key: gitobj.KeyPriority, Value: string(plan.Priority)},
			{Key: gitobj.KeyPhases, Value: strconv.Itoa(len(plan.Phases))},
			// 이슈와 같은 이유로 필수다 — 같은 초에 같은 내용의 commit 두 개가
			// 같은 OID 를 갖으면 양쪽 CAS 가 모두 성공한다 (§4.3).
			{Key: gitobj.KeyAgentSession, Value: b.identity.Session},
		},
		Author: b.signature(plan.UpdatedAt),
		Parent: parent,
	})
}

// formatPlanID 는 plan ID 다. 이슈의 T001 과 구분된다.
func formatPlanID(n int) string { return fmt.Sprintf("P%02d", n) }

func (b *Board) nextPlanNumber() (int, error) {
	refs, err := b.store.List(refstore.Plans)
	if err != nil {
		return 0, err
	}
	maximum := 0
	for _, ref := range refs {
		n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(ref.Ref, refstore.Plans), "P"))
		if err == nil && n > maximum {
			maximum = n
		}
	}
	return maximum + 1, nil
}

// nextPhaseID 는 p1, p2 … 중 비어 있는 첫 번째다.
func nextPhaseID(plan model.Plan) string {
	for n := 1; ; n++ {
		id := "p" + strconv.Itoa(n)
		if _, exists := plan.Phase(id); !exists {
			return id
		}
	}
}

func cmpOr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
