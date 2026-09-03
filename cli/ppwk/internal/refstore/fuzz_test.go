package refstore

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// F0.1 우리 검증기가 통과시킨 이름은 git 도 반드시 통과시켜야 한다.
//
// 반대 방향은 허용한다 — 우리가 더 엄격한 것은 안전하다. `..`, `@{`,
// 후행 `.lock`, 제어문자, 빈 컴포넌트를 자체 구현으로 다 막았다고 착각하기 쉽다.
func FuzzRefName(f *testing.F) {
	seeds := []string{
		"refs/ppwk/issues/T001",
		"refs/ppwk/issues/../evil",
		"refs/ppwk/issues/T001.lock",
		"refs/ppwk/issues/.hidden",
		"refs/ppwk/issues/T001@{0}",
		"refs/ppwk//T001",
		"refs/ppwk/issues/T 001",
		"refs/ppwk/issues/T001.",
		"refs/heads/@",
		"refs/ppwk/issues/T~1",
		"refs/ppwk/issues/한글",
		"",
		"@",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, ref string) {
		if ValidateRefName(ref) != nil {
			return // 우리가 거부한 것은 git 이 어떻든 상관없다.
		}
		cmd := exec.Command("git", "check-ref-format", ref)
		if err := cmd.Run(); err != nil {
			t.Fatalf("우리는 통과시켰지만 git 은 거부: %q (%v)", ref, err)
		}
	})
}

// F0.2 임의 문자열이 ErrLockBusy 로 분류되면 실패한다.
//
// 정확성이 아니라 "위험한 방향으로 틀리지 않는지" 를 본다. 잘못된 ErrLockBusy 는
// 무한 재시도나 중복 배정으로 이어진다.
func FuzzClassifyRefError(f *testing.F) {
	f.Add("fatal: not a git repository")
	f.Add("cannot lock ref")
	f.Add("error: unable to create x.lock")
	f.Add("is at abc but expected def")
	f.Add("")

	f.Fuzz(func(t *testing.T, stderr string) {
		err := classifyRefError([]byte(stderr), 128)
		if err == nil {
			t.Fatalf("classifyRefError(%q) = nil, want error", stderr)
		}
		if !errors.Is(err, ErrLockBusy) {
			return
		}
		// ErrLockBusy 로 갔다면 잠금 실패 문구가 실제로 있어야 한다.
		hasLockPhrase := strings.Contains(stderr, "cannot lock ref") ||
			(strings.Contains(stderr, "unable to create") && strings.Contains(stderr, ".lock")) ||
			(strings.Contains(stderr, "Unable to create") && strings.Contains(stderr, ".lock"))
		if !hasLockPhrase {
			t.Fatalf("근거 없이 ErrLockBusy 로 분류: %q", stderr)
		}
	})
}
