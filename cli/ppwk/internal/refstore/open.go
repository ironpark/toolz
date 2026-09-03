package refstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6"
)

// minGitMajor, minGitMinor 는 요구되는 최소 git 버전이다.
//
// 2.28 은 reference-transaction hook 이 필요로 하는 하한이다 (design §6.3).
const (
	minGitMajor = 2
	minGitMinor = 28
)

// OpenRepository 는 path 에서 저장소를 연다.
//
// 설계 §14.4 는 EnableDotGitCommonDir 을 켜라고 하지만, go-git v6 에서 그 옵션은
// 사라졌다. commondir 해석이 PlainOpen 의 기본 동작이 되었기 때문이다
// (repository.go 의 dotGitCommonDirectory). 요구사항 자체는 그대로다 —
// linked worktree 에서 공유 ref 가 보여야 §3.1 의 전제가 선다.
// T0.12 가 이 성질을 검사한다.
func OpenRepository(path string) (*git.Repository, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("저장소를 열 수 없습니다 (%s): %w", path, err)
	}
	return repo, nil
}

// checkGitBinary 는 git 이 PATH 에 있고 버전이 충분한지 본다.
//
// 런타임 중간이 아니라 시작 시점에 확인한다.
func checkGitBinary(dir string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git 을 PATH 에서 찾을 수 없습니다: %w", err)
	}
	cmd := exec.Command("git", "--version")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git --version 실패: %w", err)
	}
	major, minor, err := parseGitVersion(string(out))
	if err != nil {
		return err
	}
	if major < minGitMajor || (major == minGitMajor && minor < minGitMinor) {
		return fmt.Errorf("git %d.%d 이상이 필요합니다 (설치된 버전 %d.%d)",
			minGitMajor, minGitMinor, major, minor)
	}
	return nil
}

// parseGitVersion 은 "git version 2.43.0" 에서 major/minor 를 뽑는다.
func parseGitVersion(out string) (major, minor int, err error) {
	fields := strings.Fields(out)
	if len(fields) < 3 {
		return 0, 0, fmt.Errorf("git 버전을 읽을 수 없습니다: %q", strings.TrimSpace(out))
	}
	parts := strings.Split(fields[2], ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("git 버전을 읽을 수 없습니다: %q", strings.TrimSpace(out))
	}
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, fmt.Errorf("git 버전을 읽을 수 없습니다: %q", strings.TrimSpace(out))
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, fmt.Errorf("git 버전을 읽을 수 없습니다: %q", strings.TrimSpace(out))
	}
	return major, minor, nil
}

// commonDir 은 $GIT_COMMON_DIR 의 절대 경로를 돌려준다.
//
// worktree 마다 GIT_DIR 이 다르지만 우리가 건드리는 ref 는 전부 공유 영역에 있다.
// exec 하는 git 의 cmd.Dir 을 여기로 고정한다 (§14.4).
func commonDir(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git-common-dir 을 찾을 수 없습니다 (%s): %w", dir, err)
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return filepath.Clean(path), nil
}

// WorktreeRoot 는 작업 트리 최상단의 절대 경로다.
//
// 에이전트 문서는 보드 데이터와 달리 tracked 파일이므로 여기에 놓인다.
func WorktreeRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("작업 트리 최상단을 찾을 수 없습니다 (%s): %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// checkDir 은 cmd.Dir 로 쓸 경로가 실제 디렉터리인지 본다.
func checkDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("경로에 접근할 수 없습니다 (%s): %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("디렉터리가 아닙니다: %s", dir)
	}
	return nil
}
