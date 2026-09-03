package board

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// 검사 항목 이름 (design §9.3).
//
// 문자열을 상수로 두는 이유는 --json 소비자와 테스트가 같은 이름을 보기
// 위해서다. 이름이 바뀌면 양쪽이 함께 깨져야 한다.
const (
	CheckUnreadable      = "unreadable"
	CheckTrailerStatus   = "trailer_status"
	CheckMissingDep      = "missing_dependency"
	CheckDependencyCycle = "dependency_cycle"
	CheckOwnerNoLease    = "owner_without_lease"
	CheckTerminalInPlace = "terminal_not_archived"
	CheckSchemaVersion   = "schema_version"
	CheckMissingPlan     = "missing_plan"
	CheckMissingPhase    = "missing_phase"
	CheckPartialPlan     = "partial_plan_fields"
	CheckClosedPlanOpen  = "closed_plan_open_task"
	CheckEmptyPhase      = "empty_phase"
	CheckDuplicateSeq    = "duplicate_seq"
	CheckAdvancedPhase   = "unknown_advanced_phase"
	CheckCancelledDep    = "cancelled_dependency"
	CheckBacklogDep      = "backlog_dependency"
	CheckStaleLock       = "stale_lock"
)

// 심각도.
const (
	LevelError = "error"
	LevelWarn  = "warn"
)

// fixKind 는 --fix 가 이 발견에 대해 할 수 있는 일이다.
//
// 판단이 필요한 수정은 도구가 하지 않는다. 여기 없는 항목은 보고만 한다.
type fixKind int

const (
	fixNone fixKind = iota
	// fixTrailer 는 issue.json 을 진실로 삼아 trailer 를 다시 만든다.
	fixTrailer
	// fixArchive 는 종료 상태 이슈를 archive/ 로 옮긴다.
	fixArchive
)

// Finding 은 검사 결과 하나다.
type Finding struct {
	Check   string `json:"check"`
	Level   string `json:"level"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`
	// Fixed 는 --fix 가 실제로 고쳤는지다.
	Fixed bool `json:"fixed,omitempty"`
	// FixError 는 고치려다 실패한 이유다. 경쟁에서 밀리는 것은 정상이다.
	FixError string `json:"fix_error,omitempty"`

	fix fixKind
}

// FsckOptions 는 검사 설정이다.
type FsckOptions struct {
	// Fix 는 trailer 재생성과 archive 이동만 자동 처리한다.
	Fix bool
}

// Fsck 는 보드 무결성을 검사한다 (§9.3).
//
// 손상된 이슈 하나가 검사 전체를 멈추지 않는다. 읽지 못한 것은 그 사실만
// 보고하고 나머지를 계속 본다 — fsck 가 가장 필요한 순간이 바로 무언가
// 깨져 있을 때다.
func (b *Board) Fsck(opts FsckOptions) ([]Finding, error) {
	scan, err := b.scan()
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, check := range []func(*fsckScan) []Finding{
		checkUnreadable,
		checkSchema,
		checkTrailers,
		checkDependencies,
		checkCycles,
		checkOwners,
		checkArchivePlacement,
		checkPlanFields,
		checkPlanDocs,
		checkPhases,
	} {
		findings = append(findings, check(scan)...)
	}
	findings = append(findings, b.checkStaleLocks()...)

	slices.SortStableFunc(findings, func(a, c Finding) int {
		if a.ID != c.ID {
			return strings.Compare(a.ID, c.ID)
		}
		return strings.Compare(a.Check, c.Check)
	})
	if opts.Fix {
		b.applyFixes(findings)
	}
	return findings, nil
}

// HasErrors 는 error 수준 발견이 있는지다. 종료 코드를 정한다.
func HasErrors(findings []Finding) bool {
	for _, f := range findings {
		if f.Level == LevelError {
			return true
		}
	}
	return false
}

// issueRecord 는 이슈 하나의 두 판본이다.
//
// trailer 는 조회용 사본이고 issue.json 이 진실이다 (§3.3). 둘을 함께 들고
// 있어야 어긋남을 볼 수 있다.
type issueRecord struct {
	ID      string
	Ref     string
	Hash    plumbing.Hash
	Issue   model.Issue
	Entry   ListEntry
	Err     error
	Archive bool
}

