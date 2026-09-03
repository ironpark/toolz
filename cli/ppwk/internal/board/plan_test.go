package board

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// makePlan 은 plan 하나와 phase 들을 정상 경로로 만든다.
func makePlan(t *testing.T, b *Board, title string, priority model.Priority, phases ...model.Phase) *Plan {
	t.Helper()
	plan, err := b.AddPlan(PlanAddOptions{Title: title, Priority: priority})
	if err != nil {
		t.Fatalf("AddPlan() = %v", err)
	}
	for _, phase := range phases {
		plan, err = b.AddPhase(plan.ID, PhaseAddOptions{Title: phase.Title, Gate: phase.Gate, ID: phase.ID})
		if err != nil {
			t.Fatalf("AddPhase(%s) = %v", phase.ID, err)
		}
	}
	return plan
}

// task 는 plan/phase 에 속한 이슈 하나를 만든다.
func task(t *testing.T, b *Board, plan, phase string, seq int) *Issue {
	t.Helper()
	return mustAdd(t, b, AddOptions{
		Title: fmt.Sprintf("%s/%s seq %d", plan, phase, seq),
		Plan:  plan, Phase: phase, Seq: seq,
	})
}

// candidateIDs 는 지금 후보인 이슈 ID 다.
func candidateIDs(t *testing.T, b *Board) []string {
	t.Helper()
	return ids(mustNext(t, b, NextOptions{DryRun: true}).Candidates)
}

// T9.1 plan new / phase add / plan show 가 동작한다.
func TestPlanLifecycle(t *testing.T) {
	b := initBoard(t)
	plan, err := b.AddPlan(PlanAddOptions{Title: "storage 재작성", Priority: model.PriorityHigh})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != "P01" || plan.Status != model.PlanActive || len(plan.Phases) != 0 {
		t.Fatalf("plan = %+v", plan.Plan)
	}

	// phase ID 는 생략하면 p1, p2 … 로 채번된다.
	for _, title := range []string{"스키마 설계", "구현"} {
		if plan, err = b.AddPhase(plan.ID, PhaseAddOptions{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	if len(plan.Phases) != 2 || plan.Phases[0].ID != "p1" || plan.Phases[1].ID != "p2" {
		t.Fatalf("phases = %+v", plan.Phases)
	}
	if plan.Phases[0].Gate != model.GateAllDone {
		t.Fatalf("기본 gate = %q", plan.Phases[0].Gate)
	}

	// --before / --after 로 사이에 끼운다.
	if plan, err = b.AddPhase(plan.ID, PhaseAddOptions{Title: "리뷰", After: "p1"}); err != nil {
		t.Fatal(err)
	}
	if got := phaseIDs(plan); fmt.Sprint(got) != "[p1 p3 p2]" {
		t.Fatalf("phase 순서 = %v", got)
	}

	first := task(t, b, plan.ID, "p1", 10)
	second := task(t, b, plan.ID, "p1", 20)
	transitionAll(t, b, first.ID, ActionStart, ActionDone)

	view, err := b.ShowPlanView(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Done != 1 || view.Total != 2 {
		t.Fatalf("진행률 = %d/%d", view.Done, view.Total)
	}
	if view.Phases[0].Done != 1 || view.Phases[0].Total != 2 || view.Phases[0].State != "working" {
		t.Fatalf("p1 = %+v", view.Phases[0])
	}
	if !view.Phases[0].Current {
		t.Fatal("p1 이 현재 phase 가 아닙니다")
	}
	if len(view.Phases[0].Tasks) != 2 || view.Phases[0].Tasks[1].ID != second.ID {
		t.Fatalf("p1 tasks = %+v", view.Phases[0].Tasks)
	}
	// 두 번째 phase(p3)는 앞이 안 끝나 막혀 있다.
	if view.Phases[1].Open || view.Phases[1].State != "blocked" {
		t.Fatalf("p3 = %+v", view.Phases[1])
	}

	// plan list 는 ID 순이다.
	plans, err := b.ListPlans("")
	if err != nil || len(plans) != 1 || plans[0].ID != plan.ID {
		t.Fatalf("ListPlans() = %v, %v", plans, err)
	}
}

func phaseIDs(plan *Plan) []string {
	out := make([]string, 0, len(plan.Phases))
	for _, phase := range plan.Phases {
		out = append(out, phase.ID)
	}
	return out
}

// T9.2 all_done gate 는 직전 phase 에 미완이 하나라도 있으면 막는다.
func TestAllDoneGate(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone},
		model.Phase{ID: "p2", Title: "둘", Gate: model.GateAllDone})
	a := task(t, b, plan.ID, "p1", 10)
	c := task(t, b, plan.ID, "p1", 20)
	next := task(t, b, plan.ID, "p2", 10)

	// p1 이 안 끝났으므로 p2 의 task 는 후보가 아니다.
	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{a.ID, c.ID}) {
		t.Fatalf("후보 = %v, want [%s %s]", got, a.ID, c.ID)
	}

	transitionAll(t, b, a.ID, ActionStart, ActionDone)
	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{c.ID}) {
		t.Fatalf("하나만 끝났는데 후보 = %v", got)
	}

	// cancelled 도 all_done 을 충족한다 (§3.7.5).
	transitionAll(t, b, c.ID, ActionCancel)
	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{next.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, next.ID)
	}
}

