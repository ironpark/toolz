package refstore

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v6"
)

// git config 접근도 이 경계 안에 둔다.
//
// CLI 의존을 한 곳에 가두는 것이 §14.5 의 목적이므로, git 을 부르는 코드가
// 패키지 밖으로 새어나가지 않게 한다.

// ConfigGet 은 설정값 하나를 읽는다. 없으면 빈 문자열이다.
func (s *ExecRefStore) ConfigGet(key string) (string, error) {
	out, err := s.configOutput("--get", key)
	if err != nil {
		return "", err
	}
	return out, nil
}

// ConfigGetAll 은 여러 번 설정된 값을 전부 읽는다.
func (s *ExecRefStore) ConfigGetAll(key string) ([]string, error) {
	out, err := s.configOutput("--get-all", key)
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// ConfigBool 은 bool 설정을 읽는다. 없으면 false 다.
//
// 해석을 직접 하지 않고 --type=bool 에 맡긴다. "yes"/"on"/"1" 을 우리가 다시
// 정의하면 git 과 미묘하게 다른 참·거짓이 하나 더 생긴다.
func (s *ExecRefStore) ConfigBool(key string) (bool, error) {
	out, err := s.configOutput("--type=bool", "--get", key)
	return out == "true", err
}

// ConfigSet 은 값을 덮어쓴다.
func (s *ExecRefStore) ConfigSet(key, value string) error {
	_, err := s.configOutput(key, value)
	return err
}

// ConfigAdd 는 값을 추가한다. 여러 값을 갖는 키에 쓴다.
func (s *ExecRefStore) ConfigAdd(key, value string) error {
	_, err := s.configOutput("--add", key, value)
	return err
}

// configOutput 은 git config 를 실행한다.
//
// 값이 없을 때의 exit 1 은 오류가 아니라 "없음" 이다.
func (s *ExecRefStore) configOutput(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"config"}, args...)...)
	cmd.Dir = s.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && stderr.Len() == 0 {
			return "", nil
		}
		return "", fmt.Errorf("git config %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// Repo 는 열려 있는 go-git 저장소를 돌려준다.
//
// 객체 읽기·쓰기는 go-git 으로 하므로 도메인 쪽에서 필요하다 (§14.7).
func (s *ExecRefStore) Repo() *git.Repository { return s.repo }

// WorktreeDir 은 exec 하는 git 의 작업 디렉터리(common dir)다.
func (s *ExecRefStore) CommonDir() string { return s.dir }
