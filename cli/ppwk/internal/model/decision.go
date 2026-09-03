package model

import (
	"encoding/json"
	"fmt"
)

// Decision 은 decision.json 이다 (§3.9).
//
// **불변이다.** 상태도 updated_at 도 없다. 만들어지면 결정이고, 바꾸려면
// Supersedes 로 새 결정을 만든다 — 이력이 곧 논거의 변천이다.
//
// 엣지는 한 방향뿐이다: 결정 → 이슈 / plan / 이전 결정. 역방향
// (superseded_by, 이슈에서 결정 목록)은 조회 시 계산한다. 양방향으로 두면
// 두 ref 를 원자적으로 갱신해야 하고, 어긋나면 복구가 어렵다 (D6).
type Decision struct {
	Schema  int      `json:"schema"`
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Context string   `json:"context,omitempty"`
	Options []string `json:"options,omitempty"`
	// Chosen 은 택한 것이다. JSON 키는 "decision" 이지만 Go 이름을 나눈
	// 이유는, 이 구조체를 embed 하면 Decision.Decision 이 되어 embed 한
	// 타입 이름과 필드 이름이 충돌하기 때문이다.
	Chosen       string    `json:"decision,omitempty"`
	Consequences string    `json:"consequences,omitempty"`
	Issues       []string  `json:"issues,omitempty"`
	Plan         string    `json:"plan,omitempty"`
	Supersedes   string    `json:"supersedes,omitempty"`
	DecidedBy    string    `json:"decided_by"`
	DecidedAt    Timestamp `json:"decided_at"`

	extra map[string]json.RawMessage
}

type decisionAlias Decision

// UnmarshalJSON 은 아는 필드를 채우고 나머지를 보존한다.
func (d *Decision) UnmarshalJSON(data []byte) error {
	var alias decisionAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*d = Decision(alias)

	extra, err := unknownFields(data, knownDecisionFields)
	if err != nil {
		return err
	}
	d.extra = extra

	if d.Schema == 0 {
		d.Schema = SchemaVersion
	}
	return nil
}

// MarshalJSON 은 아는 필드와 보존한 필드를 합쳐 낸다.
func (d Decision) MarshalJSON() ([]byte, error) {
	return marshalWithExtra(decisionAlias(d), d.extra)
}

// Extra 는 보존된 미지 필드를 돌려준다.
func (d Decision) Extra() map[string]json.RawMessage { return d.extra }

var knownDecisionFields = []string{
	"schema", "id", "title", "context", "options", "decision", "consequences",
	"issues", "plan", "supersedes", "decided_by", "decided_at",
}

// Validate 는 저장 가능한 상태인지 본다.
//
// options 가 비었거나 decision 이 options 에 없는 것은 막지 않는다. 사후에
// 추가된 선택지일 수 있고, 기록을 거부하는 것보다 남기는 편이 낫다 — 그 대신
// decide 가 그 자리에서 경고한다.
func (d Decision) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("id 가 비어 있습니다")
	}
	if d.Title == "" {
		return fmt.Errorf("제목이 비어 있습니다")
	}
	if d.Supersedes == d.ID && d.ID != "" && d.Supersedes != "" {
		return fmt.Errorf("자기 자신을 대체할 수 없습니다: %s", d.ID)
	}
	return nil
}
