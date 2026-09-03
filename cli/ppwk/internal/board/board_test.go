package board

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/agentdocs"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

func newBoard(t *testing.T) (*Board, string) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main", ".")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "config", "user.email", "test@example.com")

	b, err := Open(dir, session.Identity{Agent: "agent-a", Session: "sess-1"})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	return b, dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// T1.7 init 이 log.excludeDecoration, core.filesRefLockTimeout 을 설정한다.
func TestInitSetsConfig(t *testing.T) {
	b, dir := newBoard(t)

	if _, err := b.Init(InitOptions{}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if got := runGit(t, dir, "config", "--get-all", "log.excludeDecoration"); got != "refs/ppwk/" {
		t.Fatalf("log.excludeDecoration = %q", got)
	}
	if got := runGit(t, dir, "config", "core.filesRefLockTimeout"); got != "1000" {
		t.Fatalf("core.filesRefLockTimeout = %q", got)
	}
	if got := runGit(t, dir, "cat-file", "-p", "refs/ppwk/meta/schema"); got != "1" {
		t.Fatalf("meta/schema = %q", got)
	}
}

// T1.8 init 을 두 번 실행해도 안전하다.
func TestInitIdempotent(t *testing.T) {
	b, dir := newBoard(t)

	first, err := b.Init(InitOptions{})
	if err != nil {
		t.Fatalf("첫 Init() = %v", err)
	}
	if !first.SchemaCreated {
		t.Fatal("첫 Init 이 schema 를 만들지 않았습니다")
	}

	second, err := b.Init(InitOptions{})
	if err != nil {
		t.Fatalf("두 번째 Init() = %v", err)
	}
	if second.SchemaCreated {
		t.Fatal("두 번째 Init 이 schema 를 다시 만들었습니다")
	}
	if len(second.DocsCreated) != 0 {
		t.Fatalf("두 번째 Init 이 문서를 다시 만들었습니다: %v", second.DocsCreated)
	}
	// --add 를 그냥 부르면 값이 중복된다.
	if got := runGit(t, dir, "config", "--get-all", "log.excludeDecoration"); got != "refs/ppwk/" {
		t.Fatalf("log.excludeDecoration = %q, want 중복 없음", got)
	}
}

// T1.9 core.hooksPath 는 init 이 신경 쓰지 않는다.
//
// git 훅을 설치하지 않으므로 (§6.3) 이 설정은 우리와 무관하다. 경고하면
// 설치하지도 않는 훅을 걱정하게 만든다.
func TestInitIgnoresHooksPath(t *testing.T) {
	b, dir := newBoard(t)
	runGit(t, dir, "config", "core.hooksPath", "/custom/hooks")

	result, err := b.Init(InitOptions{NoAgentsMD: true})
	if err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if joined := strings.Join(result.Warnings, " "); strings.Contains(joined, "hooksPath") {
		t.Fatalf("경고 = %q", joined)
	}
}

// T1.12 init 이 AGENTS.md 와 docs/ppwk/*.md 를 전부 만든다.
func TestInitCreatesAgentDocs(t *testing.T) {
	b, dir := newBoard(t)

	result, err := b.Init(InitOptions{})
	if err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if len(result.DocsCreated) != 9 {
		t.Fatalf("문서 %d개: %v", len(result.DocsCreated), result.DocsCreated)
	}
	if _, err := os.Stat(filepath.Join(dir, agentdocs.EntryPoint)); err != nil {
		t.Fatalf("AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, agentdocs.DocsDir, "states.md")); err != nil {
		t.Fatalf("states.md: %v", err)
	}
}

// T1.13 일부만 존재하면 없는 것만 생긴다.
func TestInitKeepsExistingDocs(t *testing.T) {
	b, dir := newBoard(t)

	custom := []byte("우리 저장소 규칙\n")
	if err := os.WriteFile(filepath.Join(dir, agentdocs.EntryPoint), custom, 0o644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	result, err := b.Init(InitOptions{})
	if err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if slices.Contains(result.DocsCreated, agentdocs.EntryPoint) {
		t.Fatalf("AGENTS.md 를 덮어썼습니다: %v", result.DocsCreated)
	}
	got, err := os.ReadFile(filepath.Join(dir, agentdocs.EntryPoint))
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if string(got) != string(custom) {
		t.Fatal("AGENTS.md 의 내용이 바뀌었습니다")
	}
}

// T1.14 --no-agents-md 로 전체 생성을 건너뛴다.
func TestInitNoAgentsMD(t *testing.T) {
	b, dir := newBoard(t)

	result, err := b.Init(InitOptions{NoAgentsMD: true})
	if err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if len(result.DocsCreated) != 0 {
		t.Fatalf("문서를 만들었습니다: %v", result.DocsCreated)
	}
	if _, err := os.Stat(filepath.Join(dir, agentdocs.EntryPoint)); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md 가 생겼습니다: %v", err)
	}
}

// 단계 1 Exit criteria: init && add && list 가 동작한다.
func TestAddAndList(t *testing.T) {
	b, _ := newBoard(t)
	if _, err := b.Init(InitOptions{}); err != nil {
		t.Fatalf("Init() = %v", err)
	}

	first, err := b.Add(AddOptions{Title: "SQLite storage 구현", Priority: model.PriorityHigh})
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}
	if first.ID != "T001" {
		t.Fatalf("ID = %q, want T001", first.ID)
	}
	second, err := b.Add(AddOptions{Title: "parser cleanup"})
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}
	if second.ID != "T002" {
		t.Fatalf("ID = %q, want T002", second.ID)
	}
	if second.Priority != model.PriorityMed {
		t.Fatalf("기본 우선순위 = %q, want med", second.Priority)
	}

	entries, err := b.List(ListOptions{})
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List() = %v, want 2개", entries)
	}
	// 목록은 trailer 만 읽는다. issue.json 을 열지 않고도 값이 맞아야 한다.
	if entries[0].ID != "T001" || entries[0].Status != model.StatusOpen || entries[0].Priority != model.PriorityHigh {
		t.Fatalf("entries[0] = %+v", entries[0])
	}
	if entries[0].Title != "SQLite storage 구현" {
		t.Fatalf("Title = %q", entries[0].Title)
	}
}

