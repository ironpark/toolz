package cli

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

func doctorCommand(_ context.Context, cmd *ucli.Command) error {
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
	reporter := &doctor.Reporter{JSON: cmd.Bool("json")}

	if err := gitrepo.EnsureRepository(cwd); err != nil {
		reporter.Add(doctor.Issue{Location: "git", Message: err.Error()})
	} else {
		reporter.Printf("PASS git repository: %s\n", location.BaseRoot)
	}

	reporter.Printf("INFO agent: %s\n", agentenv.CurrentDescription())

	settings := config.Default()
	if location.Path == "" {
		reporter.Printf("INFO config: .planr.yaml not found; using defaults\n")
	} else {
		parsed, parseErr := config.ParseFile(location.Path)
		if parseErr != nil {
			reporter.Add(doctor.Issue{Location: location.Path, Message: parseErr.Error()})
		} else {
			settings = parsed
			reporter.Printf("PASS config: %s\n", location.Path)
		}
	}

	planDirectories := settings.PlanDirs(location.BaseRoot)
	validDirectories := []string{}
	for _, plans := range planDirectories {
		info, statErr := vfs.Stat(plans)
		switch {
		case statErr != nil && os.IsNotExist(statErr):
			reporter.Add(doctor.Issue{Location: "plans_dirs", Message: fmt.Sprintf("directory does not exist: %s", plans)})
		case statErr != nil:
			reporter.Add(doctor.Issue{Location: "plans_dirs", Message: fmt.Sprintf("cannot inspect %s: %v", plans, statErr)})
		case !info.IsDir():
			reporter.Add(doctor.Issue{Location: "plans_dirs", Message: fmt.Sprintf("path is not a directory: %s", plans)})
		default:
			reporter.Printf("PASS plans_dirs: %s\n", plans)
			validDirectories = append(validDirectories, plans)
		}
	}

	plans := []doctor.Inspection{}
	for _, plansRoot := range validDirectories {
		entries, readErr := vfs.ReadDir(plansRoot)
		if readErr != nil {
			reporter.Add(doctor.Issue{Location: filepath.Base(plansRoot), Message: fmt.Sprintf("cannot read plans directory: %v", readErr)})
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
					reporter.Printf("FIXED %s/PLAN.md: synchronized checklist with phases\n", inspection.Directory)
				}
			} else {
				issues = append(issues, inspection.ChecklistIssues...)
			}
			for _, issue := range issues {
				reporter.Add(issue)
			}
			plans = append(plans, inspection)
		}
	}

	byName := map[string]doctor.Inspection{}
	for _, inspection := range plans {
		name := draft.PlanName(inspection.Directory)
		if previous, found := byName[name]; found {
			reporter.Add(doctor.Issue{Location: inspection.Directory, Message: fmt.Sprintf("duplicate plan name %q also exists at %s", name, previous.Directory)})
			continue
		}
		byName[name] = inspection
	}
	for _, inspection := range plans {
		doctor.CheckPlanDependencies(reporter, inspection, byName)
	}

	if reporter.Issues == 0 {
		if reporter.JSON {
			return jsonout.Write(jsonout.Doctor(reporter.Records))
		}
		fmt.Println("Doctor found no problems")
		return nil
	}
	if reporter.JSON {
		if err := jsonout.Write(jsonout.Doctor(reporter.Records)); err != nil {
			return err
		}
	}
	return fmt.Errorf("doctor found %d problem(s)", reporter.Issues)
}
