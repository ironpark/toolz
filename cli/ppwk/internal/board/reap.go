package board

import (
	"errors"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// ReapOptions 는 회수 동작을 조절한다.
type ReapOptions struct {
	// DryRun 은 회수 대상만 보고 쓰지 않는다.
	DryRun bool
}

// 회수 중 하나가 건너뛰어지는 정상적인 이유들이다. 문자열 비교 대신 sentinel 을
// 쓰는 이유는, Mutate 가 오류를 감싸도 errors.Is 가 계속 맞기 때문이다.
var (
	errOwnerChanged = errors.New("소유자가 바뀌었습니다")
	errStateChanged = errors.New("상태가 바뀌었습니다")
)

// Reap 은 죽은 소유자의 이슈를 회수한다 (§3.6).
//
// 생존 판정은 잠금 디렉터리 스냅샷 하나로 한다. 소유자마다 파일을 다시 열면
// 같은 명령 안에서 판정 기준이 흔들릴 수 있고, 소유자 수만큼 I/O 가 는다.
// CAS 경쟁은 예상된 일이며 나머지 회수를 멈추지 않는다.
func (b *Board) Reap(opts ReapOptions) ([]*Issue, error) {
	entries, err := b.List(ListOptions{Status: []model.Status{
		model.StatusClaimed, model.StatusWorking, model.StatusBlocked,
	}})
	if err != nil {
		return nil, err
	}

	live := make(map[string]model.Lease)
	for _, lease := range b.leases.List() {
		if b.leases.Alive(lease) {
			live[lease.Agent] = lease
		}
	}

	var targets []*Issue
	for _, entry := range entries {
		if entry.Owner == "" {
			continue
		}
		issue, err := b.Show(entry.ID)
		if err != nil {
			continue
		}
		// 정확히 같은 owner/session 의 임차만 살아 있는 것으로 본다. 같은
		// 에이전트의 새 세션은 옛 세션의 이슈를 지켜 주지 않는다.
		if lease, ok := live[issue.Owner]; ok && lease.Session == issue.Session {
			continue
		}
		targets = append(targets, issue)
	}
	if opts.DryRun || len(targets) == 0 {
		return targets, nil
	}
	if err := b.RegisterSession(); err != nil {
		return nil, err
	}

	var reaped []*Issue
	for _, target := range targets {
		// 값으로 복사한다. 클로저가 재시도 동안 이슈 본문 전체를 붙잡고
		// 있을 이유가 없다.
		owner, sess := target.Owner, target.Session
		result, err := b.Mutate(Mutation{ID: target.ID, Event: "reap", Apply: func(i *model.Issue) error {
			// 수집 시점에 본 owner/session 을 다시 확인한다. 그래야 회수와
			// 소유자의 활동이 겹쳐도 서로를 덮어쓰지 않는다.
			if i.Owner != owner || i.Session != sess {
				return errOwnerChanged
			}
			switch i.Status {
			case model.StatusClaimed, model.StatusWorking:
				i.Status = model.StatusOpen
			case model.StatusBlocked:
			default:
				return errStateChanged
			}
			i.Owner, i.Session = "", ""
			return nil
		}})
		if err != nil {
			var conflict *ConflictError
			if errors.As(err, &conflict) || errors.Is(err, ErrNotFound) ||
				errors.Is(err, errOwnerChanged) || errors.Is(err, errStateChanged) {
				continue
			}
			return reaped, err
		}
		reaped = append(reaped, result)
	}
	return reaped, nil
}

// Agents 는 잠금 디렉터리에 남은 에이전트 기록을 돌려준다 (D13).
func (b *Board) Agents() []model.Lease { return b.leases.List() }
