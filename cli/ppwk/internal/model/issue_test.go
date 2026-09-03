package model

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func sampleIssue() Issue {
	at := NewTimestamp(time.Date(2026, 8, 30, 4, 12, 0, 0, time.UTC))
	return Issue{
		Schema:    SchemaVersion,
		ID:        "T001",
		Title:     "SQLite storage 구현",
		Status:    StatusWorking,
		Priority:  PriorityHigh,
		Labels:    []string{"storage", "backend"},
		Plan:      "P01",
		Phase:     "p2",
		Seq:       30,
		Owner:     "agent-b",
		Session:   "8f3a2c1d",
		DependsOn: []string{"T000"},
		CreatedAt: at,
		UpdatedAt: at,
		UpdatedBy: "agent-b",
	}
}

// T1.1 Issue → JSON → Issue 왕복이 손실 없음
func TestIssueRoundTrip(t *testing.T) {
	want := sampleIssue()

	data, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	var got Issue
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("왕복 후 달라졌습니다:\n got %+v\nwant %+v", got, want)
	}
}

// T1.2 알 수 없는 필드가 있는 JSON 을 읽어도 보존됨 (forward compat)
func TestIssuePreservesUnknownFields(t *testing.T) {
	raw := `{
      "schema": 2,
      "id": "T001",
      "title": "x",
      "status": "open",
      "priority": "med",
      "created_at": "2026-08-30T04:12:00Z",
      "updated_at": "2026-08-30T04:12:00Z",
      "updated_by": "agent-a",
      "future_field": {"nested": [1, 2]},
      "another": "keep me"
    }`

	var issue Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}
	if len(issue.Extra()) != 2 {
		t.Fatalf("Extra() = %v, want 2개", issue.Extra())
	}

	// 아는 필드를 고쳐도 모르는 필드는 살아남아야 한다.
	issue.Status = StatusWorking
	out, err := Marshal(issue)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}

	var back map[string]any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}
	if back["another"] != "keep me" {
		t.Fatalf("미지 필드가 사라졌습니다: %v", back)
	}
	if _, ok := back["future_field"]; !ok {
		t.Fatalf("미지 필드가 사라졌습니다: %v", back)
	}
	if back["status"] != "working" {
		t.Fatalf("status = %v, want working", back["status"])
	}
}

// schema 필드가 없으면 1 로 간주한다 (§9.4).
func TestIssueDefaultsSchema(t *testing.T) {
	var issue Issue
	if err := json.Unmarshal([]byte(`{"id":"T1","title":"x","status":"open","priority":"med"}`), &issue); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}
	if issue.Schema != 1 {
		t.Fatalf("Schema = %d, want 1", issue.Schema)
	}
}

// T1.10 plan/phase/seq 없는 이슈가 정상 처리 (선택 필드)
func TestIssueWithoutPlan(t *testing.T) {
	issue := sampleIssue()
	issue.Plan, issue.Phase, issue.Seq = "", "", 0

	if err := issue.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	data, err := Marshal(issue)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}
	for _, key := range []string{"plan", "phase", "seq"} {
		if _, present := back[key]; present {
			t.Fatalf("%s 가 출력에 남아 있습니다: %v", key, back)
		}
	}
}

// T1.11 plan 만 있고 phase 없음 → 검증 오류
func TestIssuePartialPlanRejected(t *testing.T) {
	issue := sampleIssue()
	issue.Phase = ""

	if err := issue.Validate(); err == nil {
		t.Fatal("Validate() = nil, want error")
	}
}

func TestIssueValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Issue)
		wantErr bool
	}{
		{name: "정상", mutate: func(*Issue) {}},
		{name: "빈 제목", mutate: func(i *Issue) { i.Title = "" }, wantErr: true},
		{name: "빈 id", mutate: func(i *Issue) { i.ID = "" }, wantErr: true},
		{name: "알 수 없는 상태", mutate: func(i *Issue) { i.Status = "weird" }, wantErr: true},
		{name: "알 수 없는 우선순위", mutate: func(i *Issue) { i.Priority = "urgent" }, wantErr: true},
		{name: "자기 자신 의존", mutate: func(i *Issue) { i.DependsOn = []string{i.ID} }, wantErr: true},
		{name: "plan 없이 seq", mutate: func(i *Issue) { i.Plan, i.Phase = "", ""; i.Seq = 10 }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := sampleIssue()
			tt.mutate(&issue)
			err := issue.Validate()
			if tt.wantErr != (err != nil) {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// 비 ASCII 제목과 매우 긴 제목이 보존된다.
func TestIssueTitlePreserved(t *testing.T) {
	long := ""
	for range 300 {
		long += "가"
	}
	for _, title := range []string{"한글 제목 🎉", long, "Status: x 처럼 보이는 제목"} {
		issue := sampleIssue()
		issue.Title = title

		data, err := Marshal(issue)
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}
		var got Issue
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal() = %v", err)
		}
		if got.Title != title {
			t.Fatalf("Title = %q, want %q", got.Title, title)
		}
	}
}

// 직렬화는 결정적이어야 한다. content-addressing 이 걸려 있다.
func TestIssueMarshalDeterministic(t *testing.T) {
	issue := sampleIssue()
	first, err := Marshal(issue)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	for range 20 {
		next, err := Marshal(issue)
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}
		if string(next) != string(first) {
			t.Fatalf("직렬화 결과가 흔들립니다:\n%s\n%s", first, next)
		}
	}
}

// F1.2 우리 출력은 우리가 다시 읽고, 미지 필드가 유실되지 않는다.
func FuzzIssueJSONRoundTrip(f *testing.F) {
	f.Add(`{"id":"T1","title":"x","status":"open","priority":"med"}`)
	f.Add(`{"id":"T1","title":"x","status":"open","priority":"med","zz":1}`)
	f.Add(`{"schema":9,"id":"T1","title":"한글","status":"done","priority":"none","labels":["a"]}`)

	f.Fuzz(func(t *testing.T, raw string) {
		var first Issue
		if err := json.Unmarshal([]byte(raw), &first); err != nil {
			return // 우리가 만든 것이 아니면 관심 없다.
		}
		once, err := Marshal(first)
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}
		var second Issue
		if err := json.Unmarshal(once, &second); err != nil {
			t.Fatalf("우리가 낸 것을 우리가 못 읽습니다: %v\n%s", err, once)
		}
		twice, err := Marshal(second)
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}
		// 바이트로 비교한다. time.Time 은 같은 시각이어도 Location 포인터가
		// 달라 DeepEqual 이 어긋난다 — 의미 차이가 아니다.
		if string(once) != string(twice) {
			t.Fatalf("왕복이 깨집니다:\n1회 %s\n2회 %s", once, twice)
		}
		if len(first.Extra()) != len(second.Extra()) {
			t.Fatalf("미지 필드가 유실됐습니다: %v → %v", first.Extra(), second.Extra())
		}
	})
}
