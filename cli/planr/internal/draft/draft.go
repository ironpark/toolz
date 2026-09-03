package draft

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
)

var topHeading = regexp.MustCompile(`(?m)^# (GOALS|SCOPE|CONTEXT|PHASES|VERIFICATION|ORDERING|NEXT)[ \t]*$`)
var phaseHeading = regexp.MustCompile(`(?m)^## PHASE\s*(?:—|:|-)\s*(.+?)[ \t]*$`)
var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
var requiredSections = []string{"GOALS", "SCOPE", "CONTEXT", "PHASES", "VERIFICATION", "ORDERING", "NEXT"}

const (
	StatusPlanned     = "planned"
	StatusConditional = "conditional"
	StatusInProgress  = "in-progress"
	StatusDone        = "done"
)

var statuses = []string{StatusPlanned, StatusConditional, StatusInProgress, StatusDone}

func RequiredSections() []string {
	return append([]string{}, requiredSections...)
}

func Statuses() []string {
	return append([]string{}, statuses...)
}

func NewPhaseStatuses() []string {
	return []string{StatusPlanned, StatusConditional}
}

func ValidStatus(status string) bool {
	for _, candidate := range statuses {
		if status == candidate {
			return true
		}
	}
	return false
}

type Meta struct {
	Phase     int    `yaml:"phase"`
	Slug      string `yaml:"slug"`
	PerfPhase bool   `yaml:"perf_phase"`
	// References as written in the draft: phase numbers, phase slugs, or a mix.
	// parsePhases resolves them into DependsOn once every phase in the plan is
	// known, because a slug can only be resolved against its siblings.
	DependsOnRefs  []Ref   `yaml:"depends_on"`
	DependsOn      []int   `yaml:"-"`
	Status         string  `yaml:"status"`
	EntryCondition *string `yaml:"entry_condition"`
}

// Ref is one entry of a phase's depends_on list, before resolution.
type Ref struct {
	Number *int
	Slug   string
}

func (r *Ref) UnmarshalYAML(raw []byte) error {
	var number int
	if err := yaml.Unmarshal(raw, &number); err == nil {
		r.Number = &number
		return nil
	}
	var slug string
	if err := yaml.Unmarshal(raw, &slug); err == nil {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			return fmt.Errorf("depends_on entry must not be empty")
		}
		if parsed, parseErr := strconv.Atoi(slug); parseErr == nil {
			r.Number = &parsed
			return nil
		}
		r.Slug = slug
		return nil
	}
	return fmt.Errorf("depends_on entry %s must be a phase number or a phase slug", strings.TrimSpace(string(raw)))
}

func (r Ref) String() string {
	if r.Number != nil {
		return strconv.Itoa(*r.Number)
	}
	return strconv.Quote(r.Slug)
}

// ResolveRefs turns every depends_on entry into a phase number. Slugs are
// looked up among the plan's own phases; a slug that matches nothing is
// reported with the slugs that were available, since a typo here is otherwise
// indistinguishable from a missing phase.
func ResolveRefs(phases []Phase) error {
	numbers := map[string]int{}
	known := make([]string, 0, len(phases))
	for _, phase := range phases {
		numbers[phase.Meta.Slug] = phase.Meta.Phase
		known = append(known, phase.Meta.Slug)
	}
	sort.Strings(known)
	for index := range phases {
		phase := &phases[index]
		phase.Meta.DependsOn = make([]int, 0, len(phase.Meta.DependsOnRefs))
		for _, ref := range phase.Meta.DependsOnRefs {
			if ref.Number != nil {
				if *ref.Number < 0 {
					detail := fmt.Sprintf("phase %q depends_on %d: phase numbers must be non-negative", phase.Title, *ref.Number)
					return validation.NewFailure(validation.Record{Rule: "dependency_reference", Section: "PHASES", Phase: validation.IntPointer(phase.Meta.Phase), Detail: detail}, detail)
				}
				phase.Meta.DependsOn = append(phase.Meta.DependsOn, *ref.Number)
				continue
			}
			number, ok := numbers[ref.Slug]
			if !ok {
				detail := fmt.Sprintf("phase %q depends_on %q, but no phase in this plan has that slug; available slugs: %s",
					phase.Title, ref.Slug, strings.Join(known, ", "))
				return validation.NewFailure(validation.Record{Rule: "dependency_reference", Section: "PHASES", Phase: validation.IntPointer(phase.Meta.Phase), Detail: detail}, detail)
			}
			phase.Meta.DependsOn = append(phase.Meta.DependsOn, number)
		}
	}
	return nil
}

