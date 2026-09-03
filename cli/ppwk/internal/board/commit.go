package board

import (
	"errors"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// ErrNotFound 는 대상 이슈·plan·결정이 없다는 뜻이다.
var ErrNotFound = errors.New("대상을 찾을 수 없습니다")

// ErrSchemaTooNew 는 보드가 이 CLI 보다 새 스키마라는 뜻이다 (§9.4, exit 6).
var ErrSchemaTooNew = errors.New("스키마 버전 불일치")

// isNotFound 는 ref 부재를 판정한다.
func isNotFound(err error) bool {
	return errors.Is(err, refstore.ErrRefNotFound) || errors.Is(err, ErrNotFound)
}

// writeIssueCommit 은 이슈 상태 commit 하나를 만든다.
//
// ref 는 건드리지 않는다 — 호출자가 CAS 한다.
func (b *Board) writeIssueCommit(issue model.Issue, body []byte, event string, parent plumbing.Hash) (plumbing.Hash, error) {
	return gitobj.Write(b.repo, gitobj.Commit{
		Doc:      issue,
		DocName:  gitobj.FileIssue,
		Body:     body,
		Subject:  event + ": " + issue.Title,
		Trailers: issueTrailers(issue, b.identity.Session),
		Author:   b.signature(issue.UpdatedAt),
		Parent:   parent,
	})
}

// issueTrailers 는 조회용 인덱스를 만든다 (§3.3, §5.1).
//
// issue.json 이 진실이고 이것은 비정규화된 사본이다.
func issueTrailers(issue model.Issue, session string) []gitobj.Trailer {
	trailers := []gitobj.Trailer{
		{Key: gitobj.KeyStatus, Value: string(issue.Status)},
		{Key: gitobj.KeyPriority, Value: string(issue.Priority)},
	}
	if issue.Owner != "" {
		trailers = append(trailers, gitobj.Trailer{Key: gitobj.KeyOwner, Value: issue.Owner})
	}
	if issue.Plan != "" {
		trailers = append(trailers,
			gitobj.Trailer{Key: gitobj.KeyPlan, Value: issue.Plan},
			gitobj.Trailer{Key: gitobj.KeyPhase, Value: issue.Phase},
			gitobj.Trailer{Key: gitobj.KeySeq, Value: strconv.Itoa(issue.Seq)},
		)
	}
	if len(issue.DependsOn) > 0 {
		trailers = append(trailers, gitobj.Trailer{
			Key: gitobj.KeyDependsOn, Value: strings.Join(issue.DependsOn, ", "),
		})
	}
	// Agent-Session 은 OID 충돌 방지에 필요하다 (§4.3). 없으면 같은 초에 같은
	// 내용으로 만든 두 commit 의 OID 가 겹쳐 양쪽 CAS 가 모두 성공한다.
	if session != "" {
		trailers = append(trailers, gitobj.Trailer{Key: gitobj.KeyAgentSession, Value: session})
	}
	return trailers
}

// signature 는 commit 의 author·committer 다 (§7.4).
//
// user.name 을 건드리지 않는다. 소스 commit 까지 오염되기 때문이다.
// go-git 은 Signature 를 직접 받으므로 환경변수 주입도 필요 없다 (§14.7).
func (b *Board) signature(when model.Timestamp) object.Signature {
	email := b.identity.Agent + "@ppwk.local"
	if b.identity.Session != "" {
		email = b.identity.Agent + "+" + b.identity.Session + "@ppwk.local"
	}
	return object.Signature{
		Name:  b.identity.Agent,
		Email: email,
		When:  when.Time,
	}
}

// writeBlob 은 blob 하나를 저장한다.
func (b *Board) writeBlob(data []byte) (plumbing.Hash, error) {
	obj := b.repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(data)))

	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return b.repo.Storer.SetEncodedObject(obj)
}
