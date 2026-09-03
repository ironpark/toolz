package board

import (
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

func TestReapByStateAndSession(t *testing.T) {
	b := initBoard(t)
	claimed := issueIn(t, b, model.StatusClaimed)
	working := issueIn(t, b, model.StatusWorking)
	blocked := issueIn(t, b, model.StatusBlocked)
	b.leases.Now = func() time.Time { return time.Now().Add(9 * time.Hour) }
	reaped, err := b.Reap(ReapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 3 {
		t.Fatalf("reaped=%d", len(reaped))
	}
	for _, tc := range []struct {
		id     string
		status model.Status
	}{{claimed.ID, model.StatusOpen}, {working.ID, model.StatusOpen}, {blocked.ID, model.StatusBlocked}} {
		i, err := b.Show(tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if i.Status != tc.status || i.Owner != "" || i.Session != "" {
			t.Fatalf("%s = %s owner=%q session=%q", tc.id, i.Status, i.Owner, i.Session)
		}
	}
}

func TestReapLiveOwnerAndNoTargetsDoNotWrite(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)
	before := issue.Commit
	reaped, err := b.Reap(ReapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 0 {
		t.Fatalf("reaped=%v", reaped)
	}
	after, err := b.Show(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Commit != before {
		t.Fatal("live issue was written")
	}
}

func TestSameAgentDifferentSessionIsReaped(t *testing.T) {
	b := initBoard(t)
	old := issueIn(t, b, model.StatusClaimed)
	b.identity.Session = "new-session"
	b.leases.Identity = b.identity
	b.allowSharedWorktree = true
	if _, err := b.leases.Register(true); err != nil {
		t.Fatal(err)
	}
	reaped, err := b.Reap(ReapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 1 || reaped[0].ID != old.ID {
		t.Fatalf("reaped=%v", reaped)
	}
}