type Phase struct {
	Title      string
	Meta       Meta
	Planned    string
	Completion string
}
type Draft struct {
	Name, Goals, Scope, Context, Verification, Ordering, NextText string
	Description                                                   string
	DependsOn                                                     []string
	Phases                                                        []Phase
	NextPhase                                                     int
}

// DescribeSectionMismatch reports which top-level headings are missing,
// unexpected, or duplicated. A draft is rejected on the heading count alone, so
// without this the author only learns that the count was wrong.
func DescribeSectionMismatch(found []string) string {
	counts := map[string]int{}
	for _, name := range found {
		counts[name]++
	}
	required := map[string]bool{}
	missing := []string{}
	for _, name := range requiredSections {
		required[name] = true
		if counts[name] == 0 {
			missing = append(missing, name)
		}
	}
	duplicated := []string{}
	unexpected := []string{}
	for _, name := range found {
		switch {
		case !required[name]:
			unexpected = appendUnique(unexpected, name)
		case counts[name] > 1:
			duplicated = appendUnique(duplicated, name)
		}
	}
	parts := []string{}
	if len(missing) > 0 {
		parts = append(parts, "missing section(s): "+strings.Join(missing, ", "))
	}
	if len(unexpected) > 0 {
		parts = append(parts, "unexpected section(s): "+strings.Join(unexpected, ", "))
	}
	if len(duplicated) > 0 {
		parts = append(parts, "duplicated section(s): "+strings.Join(duplicated, ", "))
	}
	if len(parts) == 0 {
		// The names all line up, so the count can only be off by repetition that
		// the checks above already tolerate; report the count plainly.
		return fmt.Sprintf("document has %d top-level sections, want %d", len(found), len(requiredSections))
	}
	return strings.Join(parts, "; ")
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func Parse(raw []byte, fallback string) (Draft, error) {
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		return Draft{}, validation.Wrap(err, "frontmatter", "frontmatter")
	}
	matches := topHeading.FindAllStringSubmatchIndex(body, -1)
	if len(matches) != len(requiredSections) {
		// The caller may be an agent that never reads the document, so name the
		// sections that are actually missing or extra instead of only restating
		// what a correct draft looks like.
		found := make([]string, 0, len(matches))
		for _, m := range matches {
			found = append(found, body[m[2]:m[3]])
		}
		detail := DescribeSectionMismatch(found)
		return Draft{}, validation.NewFailure(
			validation.Record{Rule: "sections", Section: "document", Detail: detail},
			fmt.Sprintf("%s\nexpected sections: %s\nfound sections: %s",
				detail,
				strings.Join(requiredSections, ", "),
				joinOrNone(found)),
		)
	}
	if err := CheckPlaceholders(string(raw)); err != nil {
		return Draft{}, err
	}
	sections := map[string]string{}
	for i, m := range matches {
		name := body[m[2]:m[3]]
		if name != requiredSections[i] {
			detail := fmt.Sprintf("section %d must be # %s", i+1, requiredSections[i])
			return Draft{}, validation.NewFailure(validation.Record{Rule: "section_order", Section: requiredSections[i], Detail: detail}, detail)
		}
		end := len(body)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		sections[name] = strings.TrimSpace(body[m[1]:end])
	}
	name := strings.TrimSuffix(filepath.Base(fallback), filepath.Ext(fallback))
	if value, ok := front["plan_name"].(string); ok && value != "" {
		name = value
	} else if value, ok := front["name"].(string); ok && value != "" {
		name = value
	}
	if !KebabPattern.MatchString(name) {
		detail := fmt.Sprintf("plan name %q must be lowercase kebab-case", name)
		return Draft{}, validation.NewFailure(validation.Record{Rule: "plan_name", Section: "frontmatter", Detail: detail}, detail)
	}
	description, err := draftDescription(front)
	if err != nil {
		detail := fmt.Sprintf("invalid description for plan %q: %v", name, err)
		return Draft{}, validation.NewFailure(validation.Record{Rule: "description", Section: "frontmatter", Detail: err.Error()}, detail)
	}
	dependsOn, err := canonicalDependencies(mdoc.Strings(front["depends_on"]))
	if err != nil {
		detail := fmt.Sprintf("invalid plan dependencies: %v", err)
		return Draft{}, validation.NewFailure(validation.Record{Rule: "plan_dependency", Section: "frontmatter", Detail: err.Error()}, detail)
	}
	phases, err := parsePhases(sections["PHASES"])
	if err != nil {
		return Draft{}, validation.Wrap(err, "phase_document", "PHASES")
	}
	next, nextText, err := parseNext(sections["NEXT"])
	if err != nil {
		return Draft{}, validation.Wrap(err, "next_document", "NEXT")
	}
	found := false
	for _, p := range phases {
		if p.Meta.Phase == next {
			found = true
		}
	}
	if !found {
		detail := fmt.Sprintf("NEXT references undefined phase %d", next)
		return Draft{}, validation.NewFailure(validation.Record{Rule: "next_phase", Section: "NEXT", Phase: validation.IntPointer(next), Detail: detail}, detail)
	}
	return Draft{Name: name, Goals: sections["GOALS"], Scope: sections["SCOPE"], Context: sections["CONTEXT"], Description: description, DependsOn: dependsOn, Phases: phases, Verification: sections["VERIFICATION"], Ordering: sections["ORDERING"], NextPhase: next, NextText: nextText}, nil
}

