package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/toolz/cli/planr/internal/agentenv"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doctor"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/gitrepo"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
	ucli "github.com/urfave/cli/v3"
)

type doctorReporter struct {
	records []doctor.Issue
	json    bool
}

func newDoctorCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "doctor",
		Usage: "diagnose configuration, plans, and repository consistency",
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "fix", Usage: "repair PLAN.md checklists from phase files"},
			jsonFlag(),
		},
		Action: runDoctor,
	}
}

func (r *doctorReporter) add(issue doctor.Issue) {
	r.records = append(r.records, issue)
	if r.json {
		return
	}
	if issue.Location == "" {
		fmt.Printf("FAIL: %s\n", issue.Message)
	} else {
		fmt.Printf("FAIL %s: %s\n", issue.Location, issue.Message)
	}
}

func (r *doctorReporter) printf(format string, values ...any) {
	if !r.json {
		fmt.Printf(format, values...)
	}
}

func runDoctor(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() != 0 {
		return fmt.Errorf("doctor does not accept positional arguments")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	location, err := config.Discover(cwd)
	if err != nil {
		return err
	}
	reporter := &doctorReporter{json: cmd.Bool("json")}

	if err := gitrepo.EnsureRepository(cwd); err != nil {
		reporter.add(doctor.Issue{Location: "git", Message: err.Error()})
	} else {
		reporter.printf("PASS git repository: %s\n", location.BaseRoot)
	}

	reporter.printf("INFO agent: %s\n", agentenv.CurrentDescription())

	settings := config.Default()
	if location.Path == "" {
		reporter.printf("INFO config: .planr.yaml not found; using defaults\n")
	} else {
		parsed, parseErr := config.ParseFile(location.Path)
		if parseErr != nil {
			reporter.add(doctor.Issue{Location: location.Path, Message: parseErr.Error()})
		} else {
			settings = parsed
			reporter.printf("PASS config: %s\n", location.Path)
		}
	}

	planDirectories := settings.PlanDirs(location.BaseRoot)
	validDirectories := []string{}
	for _, plans := range planDirectories {
		info, statErr := vfs.Stat(plans)
		switch {
		case statErr != nil && os.IsNotExist(statErr):
			reporter.add(doctor.Issue{Location: "plans_dirs", Message: fmt.Sprintf("directory does not exist: %s", plans)})
		case statErr != nil:
			reporter.add(doctor.Issue{Location: "plans_dirs", Message: fmt.Sprintf("cannot inspect %s: %v", plans, statErr)})
		case !info.IsDir():
			reporter.add(doctor.Issue{Location: "plans_dirs", Message: fmt.Sprintf("path is not a directory: %s", plans)})
		default:
			reporter.printf("PASS plans_dirs: %s\n", plans)
			validDirectories = append(validDirectories, plans)
		}
	}

	plans := []doctor.Inspection{}
	for _, plansRoot := range validDirectories {
		entries, readErr := vfs.ReadDir(plansRoot)
		if readErr != nil {
			reporter.add(doctor.Issue{Location: filepath.Base(plansRoot), Message: fmt.Sprintf("cannot read plans directory: %v", readErr)})
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			planRoot := filepath.Join(plansRoot, entry.Name())
			inspection, issues := doctor.InspectPlan(planRoot, entry.Name())
			if cmd.Bool("fix") && len(inspection.ChecklistIssues) > 0 && inspection.PlanReadable && inspection.PlanFrontOK && inspection.PhaseDataOK && inspection.ChecklistStart >= 0 {
				repaired, repairErr := doctor.RepairChecklist(inspection.Body, inspection.Phases)
				if repairErr != nil {
					issues = append(issues, doctor.Issue{Location: inspection.Directory + "/PLAN.md", Message: fmt.Sprintf("cannot repair checklist: %v", repairErr)})
					issues = append(issues, inspection.ChecklistIssues...)
				} else if writeErr := doctor.WritePlanBody(inspection, repaired); writeErr != nil {
					issues = append(issues, doctor.Issue{Location: inspection.Directory + "/PLAN.md", Message: fmt.Sprintf("cannot repair checklist: %v", writeErr)})
					issues = append(issues, inspection.ChecklistIssues...)
				} else {
					reporter.printf("FIXED %s/PLAN.md: synchronized checklist with phases\n", inspection.Directory)
				}
			} else {
				issues = append(issues, inspection.ChecklistIssues...)
			}
			for _, issue := range issues {
				reporter.add(issue)
			}
			plans = append(plans, inspection)
		}
	}

	byName := map[string]doctor.Inspection{}
	for _, inspection := range plans {
		name := draft.PlanName(inspection.Directory)
		if previous, found := byName[name]; found {
			reporter.add(doctor.Issue{Location: inspection.Directory, Message: fmt.Sprintf("duplicate plan name %q also exists at %s", name, previous.Directory)})
			continue
		}
		byName[name] = inspection
	}
	for _, inspection := range plans {
		for _, issue := range doctor.CheckPlanDependencies(inspection, byName) {
			reporter.add(issue)
		}
	}

	if len(reporter.records) == 0 {
		if reporter.json {
			return jsonout.Write(jsonout.Doctor(reporter.records))
		}
		fmt.Println("Doctor found no problems")
		return nil
	}
	if reporter.json {
		if err := jsonout.Write(jsonout.Doctor(reporter.records)); err != nil {
			return err
		}
	}
	return fmt.Errorf("doctor found %d problem(s)", len(reporter.records))
}