// 단계 1 Exit criteria: git status 가 clean 하고 소스 브랜치에 변화가 없다.
func TestAddLeavesSourceUntouched(t *testing.T) {
	b, dir := newBoard(t)
	runGit(t, dir, "commit", "--allow-empty", "--quiet", "-m", "base")
	before := runGit(t, dir, "rev-parse", "HEAD")

	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if _, err := b.Add(AddOptions{Title: "x"}); err != nil {
		t.Fatalf("Add() = %v", err)
	}

	if got := runGit(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("git status = %q, want clean", got)
	}
	if after := runGit(t, dir, "rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD 가 %s → %s 로 바뀌었습니다", before, after)
	}
}

// 제목에 개행이 있으면 첫 줄만 subject, 나머지는 본문으로 내린다.
func TestAddSplitsMultilineTitle(t *testing.T) {
	b, _ := newBoard(t)
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}

	issue, err := b.Add(AddOptions{Title: "첫 줄\n둘째 줄\n셋째 줄"})
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}
	if issue.Title != "첫 줄" {
		t.Fatalf("Title = %q", issue.Title)
	}
	if !strings.Contains(string(issue.Body), "둘째 줄") {
		t.Fatalf("Body = %q", issue.Body)
	}

	got, err := b.Show(issue.ID)
	if err != nil {
		t.Fatalf("Show() = %v", err)
	}
	if got.Title != "첫 줄" || !strings.Contains(string(got.Body), "셋째 줄") {
		t.Fatalf("Show() = %+v, body %q", got.Issue, got.Body)
	}
}

func TestAddRejectsEmptyTitle(t *testing.T) {
	b, _ := newBoard(t)
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	for _, title := range []string{"", "   ", "\n\n"} {
		if _, err := b.Add(AddOptions{Title: title}); err == nil {
			t.Fatalf("Add(%q) = nil, want error", title)
		}
	}
}

func TestShowNotFound(t *testing.T) {
	b, _ := newBoard(t)
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if _, err := b.Show("T404"); !isNotFound(err) {
		t.Fatalf("Show() = %v, want ErrNotFound", err)
	}
}

// 미래 스키마 버전이면 읽기만 허용한다 (§9.4).
func TestFutureSchemaBlocksWrites(t *testing.T) {
	b, dir := newBoard(t)
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if _, err := b.Add(AddOptions{Title: "먼저 만든 것"}); err != nil {
		t.Fatalf("Add() = %v", err)
	}

	// 보드를 미래 버전으로 표시한다.
	path := filepath.Join(dir, "future")
	if err := os.WriteFile(path, []byte("99\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	blob := runGit(t, dir, "hash-object", "-w", path)
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	runGit(t, dir, "update-ref", "refs/ppwk/meta/schema", blob)

	if _, err := b.Add(AddOptions{Title: "막혀야 한다"}); err == nil {
		t.Fatal("Add() = nil, want error")
	}
	// 읽기는 여전히 된다.
	if _, err := b.List(ListOptions{}); err != nil {
		t.Fatalf("List() = %v, want nil", err)
	}
}

// 손상된 이슈 하나가 목록 전체를 죽이지 않는다.
func TestListSurvivesBrokenIssue(t *testing.T) {
	b, dir := newBoard(t)
	if _, err := b.Init(InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	if _, err := b.Add(AddOptions{Title: "정상"}); err != nil {
		t.Fatalf("Add() = %v", err)
	}

	// commit 이 아니라 blob 을 가리키는 ref 를 심는다.
	path := filepath.Join(dir, "junk")
	if err := os.WriteFile(path, []byte("not a commit\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	blob := runGit(t, dir, "hash-object", "-w", path)
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	runGit(t, dir, "update-ref", "refs/ppwk/issues/T999", blob)

	entries, err := b.List(ListOptions{})
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "T001" {
		t.Fatalf("List() = %v, want T001 하나", entries)
	}
}