// fsckScan 은 검사에 필요한 것을 한 번에 읽어 둔 것이다.
type fsckScan struct {
	issues map[string]*issueRecord
	order  []string
	plans  map[string]model.Plan
	// leaseAgents 는 잠금 기록이 있는 에이전트다. 생존 여부는 보지 않는다 —
	// 죽은 기록이 남아 있는 것은 정상이고, 아예 없는 것이 이상하다.
	leaseAgents map[string]bool
	schema      int
}

func (b *Board) scan() (*fsckScan, error) {
	scan := &fsckScan{
		issues:      map[string]*issueRecord{},
		plans:       map[string]model.Plan{},
		leaseAgents: map[string]bool{},
	}
	version, err := b.SchemaVersion()
	if err != nil {
		return nil, err
	}
	scan.schema = version

	for _, prefix := range []string{refstore.Issues, refstore.Archive} {
		refs, err := b.store.List(prefix)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			id := strings.TrimPrefix(ref.Ref, prefix)
			record := &issueRecord{ID: id, Ref: ref.Ref, Hash: ref.Hash,
				Archive: prefix == refstore.Archive}
			if entry, err := b.readEntry(ref.Ref, ref.Hash); err == nil {
				record.Entry = entry
			} else {
				record.Err = err
			}
			if _, _, _, err := gitobj.Read(b.repo, ref.Hash, gitobj.FileIssue, &record.Issue); err != nil {
				record.Err = err
			}
			scan.issues[id] = record
			scan.order = append(scan.order, id)
		}
	}
	slices.Sort(scan.order)

	planRefs, err := b.store.List(refstore.Plans)
	if err != nil {
		return nil, err
	}
	for _, ref := range planRefs {
		id := strings.TrimPrefix(ref.Ref, refstore.Plans)
		if plan, err := b.ShowPlan(id); err == nil {
			scan.plans[id] = plan
		}
	}
	for _, lease := range b.leases.List() {
		scan.leaseAgents[lease.Agent] = true
	}
	return scan, nil
}

// each 는 읽을 수 있었던 이슈만 ID 순으로 순회한다.
func (s *fsckScan) each(fn func(*issueRecord) []Finding) []Finding {
	var findings []Finding
	for _, id := range s.order {
		record := s.issues[id]
		if record.Err != nil {
			continue
		}
		findings = append(findings, fn(record)...)
	}
	return findings
}

func finding(check, level, id, msg string) Finding {
	return Finding{Check: check, Level: level, ID: id, Message: msg}
}

func checkUnreadable(s *fsckScan) []Finding {
	var findings []Finding
	for _, id := range s.order {
		if record := s.issues[id]; record.Err != nil {
			findings = append(findings, finding(CheckUnreadable, LevelError, id,
				"읽을 수 없습니다: "+record.Err.Error()))
		}
	}
	return findings
}

// checkSchema 는 보드와 개별 문서의 스키마 버전을 본다 (§9.4).
func checkSchema(s *fsckScan) []Finding {
	var findings []Finding
	if s.schema > model.SchemaVersion {
		findings = append(findings, finding(CheckSchemaVersion, LevelError, "",
			"보드 스키마 "+strconv.Itoa(s.schema)+" 가 이 CLI("+strconv.Itoa(model.SchemaVersion)+")보다 높습니다"))
	}
	return append(findings, s.each(func(r *issueRecord) []Finding {
		if r.Issue.Schema > model.SchemaVersion {
			return []Finding{finding(CheckSchemaVersion, LevelError, r.ID,
				"스키마 "+strconv.Itoa(r.Issue.Schema)+" 가 이 CLI("+strconv.Itoa(model.SchemaVersion)+")보다 높습니다")}
		}
		return nil
	})...)
}

// checkTrailers 는 trailer 사본이 issue.json 과 맞는지 본다.
func checkTrailers(s *fsckScan) []Finding {
	return s.each(func(r *issueRecord) []Finding {
		if r.Entry.Status == r.Issue.Status {
			return nil
		}
		f := finding(CheckTrailerStatus, LevelError, r.ID,
			"trailer 의 status("+string(r.Entry.Status)+")가 issue.json("+string(r.Issue.Status)+")과 다릅니다")
		// issue.json 을 신뢰한다 (§3.3). 그래서 자동 수정이 가능하다.
		f.fix = fixTrailer
		return []Finding{f}
	})
}

