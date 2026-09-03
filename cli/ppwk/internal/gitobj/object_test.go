package gitobj

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

func newTestRepo(t *testing.T) (string, *git.Repository) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main", ".")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "config", "user.email", "test@example.com")

	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		t.Fatalf("PlainOpen() = %v", err)
	}
	return dir, repo
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

func sampleIssue() model.Issue {
	at := model.NewTimestamp(time.Date(2026, 8, 30, 4, 12, 0, 0, time.UTC))
	return model.Issue{
		Schema:    model.SchemaVersion,
		ID:        "T001",
		Title:     "SQLite storage 구현",
		Status:    model.StatusWorking,
		Priority:  model.PriorityHigh,
		CreatedAt: at,
		UpdatedAt: at,
		UpdatedBy: "agent-b",
	}
}

func signature() object.Signature {
	return object.Signature{
		Name:  "agent-b",
		Email: "agent-b+8f3a2c1d@ppwk.local",
		When:  time.Date(2026, 8, 30, 4, 12, 0, 0, time.UTC),
	}
}

// T1.5 go-git 으로 만든 commit 을 git CLI 가 정상으로 읽는다 (trailer 포함)
func TestGitCLIReadsOurCommit(t *testing.T) {
	dir, repo := newTestRepo(t)
	issue := sampleIssue()

	hash, err := Write(repo, Commit{
		Doc:     issue,
		DocName: FileIssue,
		Body:    []byte("긴 설명\n"),
		Subject: "claim: " + issue.Title,
		Trailers: []Trailer{
			{Key: KeyStatus, Value: "working"},
			{Key: KeyOwner, Value: "agent-b"},
			{Key: KeyAgentSession, Value: "8f3a2c1d"},
		},
		Author: signature(),
	})
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	runGit(t, dir, "update-ref", "refs/ppwk/issues/T001", hash.String(), "")

	// git 의 trailer 파서가 우리 블록을 읽어야 한다. list 가 이 경로를 쓴다.
	for key, want := range map[string]string{"Status": "working", "Owner": "agent-b", "Agent-Session": "8f3a2c1d"} {
		got := runGit(t, dir, "for-each-ref",
			"--format=%(trailers:key="+key+",valueonly,unfold)", "refs/ppwk/issues/T001")
		if strings.TrimSpace(got) != want {
			t.Fatalf("trailer %s = %q, want %q", key, got, want)
		}
	}

	if subject := runGit(t, dir, "log", "-1", "--format=%s", "refs/ppwk/issues/T001"); subject != "claim: "+issue.Title {
		t.Fatalf("subject = %q", subject)
	}
	// tree 도 git 이 읽을 수 있어야 한다.
	if out := runGit(t, dir, "cat-file", "-p", "refs/ppwk/issues/T001:body.md"); out != "긴 설명" {
		t.Fatalf("body.md = %q", out)
	}
	if out := runGit(t, dir, "show", "refs/ppwk/issues/T001:issue.json"); !strings.Contains(out, `"id":"T001"`) {
		t.Fatalf("issue.json = %q", out)
	}
	// fsck 가 깨끗해야 한다 — 객체가 규격에 맞는다는 뜻이다.
	runGit(t, dir, "fsck", "--no-progress")
}

// T1.6 git CLI 로 만든 commit 을 go-git 이 정상으로 읽는다
func TestWeReadGitCLICommit(t *testing.T) {
	dir, repo := newTestRepo(t)
	issue := sampleIssue()

	raw, err := model.Marshal(issue)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	path := dir + "/issue.json"
	if err := writeFile(path, raw); err != nil {
		t.Fatalf("writeFile() = %v", err)
	}
	blob := runGit(t, dir, "hash-object", "-w", path)

	runGit(t, dir, "update-index", "--add", "--cacheinfo", "100644", blob, "issue.json")
	tree := runGit(t, dir, "write-tree")

	message := "claim: " + issue.Title + "\n\nStatus: working\nOwner: agent-b\n"
	commit := runGitStdin(t, dir, message, "commit-tree", tree)

	var got model.Issue
	body, subject, trailers, err := Read(repo, plumbing.NewHash(commit), FileIssue, &got)
	if err != nil {
		t.Fatalf("Read() = %v", err)
	}
	if got.ID != issue.ID || got.Title != issue.Title {
		t.Fatalf("issue = %+v", got)
	}
	if subject != "claim: "+issue.Title {
		t.Fatalf("subject = %q", subject)
	}
	if TrailerValue(trailers, KeyStatus) != "working" {
		t.Fatalf("trailers = %v", trailers)
	}
	if len(body) != 0 {
		t.Fatalf("body = %q, want 없음", body)
	}
}

// 같은 내용은 같은 OID 로 수렴한다 (§14.7).
func TestWriteIsContentAddressed(t *testing.T) {
	_, repo := newTestRepo(t)
	c := Commit{
		Doc:      sampleIssue(),
		DocName:  FileIssue,
		Subject:  "create: x",
		Trailers: []Trailer{{Key: KeyStatus, Value: "open"}},
		Author:   signature(),
	}
	first, err := Write(repo, c)
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	second, err := Write(repo, c)
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	if first != second {
		t.Fatalf("OID 가 다릅니다: %s vs %s", first, second)
	}
}

// 우리가 쓴 것을 우리가 읽는다.
func TestWriteReadRoundTrip(t *testing.T) {
	_, repo := newTestRepo(t)
	want := sampleIssue()

	hash, err := Write(repo, Commit{
		Doc:      want,
		DocName:  FileIssue,
		Body:     []byte("본문\n"),
		Subject:  "create: " + want.Title,
		Trailers: []Trailer{{Key: KeyStatus, Value: string(want.Status)}},
		Author:   signature(),
	})
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	var got model.Issue
	body, subject, trailers, err := Read(repo, hash, FileIssue, &got)
	if err != nil {
		t.Fatalf("Read() = %v", err)
	}
	if got.ID != want.ID || got.Title != want.Title || got.Status != want.Status {
		t.Fatalf("issue = %+v, want %+v", got, want)
	}
	if string(body) != "본문\n" {
		t.Fatalf("body = %q", body)
	}
	if subject != "create: "+want.Title {
		t.Fatalf("subject = %q", subject)
	}
	if TrailerValue(trailers, KeyStatus) != "working" {
		t.Fatalf("trailers = %v", trailers)
	}
}
