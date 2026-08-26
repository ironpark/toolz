package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

var topHeading = regexp.MustCompile(`(?m)^# (GOALS|SCOPE|CONTEXT|PHASES|VERIFICATION|ORDERING|NEXT)[ \t]*$`)
var phaseHeading = regexp.MustCompile(`(?m)^## PHASE\s*(?:—|:|-)\s*(.+?)[ \t]*$`)
var kebab = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
var requiredSections = []string{"GOALS", "SCOPE", "CONTEXT", "PHASES", "VERIFICATION", "ORDERING", "NEXT"}

type phaseMeta struct {
	Phase     int    `yaml:"phase"`
	Slug      string `yaml:"slug"`
	PerfPhase bool   `yaml:"perf_phase"`
	// References as written in the draft: phase numbers, phase slugs, or a mix.
	// parsePhases resolves them into DependsOn once every phase in the plan is
	// known, because a slug can only be resolved against its siblings.
	DependsOnRefs  []phaseRef `yaml:"depends_on"`
	DependsOn      []int      `yaml:"-"`
	Status         string     `yaml:"status"`
	EntryCondition *string    `yaml:"entry_condition"`
}

// phaseRef is one entry of a phase's depends_on list, before resolution.
type phaseRef struct {
	number *int
	slug   string
}

func (r *phaseRef) UnmarshalYAML(raw []byte) error {
	var number int
	if err := yaml.Unmarshal(raw, &number); err == nil {
		r.number = &number
		return nil
	}
	var slug string
	if err := yaml.Unmarshal(raw, &slug); err == nil {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			return fmt.Errorf("depends_on entry must not be empty")
		}
		r.slug = slug
		return nil
	}
	return fmt.Errorf("depends_on entry %s must be a phase number or a phase slug", strings.TrimSpace(string(raw)))
}

func (r phaseRef) String() string {
	if r.number != nil {
		return strconv.Itoa(*r.number)
	}
	return strconv.Quote(r.slug)
}

// resolvePhaseRefs turns every depends_on entry into a phase number. Slugs are
// looked up among the plan's own phases; a slug that matches nothing is
// reported with the slugs that were available, since a typo here is otherwise
// indistinguishable from a missing phase.
func resolvePhaseRefs(phases []draftPhase) error {
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
			if ref.number != nil {
				if *ref.number < 0 {
					return fmt.Errorf("phase %q depends_on %d: phase numbers must be non-negative", phase.Title, *ref.number)
				}
				phase.Meta.DependsOn = append(phase.Meta.DependsOn, *ref.number)
				continue
			}
			number, ok := numbers[ref.slug]
			if !ok {
				return fmt.Errorf("phase %q depends_on %q, but no phase in this plan has that slug; available slugs: %s",
					phase.Title, ref.slug, strings.Join(known, ", "))
			}
			phase.Meta.DependsOn = append(phase.Meta.DependsOn, number)
		}
	}
	return nil
}

type draftPhase struct {
	Title      string
	Meta       phaseMeta
	Planned    string
	Completion string
}
type draft struct {
	Name, Goals, Scope, Context, Verification, Ordering, NextText string
	Description                                                   string
	DependsOn                                                     []string
	Phases                                                        []draftPhase
	NextPhase                                                     int
}