// Placeholder marks the spots a freshly generated draft leaves for the
// author to fill in. `planr new` emits a skeleton that is deliberately not yet
// registrable; reporting every remaining marker at once keeps the author from
// discovering the requirements one failed plan application at a time.
const Placeholder = "TODO(planr)"

func CheckPlaceholders(raw string) error {
	lines := []string{}
	// The template documents the marker inside an HTML comment, so comment
	// bodies are not themselves placeholders.
	for number, line := range strings.Split(blankHTMLComments(raw), "\n") {
		if strings.Contains(line, Placeholder) {
			lines = append(lines, fmt.Sprintf("  line %d: %s", number+1, strings.TrimSpace(line)))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	// Naming the mistake explicitly: the common failure is answering on the
	// following line, which leaves the marker itself untouched.
	message := fmt.Sprintf("draft still has %d unfilled %s placeholder(s):\n%s\noverwrite each of these lines with real content -- adding text below a line leaves its marker in place -- then run planr apply again",
		len(lines), Placeholder, strings.Join(lines, "\n"))
	return validation.NewFailures(placeholderRecords(raw), message)
}

func placeholderRecords(raw string) []validation.Record {
	records := []validation.Record{}
	visible := blankHTMLComments(raw)
	lines := strings.Split(visible, "\n")
	for index, line := range lines {
		if !strings.Contains(line, Placeholder) {
			continue
		}
		section, phase := draftLocation(lines, index)
		record := validation.Record{
			Rule:    "placeholder",
			Section: section,
			Line:    index + 1,
			Detail:  strings.TrimSpace(line),
		}
		if phase != nil {
			record.Phase = phase
		}
		records = append(records, record)
	}
	if len(records) > 0 && records[0].Section == "" {
		if front, _, err := mdoc.Split(raw); err == nil {
			if kind, ok := front["planr_new"].(string); ok && kind == "phase" {
				for index := range records {
					records[index].Section = "PHASE"
				}
			}
		}
	}
	return records
}

// phaseNumberLine matches the `phase:` metadata line inside a draft phase block.
var phaseNumberLine = regexp.MustCompile(`^phase:\s*(-?\d+)\s*$`)

func draftLocation(lines []string, line int) (string, *int) {
	section := ""
	phaseHeadingLine := -1
	for index := 0; index <= line && index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if match := topHeading.FindStringSubmatch(trimmed); len(match) == 2 {
			section = match[1]
			phaseHeadingLine = -1
		}
		if section == "PHASES" && phaseHeading.MatchString(lines[index]) {
			phaseHeadingLine = index
		}
	}
	if section != "PHASES" || phaseHeadingLine < 0 {
		return section, nil
	}
	for index := phaseHeadingLine + 1; index <= line && index < len(lines); index++ {
		if match := phaseNumberLine.FindStringSubmatch(strings.TrimSpace(lines[index])); len(match) == 2 {
			if value, err := strconv.Atoi(match[1]); err == nil {
				return section, validation.IntPointer(value)
			}
		}
	}
	return section, nil
}

// blankHTMLComments replaces the contents of every HTML comment with spaces,
// keeping newlines so line numbers reported elsewhere stay accurate.
func blankHTMLComments(raw string) string {
	return htmlComment.ReplaceAllStringFunc(raw, func(match string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' {
				return r
			}
			return ' '
		}, match)
	})
}

