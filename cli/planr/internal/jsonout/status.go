package jsonout

import (
	"path/filepath"

	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

type StatusOutput struct {
	Plans []statusPlan `json:"plans"`
}

type statusPlan struct {
	Name        string        `json:"name"`
	Directory   string        `json:"directory"`
	Status      string        `json:"status"`
	DonePhases  int           `json:"done_phases"`
	TotalPhases int           `json:"total_phases"`
	Remaining   []statusPhase `json:"remaining"`
	Wait        []string      `json:"wait"`
}

type statusPhase struct {
	PhaseNumber int    `json:"phase_number"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Status      string `json:"status"`
}

type OverviewOutput struct {
	Plans []overviewPlan `json:"plans"`
}

type overviewPlan struct {
	Name        string   `json:"name"`
	Directory   string   `json:"directory"`
	Status      string   `json:"status"`
	DonePhases  int      `json:"done_phases"`
	TotalPhases int      `json:"total_phases"`
	NextPhase   string   `json:"next_phase"`
	Wait        []string `json:"wait"`
}

func Status(summaries []plan.Summary) StatusOutput {
	plans := make([]statusPlan, 0, len(summaries))
	for _, summary := range summaries {
		done, total, _ := summary.Progress()
		remaining := make([]statusPhase, 0)
		for _, phase := range summary.Phases {
			if phase.Status == "done" {
				continue
			}
			remaining = append(remaining, statusPhase{
				PhaseNumber: phase.ID,
				Slug:        phase.Slug,
				Title:       phase.Title,
				Status:      phase.Status,
			})
		}
		plans = append(plans, statusPlan{
			Name:        summary.Name,
			Directory:   filepath.ToSlash(summary.Label),
			Status:      summary.Status,
			DonePhases:  done,
			TotalPhases: total,
			Remaining:   remaining,
			Wait:        append([]string{}, summary.Wait...),
		})
	}
	return StatusOutput{Plans: plans}
}

func Overview(summaries []plan.Summary) OverviewOutput {
	plans := make([]overviewPlan, 0, len(summaries))
	for _, summary := range summaries {
		status := summary.Status
		if status == "" {
			status = "unknown"
		}
		done, total, next := summary.Progress()
		plans = append(plans, overviewPlan{
			Name:        summary.Name,
			Directory:   filepath.ToSlash(summary.Label),
			Status:      status,
			DonePhases:  done,
			TotalPhases: total,
			NextPhase:   next,
			Wait:        append([]string{}, summary.Wait...),
		})
	}
	return OverviewOutput{Plans: plans}
}
