package gitobj

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// tree 안의 파일 이름 (§3.3).
const (
	FileIssue    = "issue.json"
	FilePlan     = "plan.json"
	FileDecision = "decision.json"
	FileBody     = "body.md"
)

// Commit 은 만들 commit 하나를 서술한다.
//
// 객체 생성이 go-git 인 것이 안전한 이유는 content-addressed 이기 때문이다.
// 같은 내용은 같은 OID 로 수렴하므로 두 프로세스가 동시에 같은 객체를 써도
// 결과가 같다. 경쟁은 ref 갱신 지점에만 존재한다 (§14.7).
type Commit struct {
	// Doc 은 issue.json 또는 plan.json 이 될 문서다.
	Doc any
	// DocName 은 그 문서의 파일 이름이다.
	DocName string
	// Body 는 body.md 다. 비어 있으면 tree 에 넣지 않는다.
	Body []byte
	// Subject 는 이벤트 한 줄이다. git log 가 곧 history 가 된다.
	Subject string
	// Trailers 는 조회용 인덱스다.
	Trailers []Trailer
	// Author 는 에이전트 신원이다. committer 도 같은 값을 쓴다.
	Author object.Signature
	// Parent 는 직전 상태 commit 이다. 최초 생성이면 ZeroHash.
	Parent plumbing.Hash
}

// Write 는 blob·tree·commit 을 만들고 commit 의 OID 를 돌려준다.
//
// ref 는 건드리지 않는다. 그것은 RefStore 의 일이다.
func Write(repo *git.Repository, c Commit) (plumbing.Hash, error) {
	// model.Marshal 을 쓰는 이유는 HTML 이스케이프를 끄기 위함이다. 보존한
	// 미지 필드의 원본 바이트가 다시 쓰이면 안 된다.
	doc, err := model.Marshal(c.Doc)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("문서를 직렬화할 수 없습니다: %w", err)
	}
	docHash, err := writeBlob(repo, doc)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	// tree 항목은 이름 순이어야 한다. body.md 가 issue.json 보다 앞선다.
	entries := make([]object.TreeEntry, 0, 2)
	if len(c.Body) > 0 {
		bodyHash, err := writeBlob(repo, c.Body)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{
			Name: FileBody, Mode: filemode.Regular, Hash: bodyHash,
		})
	}
	entries = append(entries, object.TreeEntry{
		Name: c.DocName, Mode: filemode.Regular, Hash: docHash,
	})

	treeHash, err := writeTree(repo, entries)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	message, err := BuildMessage(c.Subject, c.Trailers)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	commit := &object.Commit{
		Author:    c.Author,
		Committer: c.Author,
		Message:   message,
		TreeHash:  treeHash,
	}
	if !c.Parent.IsZero() {
		commit.ParentHashes = []plumbing.Hash{c.Parent}
	}

	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(obj)
}

// writeBlob 은 blob 하나를 저장한다.
func writeBlob(repo *git.Repository, data []byte) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
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
	return repo.Storer.SetEncodedObject(obj)
}

// writeTree 는 tree 하나를 저장한다.
func writeTree(repo *git.Repository, entries []object.TreeEntry) (plumbing.Hash, error) {
	tree := &object.Tree{Entries: entries}
	obj := repo.Storer.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(obj)
}

// Read 는 commit 에서 문서와 본문, subject, trailer 를 꺼낸다.
func Read(repo *git.Repository, hash plumbing.Hash, docName string, doc any) (body []byte, subject string, trailers []Trailer, err error) {
	commit, err := object.GetCommit(repo.Storer, hash)
	if err != nil {
		return nil, "", nil, fmt.Errorf("commit 을 읽을 수 없습니다 (%s): %w", hash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, "", nil, err
	}

	raw, err := readFile(tree, docName)
	if err != nil {
		return nil, "", nil, err
	}
	if err := json.Unmarshal(raw, doc); err != nil {
		return nil, "", nil, fmt.Errorf("%s 를 읽을 수 없습니다: %w", docName, err)
	}

	// body.md 는 선택이다. 없으면 그냥 비어 있다.
	body, err = readFile(tree, FileBody)
	if err != nil {
		body = nil
	}

	subject, trailers = ParseMessage(commit.Message)
	return body, subject, trailers, nil
}

// readFile 은 tree 안의 파일 하나를 읽는다.
func readFile(tree *object.Tree, name string) ([]byte, error) {
	file, err := tree.File(name)
	if err != nil {
		return nil, err
	}
	reader, err := file.Blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
