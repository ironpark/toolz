package refstore

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

// ExecRefStore 는 v1 구현이다.
//
// 읽기는 go-git 으로, ref 갱신은 git update-ref exec 으로 한다 (design §14.1).
// go-git 의 CheckAndSetReference 는 이름만 CAS 이고 실제로는 read-then-write 라
// 프로세스 간 원자성을 보장하지 못한다 (§14.2). 이 시스템은 그 위에 서 있으므로
// 갱신 경로만큼은 git 의 잠금 프로토콜을 그대로 빌린다.
type ExecRefStore struct {
	repo *git.Repository
	// dir 은 exec 하는 git 의 작업 디렉터리다. common dir 로 고정한다.
	dir string
}

var _ RefStore = (*ExecRefStore)(nil)

// NewExecRefStore 는 path 의 저장소를 열고 git 환경을 검증한다.
//
// git 부재·구버전·잘못된 경로는 전부 여기서 걸린다. 런타임에 발견되면 안 된다.
func NewExecRefStore(path string) (*ExecRefStore, error) {
	if err := checkDir(path); err != nil {
		return nil, err
	}
	if err := checkGitBinary(path); err != nil {
		return nil, err
	}
	repo, err := OpenRepository(path)
	if err != nil {
		return nil, err
	}
	dir, err := commonDir(path)
	if err != nil {
		return nil, err
	}
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	return &ExecRefStore{repo: repo, dir: dir}, nil
}

// Get 은 ref 가 가리키는 해시를 돌려준다.
func (s *ExecRefStore) Get(ref string) (plumbing.Hash, error) {
	if err := ValidateRefName(ref); err != nil {
		return plumbing.ZeroHash, err
	}
	r, err := s.repo.Reference(plumbing.ReferenceName(ref), false)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return plumbing.ZeroHash, fmt.Errorf("%s: %w", ref, ErrRefNotFound)
	}
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return r.Hash(), nil
}

// List 는 prefix 로 시작하는 ref 를 돌려준다.
//
// 순회 도중 다른 프로세스가 ref 를 지울 수 있다. 그 경우 부분 결과를 돌려주며
// panic 하지 않는다.
func (s *ExecRefStore) List(prefix string) ([]RefEntry, error) {
	iter, err := s.repo.References()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var entries []RefEntry
	err = iter.ForEach(func(r *plumbing.Reference) error {
		if r.Type() != plumbing.HashReference {
			return nil
		}
		name := r.Name().String()
		if !strings.HasPrefix(name, prefix) {
			return nil
		}
		entries = append(entries, RefEntry{Ref: name, Hash: r.Hash()})
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		// 순회 중 사라진 ref 는 오류가 아니라 경쟁의 정상적인 결과다.
		return entries, nil
	}
	return entries, nil
}

// CAS 는 ref 가 old 일 때만 new 로 바꾼다.
func (s *ExecRefStore) CAS(ref string, new, old plumbing.Hash) error {
	if err := ValidateRefName(ref); err != nil {
		return err
	}
	if new.IsZero() && old.IsZero() {
		return fmt.Errorf("%s: new 와 old 가 모두 비어 있습니다", ref)
	}

	var args []string
	if new.IsZero() {
		// 삭제. old 를 붙여 남의 갱신을 지우지 않도록 한다.
		args = []string{"update-ref", "-d", ref, old.String()}
	} else {
		// 생성은 old 가 ZeroHash 다. git 은 이를 "존재하면 안 됨" 으로 읽는다.
		args = []string{"update-ref", ref, new.String(), old.String()}
	}
	return s.run(args, nil)
}

// Transaction 은 ops 를 전부 적용하거나 전부 적용하지 않는다 (design §4.4).
func (s *ExecRefStore) Transaction(ops []RefOp) error {
	if len(ops) == 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("start\n")
	for _, op := range ops {
		if err := ValidateRefName(op.Ref); err != nil {
			return err
		}
		switch op.Kind {
		case OpCreate:
			fmt.Fprintf(&buf, "create %s %s\n", op.Ref, op.New.String())
		case OpUpdate:
			fmt.Fprintf(&buf, "update %s %s %s\n", op.Ref, op.New.String(), op.Old.String())
		case OpDelete:
			fmt.Fprintf(&buf, "delete %s %s\n", op.Ref, op.Old.String())
		default:
			return fmt.Errorf("알 수 없는 연산: %d", op.Kind)
		}
	}
	buf.WriteString("prepare\ncommit\n")

	return s.run([]string{"update-ref", "--stdin"}, &buf)
}

// run 은 git 을 실행하고 실패를 분류한다.
//
// stale .lock 을 만나면 ErrLockBusy 를 그대로 전파한다. 진짜로 다른 프로세스가
// 작업 중일 수 있으므로 도구가 임의로 지우지 않는다.
func (s *ExecRefStore) run(args []string, stdin *bytes.Buffer) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.dir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return classifyRefError(stderr.Bytes(), exitCode)
	}
	return nil
}