func draftDescription(front map[string]any) (string, error) {
	value, found := front["description"]
	if !found {
		return "", nil
	}
	description, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("description must be a string of 200 characters or fewer")
	}
	return NormalizeDescription(description, false)
}

func parsePhases(section string) ([]Phase, error) {
	matches := phaseHeading.FindAllStringSubmatchIndex(section, -1)
	if len(matches) == 0 {
		return nil, validation.NewFailure(validation.Record{Rule: "phase_document", Section: "PHASES", Detail: "PHASES must contain at least one phase"}, "PHASES must contain at least one phase")
	}
	var result []Phase
	ids := map[int]bool{}
	slugs := map[string]bool{}
	for i, m := range matches {
		end := len(section)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		title := strings.TrimSpace(section[m[2]:m[3]])
		block := strings.TrimSpace(section[m[1]:end])
		if !strings.HasPrefix(block, "```yaml\n") {
			return nil, phaseParseFailure(title, nil, fmt.Sprintf("phase %q must start with yaml metadata", title))
		}
		close := strings.Index(block[8:], "\n```")
		if close < 0 {
			return nil, phaseParseFailure(title, nil, fmt.Sprintf("phase %q has unterminated yaml metadata", title))
		}
		var meta Meta
		if err := yaml.Unmarshal([]byte(block[8:close+8]), &meta); err != nil {
			return nil, phaseParseFailure(title, nil, fmt.Sprintf("parse phase %q: %v", title, err))
		}
		if meta.Phase < 0 {
			detail := fmt.Sprintf("phase %q has invalid number %d; phase numbers must be non-negative", title, meta.Phase)
			return nil, phaseParseFailure(title, validation.IntPointer(meta.Phase), detail)
		}
		if !KebabPattern.MatchString(meta.Slug) || (meta.Status != StatusPlanned && meta.Status != StatusConditional) {
			return nil, phaseParseFailure(title, validation.IntPointer(meta.Phase), fmt.Sprintf("phase %q has invalid slug or status", title))
		}
		if meta.Status == StatusConditional && (meta.EntryCondition == nil || strings.TrimSpace(*meta.EntryCondition) == "") {
			return nil, phaseParseFailure(title, validation.IntPointer(meta.Phase), fmt.Sprintf("conditional phase %q requires entry_condition", title))
		}
		if meta.Status == StatusPlanned && meta.EntryCondition != nil {
			return nil, phaseParseFailure(title, validation.IntPointer(meta.Phase), fmt.Sprintf("planned phase %q requires entry_condition: null", title))
		}
		rest := strings.TrimSpace(block[close+12:])
		planned, completion, err := splitPhaseSections(title, rest)
		if err != nil {
			return nil, phaseParseFailure(title, validation.IntPointer(meta.Phase), err.Error())
		}
		if planned == "" || completion == "" {
			return nil, phaseParseFailure(title, validation.IntPointer(meta.Phase), fmt.Sprintf("phase %q work and completion must not be empty", title))
		}
		if ids[meta.Phase] || slugs[meta.Slug] {
			return nil, phaseParseFailure(title, validation.IntPointer(meta.Phase), fmt.Sprintf("duplicate phase %d", meta.Phase))
		}
		ids[meta.Phase] = true
		slugs[meta.Slug] = true
		result = append(result, Phase{title, meta, planned, completion})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Meta.Phase < result[j].Meta.Phase })
	if err := ResolveRefs(result); err != nil {
		return nil, err
	}
	return result, nil
}

