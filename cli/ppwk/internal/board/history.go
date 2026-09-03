package board

import (
	"fmt"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// Event 는 이력 한 줄이다 (§5.3).
type Event struct {
	Commit  string `json:"commit"`
	Short   string `json:"short"`
	When    string `json:"when"`
	Who     string `json:"who"`
	Subject string `json:"subject"`
}

// History 는 이슈의 commit chain 을 최신순으로 돌려준다.
//
// 별도 이력 자료구조가 없다. subject 가 이벤트명이고 parent 사슬이 순서이므로
// commit 을 거슬러 올라가는 것이 곧 이력이다 (§3.3).
func (b *Board) History(id string, limit int) ([]Event, error) {
	if err := refstore.ValidateID(id); err != nil {
		return nil, err
	}

	var head plumbing.Hash
	found := false
	for _, ref := range []string{refstore.Issues + id, refstore.Archive + id} {
		hash, err := b.store.Get(ref)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		head, found = hash, true
		break
	}
	if !found {
		return nil, fmt.Errorf("%s: %w", id, ErrNotFound)
	}

	var events []Event
	for hash := head; !hash.IsZero(); {
		commit, err := object.GetCommit(b.repo.Storer, hash)
		if err != nil {
			return nil, err
		}
		subject, _ := gitobj.ParseMessage(commit.Message)
		events = append(events, Event{
			Commit:  hash.String(),
			Short:   hash.String()[:7],
			When:    commit.Author.When.UTC().Format("2006-01-02 15:04:05"),
			Who:     commit.Author.Name,
			Subject: subject,
		})
		if limit > 0 && len(events) >= limit {
			break
		}
		if len(commit.ParentHashes) == 0 {
			break
		}
		hash = commit.ParentHashes[0]
	}
	return events, nil
}
