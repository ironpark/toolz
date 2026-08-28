package draft

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// KebabPattern matches the lowercase kebab-case names planr accepts.
var KebabPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Dependency struct {
	Plan  string
	Phase *int
}

func RequireDescription(value string) (string, error) {
	return NormalizeDescription(value, true)
}

func NormalizeDescription(value string, required bool) (string, error) {
	count := utf8.RuneCountInString(value)
	if count > 200 {
		return "", fmt.Errorf("description must be 200 characters or fewer (including spaces); got %d", count)
	}
	description := strings.TrimSpace(value)
	if required && description == "" {
		return "", fmt.Errorf("new requires --description (a short description up to 200 characters)")
	}
	return description, nil
}

func NormalizeDependencies(values []string, planName string) ([]string, error) {
	result, err := CanonicalDependencies(values)
	if err != nil {
		return nil, err
	}
	for _, dependency := range result {
		parsed, _ := ParseDependency(dependency)
		if parsed.Plan == planName {
			return nil, fmt.Errorf("plan %q cannot depend on itself (dependency %q)", planName, dependency)
		}
	}
	return result, nil
}

func CanonicalDependencies(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("--depends-on must not be empty")
		}
		dependency, err := ParseDependency(value)
		if err != nil {
			return nil, err
		}
		canonical := dependency.Plan
		if dependency.Phase != nil {
			canonical = fmt.Sprintf("%s#%d", canonical, *dependency.Phase)
		}
		if !seen[canonical] {
			seen[canonical] = true
			result = append(result, canonical)
		} else {
			return nil, fmt.Errorf("duplicate plan dependency %q", canonical)
		}
	}
	return result, nil
}

func ParseDependency(value string) (Dependency, error) {
	plan, phaseText, hasPhase := strings.Cut(value, "#")
	plan = Name(plan)
	if !KebabPattern.MatchString(plan) {
		return Dependency{}, fmt.Errorf("dependency %q must use plan-name or plan-name#phase-number", value)
	}
	if !hasPhase {
		return Dependency{Plan: plan}, nil
	}
	phase, err := strconv.Atoi(phaseText)
	if err != nil || phase < 0 {
		return Dependency{}, fmt.Errorf("dependency %q must use a non-negative phase number", value)
	}
	return Dependency{Plan: plan, Phase: &phase}, nil
}

func SameDependencyPhase(left, right Dependency) bool {
	if left.Phase == nil || right.Phase == nil {
		return left.Phase == nil && right.Phase == nil
	}
	return *left.Phase == *right.Phase
}

func DependencyLabel(dependency Dependency) string {
	if dependency.Phase == nil {
		return dependency.Plan
	}
	return fmt.Sprintf("%s#%d", dependency.Plan, *dependency.Phase)
}

func Name(directory string) string {
	parts := strings.SplitN(directory, "-", 2)
	if len(parts) == 2 {
		if len(parts[0]) >= 2 {
			if _, err := strconv.Atoi(parts[0]); err == nil {
				return parts[1]
			}
		}
	}
	return directory
}