func phaseParseFailure(title string, phase *int, detail string) error {
	return validation.NewFailure(validation.Record{Rule: "phase_document", Section: "PHASES", Phase: phase, Detail: detail}, detail)
}

// splitPhaseSections divides a phase block's prose into its planned-work and
// done-when halves. A draft may use the headings of any supported language, not
// only the configured one, so a plan authored elsewhere still parses here.
func splitPhaseSections(title, rest string) (string, string, error) {
	return splitPhaseSectionsWithHeading(title, rest, "### ")
}

// SplitPhaseDocumentSections reads the two prose sections from a registered
// phase document. Registered documents use level-two headings and include a
// title and NEXT marker before them; the heading names themselves still come
// from the language table used by draft parsing.
func SplitPhaseDocumentSections(title, body string) (string, string, error) {
	start := -1
	for _, pair := range doc.PhaseSectionHeadings() {
		candidate := phaseDocumentHeadingOffset(body, "## "+pair[0])
		if candidate >= 0 && (start < 0 || candidate < start) {
			start = candidate
		}
	}
	if start >= 0 {
		return splitPhaseSectionsWithHeading(title, body[start:], "## ")
	}
	return splitPhaseSectionsWithHeading(title, strings.TrimSpace(body), "## ")
}

func phaseDocumentHeadingOffset(body, heading string) int {
	offset := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		value := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if value == heading {
			return offset
		}
		offset += len(line)
	}
	return -1
}

func splitPhaseSectionsWithHeading(title, rest, headingPrefix string) (string, string, error) {
	for _, pair := range doc.PhaseSectionHeadings() {
		work, done := headingPrefix+pair[0], headingPrefix+pair[1]
		if !strings.HasPrefix(rest, work) {
			continue
		}
		split := strings.Index(rest, "\n"+done)
		if split < 0 {
			return "", "", fmt.Errorf("phase %q must contain %s", title, done)
		}
		return strings.TrimSpace(rest[len(work):split]), strings.TrimSpace(rest[split+len(done)+1:]), nil
	}
	expected := make([]string, 0, len(doc.PhaseSectionHeadings()))
	for _, pair := range doc.PhaseSectionHeadings() {
		expected = append(expected, fmt.Sprintf("%s%s / %s%s", headingPrefix, pair[0], headingPrefix, pair[1]))
	}
	return "", "", fmt.Errorf("phase %q must contain a planned-work and a done-when section; expected one of: %s",
		title, strings.Join(expected, " | "))
}

func ValidateDependencies(d *Draft) error {
	dependencies, err := NormalizeDependencies(d.DependsOn, d.Name)
	if err != nil {
		message := fmt.Sprintf("invalid dependencies for plan %q: %v", d.Name, err)
		return validation.NewFailure(validation.Record{Rule: "plan_dependency", Section: "frontmatter", Detail: err.Error()}, message)
	}
	d.DependsOn = dependencies
	if err := ValidatePhaseDependencies(d.Phases); err != nil {
		message := fmt.Sprintf("invalid phase dependencies in plan %q: %v", d.Name, err)
		records := validation.Records(err)
		if len(records) == 0 {
			records = []validation.Record{{Rule: "dependency", Section: "PHASES", Detail: err.Error()}}
		}
		return validation.NewFailures(records, message)
	}
	return nil
}

