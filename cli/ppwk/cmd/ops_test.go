package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

// runCLIErr 는 명령을 실행하고 stdout 과 오류를 함께 돌려준다.
//
// runCLI 는 오류가 나면 테스트를 실패시킨다. 종료 코드 자체를 보려면
// 오류를 손에 쥐어야 한다.
func runCLIErr(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	root := New(Version{CLI: "test", Schema: "1"}, &stdout, io.Discard)
	err := root.Run(context.Background(), append([]string{"ppwk", "-C", dir}, args...))
	return stdout.String(), err
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return ExitOK
	}
	var coder cli.ExitCoder
	if !errors.As(err, &coder) {
		t.Fatalf("%v 에 종료 코드가 없습니다", err)
	}
	return coder.ExitCode()
}

// T8.1 export json 은 유효한 JSON 이다.
func TestExportJSONCommand(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "add", "대상")

	out := runCLI(t, dir, "export", "--format", "json")
	var payload struct {
		Warning string `json:"warning"`
		Issues  []struct {
			ID string `json:"id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("유효한 JSON 이 아닙니다: %v\n%s", err, out)
	}
	if payload.Warning == "" || len(payload.Issues) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
}

// -o 는 파일로 쓴다.
func TestExportWritesFile(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "add", "대상")
	path := filepath.Join(t.TempDir(), "BOARD.md")

	runCLI(t, dir, "export", "--format", "md", "-o", path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "파생물") {
		t.Fatalf("경고가 없습니다:\n%s", data)
	}
}

// 알 수 없는 형식은 사용법 오류다 (exit 2).
func TestExportUnknownFormatIsUsageError(t *testing.T) {
	dir := doctorRepo(t)
	_, err := runCLIErr(t, dir, "export", "--format", "yaml")
	if got := exitCode(t, err); got != ExitUsage {
		t.Fatalf("exit %d, want %d", got, ExitUsage)
	}
}

// T8.6 정상 저장소의 fsck 는 exit 0 이다.
func TestFsckCleanExitsZero(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "add", "대상")

	out, err := runCLIErr(t, dir, "fsck", "--json")
	if got := exitCode(t, err); got != ExitOK {
		t.Fatalf("exit %d: %s", got, out)
	}
	var payload struct {
		Data struct {
			Findings []map[string]any `json:"findings"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("fsck --json: %v\n%s", err, out)
	}
	if len(payload.Data.Findings) != 0 {
		t.Fatalf("findings = %v", payload.Data.Findings)
	}
}

// error 수준 발견이 있으면 exit 1 이다.
func TestFsckErrorsExitOne(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "add", "대상", "--depends-on", "T999")

	out, err := runCLIErr(t, dir, "fsck")
	if got := exitCode(t, err); got != ExitGeneral {
		t.Fatalf("exit %d, want %d: %s", got, ExitGeneral, out)
	}
	if !strings.Contains(out, "missing_dependency") {
		t.Fatalf("출력에 항목이 없습니다:\n%s", out)
	}
}

// archive --sweep 은 종료 상태인데 남아 있는 것을 걷는다.
func TestArchiveSweepCommand(t *testing.T) {
	dir := doctorRepo(t)
	id := strings.TrimSpace(runCLI(t, dir, "add", "대상"))
	runCLI(t, dir, "start", id)
	runCLI(t, dir, "done", id)

	// done 이 이미 옮겼으므로 걷을 것이 없다.
	out := runCLI(t, dir, "archive", "--sweep")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("걷을 것이 없어야 합니다: %q", out)
	}
	if !strings.Contains(runCLI(t, dir, "list", "--archived"), id) {
		t.Fatal("archive 에 없습니다")
	}
	// ID 와 --sweep 을 함께 주면 사용법 오류다.
	if _, err := runCLIErr(t, dir, "archive", "--sweep", id); exitCode(t, err) != ExitUsage {
		t.Fatalf("exit %d, want %d", exitCode(t, err), ExitUsage)
	}
}

// unarchive 는 v1 에서 명시적 오류다.
func TestUnarchiveIsRejected(t *testing.T) {
	dir := doctorRepo(t)
	if _, err := runCLIErr(t, dir, "unarchive", "T001"); exitCode(t, err) != ExitUsage {
		t.Fatalf("exit %d, want %d", exitCode(t, err), ExitUsage)
	}
}
