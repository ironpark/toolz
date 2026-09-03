package model

import (
	"encoding/json"
	"fmt"
)

// PlanStatus 는 plan 의 상태다 (§3.7.6).
type PlanStatus string

const (
	PlanActive    PlanStatus = "active"
	PlanClosed    PlanStatus = "closed"
	PlanCancelled PlanStatus = "cancelled"
)

// Valid 는 아는 상태인지 본다.
func (s PlanStatus) Valid() bool {
	switch s {
	case PlanActive, PlanClosed, PlanCancelled:
		return true
	}
	return false
}

// Gate 는 phase 가 열리기 위한 조건이다.
type Gate string

const (
	GateAllDone Gate = "all_done"
	GateAnyDone Gate = "any_done"
	GateManual  Gate = "manual"
)

// Valid 는 아는 gate 인지 본다.
func (g Gate) Valid() bool {
	switch g {
	case GateAllDone, GateAnyDone, GateManual:
		return true
	}
	return false
}

// Phase 는 plan 안의 한 단계다.
//
// 별도 ref 가 아니라 plan 문서 안의 배열이다. phase 는 독립적으로 claim 되지 않고
// 수명이 plan 에 종속되기 때문이다 (§3.1).
type Phase struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Gate  Gate   `json:"gate"`
}

// Plan 은 plan.json 이다 (§3.7.2).
//
// task 목록을 갖지 않는다. 엣지는 task 가 위로 가리키는 한 방향뿐이다.
// 진행률도 저장하지 않는다 — 조회 시점에 계산한다.
type Plan struct {
	Schema   int        `json:"schema"`
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Status   PlanStatus `json:"status"`
	Priority Priority   `json:"priority"`
	Phases   []Phase    `json:"phases"`
	// AdvancedPhases 는 manual gate 를 사람이 연 phase 다.
	AdvancedPhases []string  `json:"advanced_phases"`
	CreatedAt      Timestamp `json:"created_at"`
	UpdatedAt      Timestamp `json:"updated_at"`

	extra map[string]json.RawMessage
}

type planAlias Plan

// UnmarshalJSON 은 아는 필드를 채우고 나머지를 보존한다.
func (p *Plan) UnmarshalJSON(data []byte) error {
	var alias planAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*p = Plan(alias)

	extra, err := unknownFields(data, knownPlanFields)
	if err != nil {
		return err
	}
	p.extra = extra

	if p.Schema == 0 {
		p.Schema = SchemaVersion
	}
	return nil
}

// MarshalJSON 은 아는 필드와 보존한 필드를 합쳐 낸다.
func (p Plan) MarshalJSON() ([]byte, error) {
	return marshalWithExtra(planAlias(p), p.extra)
}

// Extra 는 보존된 미지 필드를 돌려준다.
func (p Plan) Extra() map[string]json.RawMessage {
	return p.extra
}

var knownPlanFields = []string{
	"schema", "id", "title", "status", "priority",
	"phases", "advanced_phases", "created_at", "updated_at",
}

// Phase 는 id 에 해당하는 phase 를 찾는다.
func (p Plan) Phase(id string) (Phase, bool) {
	for _, phase := range p.Phases {
		if phase.ID == id {
			return phase, true
		}
	}
	return Phase{}, false
}

// Validate 는 저장 가능한 상태인지 본다.
func (p Plan) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("id 가 비어 있습니다")
	}
	if p.Title == "" {
		return fmt.Errorf("제목이 비어 있습니다")
	}
	if !p.Status.Valid() {
		return fmt.Errorf("알 수 없는 plan 상태입니다: %q", p.Status)
	}
	if !p.Priority.Valid() {
		return fmt.Errorf("알 수 없는 우선순위입니다: %q", p.Priority)
	}
	seen := make(map[string]bool, len(p.Phases))
	for _, phase := range p.Phases {
		if phase.ID == "" {
			return fmt.Errorf("phase id 가 비어 있습니다")
		}
		if seen[phase.ID] {
			return fmt.Errorf("phase id 가 중복됩니다: %s", phase.ID)
		}
		seen[phase.ID] = true
		if !phase.Gate.Valid() {
			return fmt.Errorf("알 수 없는 gate 입니다: %q", phase.Gate)
		}
	}
	for _, id := range p.AdvancedPhases {
		if !seen[id] {
			return fmt.Errorf("advanced_phases 가 없는 phase 를 가리킵니다: %s", id)
		}
	}
	return nil
}