func ValidatePhaseDependencies(phases []Phase) error {
	defined := map[int]bool{}
	phaseTitles := map[int]string{}
	for _, phase := range phases {
		defined[phase.Meta.Phase] = true
		phaseTitles[phase.Meta.Phase] = phase.Title
	}
	for _, phase := range phases {
		seen := map[int]bool{}
		for _, dependency := range phase.Meta.DependsOn {
			if dependency == phase.Meta.Phase {
				detail := fmt.Sprintf("phase %d %q cannot depend on itself; remove %d from depends_on", phase.Meta.Phase, phase.Title, dependency)
				return validation.NewFailure(validation.Record{Rule: "dependency_self", Section: "PHASES", Phase: validation.IntPointer(phase.Meta.Phase), Detail: detail}, detail)
			}
			if !defined[dependency] {
				detail := fmt.Sprintf("phase %d %q depends on phase %d, but phase %d is not defined in this plan", phase.Meta.Phase, phase.Title, dependency, dependency)
				return validation.NewFailure(validation.Record{Rule: "dependency_missing", Section: "PHASES", Phase: validation.IntPointer(phase.Meta.Phase), Detail: detail}, detail)
			}
			if seen[dependency] {
				detail := fmt.Sprintf("phase %d %q lists phase %d more than once in depends_on", phase.Meta.Phase, phase.Title, dependency)
				return validation.NewFailure(validation.Record{Rule: "dependency_duplicate", Section: "PHASES", Phase: validation.IntPointer(phase.Meta.Phase), Detail: detail}, detail)
			}
			seen[dependency] = true
		}
	}

	state := map[int]int{}
	stack := []int{}
	var visit func(int) error
	visit = func(id int) error {
		state[id] = 1
		stack = append(stack, id)
		var phase Phase
		for _, candidate := range phases {
			if candidate.Meta.Phase == id {
				phase = candidate
				break
			}
		}
		for _, dependency := range phase.Meta.DependsOn {
			if state[dependency] == 1 {
				cycleStart := 0
				for index, value := range stack {
					if value == dependency {
						cycleStart = index
						break
					}
				}
				cycle := append(append([]int{}, stack[cycleStart:]...), dependency)
				detail := fmt.Sprintf("phase dependency cycle detected: %s", formatPhaseCycle(cycle, phaseTitles))
				cyclePhases := append([]int{}, cycle[:len(cycle)-1]...)
				return validation.NewFailure(validation.Record{Rule: "dependency_cycle", Section: "PHASES", Phases: cyclePhases, Detail: detail}, detail)
			}
			if state[dependency] == 0 {
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		return nil
	}
	for _, phase := range phases {
		if state[phase.Meta.Phase] == 0 {
			if err := visit(phase.Meta.Phase); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatPhaseCycle(cycle []int, titles map[int]string) string {
	parts := make([]string, len(cycle))
	for index, id := range cycle {
		parts[index] = fmt.Sprintf("%d %q", id, titles[id])
	}
	return strings.Join(parts, " -> ")
}

func parseNext(section string) (int, string, error) {
	text := strings.TrimSpace(section)
	if !strings.HasPrefix(text, "```yaml\n") {
		return 0, "", fmt.Errorf("NEXT must start with yaml metadata")
	}
	close := strings.Index(text[8:], "\n```")
	if close < 0 {
		return 0, "", fmt.Errorf("NEXT has unterminated yaml metadata")
	}
	var value struct {
		NextPhase int `yaml:"next_phase"`
	}
	if err := yaml.Unmarshal([]byte(text[8:close+8]), &value); err != nil {
		return 0, "", err
	}
	summary := strings.TrimSpace(text[close+12:])
	if summary == "" {
		return 0, "", fmt.Errorf("NEXT description must not be empty")
	}
	return value.NextPhase, strings.Split(summary, "\n")[0], nil
}
