package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// E2E-23: 소스 히스토리 무오염.
//
// 모든 시나리오의 공통 종료 조건이다 (§7). 여기서는 전체 워크플로우를 한 번
// 돌린 뒤 기준선과 대조한다.
func TestSourceHistoryStaysClean(t *testing.T) {
	f := newFixture(t)
	before := lines(f.Git("log", "--format=%H", "main"))

	// 이슈·plan·결정을 두루 만든다.
	f.MustRun("plan", "new", "P", "--id", "P01")
	f.MustRun("plan", "phase", "add", "P01", "p1", "--id", "p1")
	planned := f.add("계획된 작업", "--plan", "P01", "--phase", "p1", "--seq", "10")
	loose := f.add("계획 밖 작업")
	f.decide("어떻게 할지", "--decision", "이렇게", "--issue", loose)
	f.finish(planned)
	f.MustRun("claim", loose)
	f.MustRun("release", loose)

	f.expectCleanTree()
	if after := lines(f.Git("log", "--format=%H", "main")); len(after) != len(before) {
		t.Fatalf("main 의 commit 이 %d → %d 로 늘었습니다", len(before), len(after))
	}
	if diff := f.Git("diff", f.Base, "HEAD"); strings.TrimSpace(diff) != "" {
		t.Fatalf("기준선과 차이가 있습니다:\n%s", diff)
	}
	// 에이전트 ID 가 소스 이력의 author 로 새지 않는다.
	for _, who := range lines(f.Git("log", "--format=%an", "main")) {
		if who != "e2e" {
			t.Fatalf("main 의 author 에 %q 가 있습니다", who)
		}
	}
	if status := f.Git("status", "--porcelain", "--untracked-files=all"); strings.TrimSpace(status) != "" {
		t.Fatalf("워킹 디렉터리에 새 파일이 있습니다:\n%s", status)
	}
}

// E2E-24: 원격 미노출.
//
// 두 번째 절반은 "안 되어야 한다" 가 아니라 문서의 경고가 사실인지 확인하는
// 것이다 (§9.1).
func TestRefsAreNotPushedByDefault(t *testing.T) {
	f := newFixture(t)
	id := f.add("작업")
	f.MustRun("claim", id)

	remote := tempRepo(t)
	f.gitIn(remote, "init", "--quiet", "--bare", ".")
	f.Git("remote", "add", "origin", remote)
	f.Git("push", "--quiet", "origin", "main")

	if refs := remotePpwkRefs(t, f, remote); len(refs) != 0 {
		t.Fatalf("기본 push 가 ppwk ref 를 보냈습니다: %v", refs)
	}
	// clone 한 사본에도 없다.
	clone := tempRepo(t)
	f.gitIn(filepath.Dir(clone), "clone", "--quiet", remote, clone)
	if refs := remotePpwkRefs(t, f, clone); len(refs) != 0 {
		t.Fatalf("clone 에 ppwk ref 가 있습니다: %v", refs)
	}

	// --mirror 는 보낸다. 문서의 경고가 사실이다.
	f.Git("push", "--quiet", "--mirror", "origin")
	if refs := remotePpwkRefs(t, f, remote); len(refs) == 0 {
		t.Fatal("--mirror 가 ppwk ref 를 보내지 않았습니다 — 문서의 경고가 사실이 아닙니다")
	}
}

func remotePpwkRefs(t *testing.T, f *Fixture, dir string) []string {
	t.Helper()
	return lines(f.GitIn(dir, "for-each-ref", "--format=%(refname)", "refs/ppwk/"))
}

// E2E-25: SHA-256 저장소.
//
// zero OID 를 문자열로 비교하지 않는 것이 여기서 값을 한다.
func TestSHA256Repository(t *testing.T) {
	requireGitSupport(t, "--object-format=sha256")
	f := newFixture(t, "--object-format=sha256")
	if got := strings.TrimSpace(f.Git("rev-parse", "--show-object-format")); got != "sha256" {
		t.Fatalf("object format = %q", got)
	}
	f.runFullWorkflow(t)
}