func checkDependencies(s *fsckScan) []Finding {
	return s.each(func(r *issueRecord) []Finding {
		var findings []Finding
		for _, dep := range r.Issue.DependsOn {
			target, ok := s.issues[dep]
			if !ok {
				findings = append(findings, finding(CheckMissingDep, LevelError, r.ID,
					"depends_on 이 없는 이슈 "+dep+" 를 가리킵니다"))
				continue
			}
			if target.Err != nil {
				continue
			}
			// 아래 둘은 경고다. 데이터가 깨진 것은 아니지만, 후속 작업이
			// 영원히 후보에 오르지 못하는 상태라 사람이 알아야 한다.
			if target.Issue.Status == model.StatusCancelled {
				findings = append(findings, finding(CheckCancelledDep, LevelWarn, r.ID,
					"의존 대상 "+dep+" 가 cancelled 입니다. 의존은 충족되지 않습니다"))
			}
			if target.Issue.Priority == model.PriorityNone && !target.Issue.Status.Terminal() {
				findings = append(findings, finding(CheckBacklogDep, LevelWarn, r.ID,
					"의존 대상 "+dep+" 가 백로그(priority none)입니다. 영원히 후보가 아닙니다"))
			}
		}
		return findings
	})
}

// checkCycles 는 depends_on 순환을 찾는다.
//
// 순환에 속한 이슈는 next 가 영원히 고르지 않는다. 조용히 멈추는 종류의
// 고장이라 검사로 드러내지 않으면 아무도 눈치채지 못한다.
func checkCycles(s *fsckScan) []Finding {
	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)
	state := map[string]int{}
	var findings []Finding

	var walk func(id string)
	walk = func(id string) {
		record, ok := s.issues[id]
		if !ok || record.Err != nil {
			return
		}
		state[id] = inStack
		for _, dep := range record.Issue.DependsOn {
			switch state[dep] {
			case unvisited:
				walk(dep)
			case inStack:
				findings = append(findings, finding(CheckDependencyCycle, LevelError, id,
					"의존성 순환: "+id+" → "+dep))
			}
		}
		state[id] = done
	}
	for _, id := range s.order {
		if state[id] == unvisited {
			walk(id)
		}
	}
	return findings
}

func checkOwners(s *fsckScan) []Finding {
	return s.each(func(r *issueRecord) []Finding {
		if r.Issue.Owner == "" || s.leaseAgents[r.Issue.Owner] {
			return nil
		}
		return []Finding{finding(CheckOwnerNoLease, LevelError, r.ID,
			"소유자 "+r.Issue.Owner+" 에 대응하는 잠금 기록이 없습니다")}
	})
}

func checkArchivePlacement(s *fsckScan) []Finding {
	return s.each(func(r *issueRecord) []Finding {
		if !r.Issue.Status.Terminal() || r.Archive {
			return nil
		}
		f := finding(CheckTerminalInPlace, LevelError, r.ID,
			string(r.Issue.Status)+" 인데 issues/ 에 남아 있습니다")
		f.fix = fixArchive
		return []Finding{f}
	})
}

// checkPlanFields 는 plan/phase/seq 가 함께 있는지 본다 (§3.4).
func checkPlanFields(s *fsckScan) []Finding {
	return s.each(func(r *issueRecord) []Finding {
		switch {
		case (r.Issue.Plan == "") != (r.Issue.Phase == ""):
			return []Finding{finding(CheckPartialPlan, LevelError, r.ID,
				"plan 과 phase 는 함께 있어야 합니다 (plan="+r.Issue.Plan+" phase="+r.Issue.Phase+")")}
		case r.Issue.Plan == "" && r.Issue.Seq != 0:
			return []Finding{finding(CheckPartialPlan, LevelError, r.ID,
				"plan 없이 seq 가 있습니다")}
		}
		return nil
	})
}

// checkPlanDocs 는 task 가 가리키는 plan/phase 가 실재하는지 본다.
func checkPlanDocs(s *fsckScan) []Finding {
	return s.each(func(r *issueRecord) []Finding {
		if r.Issue.Plan == "" {
			return nil
		}
		plan, ok := s.plans[r.Issue.Plan]
		if !ok {
			return []Finding{finding(CheckMissingPlan, LevelError, r.ID,
				"없는 plan "+r.Issue.Plan+" 을 가리킵니다")}
		}
		if _, ok := plan.Phase(r.Issue.Phase); !ok {
			return []Finding{finding(CheckMissingPhase, LevelError, r.ID,
				plan.ID+" 에 없는 phase "+r.Issue.Phase+" 를 가리킵니다")}
		}
		return nil
	})
}

