package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
)

// editEnvelopeKeys are the frontmatter keys that belong to planr's document
// envelopes (edit checkouts and new-phase drafts), not to the stored document.
var editEnvelopeKeys = []string{
	"planr_edit",
	"planr_target",
	"planr_base",
	"planr_phase",
	"planr_slug",
	"planr_section",
	"planr_new",
	"planr_plan",
	"phase_title",
	"phase",
	"slug",
}

// PhaseDraft is a parsed new-phase draft: the plan it targets plus the phase
// document itself. The phase number is assigned when the draft is applied.
type PhaseDraft struct {
	Plan  string
	Title string
	draft.Phase
}

// Detect classifies a raw document and returns the decoded value: a
// draft.Draft for KindPlan, a PhaseDraft for KindPhase, and the raw bytes for
// KindEdit.
func Detect(raw []byte, fallback string) (string, any, error) {
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		return "", nil, validation.Wrap(err, "frontmatter", "frontmatter")
	}
	if value, ok := front["planr_edit"].(string); ok && strings.TrimSpace(value) != "" {
		return KindEdit, raw, nil
	}
	if value, ok := front["planr_new"].(string); ok {
		if value != KindPhase {
			return "", nil, fmt.Errorf("unknown planr_new document kind %q", value)
		}
		phase, err := parsePhaseDraft(raw)
		if err != nil {
			return "", nil, err
		}
		return KindPhase, phase, nil
	}
	parsed, err := draft.Parse(raw, fallback)
	if err != nil {
		return "", nil, err
	}
	if err := draft.ValidateDependencies(&parsed); err != nil {
		return "", nil, err
	}
	return KindPlan, parsed, nil
}

// UnwrapJSONDocument lets an agent pass the JSON result of `new --json` or
// `edit --json` directly to `apply --stdin`. Raw Markdown remains the primary
// stdin format; this compatibility envelope only makes the stdout-to-stdin
// path composable without a temporary file.
func UnwrapJSONDocument(raw []byte) []byte {
	var envelope struct {
		Template string `json:"template"`
		Document string `json:"document"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	if envelope.Document != "" {
		return []byte(envelope.Document)
	}
	if envelope.Template != "" {
		return []byte(envelope.Template)
	}
	return raw
}

// parsePhaseDraft validates a `planr_new: phase` draft document.
func parsePhaseDraft(raw []byte) (PhaseDraft, error) {
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		return PhaseDraft{}, validation.Wrap(err, "frontmatter", "frontmatter")
	}
	if err := draft.CheckPlaceholders(string(raw)); err != nil {
		return PhaseDraft{}, err
	}
	planName, ok := front["planr_plan"].(string)
	if !ok || strings.TrimSpace(planName) == "" {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", "phase draft requires planr_plan")
	}
	planName = strings.TrimSpace(planName)
	title, ok := front["phase_title"].(string)
	if !ok || strings.TrimSpace(title) == "" {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", "phase draft requires phase_title")
	}
	title = strings.TrimSpace(title)
	if strings.ContainsAny(title, "\r\n") {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", "phase title must be a single line")
	}
	if _, found := front["phase"]; found {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", "phase number is assigned by planr apply; remove phase from the draft")
	}
	slug, ok := front["slug"].(string)
	if !ok || !draft.KebabPattern.MatchString(strings.TrimSpace(slug)) {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("phase slug %q must be lowercase kebab-case", slug))
	}
	slug = strings.TrimSpace(slug)

	var meta draft.Meta
	frontYAML, err := yaml.Marshal(front)
	if err != nil {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("parse phase metadata: %v", err))
	}
	if err := yaml.Unmarshal(frontYAML, &meta); err != nil {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("parse phase metadata: %v", err))
	}
	meta.Phase = -1
	meta.Slug = slug
	if meta.Status != "planned" && meta.Status != "conditional" {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("invalid new phase status %q; use planned or conditional", meta.Status))
	}
	if meta.Status == "conditional" && (meta.EntryCondition == nil || strings.TrimSpace(*meta.EntryCondition) == "") {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", "conditional phase requires entry_condition")
	}
	if meta.Status == "planned" && meta.EntryCondition != nil {
		return PhaseDraft{}, phaseDraftValidationError("frontmatter", "planned phase cannot set entry_condition")
	}
	if mdoc.Title(body) != title {
		return PhaseDraft{}, phaseDraftValidationError("phase", fmt.Sprintf("phase title %q in the body does not match phase_title %q", mdoc.Title(body), title))
	}
	planned, completion, err := draft.SplitPhaseDocumentSections(title, body)
	if err != nil {
		return PhaseDraft{}, phaseDraftValidationError("phase", err.Error())
	}
	if planned == "" || completion == "" {
		return PhaseDraft{}, phaseDraftValidationError("phase", "phase work and completion must not be empty")
	}
	return PhaseDraft{Plan: planName, Title: title, Phase: draft.Phase{Title: title, Meta: meta, Planned: planned, Completion: completion}}, nil
}

func phaseDraftValidationError(section, detail string) error {
	return validation.NewFailure(validation.Record{Rule: "phase_document", Section: section, Detail: detail}, detail)
}

// ParseEditSelector splits a `<plan-name>#<phase-number>` phase edit selector
// into the plan name and phase number.
func ParseEditSelector(selector string) (string, int, error) {
	if strings.Count(selector, "#") != 1 {
		return "", 0, fmt.Errorf("planr_edit must use <plan-name>#<phase-number>")
	}
	planName, suffix, _ := strings.Cut(selector, "#")
	planName = strings.TrimSpace(planName)
	suffix = strings.TrimSpace(suffix)
	if planName == "" || suffix == "" {
		return "", 0, fmt.Errorf("planr_edit has an empty plan or document selector")
	}
	phaseID, err := strconv.Atoi(suffix)
	if err != nil || phaseID < 0 {
		return "", 0, fmt.Errorf("edit phase number %q must be a non-negative integer", suffix)
	}
	return planName, phaseID, nil
}

func parseEditDocumentSelector(selector string, front map[string]any) (string, string, int, string, error) {
	if value, found := front["planr_section"]; found {
		section, ok := value.(string)
		if !ok || strings.TrimSpace(section) == "" {
			return "", "", 0, "", fmt.Errorf("planr_section must be goals, context, or plan")
		}
		section = strings.TrimSpace(section)
		if !plan.ValidSection(section) {
			return "", "", 0, "", fmt.Errorf("invalid edit section %q; use goals, context, or plan", section)
		}
		if strings.Contains(selector, "#") {
			return "", "", 0, "", fmt.Errorf("section edit selector must contain only the plan name")
		}
		if strings.TrimSpace(selector) == "" {
			return "", "", 0, "", fmt.Errorf("planr_edit must contain a plan name")
		}
		return strings.TrimSpace(selector), "section", -1, section, nil
	}
	planName, phaseID, err := ParseEditSelector(selector)
	if err != nil {
		return "", "", 0, "", err
	}
	return planName, "phase", phaseID, "", nil
}

// RelativeTargetPath normalises a planr_target value into a repository-relative
// slash path, rejecting anything that escapes the repository.
func RelativeTargetPath(repoRoot, value string) (string, error) {
	path := filepath.FromSlash(strings.TrimSpace(value))
	if filepath.IsAbs(path) {
		path = filepath.Clean(path)
	} else {
		path = filepath.Join(repoRoot, path)
	}
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("planr_target %q must identify a document inside the repository", value)
	}
	return filepath.ToSlash(filepath.Clean(relative)), nil
}