func parseDraft(raw []byte, fallback string) (draft, error) {
	front, body, err := frontmatter(string(raw))
	if err != nil {
		return draft{}, err
	}
	matches := topHeading.FindAllStringSubmatchIndex(body, -1)
	if len(matches) != len(requiredSections) {
		return draft{}, fmt.Errorf("expected sections: %s", strings.Join(requiredSections, ", "))
	}
	if err := checkDraftPlaceholders(string(raw)); err != nil {
		return draft{}, err
	}
	sections := map[string]string{}
	for i, m := range matches {
		name := body[m[2]:m[3]]
		if name != requiredSections[i] {
			return draft{}, fmt.Errorf("section %d must be # %s", i+1, requiredSections[i])
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
	if !kebab.MatchString(name) {
		return draft{}, fmt.Errorf("plan name %q must be lowercase kebab-case", name)
	}
	description, err := draftDescription(front)
	if err != nil {
		return draft{}, fmt.Errorf("invalid description for plan %q: %w", name, err)
	}
	dependsOn, err := canonicalPlanDependencies(yamlStrings(front["depends_on"]))
	if err != nil {
		return draft{}, fmt.Errorf("invalid plan dependencies: %w", err)
	}
	phases, err := parsePhases(sections["PHASES"])
	if err != nil {
		return draft{}, err
	}
	next, nextText, err := parseNext(sections["NEXT"])
	if err != nil {
		return draft{}, err
	}
	found := false
	for _, p := range phases {
		if p.Meta.Phase == next {
			found = true
		}
	}
	if !found {
		return draft{}, fmt.Errorf("NEXT references undefined phase %d", next)
	}
	return draft{Name: name, Goals: sections["GOALS"], Scope: sections["SCOPE"], Context: sections["CONTEXT"], Description: description, DependsOn: dependsOn, Phases: phases, Verification: sections["VERIFICATION"], Ordering: sections["ORDERING"], NextPhase: next, NextText: nextText}, nil
}

// draftPlaceholder marks the spots a freshly generated draft leaves for the
// author to fill in. `planr new` emits a skeleton that is deliberately not yet
// registrable; reporting every remaining marker at once keeps the author from
// discovering the requirements one failed `planr add` at a time.
const draftPlaceholder = "TODO(planr)"

func checkDraftPlaceholders(raw string) error {
	lines := []string{}
	// The template documents the marker inside an HTML comment, so comment
	// bodies are not themselves placeholders.
	for number, line := range strings.Split(blankHTMLComments(raw), "\n") {
		if strings.Contains(line, draftPlaceholder) {
			lines = append(lines, fmt.Sprintf("  line %d: %s", number+1, strings.TrimSpace(line)))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("draft still has %d unfilled %s placeholder(s):\n%s\nreplace each line with real content, then run planr add again",
		len(lines), draftPlaceholder, strings.Join(lines, "\n"))
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
	return normalizeDescription(description, false)
}

func frontmatter(input string) (map[string]any, string, error) {
	if !strings.HasPrefix(input, "---\n") {
		return map[string]any{}, input, nil
	}
	end := strings.Index(input[4:], "\n---\n")
	if end < 0 {
		return nil, "", fmt.Errorf("unterminated document frontmatter")
	}
	values := map[string]any{}
	if err := yaml.Unmarshal([]byte(input[4:end+4]), &values); err != nil {
		return nil, "", fmt.Errorf("parse document frontmatter: %w", err)
	}
	return values, input[end+9:], nil
}

func parsePhases(section string) ([]draftPhase, error) {
	matches := phaseHeading.FindAllStringSubmatchIndex(section, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("PHASES must contain at least one phase")
	}
	var result []draftPhase
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
			return nil, fmt.Errorf("phase %q must start with yaml metadata", title)
		}
		close := strings.Index(block[8:], "\n```")
		if close < 0 {
			return nil, fmt.Errorf("phase %q has unterminated yaml metadata", title)
		}
		var meta phaseMeta
		if err := yaml.Unmarshal([]byte(block[8:close+8]), &meta); err != nil {
			return nil, fmt.Errorf("parse phase %q: %w", title, err)
		}
		if meta.Phase < 0 {
			return nil, fmt.Errorf("phase %q has invalid number %d; phase numbers must be non-negative", title, meta.Phase)
		}
		if !kebab.MatchString(meta.Slug) || (meta.Status != "planned" && meta.Status != "conditional") {
			return nil, fmt.Errorf("phase %q has invalid slug or status", title)
		}
		if meta.Status == "conditional" && (meta.EntryCondition == nil || strings.TrimSpace(*meta.EntryCondition) == "") {
			return nil, fmt.Errorf("conditional phase %q requires entry_condition", title)
		}
		if meta.Status == "planned" && meta.EntryCondition != nil {
			return nil, fmt.Errorf("planned phase %q requires entry_condition: null", title)
		}
		rest := strings.TrimSpace(block[close+12:])
		planned, completion, err := splitPhaseSections(title, rest)
		if err != nil {
			return nil, err
		}
		if planned == "" || completion == "" {
			return nil, fmt.Errorf("phase %q work and completion must not be empty", title)
		}
		if ids[meta.Phase] || slugs[meta.Slug] {
			return nil, fmt.Errorf("duplicate phase %d", meta.Phase)
		}
		ids[meta.Phase] = true
		slugs[meta.Slug] = true
		result = append(result, draftPhase{title, meta, planned, completion})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Meta.Phase < result[j].Meta.Phase })
	if err := resolvePhaseRefs(result); err != nil {
		return nil, err
	}
	return result, nil
}

// splitPhaseSections divides a phase block's prose into its planned-work and
// done-when halves. A draft may use the headings of any supported language, not
// only the configured one, so a plan authored elsewhere still parses here.
func splitPhaseSections(title, rest string) (string, string, error) {
	for _, pair := range phaseSectionHeadings() {
		work, done := "### "+pair[0], "### "+pair[1]
		if !strings.HasPrefix(rest, work) {
			continue
		}
		split := strings.Index(rest, "\n"+done)
		if split < 0 {
			return "", "", fmt.Errorf("phase %q must contain %s", title, done)
		}
		return strings.TrimSpace(rest[len(work):split]), strings.TrimSpace(rest[split+len(done)+1:]), nil
	}
	expected := make([]string, 0, len(phaseSectionHeadings()))
	for _, pair := range phaseSectionHeadings() {
		expected = append(expected, fmt.Sprintf("### %s / ### %s", pair[0], pair[1]))
	}
	return "", "", fmt.Errorf("phase %q must contain a planned-work and a done-when section; expected one of: %s",
		title, strings.Join(expected, " | "))
}

func validateDraftDependencies(d *draft) error {
	dependencies, err := normalizePlanDependencies(d.DependsOn, d.Name)
	if err != nil {
		return fmt.Errorf("invalid dependencies for plan %q: %w", d.Name, err)
	}
	d.DependsOn = dependencies
	if err := validatePhaseDependencies(d.Phases); err != nil {
		return fmt.Errorf("invalid phase dependencies in plan %q: %w", d.Name, err)
	}
	return nil
}

func validatePhaseDependencies(phases []draftPhase) error {
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
				return fmt.Errorf("phase %d %q cannot depend on itself; remove %d from depends_on", phase.Meta.Phase, phase.Title, dependency)
			}
			if !defined[dependency] {
				return fmt.Errorf("phase %d %q depends on phase %d, but phase %d is not defined in this plan", phase.Meta.Phase, phase.Title, dependency, dependency)
			}
			if seen[dependency] {
				return fmt.Errorf("phase %d %q lists phase %d more than once in depends_on", phase.Meta.Phase, phase.Title, dependency)
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
		var phase draftPhase
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
				return fmt.Errorf("phase dependency cycle detected: %s", formatPhaseCycle(cycle, phaseTitles))
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