// E2E-26: reftable backend.
//
// 명세는 "모든 기능 정상" 을 요구하지만 지금은 그렇지 않다. go-git v6 가
// refstorage 확장을 모르기 때문에 저장소를 열지조차 못한다. 읽기를 전부
// git 실행으로 바꾸지 않는 한 고칠 수 없고, 그것은 §14.1 의 선택을 되돌리는
// 일이다.
//
// 그래서 지금 지킬 수 있는 것만 지킨다 — 알아들을 수 있게 거부하는 것.
// 지원이 생기면 이 테스트가 실패하고, 그때 위쪽 절반으로 옮기면 된다.
func TestReftableBackend(t *testing.T) {
	requireGitSupport(t, "--ref-format=reftable")
	dir := tempRepo(t)
	f := &Fixture{t: t, Root: dir, Env: []string{"PPWK_AGENT=e2e-agent", "PPWK_SESSION=e2e-session"}}
	f.gitIn(dir, "init", "--quiet", "--initial-branch=main", "--ref-format=reftable", ".")
	f.gitIn(dir, "config", "user.name", "e2e")
	f.gitIn(dir, "config", "user.email", "e2e@example.invalid")
	f.gitIn(dir, "commit", "--quiet", "--allow-empty", "-m", "initial")

	r := f.Run("init")
	if r.ExitCode == 0 {
		t.Fatal("reftable 이 동작합니다 — 이 테스트를 runFullWorkflow 로 바꾸세요")
	}
	if !strings.Contains(r.Stderr, "reftable") || !strings.Contains(r.Stderr, "--ref-format=files") {
		t.Fatalf("거부 사유가 무엇을 해야 하는지 알려주지 않습니다:\n%s", r)
	}
}

// runFullWorkflow 는 저장소 형식에 무관하게 전부 도는지 본다.
func (f *Fixture) runFullWorkflow(t *testing.T) {
	t.Helper()
	w := f.watch()

	id := f.add("작업")
	ref := "refs/ppwk/issues/" + id
	waitFor(t, 15*time.Second, "created 감지", func() bool { return w.seen(ref, "created") })

	f.MustRun("claim", id)
	f.MustRun("start", id)
	f.MustRun("done", id)
	waitFor(t, 15*time.Second, "deleted 감지", func() bool { return w.seen(ref, "deleted") })
	waitFor(t, 15*time.Second, "archive 감지", func() bool {
		return w.seen("refs/ppwk/archive/"+id, "created")
	})

	if !f.HasRef("refs/ppwk/archive/" + id) {
		t.Fatalf("%s 가 archive 에 없습니다", id)
	}
	f.decide("결정", "--decision", "값")
	f.MustRun("export")
	f.MustRun("fsck")
	if r := f.Run("doctor"); r.ExitCode != 0 {
		t.Fatalf("doctor:\n%s", r)
	}
	f.expectCleanTree()
}

// requireGitSupport 는 이 git 이 해당 init 옵션을 지원하는지 본다.
func requireGitSupport(t *testing.T, arg string) {
	t.Helper()
	dir := tempRepo(t)
	cmd := exec.Command("git", "init", "--quiet", arg, ".")
	cmd.Dir = dir
	cmd.Env = baseEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("이 git 은 %s 를 지원하지 않습니다: %s", arg, out)
	}
}

// E2E-28: 최소 git 버전.
//
// 지원하지 않는 git 에서 조용히 이상하게 동작하는 것보다, 시작할 때 분명히
// 거부하는 편이 낫다.
func TestMinimumGitVersion(t *testing.T) {
	f := newFixture(t)
	// 가짜 git 을 PATH 앞에 둔다. 버전 확인이 실제로 git 에게 묻는지까지
	// 함께 검증된다.
	fake := tempRepo(t)
	writeFile(t, filepath.Join(fake, "git"), "#!/bin/sh\necho 'git version 2.27.0'\n")
	if err := chmod(filepath.Join(fake, "git"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"doctor"}, {"init"}} {
		r := f.execWithPath(fake, args...)
		if r.ExitCode == 0 {
			t.Fatalf("git 2.27 에서 %v 가 성공했습니다:\n%s", args, r)
		}
		if !strings.Contains(r.Stderr+r.Stdout, "2.28") {
			t.Fatalf("%v 의 거부 사유가 최소 버전을 밝히지 않습니다:\n%s", args, r)
		}
	}
}
