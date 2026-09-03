package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

// T12.1 decide 가 ID 를 내고, decisions 가 그것을 보여준다.
func TestDecideAndList(t *testing.T) {
	dir := doctorRepo(t)
	issue := strings.TrimSpace(runCLI(t, dir, "add", "SQLite storage 구현"))

	id := strings.TrimSpace(runCLI(t, dir, "decide", "저장소는 SQLite",
		"--context", "단일 머신, 동시 쓰기 적음",
		"--option", "SQLite", "--option", "PostgreSQL",
		"--decision", "SQLite",
		"--consequences", "동시 쓰기 확장 시 재검토",
		"--issue", issue))
	if id != "D001" {
		t.Fatalf("ID = %q", id)
	}

	out := runCLI(t, dir, "decisions")
	if !strings.Contains(out, "D001") || !strings.Contains(out, "저장소는 SQLite") {
		t.Fatalf("목록:\n%s", out)
	}

	// T12.8 show 가 연결된 결정을 함께 낸다.
	shown := runCLI(t, dir, "show", issue)
	if !strings.Contains(shown, "D001 저장소는 SQLite") {
		t.Fatalf("show 에 결정이 없습니다:\n%s", shown)
	}
}

// T12.3 수정 명령이 CLI 에 없다.
//
// 불변이라는 성질이 이 기능의 전부다. "편의를 위해" edit 을 더하고 싶어지는
// 지점이라 명령 트리를 직접 훑어 못박는다.
func TestDecisionEditCommandsAbsent(t *testing.T) {
	banned := []string{"edit", "update", "delete", "remove", "amend", "revise", "set"}
	root := New(Version{CLI: "test", Schema: "1"}, io.Discard, io.Discard)

	var walk func(cmds []*cli.Command, path string)
	walk = func(cmds []*cli.Command, path string) {
		for _, c := range cmds {
			full := path + c.Name
			if strings.HasPrefix(full, "decide") || strings.HasPrefix(full, "decisions") {
				if slices.Contains(banned, c.Name) {
					t.Fatalf("%q 명령이 존재합니다 — 결정은 불변입니다 (§3.9)", full)
				}
			}
			walk(c.Commands, full+" ")
		}
	}
	walk(root.Commands, "")

	// 알 수 없는 하위 명령은 조용히 목록으로 떨어지지 않고 거부된다.
	dir := doctorRepo(t)
	runCLI(t, dir, "decide", "결정", "--option", "A", "--decision", "A")
	if _, err := runCLIErr(t, dir, "decisions", "edit", "D001"); err == nil {
		t.Fatal("decisions edit 이 성공했습니다")
	}
}

// T12.5 / T12.6 superseded 는 기본 목록에서 빠지고, --all 에만 나온다.
func TestDecisionsSupersededFiltering(t *testing.T) {
	dir := doctorRepo(t)
	first := strings.TrimSpace(runCLI(t, dir, "decide", "첫째", "--option", "A", "--decision", "A"))
	second := strings.TrimSpace(runCLI(t, dir, "decide", "둘째",
		"--option", "B", "--decision", "B", "--supersedes", first))

	var entries []struct {
		ID           string   `json:"id"`
		SupersededBy []string `json:"superseded_by"`
	}
	decode := func(out string) {
		t.Helper()
		var payload struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		entries = nil
		if err := json.Unmarshal(payload.Data, &entries); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}

	decode(runCLI(t, dir, "decisions", "--json"))
	if len(entries) != 1 || entries[0].ID != second {
		t.Fatalf("기본 목록 = %+v", entries)
	}

	decode(runCLI(t, dir, "decisions", "--all", "--json"))
	if len(entries) != 2 {
		t.Fatalf("--all = %+v", entries)
	}
	if len(entries[0].SupersededBy) != 1 || entries[0].SupersededBy[0] != second {
		t.Fatalf("superseded_by = %+v", entries[0])
	}

	// history 는 체인을 거슬러 올라간다.
	history := runCLI(t, dir, "decisions", "history", second)
	if !strings.Contains(history, first) || !strings.Contains(history, second) {
		t.Fatalf("history:\n%s", history)
	}
}

// 없는 결정을 대체하려 하면 exit 5 다.
func TestSupersedeMissingIsNotFound(t *testing.T) {
	dir := doctorRepo(t)
	_, err := runCLIErr(t, dir, "decide", "결정", "--supersedes", "D999")
	if got := exitCode(t, err); got != ExitNotFound {
		t.Fatalf("exit %d, want %d (%v)", got, ExitNotFound, err)
	}
}

// 선택지가 없거나 택한 것이 목록 밖이면 기록은 하되 경고한다.
//
// fsck 로 미루지 않는 이유는 결정이 불변이기 때문이다. 나중에 발견해도
// 고칠 수 없다.
func TestDecideWarnsButRecords(t *testing.T) {
	dir := doctorRepo(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"선택지 없음", []string{"decide", "옵션 없음"}, "검토한 선택지가 없습니다"},
		{"목록 밖", []string{"decide", "밖", "--option", "A", "--decision", "B"}, "목록에 없습니다"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCLIStreams(t, dir, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(stdout) == "" {
				t.Fatal("기록되지 않았습니다")
			}
			if !strings.Contains(stderr, tc.want) {
				t.Fatalf("경고가 없습니다: %q", stderr)
			}
		})
	}
}

// T12.9 export --decisions 는 결정당 파일 하나를 만든다.
func TestExportDecisionsCommand(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "decide", "첫째", "--option", "A", "--decision", "A")
	runCLI(t, dir, "decide", "둘째", "--option", "B", "--decision", "B")

	out := filepath.Join(t.TempDir(), "docs", "decisions")
	runCLI(t, dir, "export", "--decisions", "-o", out)

	for _, id := range []string{"D001", "D002"} {
		data, err := os.ReadFile(filepath.Join(out, id+".md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "파생물") {
			t.Fatalf("%s 에 경고가 없습니다:\n%s", id, data)
		}
	}
	// -o 없이 부르면 사용법 오류다. 파일이 여러 개라 stdout 으로 낼 수 없다.
	if _, err := runCLIErr(t, dir, "export", "--decisions"); exitCode(t, err) != ExitUsage {
		t.Fatalf("exit %d, want %d", exitCode(t, err), ExitUsage)
	}
}

// T12.10 fsck 가 매달린 참조를 보고한다.
func TestFsckReportsDanglingDecisionRef(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "decide", "결정", "--option", "A", "--decision", "A", "--issue", "T999")

	out, err := runCLIErr(t, dir, "fsck")
	if exitCode(t, err) != ExitGeneral || !strings.Contains(out, "decision_dangling_ref") {
		t.Fatalf("fsck = %v\n%s", err, out)
	}
}
