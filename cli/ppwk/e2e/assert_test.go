package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// add 는 이슈 하나를 만들고 ID 를 돌려준다.
func (f *Fixture) add(title string, args ...string) string {
	f.t.Helper()
	return issueID(f.t, f.RunJSON(append([]string{"add", title}, args...)...))
}

// issueID 는 --json 결과에서 ID 를 꺼낸다.
func issueID(t *testing.T, data any) string {
	t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("이슈 객체가 아닙니다: %#v", data)
	}
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatalf("id 가 없습니다: %#v", data)
	}
	return id
}

// show 는 이슈 하나를 map 으로 읽는다.
func (f *Fixture) show(id string) map[string]any {
	f.t.Helper()
	m, ok := f.RunJSON("show", id).(map[string]any)
	if !ok {
		f.t.Fatalf("show %s 가 객체가 아닙니다", id)
	}
	return m
}

// expectStatus 는 CLI 와 git 양쪽에서 상태를 확인한다 (§0.2 의 이중화).
func (f *Fixture) expectStatus(id, want string) {
	f.t.Helper()
	if got := f.show(id)["status"]; got != want {
		f.t.Fatalf("show %s status = %v, want %s", id, got, want)
	}
	ref := "refs/ppwk/issues/" + id
	if !f.HasRef(ref) {
		ref = "refs/ppwk/archive/" + id
	}
	message := f.Git("log", "-1", "--format=%B", ref)
	if !strings.Contains(message, "Status: "+want) {
		f.t.Fatalf("%s 의 trailer 에 Status: %s 가 없습니다:\n%s", id, want, message)
	}
}

// agentID 는 이 실행 환경에서 결정된 에이전트 ID 다.
func (f *Fixture) agentID() string {
	f.t.Helper()
	data, _ := f.RunJSON("doctor").(map[string]any)
	checks, _ := data["checks"].([]any)
	for _, raw := range checks {
		check, _ := raw.(map[string]any)
		if check["name"] == "agent id" {
			return fmt.Sprint(check["value"])
		}
	}
	f.t.Fatal("doctor 에 agent id 항목이 없습니다")
	return ""
}

// doctorCheck 는 doctor 의 항목 하나를 돌려준다.
func (f *Fixture) doctorCheck(name string) map[string]any {
	f.t.Helper()
	return f.doctorCheckIn(f.Root, nil, name)
}

func (f *Fixture) doctorCheckIn(dir string, extra []string, name string) map[string]any {
	f.t.Helper()
	data, _ := f.runJSONIn(dir, extra, "doctor").(map[string]any)
	checks, _ := data["checks"].([]any)
	for _, raw := range checks {
		check, _ := raw.(map[string]any)
		if check["name"] == name {
			return check
		}
	}
	f.t.Fatalf("doctor 에 %q 항목이 없습니다: %v", name, checks)
	return nil
}

// listIDs 는 조건에 맞는 이슈 ID 목록이다.
func (f *Fixture) listIDs(args ...string) []string {
	f.t.Helper()
	items, _ := f.RunJSON(append([]string{"list"}, args...)...).([]any)
	var ids []string
	for _, raw := range items {
		if m, ok := raw.(map[string]any); ok {
			ids = append(ids, fmt.Sprint(m["id"]))
		}
	}
	return ids
}

// candidates 는 next --dry-run 의 후보 ID 목록이다.
func (f *Fixture) candidates(args ...string) []string {
	f.t.Helper()
	data, _ := f.RunJSON(append([]string{"next", "--dry-run"}, args...)...).(map[string]any)
	raw, _ := data["candidates"].([]any)
	var ids []string
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			ids = append(ids, fmt.Sprint(m["id"]))
		}
	}
	return ids
}

// expectCleanTree 는 워킹 디렉터리에 아무것도 새로 생기지 않았음을 본다 (§7).
func (f *Fixture) expectCleanTree() {
	f.t.Helper()
	for _, dir := range append([]string{f.Root}, f.worktreePaths()...) {
		if out := strings.TrimSpace(f.GitIn(dir, "status", "--porcelain")); out != "" {
			f.t.Fatalf("%s 가 깨끗하지 않습니다:\n%s", dir, out)
		}
	}
}

