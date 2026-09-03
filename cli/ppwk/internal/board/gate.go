package board

import (
	"slices"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// phaseCounts 는 한 phase 의 task 상태 집계다.
//
// 저장되지 않는다. gate 판정과 진행률은 매번 task 에서 파생한다 — plan 에
// 진행 상태를 담는 순간 task 상태가 바뀔 때마다 plan ref 에서 경쟁이
// 벌어지고, 이슈별로 ref 를 나눈 이득이 통째로 사라진다 (§3.7.1).
type phaseCounts struct {
	total int
	// done 은 done 만이다.
	done int
	// terminal 은 done 과 cancelled 를 합한 것이다.
	terminal int
}

// gateOpen 은 phaseID 가 열려 있는지 판정한다 (§3.7.5).
//
// gate 는 "그 phase 가 열리기 위한 조건" 이며 **직전 phase 에 적용된다.**
// 첫 phase 의 gate 는 무시된다.
//
// 순수 함수다. 저장소를 보지 않으므로 임의의 구성으로 대량 검증할 수 있다
// (F9.1) — gate 는 조회 시점 판단이라 틀려도 데이터에 흔적이 남지 않고,
// 그래서 사후에 발견하기 어렵다.
func gateOpen(plan model.Plan, phaseID string, counts map[string]phaseCounts) bool {
	index := slices.IndexFunc(plan.Phases, func(p model.Phase) bool { return p.ID == phaseID })
	switch {
	case index < 0:
		// plan 은 있는데 이 phase 는 없다. 순서를 정할 근거가 없으므로 열지
		// 않는다. fsck 가 이 참조를 보고한다.
		return false
	case index == 0:
		return true
	}

	gate := plan.Phases[index].Gate
	if gate == model.GateManual {
		return slices.Contains(plan.AdvancedPhases, phaseID)
	}

	previous := counts[plan.Phases[index-1].ID]
	switch gate {
	case model.GateAllDone:
		// task 가 없으면 공허참으로 통과한다. 의도와 다를 수 있어 fsck 가
		// 경고한다 (§3.7.5).
		return previous.terminal == previous.total
	case model.GateAnyDone:
		return previous.done > 0
	}
	// 알 수 없는 gate 는 닫아 둔다. 모르는 조건을 통과시키는 것보다
	// 멈추는 편이 낫다 — fsck 가 값 자체를 보고한다.
	return false
}

// planIndex 는 후보 판정에 필요한 plan 정보를 호출당 한 번만 읽는다.
//
// plan priority, plan 상태, gate 가 모두 같은 문서를 본다. 따로 읽으면 같은
// plan 에 속한 이슈 수만큼 ref 를 다시 연다.
type planIndex struct {
	board *Board
	// plans 는 읽은 plan 이다. nil 값은 "없음" 을 캐시한 것이다.
	plans map[string]*Plan
	// counts 는 plan 별 phase 집계다. gate 를 처음 물을 때 채운다.
	counts map[string]map[string]phaseCounts
}

func newPlanIndex(b *Board) *planIndex {
	return &planIndex{board: b, plans: map[string]*Plan{}, counts: map[string]map[string]phaseCounts{}}
}

func (x *planIndex) plan(id string) *Plan {
	if plan, cached := x.plans[id]; cached {
		return plan
	}
	plan, err := x.board.ShowPlan(id)
	if err != nil {
		plan = nil
	}
	x.plans[id] = plan
	return plan
}

// priority 는 소속 plan 의 우선순위다.
func (x *planIndex) priority(id string) model.Priority {
	// plan 에 속하지 않은 이슈는 med 로 본다. 계획된 작업들 사이에 자연스럽게
	// 섞이게 하기 위함이다 (§7.2).
	if id == "" {
		return model.PriorityMed
	}
	if plan := x.plan(id); plan != nil && plan.Priority.Valid() {
		return plan.Priority
	}
	return model.PriorityMed
}

// phaseIndex 는 plan 안에서 phase 가 몇 번째인지다.
//
// plan 이나 phase 를 못 찾으면 0 이다. 그 경우 정렬에서 첫 phase 취급을
// 받으며, 참조가 매달렸다는 사실 자체는 fsck 가 보고한다.
func (x *planIndex) phaseIndex(planID, phaseID string) int {
	plan := x.plan(planID)
	if plan == nil {
		return 0
	}
	if at := slices.IndexFunc(plan.Phases, func(p model.Phase) bool { return p.ID == phaseID }); at >= 0 {
		return at
	}
	return 0
}

// eligible 은 이 이슈가 plan 쪽 조건을 통과하는지다.
func (x *planIndex) eligible(planID, phaseID string) bool {
	if planID == "" {
		return true
	}
	plan := x.plan(planID)
	if plan == nil {
		// plan 문서가 없으면 강제할 gate 도 없다. 여기서 task 를 조용히
		// 빼면 계획 문서 하나가 사라졌을 때 일감이 통째로 증발한다.
		// 매달린 참조는 fsck 가 보고한다.
		return true
	}
	// closed / cancelled plan 에 속한 task 는 후보가 아니다 (§7.2 4단계).
	if plan.Status != model.PlanActive {
		return false
	}
	return gateOpen(plan.Plan, phaseID, x.phaseCounts(planID))
}

// phaseCounts 는 plan 하나의 phase 별 집계를 만든다.
//
// archive 까지 본다. 완료된 task 는 archive 로 옮겨지므로, issues/ 만 세면
// all_done gate 가 영원히 열리지 않는다.
func (x *planIndex) phaseCounts(planID string) map[string]phaseCounts {
	if counts, cached := x.counts[planID]; cached {
		return counts
	}
	counts := map[string]phaseCounts{}
	entries, err := x.board.List(ListOptions{All: true, Plan: planID})
	if err == nil {
		for _, entry := range entries {
			c := counts[entry.Phase]
			c.total++
			switch entry.Status {
			case model.StatusDone:
				c.done++
				c.terminal++
			case model.StatusCancelled:
				c.terminal++
			}
			counts[entry.Phase] = c
		}
	}
	x.counts[planID] = counts
	return counts
}

// PhaseView 는 plan show 의 phase 한 줄이다. 전부 파생값이다.
type PhaseView struct {
	ID    string     `json:"id"`
	Title string     `json:"title"`
	Gate  model.Gate `json:"gate"`
	Done  int        `json:"done"`
	Total int        `json:"total"`
	// Open 은 gate 가 열려 있는지다. 저장된 상태가 아니다.
	Open bool `json:"open"`
	// State 는 표시용 파생 상태다: done / working / open / blocked.
	//
	// blocked 는 gate 로 막혔다는 뜻이며, 소속 task 의 status 는 그대로
	// open 이다. 이 둘을 섞으면 phase 가 열릴 때 task 를 일괄로 되돌려야
	// 하고, 그것이 곧 plan 단위 경쟁이 된다 (T9.10).
	State   string      `json:"state"`
	Current bool        `json:"current"`
	Tasks   []ListEntry `json:"tasks"`
}

// PlanView 는 plan show 의 결과다 (features §5).
type PlanView struct {
	Plan   model.Plan  `json:"plan"`
	Phases []PhaseView `json:"phases"`
	Done   int         `json:"done"`
	Total  int         `json:"total"`
}

// ShowPlanView 는 진행률과 현재 phase 를 파생 계산한다.
func (b *Board) ShowPlanView(planID string) (*PlanView, error) {
	plan, err := b.ShowPlan(planID)
	if err != nil {
		return nil, err
	}
	entries, err := b.List(ListOptions{All: true, Plan: planID})
	if err != nil {
		return nil, err
	}

	byPhase := map[string][]ListEntry{}
	counts := map[string]phaseCounts{}
	for _, entry := range entries {
		byPhase[entry.Phase] = append(byPhase[entry.Phase], entry)
		c := counts[entry.Phase]
		c.total++
		switch entry.Status {
		case model.StatusDone:
			c.done++
			c.terminal++
		case model.StatusCancelled:
			c.terminal++
		}
		counts[entry.Phase] = c
	}

	view := &PlanView{Plan: plan.Plan, Phases: make([]PhaseView, 0, len(plan.Phases))}
	currentFound := false
	for _, phase := range plan.Phases {
		c := counts[phase.ID]
		tasks := byPhase[phase.ID]
		slices.SortFunc(tasks, func(a, d ListEntry) int {
			if a.Seq != d.Seq {
				return a.Seq - d.Seq
			}
			if a.ID < d.ID {
				return -1
			}
			return 1
		})
		pv := PhaseView{ID: phase.ID, Title: phase.Title, Gate: phase.Gate,
			Done: c.done, Total: c.total, Tasks: tasks}
		pv.Open = gateOpen(plan.Plan, phase.ID, counts)
		pv.State = phaseState(pv, c)
		// 현재 phase 는 열려 있으면서 아직 안 끝난 첫 번째다.
		if !currentFound && pv.Open && pv.State != "done" {
			pv.Current = true
			currentFound = true
		}
		view.Phases = append(view.Phases, pv)
		view.Done += c.done
		view.Total += c.total
	}
	return view, nil
}

func phaseState(pv PhaseView, c phaseCounts) string {
	switch {
	case !pv.Open:
		return "blocked"
	case c.total > 0 && c.terminal == c.total:
		return "done"
	case c.done > 0 || hasActive(pv.Tasks):
		return "working"
	}
	return "open"
}

func hasActive(tasks []ListEntry) bool {
	for _, task := range tasks {
		switch task.Status {
		case model.StatusClaimed, model.StatusWorking, model.StatusBlocked:
			return true
		}
	}
	return false
}