// checkPhases 는 plan 쪽에서 본 문제들이다.
func checkPhases(s *fsckScan) []Finding {
	var findings []Finding
	planIDs := make([]string, 0, len(s.plans))
	for id := range s.plans {
		planIDs = append(planIDs, id)
	}
	slices.Sort(planIDs)

	for _, planID := range planIDs {
		plan := s.plans[planID]
		known := map[string]bool{}
		for _, phase := range plan.Phases {
			known[phase.ID] = true
		}
		for _, id := range plan.AdvancedPhases {
			if !known[id] {
				findings = append(findings, finding(CheckAdvancedPhase, LevelError, planID,
					"advanced_phases 가 없는 phase "+id+" 를 가리킵니다"))
			}
		}

		tasks := map[string][]*issueRecord{}
		openTasks := 0
		for _, issueID := range s.order {
			r := s.issues[issueID]
			if r.Err != nil || r.Issue.Plan != planID {
				continue
			}
			tasks[r.Issue.Phase] = append(tasks[r.Issue.Phase], r)
			if !r.Issue.Status.Terminal() {
				openTasks++
			}
		}
		if plan.Status == model.PlanClosed && openTasks > 0 {
			findings = append(findings, finding(CheckClosedPlanOpen, LevelError, planID,
				"closed 인데 미완 task 가 "+strconv.Itoa(openTasks)+"개 남았습니다"))
		}

		for _, phase := range plan.Phases {
			members := tasks[phase.ID]
			if len(members) == 0 {
				// gate 가 공허참으로 통과한다. 의도와 다를 수 있다 (§3.7.5).
				findings = append(findings, finding(CheckEmptyPhase, LevelWarn, planID,
					"phase "+phase.ID+" 에 task 가 없습니다. gate 가 공허참으로 통과합니다"))
				continue
			}
			seen := map[int]string{}
			for _, r := range members {
				if first, dup := seen[r.Issue.Seq]; dup {
					findings = append(findings, finding(CheckDuplicateSeq, LevelWarn, r.ID,
						"phase "+phase.ID+" 안에서 seq "+strconv.Itoa(r.Issue.Seq)+" 가 "+first+" 와 겹칩니다"))
					continue
				}
				seen[r.Issue.Seq] = r.ID
			}
		}
	}
	return findings
}

// checkStaleLocks 는 남아 있는 .lock 파일을 경고한다.
//
// 지우지 않는다. 진짜로 다른 프로세스가 작업 중일 수 있고, 그 경우 지우는
// 것이 바로 우리가 막으려던 손상을 만든다 (§9.3).
func (b *Board) checkStaleLocks() []Finding {
	pattern := filepath.Join(b.git.CommonDir(), "refs", "ppwk", "*", "*.lock")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	slices.Sort(matches)
	findings := make([]Finding, 0, len(matches))
	for _, path := range matches {
		findings = append(findings, finding(CheckStaleLock, LevelWarn, "",
			".lock 파일이 남아 있습니다: "+path+" (자동으로 지우지 않습니다)"))
	}
	return findings
}

// applyFixes 는 자동 처리 가능한 것만 고친다.
//
// CAS 를 거치므로 다른 에이전트가 동시에 작업 중이어도 안전하다. 밀리면
// 고치지 못했다고 보고할 뿐이다.
func (b *Board) applyFixes(findings []Finding) {
	for i := range findings {
		var err error
		switch findings[i].fix {
		case fixTrailer:
			// 내용을 바꾸지 않는 commit 을 하나 얹는다. trailer 는 issue.json
			// 에서 다시 만들어지므로 이것만으로 사본이 맞춰진다 (§3.3).
			_, err = b.Mutate(Mutation{ID: findings[i].ID, Event: "fsck",
				Apply: func(*model.Issue) error { return nil }})
		case fixArchive:
			_, err = b.Archive(findings[i].ID)
		default:
			continue
		}
		if err != nil {
			findings[i].FixError = err.Error()
			continue
		}
		findings[i].Fixed = true
	}
}