// T9.3 any_done gate 는 하나만 done 되어도 열린다.
func TestAnyDoneGate(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone},
		model.Phase{ID: "p2", Title: "둘", Gate: model.GateAnyDone})
	a := task(t, b, plan.ID, "p1", 10)
	task(t, b, plan.ID, "p1", 20)
	next := task(t, b, plan.ID, "p2", 10)

	if got := candidateIDs(t, b); len(got) != 2 {
		t.Fatalf("후보 = %v", got)
	}
	transitionAll(t, b, a.ID, ActionStart, ActionDone)
	if got := candidateIDs(t, b); len(got) != 2 || got[1] != next.ID {
		t.Fatalf("후보 = %v, want %s 포함", got, next.ID)
	}
}

// any_done 은 cancelled 로 열리지 않는다. 취소는 일이 끝난 것이 아니다.
func TestAnyDoneGateIgnoresCancelled(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone},
		model.Phase{ID: "p2", Title: "둘", Gate: model.GateAnyDone})
	a := task(t, b, plan.ID, "p1", 10)
	next := task(t, b, plan.ID, "p2", 10)

	transitionAll(t, b, a.ID, ActionCancel)
	for _, id := range candidateIDs(t, b) {
		if id == next.ID {
			t.Fatal("cancelled 가 any_done 을 열었습니다")
		}
	}
}

// T9.4 manual gate 는 advance 전까지 막히고, 이후 열린다.
func TestManualGate(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone},
		model.Phase{ID: "p2", Title: "둘", Gate: model.GateManual})
	a := task(t, b, plan.ID, "p1", 10)
	next := task(t, b, plan.ID, "p2", 10)

	// 직전 phase 를 다 끝내도 manual 은 열리지 않는다.
	transitionAll(t, b, a.ID, ActionStart, ActionDone)
	if got := candidateIDs(t, b); len(got) != 0 {
		t.Fatalf("advance 전인데 후보 = %v", got)
	}

	if _, err := b.AdvancePhase(plan.ID, "p2"); err != nil {
		t.Fatal(err)
	}
	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{next.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, next.ID)
	}
	// 없는 phase 를 advance 하면 거부한다.
	if _, err := b.AdvancePhase(plan.ID, "p9"); err == nil {
		t.Fatal("없는 phase 를 열었습니다")
	}
}

// T9.5 task 없는 phase 는 all_done 을 공허참으로 통과시키고, fsck 가 경고한다.
func TestEmptyPhasePassesGateAndWarns(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "빈 단계", Gate: model.GateAllDone},
		model.Phase{ID: "p2", Title: "둘", Gate: model.GateAllDone})
	next := task(t, b, plan.ID, "p2", 10)

	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{next.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, next.ID)
	}
	got := findingsFor(t, b, CheckEmptyPhase)
	if len(got) != 1 || got[0].Level != LevelWarn {
		t.Fatalf("fsck = %v", got)
	}
}

// T9.7 plan 을 닫으면 소속 open task 가 후보에서 빠진다.
func TestClosedPlanExcludesTasks(t *testing.T) {
	for _, status := range []model.PlanStatus{model.PlanClosed, model.PlanCancelled} {
		t.Run(string(status), func(t *testing.T) {
			b := initBoard(t)
			plan := makePlan(t, b, "계획", model.PriorityMed,
				model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone})
			member := task(t, b, plan.ID, "p1", 10)
			loose := mustAdd(t, b, AddOptions{Title: "plan 밖"})

			if got := candidateIDs(t, b); len(got) != 2 {
				t.Fatalf("후보 = %v", got)
			}
			if _, err := b.SetPlanStatus(plan.ID, status); err != nil {
				t.Fatal(err)
			}
			if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{loose.ID}) {
				t.Fatalf("후보 = %v, want [%s]", got, loose.ID)
			}
			// 상태는 그대로 open 이다. 후보 판정은 저장하지 않는다.
			after, err := b.Show(member.ID)
			if err != nil || after.Status != model.StatusOpen {
				t.Fatalf("%s = %v, %v", member.ID, after.Status, err)
			}
			// fsck 가 이 상황을 보고한다.
			if got := findingsFor(t, b, CheckClosedPlanOpen); len(got) != 1 {
				t.Fatalf("fsck = %v", got)
			}
		})
	}
}

