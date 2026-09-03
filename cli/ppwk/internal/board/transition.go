package board

import (
	"fmt"
	"slices"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// Action 은 상태 전이 명령 하나다 (§3.5, features §3).
type Action string

const (
	ActionClaim   Action = "claim"
	ActionStart   Action = "start"
	ActionDone    Action = "done"
	ActionBlock   Action = "block"
	ActionUnblock Action = "unblock"
	ActionRelease Action = "release"
	ActionCancel  Action = "cancel"
)

// TransitionOptions 는 전이 명령의 입력이다.
type TransitionOptions struct {
	// Message 는 이벤트 subject 에 붙일 사유다.
	Message string
	// On 은 block 의 차단 원인 이슈 ID 다.
	On string
	// Force 는 소유자가 아니어도 진행한다 (release, cancel).
	Force bool
	// Retry 는 CAS 경쟁 실패 시 재시도 횟수다. 기본 0 — 즉시 exit 4.
	Retry int
}

// transition 은 전이 규칙 하나다.
type transition struct {
	// from 은 허용되는 출발 상태다.
	from []model.Status
	// to 는 도착 상태다.
	to model.Status
	// ownership 은 소유권을 어떻게 다루는지다.
	ownership ownership
}

// ownership 은 전이가 소유권에 대해 요구하고 남기는 것이다.
type ownership int

const (
	// ownTake 는 소유자가 없으면 가져오고, 있으면 나여야 한다.
	ownTake ownership = iota
	// ownRequire 는 소유자가 나여야 한다.
	ownRequire
	// ownClear 는 소유자를 지운다. 나여야 하지만 --force 로 넘길 수 있다.
	ownClear
)

// transitions 는 §3.5 의 전이 표 전체다.
//
// 표를 코드로 두는 이유는 이것이 규칙의 유일한 출처여야 하기 때문이다.
// 명령마다 조건문을 흩뿌리면 어떤 조합이 빠졌는지 아무도 알 수 없게 된다.
var transitions = map[Action]transition{
	ActionClaim: {
		from: []model.Status{model.StatusOpen}, to: model.StatusClaimed, ownership: ownTake,
	},
	// open 에서도 허용된다 — claim 과 start 를 한 CAS 로 수행한다 (D16).
	ActionStart: {
		from: []model.Status{model.StatusOpen, model.StatusClaimed}, to: model.StatusWorking, ownership: ownTake,
	},
	ActionDone: {
		from: []model.Status{model.StatusWorking}, to: model.StatusDone, ownership: ownRequire,
	},
	ActionBlock: {
		from: []model.Status{model.StatusWorking}, to: model.StatusBlocked, ownership: ownRequire,
	},
	ActionUnblock: {
		from: []model.Status{model.StatusBlocked}, to: model.StatusWorking, ownership: ownRequire,
	},
	// --force 는 working 까지 허용한다. 멈춘 에이전트를 사람이 회수하는
	// 경로다 (§4.5) — 그 판단은 사람이 하고, CLI 는 실수를 막기만 한다.
	ActionRelease: {
		from: []model.Status{model.StatusClaimed}, to: model.StatusOpen, ownership: ownClear,
	},
	ActionCancel: {
		from: []model.Status{
			model.StatusOpen, model.StatusClaimed, model.StatusWorking, model.StatusBlocked,
		},
		to: model.StatusCancelled, ownership: ownClear,
	},
}

// forceExtends 는 --force 가 추가로 허용하는 출발 상태다.
var forceExtends = map[Action][]model.Status{
	ActionRelease: {model.StatusWorking},
}

// Transition 은 상태 전이 명령 하나를 수행한다.
//
// 모든 전이가 §4.1 의 CAS 루프를 탄다. 규칙 검사는 Apply 안에서 하므로,
// 경쟁에 밀려 다시 읽은 상태에 대해서도 자동으로 다시 검사된다.
func (b *Board) Transition(action Action, id string, opts TransitionOptions) (*Issue, error) {
	rule, ok := transitions[action]
	if !ok {
		return nil, fmt.Errorf("알 수 없는 전이: %s", action)
	}

	// block --on 은 대상이 실재하고 순환을 만들지 않아야 한다. CAS 루프 밖에서
	// 한 번만 확인한다 — 다른 이슈의 상태이므로 재시도해도 답이 같다.
	if action == ActionBlock && opts.On != "" {
		if err := b.checkBlockTarget(id, opts.On); err != nil {
			return nil, err
		}
	}

	sub := b
	if opts.Retry > 0 {
		backoff := b.backoff
		backoff.CASAttempts = opts.Retry + 1
		sub = b.WithBackoff(backoff)
	}

	return sub.Mutate(Mutation{
		ID:      id,
		Event:   string(action),
		Message: opts.Message,
		Apply: func(issue *model.Issue) error {
			return b.applyTransition(action, rule, issue, opts)
		},
	})
}

// applyTransition 은 규칙을 검사하고 상태를 바꾼다 (§4.1 3단계).
func (b *Board) applyTransition(action Action, rule transition, issue *model.Issue, opts TransitionOptions) error {
	reject := func(reason string) error {
		return &TransitionError{ID: issue.ID, From: issue.Status, To: rule.to, Reason: reason}
	}

	// 종료 상태는 어떤 전이도 받지 않는다. cancel 도 예외가 아니다 —
	// 이미 끝난 일을 다시 끝내려는 것은 실수이고, 멱등 성공으로 감추면
	// 그 실수가 드러나지 않는다 (features §3).
	if issue.Status.Terminal() {
		return reject("이미 종료된 이슈입니다")
	}

	allowed := rule.from
	if opts.Force {
		allowed = append(slices.Clone(allowed), forceExtends[action]...)
	}
	if !slices.Contains(allowed, issue.Status) {
		return reject(fmt.Sprintf("%s 전이는 %v 에서만 가능합니다", action, allowed))
	}

	if err := b.checkOwnership(rule.ownership, issue, opts, reject); err != nil {
		return err
	}

	if action == ActionBlock && opts.On != "" {
		// 차단 원인은 depends_on 에 기록한다. 별도 필드를 두지 않는 이유는
		// 이것이 의존 관계 그 자체이기 때문이다 — next 의 후보 판정과 fsck 의
		// 순환 검출이 이미 depends_on 을 본다.
		if !slices.Contains(issue.DependsOn, opts.On) {
			issue.DependsOn = append(slices.Clone(issue.DependsOn), opts.On)
		}
	}

	issue.Status = rule.to
	return nil
}

// checkOwnership 은 소유권 규칙을 적용한다.
func (b *Board) checkOwnership(own ownership, issue *model.Issue, opts TransitionOptions, reject func(string) error) error {
	mine := issue.Owner == "" || issue.Owner == b.identity.Agent

	switch own {
	case ownTake:
		if !mine {
			return reject(fmt.Sprintf("%s 가 소유한 이슈입니다", issue.Owner))
		}
		issue.Owner, issue.Session = b.identity.Agent, b.identity.Session
	case ownRequire:
		if issue.Owner == "" {
			return reject("소유자가 없습니다. 먼저 start 하세요")
		}
		if !mine {
			return reject(fmt.Sprintf("%s 가 소유한 이슈입니다", issue.Owner))
		}
	case ownClear:
		if !mine && !opts.Force {
			return reject(fmt.Sprintf("%s 가 소유한 이슈입니다. --force 가 필요합니다", issue.Owner))
		}
		// 소유자와 세션을 함께 지운다. owner 만 지우면 --mine 필터가
		// 반납된 이슈를 계속 잡는다.
		issue.Owner, issue.Session = "", ""
	}
	return nil
}

// checkBlockTarget 은 block --on 의 대상을 검사한다.
func (b *Board) checkBlockTarget(id, on string) error {
	if on == id {
		return &TransitionError{ID: id, From: model.StatusWorking, To: model.StatusBlocked,
			Reason: "자기 자신을 차단할 수 없습니다"}
	}
	if err := refstore.ValidateID(on); err != nil {
		return err
	}
	if _, err := b.Show(on); err != nil {
		return fmt.Errorf("차단 원인 %s: %w", on, err)
	}
	// 순환은 여기서 막는다. 만들어진 뒤에 fsck 로 찾는 것보다, 만들어지는
	// 순간에 거부하는 편이 낫다 — 순환이 생기면 next 가 영원히 후보를 못 찾는다.
	if b.dependsOnTransitively(on, id) {
		return &TransitionError{ID: id, From: model.StatusWorking, To: model.StatusBlocked,
			Reason: fmt.Sprintf("%s 는 이미 %s 에 의존합니다 (순환)", on, id)}
	}
	return nil
}

// dependsOnTransitively 는 from 이 target 에 (간접적으로라도) 의존하는지 본다.
func (b *Board) dependsOnTransitively(from, target string) bool {
	seen := map[string]bool{}
	var walk func(id string) bool
	walk = func(id string) bool {
		if id == target {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		issue, err := b.Show(id)
		if err != nil {
			// 없는 이슈는 의존 사슬의 끝이다. fsck 가 따로 잡는다.
			return false
		}
		for _, dep := range issue.DependsOn {
			if walk(dep) {
				return true
			}
		}
		return false
	}
	return walk(from)
}
