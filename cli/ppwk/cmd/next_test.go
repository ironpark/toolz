package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// nextJSON 은 next --json 의 payload 를 읽는다.
func nextJSON(t *testing.T, dir string, args ...string) (candidates []string, claimed string) {
	t.Helper()
	var payload struct {
		Data struct {
			Candidates []struct {
				ID string `json:"id"`
			} `json:"candidates"`
			Claimed *struct {
				ID string `json:"id"`
			} `json:"claimed"`
		} `json:"data"`
	}
	out := runCLI(t, dir, append([]string{"next", "--json"}, args...)...)
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("next --json: %v\n%s", err, out)
	}
	for _, c := range payload.Data.Candidates {
		candidates = append(candidates, c.ID)
	}
	if payload.Data.Claimed != nil {
		claimed = payload.Data.Claimed.ID
	}
	return candidates, claimed
}

// T5.1 후보가 없어도 오류가 아니다 — exit 0 에 빈 결과다.
//
// runCLI 는 오류가 나면 실패하므로, 이 호출이 통과하는 것 자체가 exit 0 이다.
func TestNextEmptyIsNotAnError(t *testing.T) {
	dir := doctorRepo(t)
	candidates, claimed := nextJSON(t, dir, "--claim")
	if len(candidates) != 0 || claimed != "" {
		t.Fatalf("후보=%v claimed=%q", candidates, claimed)
	}
}

// next --claim 은 이슈를 가져오고 ID 를 낸다.
func TestNextClaimsAndPrintsID(t *testing.T) {
	dir := doctorRepo(t)
	id := strings.TrimSpace(runCLI(t, dir, "add", "일감"))

	if got := strings.TrimSpace(runCLI(t, dir, "next", "--claim")); got != id {
		t.Fatalf("출력 = %q, want %q", got, id)
	}
	if out := runCLI(t, dir, "show", id); !strings.Contains(out, "claimed") {
		t.Fatalf("상태가 claimed 가 아닙니다:\n%s", out)
	}
	// 이미 가져갔으므로 다음 호출은 빈 결과다.
	if _, claimed := nextJSON(t, dir, "--claim"); claimed != "" {
		t.Fatalf("두 번째 호출이 %q 를 가져왔습니다", claimed)
	}
}

// --dry-run 은 후보만 보여주고 아무것도 가져가지 않는다.
func TestNextDryRunDoesNotClaim(t *testing.T) {
	dir := doctorRepo(t)
	id := strings.TrimSpace(runCLI(t, dir, "add", "일감"))

	candidates, claimed := nextJSON(t, dir, "--dry-run", "--claim")
	if claimed != "" {
		t.Fatalf("dry-run 이 %q 를 가져갔습니다", claimed)
	}
	if len(candidates) != 1 || candidates[0] != id {
		t.Fatalf("후보 = %v, want [%s]", candidates, id)
	}
	if out := runCLI(t, dir, "show", id); !strings.Contains(out, "open") {
		t.Fatalf("상태가 open 이 아닙니다:\n%s", out)
	}
}
