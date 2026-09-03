package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// T9.1 plan new / phase add / plan show 가 CLI 로 동작한다.
func TestPlanCommands(t *testing.T) {
	dir := doctorRepo(t)

	planID := strings.TrimSpace(runCLI(t, dir, "plan", "new", "storage 재작성", "--priority", "high"))
	if planID != "P01" {
		t.Fatalf("plan ID = %q", planID)
	}
	runCLI(t, dir, "plan", "phase", "add", planID, "스키마 설계")
	runCLI(t, dir, "plan", "phase", "add", planID, "구현", "--gate", "manual")

	first := strings.TrimSpace(runCLI(t, dir, "add", "테이블 정의",
		"--plan", planID, "--phase", "p1", "--seq", "10"))
	second := strings.TrimSpace(runCLI(t, dir, "add", "migration",
		"--plan", planID, "--phase", "p2", "--seq", "10"))

	// p2 는 manual gate 라 막혀 있다. p1 의 task 만 후보다.
	if got := strings.TrimSpace(runCLI(t, dir, "next", "--claim")); got != first {
		t.Fatalf("next = %q, want %s", got, first)
	}

	out := runCLI(t, dir, "plan", "show", planID)
	if !strings.Contains(out, "blocked (gate: manual)") {
		t.Fatalf("gate 표시가 없습니다:\n%s", out)
	}
	if !strings.Contains(out, "← 현재 phase") {
		t.Fatalf("현재 phase 표시가 없습니다:\n%s", out)
	}

	// gate 로 막혀도 저장된 상태는 open 이다 (T9.10).
	if !strings.Contains(runCLI(t, dir, "show", second), "open") {
		t.Fatalf("%s 의 상태가 open 이 아닙니다", second)
	}

	runCLI(t, dir, "plan", "advance", planID, "p2")
	if got := strings.TrimSpace(runCLI(t, dir, "next", "--claim")); got != second {
		t.Fatalf("advance 후 next = %q, want %s", got, second)
	}
}

// plan show --json 은 진행률을 파생값으로 낸다.
func TestPlanShowJSON(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "plan", "new", "계획")
	runCLI(t, dir, "plan", "phase", "add", "P01", "하나")
	id := strings.TrimSpace(runCLI(t, dir, "add", "일감", "--plan", "P01", "--phase", "p1"))
	runCLI(t, dir, "start", id)
	runCLI(t, dir, "done", id)

	var payload struct {
		Data struct {
			Done   int `json:"done"`
			Total  int `json:"total"`
			Phases []struct {
				ID    string `json:"id"`
				State string `json:"state"`
				Open  bool   `json:"open"`
			} `json:"phases"`
		} `json:"data"`
	}
	out := runCLI(t, dir, "plan", "show", "P01", "--json")
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("plan show --json: %v\n%s", err, out)
	}
	if payload.Data.Done != 1 || payload.Data.Total != 1 {
		t.Fatalf("진행률 = %d/%d", payload.Data.Done, payload.Data.Total)
	}
	if len(payload.Data.Phases) != 1 || payload.Data.Phases[0].State != "done" {
		t.Fatalf("phases = %+v", payload.Data.Phases)
	}
}

// plan close 는 소속 task 를 후보에서 뺀다. fsck 는 그 사실을 보고한다.
func TestPlanCloseExcludesTasks(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "plan", "new", "계획")
	runCLI(t, dir, "plan", "phase", "add", "P01", "하나")
	runCLI(t, dir, "add", "일감", "--plan", "P01", "--phase", "p1")
	runCLI(t, dir, "plan", "close", "P01")

	if got := strings.TrimSpace(runCLI(t, dir, "next", "--claim")); got != "" {
		t.Fatalf("closed plan 의 task 가 배정됐습니다: %q", got)
	}
	out, err := runCLIErr(t, dir, "fsck")
	if exitCode(t, err) != ExitGeneral || !strings.Contains(out, "closed_plan_open_task") {
		t.Fatalf("fsck = %v\n%s", err, out)
	}
}

// 소속 task 가 있는 phase 는 제거를 거부한다 (exit 3).
func TestPhaseRemoveRejected(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "plan", "new", "계획")
	runCLI(t, dir, "plan", "phase", "add", "P01", "하나")
	runCLI(t, dir, "add", "일감", "--plan", "P01", "--phase", "p1")

	_, err := runCLIErr(t, dir, "plan", "phase", "remove", "P01", "p1")
	if got := exitCode(t, err); got != ExitTransition {
		t.Fatalf("exit %d, want %d (%v)", got, ExitTransition, err)
	}
}
