package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type statusJSONOutput struct {
	Plans []statusPlanJSON `json:"plans"`
}

type statusPlanJSON struct {
	Name        string            `json:"name"`
	Directory   string            `json:"directory"`
	Status      string            `json:"status"`
	DonePhases  int               `json:"done_phases"`
	TotalPhases int               `json:"total_phases"`
	Remaining   []statusPhaseJSON `json:"remaining"`
	Wait        []string          `json:"wait"`
}

type statusPhaseJSON struct {
	PhaseNumber int    `json:"phase_number"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Status      string `json:"status"`
}

type overviewJSONOutput struct {
	Plans []overviewPlanJSON `json:"plans"`
}

type overviewPlanJSON struct {
	Name        string   `json:"name"`
	Directory   string   `json:"directory"`
	Status      string   `json:"status"`
	DonePhases  int      `json:"done_phases"`
	TotalPhases int      `json:"total_phases"`
	NextPhase   string   `json:"next_phase"`
	Wait        []string `json:"wait"`
}

type notesJSONOutput struct {
	Notes []noteJSON `json:"notes"`
}

type noteJSON struct {
	CompletedAt string `json:"completed_at"`
	Plan        string `json:"plan"`
	Event       string `json:"event"`
	Phase       string `json:"phase"`
	Commit      string `json:"commit"`
	ShortCommit string `json:"short_commit"`
	Subject     string `json:"subject"`
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}

func makeStatusJSON(summaries []planSummary) statusJSONOutput {
	plans := make([]statusPlanJSON, 0, len(summaries))
	for _, summary := range summaries {
		done, total, _ := summary.progress()
		remaining := make([]statusPhaseJSON, 0)
		for _, phase := range summary.phases {
			if phase.status == "done" {
				continue
			}
			remaining = append(remaining, statusPhaseJSON{
				PhaseNumber: phase.id,
				Slug:        phase.slug,
				Title:       phase.title,
				Status:      phase.status,
			})
		}
		plans = append(plans, statusPlanJSON{
			Name:        summary.name,
			Directory:   filepath.ToSlash(summary.label),
			Status:      summary.status,
			DonePhases:  done,
			TotalPhases: total,
			Remaining:   remaining,
			Wait:        append([]string{}, summary.wait...),
		})
	}
	return statusJSONOutput{Plans: plans}
}

func makeOverviewJSON(summaries []planSummary) overviewJSONOutput {
	plans := make([]overviewPlanJSON, 0, len(summaries))
	for _, summary := range summaries {
		status := summary.status
		if status == "" {
			status = "unknown"
		}
		done, total, next := summary.progress()
		plans = append(plans, overviewPlanJSON{
			Name:        summary.name,
			Directory:   filepath.ToSlash(summary.label),
			Status:      status,
			DonePhases:  done,
			TotalPhases: total,
			NextPhase:   next,
			Wait:        append([]string{}, summary.wait...),
		})
	}
	return overviewJSONOutput{Plans: plans}
}

func makeNotesJSON(notes []planNote) notesJSONOutput {
	values := make([]noteJSON, 0, len(notes))
	for _, note := range notes {
		values = append(values, noteJSON{
			CompletedAt: note.at,
			Plan:        note.plan,
			Event:       note.event,
			Phase:       note.phase,
			Commit:      note.commit,
			ShortCommit: note.shortHash,
			Subject:     note.subject,
		})
	}
	return notesJSONOutput{Notes: values}
}
