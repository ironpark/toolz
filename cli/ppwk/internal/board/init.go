package board

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/agentdocs"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// InitOptions 는 init 의 선택지다.
type InitOptions struct {
	// Hooks 는 reference-transaction hook 을 설치한다.
	Hooks bool
	// NoAgentsMD 는 에이전트 문서 생성을 건너뛴다.
	NoAgentsMD bool
	// Force 는 기존 hook 을 덮어쓴다.
	Force bool
}

// InitResult 는 init 이 한 일과 사용자에게 할 말이다.
type InitResult struct {
	// SchemaCreated 는 meta/schema 를 이번에 만들었는지다.
	SchemaCreated bool
	// DocsCreated 는 새로 만든 에이전트 문서의 경로다.
	DocsCreated []string
	// Warnings 는 알아둬야 할 것들이다.
	Warnings []string
	// Notes 는 안내다.
	Notes []string
}

// Init 은 보드를 초기화한다 (design §8).
//
// 멱등하다. 두 번 실행해도 안전하다.
func (b *Board) Init(opts InitOptions) (*InitResult, error) {
	result := &InitResult{}

	// 1. meta/schema ref 생성 (없으면)
	created, err := b.ensureSchemaRef()
	if err != nil {
		return nil, err
	}
	result.SchemaCreated = created

	// 2. git log --all 에 이슈 커밋의 데코레이션이 섞이지 않게 한다.
	if err := b.ensureConfigContains("log.excludeDecoration", refstore.Prefix); err != nil {
		return nil, err
	}

	// 3. CAS 경쟁이 잦으므로 잠금 대기를 기본값(100ms)보다 늘린다 (§4.2).
	if err := b.store.ConfigSet("core.filesRefLockTimeout", "1000"); err != nil {
		return nil, err
	}

	// 4. hook 설치.
	if opts.Hooks {
		return nil, errors.New("--hooks 는 아직 구현되지 않았습니다 (구현 단계 10)")
	}

	// 5. 에이전트 문서.
	if !opts.NoAgentsMD {
		docs, err := agentdocs.Write(b.root)
		if err != nil {
			return nil, err
		}
		result.DocsCreated = docs
	}

	warnings, notes, err := b.initAdvice()
	if err != nil {
		return nil, err
	}
	result.Warnings = warnings
	result.Notes = notes
	return result, nil
}

// ensureSchemaRef 는 meta/schema 를 만든다. 이미 있으면 아무것도 하지 않는다.
func (b *Board) ensureSchemaRef() (bool, error) {
	if _, err := b.store.Get(refstore.Schema); err == nil {
		return false, nil
	} else if !isNotFound(err) {
		return false, err
	}

	// 스키마 버전 문자열을 담은 blob 하나를 ref 가 직접 가리킨다 (§3.1).
	hash, err := b.writeBlob([]byte(strconv.Itoa(model.SchemaVersion) + "\n"))
	if err != nil {
		return false, err
	}
	if err := b.store.CAS(refstore.Schema, hash, plumbing.ZeroHash); err != nil {
		// 다른 프로세스가 먼저 만들었다면 그것으로 충분하다.
		if errors.Is(err, refstore.ErrCASConflict) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SchemaVersion 은 보드에 적힌 스키마 버전이다.
//
// ref 가 없으면 아직 init 전이므로 현재 버전으로 본다. 값이 비어 있거나 읽을 수
// 없으면 1 로 간주한다 (§9.4).
func (b *Board) SchemaVersion() (int, error) {
	hash, err := b.store.Get(refstore.Schema)
	if isNotFound(err) {
		return model.SchemaVersion, nil
	}
	if err != nil {
		return 0, err
	}
	blob, err := b.repo.BlobObject(hash)
	if err != nil {
		return 1, nil
	}
	reader, err := blob.Reader()
	if err != nil {
		return 1, nil
	}
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return 1, nil
	}
	version, err := strconv.Atoi(strings.TrimSpace(buf.String()))
	if err != nil {
		return 1, nil
	}
	return version, nil
}

// ensureConfigContains 는 다중 값 설정에 value 가 없으면 추가한다.
//
// --add 를 그냥 부르면 두 번째 init 에서 값이 중복된다. 멱등해야 한다.
func (b *Board) ensureConfigContains(key, value string) error {
	values, err := b.store.ConfigGetAll(key)
	if err != nil {
		return err
	}
	if slices.Contains(values, value) {
		return nil
	}
	return b.store.ConfigAdd(key, value)
}

// initAdvice 는 사용자가 알아야 할 것을 모은다 (features §1.1).
func (b *Board) initAdvice() (warnings, notes []string, err error) {
	hooksPath, err := b.store.ConfigGet("core.hooksPath")
	if err != nil {
		return nil, nil, err
	}
	if hooksPath != "" {
		warnings = append(warnings, fmt.Sprintf(
			"core.hooksPath 가 %q 로 설정되어 있습니다. hook 은 저장소 기본 위치가 아니라 그쪽에 설치됩니다.", hooksPath))
	}
	notes = append(notes,
		`git log --all 에 이슈 커밋이 섞입니다. 별칭을 권합니다:`+
			"\n    git config alias.la \"log --exclude=refs/ppwk/* --all\"",
		"git push --mirror 는 이슈 제목·본문·에이전트 신원을 원격에 그대로 올립니다.",
	)
	return warnings, notes, nil
}
