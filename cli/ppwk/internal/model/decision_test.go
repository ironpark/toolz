package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// 결정은 불변이고 엣지는 한 방향이다. 스키마에 그 자리가 생기면 누군가 채운다.
//
// 직렬화 결과가 아니라 구조체 필드를 본다. omitempty 가 붙으면 비어 있는 동안
// 출력에 나타나지 않으므로, 출력만 보는 검사는 필드가 생긴 것을 놓친다.
func TestDecisionHasNoMutableFields(t *testing.T) {
	banned := map[string]string{
		"updated_at":    "결정은 불변이라 갱신 시각이 없다",
		"status":        "결정에 상태 머신을 두지 않는다",
		"superseded_by": "역방향 엣지는 조회 시 계산한다 (D6)",
	}
	typ := reflect.TypeOf(Decision{})
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		for name, why := range banned {
			if tag == name || strings.EqualFold(field.Name, name) {
				t.Fatalf("Decision 에 %q 필드가 있습니다 — %s (§3.9)", name, why)
			}
		}
	}
	// knownDecisionFields 도 함께 본다. 필드 없이 여기만 늘어나면 미지 필드
	// 보존이 조용히 깨진다.
	for _, known := range knownDecisionFields {
		if why, bad := banned[known]; bad {
			t.Fatalf("knownDecisionFields 에 %q 가 있습니다 — %s (§3.9)", known, why)
		}
	}
}

func TestDecisionValidate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision Decision
		ok       bool
	}{
		{"정상", Decision{ID: "D001", Title: "t"}, true},
		{"id 없음", Decision{Title: "t"}, false},
		{"제목 없음", Decision{ID: "D001"}, false},
		{"자기 자신 대체", Decision{ID: "D001", Title: "t", Supersedes: "D001"}, false},
		// 아래 둘은 허용한다. 기록을 거부하는 것보다 남기는 편이 낫다.
		{"선택지 없음", Decision{ID: "D001", Title: "t", Chosen: "A"}, true},
		{"택한 것이 목록 밖", Decision{ID: "D001", Title: "t", Options: []string{"A"}, Chosen: "B"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decision.Validate()
			if (err == nil) != tc.ok {
				t.Fatalf("Validate() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

// F12.1 우리 출력은 우리가 다시 읽고, 미지 필드가 유실되지 않는다.
func FuzzDecisionJSONRoundTrip(f *testing.F) {
	f.Add(`{"id":"D1","title":"x"}`)
	f.Add(`{"id":"D1","title":"x","zz":1}`)
	f.Add(`{"schema":9,"id":"D7","title":"한글","options":["a","b"],"decision":"a","issues":["T001"]}`)

	f.Fuzz(func(t *testing.T, raw string) {
		var first Decision
		if err := json.Unmarshal([]byte(raw), &first); err != nil {
			return // 우리가 만든 것이 아니면 관심 없다.
		}
		once, err := Marshal(first)
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}
		var second Decision
		if err := json.Unmarshal(once, &second); err != nil {
			t.Fatalf("우리가 낸 것을 우리가 못 읽습니다: %v\n%s", err, once)
		}
		twice, err := Marshal(second)
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}
		if string(once) != string(twice) {
			t.Fatalf("왕복이 깨집니다:\n1회 %s\n2회 %s", once, twice)
		}
		if len(first.Extra()) != len(second.Extra()) {
			t.Fatalf("미지 필드가 유실됐습니다: %v → %v", first.Extra(), second.Extra())
		}
	})
}