// T9.8 같은 plan 안에서는 seq 가 priority 를 앞선다.
func TestSeqBeatsPriorityAcrossPhases(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone})
	early := mustAdd(t, b, AddOptions{Title: "med seq 10", Plan: plan.ID, Phase: "p1", Seq: 10,
		Priority: model.PriorityMed})
	late := mustAdd(t, b, AddOptions{Title: "high seq 30", Plan: plan.ID, Phase: "p1", Seq: 30,
		Priority: model.PriorityHigh})

	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{early.ID, late.ID}) {
		t.Fatalf("순서 = %v, want [%s %s]", got, early.ID, late.ID)
	}
}

// T9.9 plan 에 속하지 않은 이슈가 계획 이슈들과 함께 정렬된다.
func TestLooseIssueSortsWithPlanned(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone})
	first := task(t, b, plan.ID, "p1", 10)
	second := task(t, b, plan.ID, "p1", 20)
	loose := mustAdd(t, b, AddOptions{Title: "plan 밖 med", Priority: model.PriorityMed})

	// plan 밖 med 는 자리 15 — seq 10 과 20 사이다.
	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{first.ID, loose.ID, second.ID}) {
		t.Fatalf("순서 = %v", got)
	}
}

// T9.10 gate 로 막힌 task 의 status 는 open 이지 blocked 가 아니다.
//
// gate 는 조회 시점 판단이지 저장된 상태가 아니다. 이걸 어기면 phase 가
// 열릴 때 task 를 일괄로 되돌려야 하고, 그것이 plan 단위 경쟁이 된다.
func TestGateDoesNotChangeStoredStatus(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone},
		model.Phase{ID: "p2", Title: "둘", Gate: model.GateManual})
	task(t, b, plan.ID, "p1", 10)
	blocked := task(t, b, plan.ID, "p2", 10)

	before, err := b.Show(blocked.ID)
	if err != nil {
		t.Fatal(err)
	}
	candidateIDs(t, b)

	after, err := b.Show(blocked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != model.StatusOpen {
		t.Fatalf("상태 = %s, want open", after.Status)
	}
	if after.Commit != before.Commit {
		t.Fatal("후보 판정이 이슈를 썼습니다")
	}
	// 표시상으로만 blocked 다.
	view, err := b.ShowPlanView(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Phases[1].State != "blocked" || view.Phases[1].Tasks[0].Status != model.StatusOpen {
		t.Fatalf("p2 = %+v", view.Phases[1])
	}
}

// phase 제거는 소속 task 가 있으면 거부한다.
func TestRemovePhaseRefusesWhenInUse(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone},
		model.Phase{ID: "p2", Title: "둘", Gate: model.GateAllDone})
	member := task(t, b, plan.ID, "p2", 10)

	if _, err := b.RemovePhase(plan.ID, "p2"); !errors.Is(err, ErrPhaseInUse) {
		t.Fatalf("RemovePhase() = %v, want ErrPhaseInUse", err)
	}
	// task 가 끝나 archive 로 가도 여전히 소속이다.
	transitionAll(t, b, member.ID, ActionStart, ActionDone)
	if _, err := b.RemovePhase(plan.ID, "p2"); !errors.Is(err, ErrPhaseInUse) {
		t.Fatalf("archive 된 task 를 고아로 만들었습니다: %v", err)
	}
	// 빈 phase 는 지워진다.
	after, err := b.RemovePhase(plan.ID, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(phaseIDs(after)) != "[p2]" {
		t.Fatalf("phases = %v", phaseIDs(after))
	}
}

