package refstore

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
)

// emptyTree 는 빈 tree 의 SHA-1 OID 다. commit-tree 로 OID 를 찍어내는 데 쓴다.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// newTestRepo 는 빈 git 저장소를 만든다.
func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main", ".")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

// runGit 은 git 을 실행하고 실패하면 테스트를 멈춘다.
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

// makeCommits 는 서로 다른 commit OID 를 n 개 만든다.
//
// 어떤 ref 도 가리키지 않는 dangling commit 이지만 객체는 실재하므로
// update-ref 가 받아들인다.
func makeCommits(t *testing.T, dir string, n int) []plumbing.Hash {
	t.Helper()
	hashes := make([]plumbing.Hash, 0, n)
	for i := range n {
		out := runGit(t, dir, "commit-tree", emptyTree, "-m", string(rune('a'+i)))
		hashes = append(hashes, plumbing.NewHash(out))
	}
	return hashes
}

// fakeHashes 는 객체 존재를 확인하지 않는 구현을 위한 가짜 OID 다.
func fakeHashes(n int) []plumbing.Hash {
	hashes := make([]plumbing.Hash, 0, n)
	for i := range n {
		hashes = append(hashes, plumbing.NewHash(strings.Repeat(string(rune('a'+i)), 40)))
	}
	return hashes
}