func (f *Fixture) worktreePaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var paths []string
	for _, wt := range f.worktrees {
		paths = append(paths, wt.Path)
	}
	return paths
}

// sameJSON 은 두 값이 같은 JSON 인지다.
func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// jsonUnmarshal 은 문자열을 파싱한다. 실패를 호출자가 다루게 둔다.
func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

// fsckChecks 는 fsck 가 보고한 검사 항목 이름의 집합이다.
func (f *Fixture) fsckChecks() map[string]bool {
	f.t.Helper()
	r := f.exec(f.Root, nil, "--json", "fsck")
	var envelope struct {
		Data struct {
			Findings []struct {
				Check string `json:"check"`
			} `json:"findings"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(r.Stdout, &envelope); err != nil {
		f.t.Fatalf("fsck JSON: %v\n%s", err, r.Stdout)
	}
	out := map[string]bool{}
	for _, finding := range envelope.Data.Findings {
		out[finding.Check] = true
	}
	return out
}

// expectGitFsckClean 은 저장소 자체가 성한지 본다.
//
// dangling 은 허용된다 — 진 쪽이 만든 commit 이 남는 것은 정상이고 gc 대상이다.
func (f *Fixture) expectGitFsckClean() {
	f.t.Helper()
	out := f.Git("fsck", "--no-progress", "--no-dangling")
	if strings.TrimSpace(out) != "" {
		f.t.Fatalf("git fsck 가 문제를 보고했습니다:\n%s", out)
	}
}

// ageLease 는 잠금 기록의 last_activity 를 과거로 되돌린다.
//
// 시간을 실제로 흘려보내지 않고 임계값 초과를 재현하는 유일한 방법이다.
// 파일을 직접 고치므로 CLI 의 협조가 필요 없다.
func (f *Fixture) ageLease(agent string, by time.Duration) {
	f.t.Helper()
	dir := filepath.Join(f.commonDir(), "ppwk", "locks")
	paths, _ := filepath.Glob(filepath.Join(dir, "*.lock"))
	changed := 0
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var record map[string]any
		if json.Unmarshal(raw, &record) != nil {
			continue
		}
		if record["agent"] != agent {
			continue
		}
		when, err := time.Parse(time.RFC3339, fmt.Sprint(record["last_activity"]))
		if err != nil {
			f.t.Fatalf("last_activity 파싱: %v", err)
		}
		record["last_activity"] = when.Add(-by).UTC().Format(time.RFC3339)
		out, _ := json.Marshal(record)
		if err := os.WriteFile(p, out, 0o600); err != nil {
			f.t.Fatal(err)
		}
		changed++
	}
	if changed == 0 {
		f.t.Fatalf("%s 의 잠금 기록을 찾지 못했습니다", agent)
	}
}

// killDuring 은 명령을 띄우고 delay 뒤에 SIGKILL 한다.
//
// 쓰기 도중 임의 시점에 죽는 상황을 만든다. 명령이 그 전에 끝나 버리는 것도
// 정상이다 — 검증 대상은 죽은 시점이 아니라 남은 상태다.
func (f *Fixture) killDuring(delay time.Duration, args ...string) {
	f.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = f.Root
	cmd.Env = append(baseEnv(), f.Env...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		f.t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(delay):
		cmd.Process.Signal(syscall.SIGKILL)
		<-done
	}
}

// issueTimeline 은 실패 시 수집물이다 (§10.3).
//
// 어느 프로세스가 어느 commit 을 만들었는지가 유일한 단서다.
func (f *Fixture) issueTimeline(id string) string {
	f.t.Helper()
	ref := "refs/ppwk/issues/" + id
	if !f.HasRef(ref) {
		ref = "refs/ppwk/archive/" + id
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s 이력\n%s", id, f.Git("log", "--format=%h %an %cI %s", ref))
	fmt.Fprintf(&b, "--- 잠금 기록\n")
	for _, lease := range f.leases() {
		fmt.Fprintf(&b, "%s %s %s hook_pid=%v\n",
			lease.Agent, lease.Session, lease.LastActivity, lease.HookPID)
	}
	return b.String()
}

// finish 는 이슈를 start → done 까지 보낸다.
func (f *Fixture) finish(id string) {
	f.t.Helper()
	for _, step := range []string{"start", "done"} {
		if r := f.Run(step, id); r.ExitCode != 0 {
			f.t.Fatalf("%s %s:\n%s", step, id, r)
		}
	}
}

// expectCandidates 는 next 후보가 정확히 이 집합인지 본다.
func (f *Fixture) expectCandidates(want []string) {
	f.t.Helper()
	got := f.candidates()
	slices.Sort(got)
	sorted := slices.Clone(want)
	slices.Sort(sorted)
	if !slices.Equal(got, sorted) {
		f.t.Fatalf("후보 = %v, want %v", got, sorted)
	}
}

// expectProgress 는 plan show 의 파생 진행률과 현재 phase 를 본다.
func (f *Fixture) expectProgress(plan string, done, total int, current string) {
	f.t.Helper()
	data, _ := f.RunJSON("plan", "show", plan).(map[string]any)
	if int(data["done"].(float64)) != done || int(data["total"].(float64)) != total {
		f.t.Fatalf("진행률 = %v/%v, want %d/%d", data["done"], data["total"], done, total)
	}
	phases, _ := data["phases"].([]any)
	for _, raw := range phases {
		phase, _ := raw.(map[string]any)
		if phase["current"] == true {
			if phase["id"] != current {
				f.t.Fatalf("현재 phase = %v, want %s", phase["id"], current)
			}
			return
		}
	}
	if current != "" {
		f.t.Fatalf("현재 phase 가 없습니다, want %s: %v", current, phases)
	}
}

// decide 는 결정 하나를 기록하고 ID 를 돌려준다.
func (f *Fixture) decide(title string, args ...string) string {
	f.t.Helper()
	data, _ := f.RunJSON(append([]string{"decide", title}, args...)...).(map[string]any)
	id := fmt.Sprint(data["id"])
	if id == "" || id == "<nil>" {
		f.t.Fatalf("결정 ID 가 없습니다: %v", data)
	}
	return id
}

// decisionIDs 는 결정 목록의 ID 다.
func (f *Fixture) decisionIDs(args ...string) []string {
	f.t.Helper()
	items, _ := f.RunJSON(append([]string{"decisions"}, args...)...).([]any)
	var ids []string
	for _, raw := range items {
		if m, ok := raw.(map[string]any); ok {
			ids = append(ids, fmt.Sprint(m["id"]))
		}
	}
	return ids
}

// readExported 는 내보낸 결정 파일을 찾아 읽는다.
func (f *Fixture) readExported(dir, id string) string {
	f.t.Helper()
	paths, _ := filepath.Glob(filepath.Join(f.Root, dir, "*"))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.Contains(filepath.Base(p), id) || strings.Contains(string(raw), id) {
			return string(raw)
		}
	}
	f.t.Fatalf("%s 의 내보낸 파일을 찾지 못했습니다: %v", id, paths)
	return ""
}

// expectHookStatus 는 도구별 훅 설치 상태를 본다.
func (f *Fixture) expectHookStatus(tool string, want bool) {
	f.t.Helper()
	items, _ := f.RunJSON("hook", "status").([]any)
	for _, raw := range items {
		row, _ := raw.(map[string]any)
		if row["tool"] != tool {
			continue
		}
		if row["installed"] != want {
			f.t.Fatalf("%s 의 installed = %v, want %v", tool, row["installed"], want)
		}
		return
	}
	f.t.Fatalf("hook status 에 %s 가 없습니다: %v", tool, items)
}