// phase 를 재정렬해도 기존 task 는 영향받지 않는다. phase 필드는 id 참조다.
func TestPhaseReorderDoesNotTouchTasks(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone})
	member := task(t, b, plan.ID, "p1", 10)
	before, err := b.Show(member.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := b.AddPhase(plan.ID, PhaseAddOptions{Title: "앞에 끼움", ID: "p0", Before: "p1"}); err != nil {
		t.Fatal(err)
	}
	after, err := b.Show(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Commit != before.Commit || after.Phase != "p1" {
		t.Fatalf("task 가 바뀌었습니다: %s %s", after.Phase, after.Commit)
	}
	// 이제 p1 은 두 번째이고 앞이 비어 all_done 이 공허참으로 통과한다.
	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{member.ID}) {
		t.Fatalf("후보 = %v", got)
	}
}

// seq 음수는 허용된다. 선두 삽입 용도다.
func TestNegativeSeqSortsFirst(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "계획", model.PriorityMed,
		model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone})
	normal := task(t, b, plan.ID, "p1", 10)
	urgent := task(t, b, plan.ID, "p1", -10)

	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{urgent.ID, normal.ID}) {
		t.Fatalf("순서 = %v", got)
	}
}

// plan 문서가 없으면 gate 도 없다. 계획이 사라졌다고 일감까지 증발하면 안 된다.
func TestMissingPlanDoesNotHideTasks(t *testing.T) {
	b := initBoard(t)
	orphan := mustAdd(t, b, AddOptions{Title: "고아", Plan: "P99", Phase: "p1", Seq: 10})

	if got := candidateIDs(t, b); fmt.Sprint(got) != fmt.Sprint([]string{orphan.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, orphan.ID)
	}
	if got := findingsFor(t, b, CheckMissingPlan); len(got) != 1 {
		t.Fatalf("fsck 가 보고하지 않았습니다: %v", got)
	}
}

// ---- fuzz ----

// F9.1 gate 계산 불변식.
//
// gate 는 조회 시점 판단이라 틀려도 데이터에 흔적이 남지 않는다. 그래서
// 사후에 발견하기 어렵고, 임의 구성으로 미리 흔들어보는 값이 크다.
func FuzzPhaseGate(f *testing.F) {
	f.Add(uint64(1))
	f.Add(uint64(42))
	f.Fuzz(func(t *testing.T, seed uint64) {
		plan, counts := generatePlan(seed)
		for index, phase := range plan.Phases {
			open := gateOpen(plan, phase.ID, counts)

			if index == 0 && !open {
				t.Fatalf("첫 phase %s 가 닫혔습니다", phase.ID)
			}
			if index == 0 {
				continue
			}
			previous := counts[plan.Phases[index-1].ID]
			switch phase.Gate {
			case model.GateAllDone:
				if open != (previous.terminal == previous.total) {
					t.Fatalf("all_done %s: open=%v, 직전 %+v", phase.ID, open, previous)
				}
			case model.GateAnyDone:
				if open != (previous.done > 0) {
					t.Fatalf("any_done %s: open=%v, 직전 %+v", phase.ID, open, previous)
				}
			case model.GateManual:
				want := false
				for _, id := range plan.AdvancedPhases {
					if id == phase.ID {
						want = true
					}
				}
				if open != want {
					t.Fatalf("manual %s: open=%v, want %v", phase.ID, open, want)
				}
			}
		}
		// 없는 phase 를 물어도 panic 하지 않고 닫힘이다.
		if gateOpen(plan, "없는-phase", counts) {
			t.Fatal("없는 phase 가 열렸습니다")
		}
	})
}

// generatePlan 은 seed 로 phase 구성과 상태 분포를 결정적으로 만든다.
func generatePlan(seed uint64) (model.Plan, map[string]phaseCounts) {
	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
	gates := []model.Gate{model.GateAllDone, model.GateAnyDone, model.GateManual, model.Gate("알 수 없음")}

	n := rng.IntN(8)
	plan := model.Plan{Schema: 1, ID: "P01", Title: "t", Status: model.PlanActive,
		Priority: model.PriorityMed}
	counts := map[string]phaseCounts{}
	for i := range n {
		id := fmt.Sprintf("p%d", i+1)
		plan.Phases = append(plan.Phases, model.Phase{ID: id, Title: id, Gate: gates[rng.IntN(len(gates))]})

		total := rng.IntN(5)
		done := rng.IntN(total + 1)
		cancelled := rng.IntN(total - done + 1)
		counts[id] = phaseCounts{total: total, done: done, terminal: done + cancelled}

		if rng.IntN(3) == 0 {
			plan.AdvancedPhases = append(plan.AdvancedPhases, id)
		}
	}
	return plan, counts
}
