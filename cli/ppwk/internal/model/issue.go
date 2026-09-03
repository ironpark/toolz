// Package model 은 보드에 저장되는 문서의 스키마다 (design §3.4, §3.7.2, §3.6).
package model

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion 은 이 CLI 가 쓰는 스키마 버전이다 (§9.4).
const SchemaVersion = 1

// Status 는 이슈 상태다 (§3.5).
type Status string

const (
	StatusOpen      Status = "open"
	StatusClaimed   Status = "claimed"
	StatusWorking   Status = "working"
	StatusBlocked   Status = "blocked"
	StatusDone      Status = "done"
	StatusCancelled Status = "cancelled"
)

// Terminal 은 종료 상태인지 알려준다. 종료 상태는 archive/ 로 옮겨진다.
func (s Status) Terminal() bool {
	return s == StatusDone || s == StatusCancelled
}

// Valid 는 아는 상태인지 본다.
func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusClaimed, StatusWorking, StatusBlocked, StatusDone, StatusCancelled:
		return true
	}
	return false
}

// Priority 는 우선순위다. none 은 next 후보에서 빠지는 백로그다.
type Priority string

const (
	PriorityHigh Priority = "high"
	PriorityMed  Priority = "med"
	PriorityLow  Priority = "low"
	PriorityNone Priority = "none"
)

// Valid 는 아는 우선순위인지 본다.
func (p Priority) Valid() bool {
	switch p {
	case PriorityHigh, PriorityMed, PriorityLow, PriorityNone:
		return true
	}
	return false
}

// Issue 는 issue.json 이다 (§3.4).
//
// 생존 정보는 여기 두지 않는다 — 잠금 파일이 갖는다 (§3.6).
type Issue struct {
	Schema    int       `json:"schema"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    Status    `json:"status"`
	Priority  Priority  `json:"priority"`
	Labels    []string  `json:"labels,omitempty"`
	Plan      string    `json:"plan,omitempty"`
	Phase     string    `json:"phase,omitempty"`
	Seq       int       `json:"seq,omitempty"`
	Owner     string    `json:"owner,omitempty"`
	Session   string    `json:"session,omitempty"`
	DependsOn []string  `json:"depends_on,omitempty"`
	CreatedAt Timestamp `json:"created_at"`
	UpdatedAt Timestamp `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`

	// extra 는 이 CLI 가 모르는 필드다.
	//
	// 미래 버전이 추가한 필드를 구버전이 지우면 데이터가 소실된다. 읽은 원본을
	// 보존하고 아는 필드만 갱신한다 (§9.4).
	extra map[string]json.RawMessage
}

// issueAlias 는 재귀를 피하기 위한 별칭이다.
type issueAlias Issue

// UnmarshalJSON 은 아는 필드를 채우고 나머지를 보존한다.
func (i *Issue) UnmarshalJSON(data []byte) error {
	var alias issueAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*i = Issue(alias)

	extra, err := unknownFields(data, knownIssueFields)
	if err != nil {
		return err
	}
	i.extra = extra

	// schema 필드가 없으면 1 로 간주한다 (§9.4).
	if i.Schema == 0 {
		i.Schema = SchemaVersion
	}
	return nil
}

// MarshalJSON 은 아는 필드와 보존한 필드를 합쳐 낸다.
func (i Issue) MarshalJSON() ([]byte, error) {
	return marshalWithExtra(issueAlias(i), i.extra)
}

// Extra 는 보존된 미지 필드를 돌려준다. 테스트와 fsck 용이다.
func (i Issue) Extra() map[string]json.RawMessage {
	return i.extra
}

var knownIssueFields = []string{
	"schema", "id", "title", "status", "priority", "labels",
	"plan", "phase", "seq", "owner", "session", "depends_on",
	"created_at", "updated_at", "updated_by",
}

// Validate 는 저장 가능한 상태인지 본다.
func (i Issue) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("id 가 비어 있습니다")
	}
	if i.Title == "" {
		return fmt.Errorf("제목이 비어 있습니다")
	}
	if !i.Status.Valid() {
		return fmt.Errorf("알 수 없는 상태입니다: %q", i.Status)
	}
	if !i.Priority.Valid() {
		return fmt.Errorf("알 수 없는 우선순위입니다: %q", i.Priority)
	}
	// plan/phase/seq 는 셋이 함께 있거나 함께 없어야 한다 (§3.4).
	if (i.Plan == "") != (i.Phase == "") {
		return fmt.Errorf("plan 과 phase 는 함께 지정해야 합니다 (plan=%q phase=%q)", i.Plan, i.Phase)
	}
	if i.Plan == "" && i.Seq != 0 {
		return fmt.Errorf("plan 없이 seq 를 지정할 수 없습니다")
	}
	for _, dep := range i.DependsOn {
		if dep == i.ID {
			return fmt.Errorf("자기 자신에 의존할 수 없습니다: %s", i.ID)
		}
	}
	return nil
}
