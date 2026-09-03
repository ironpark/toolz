package gitrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

func EnsureCleanSource(repoRoot string, planDirectories, ignore []string) error {
	paths, err := UncommittedSourcePaths(repoRoot, planDirectories, ignore)
	if err != nil {
		return fmt.Errorf("cannot check uncommitted source changes: %w; use --force to bypass this check", err)
	}
	if len(paths) == 0 {
		return nil
	}
	lines := make([]string, len(paths))
	for index, path := range paths {
		lines[index] = "  - " + path
	}
	return fmt.Errorf("cannot mark phase done while source changes are uncommitted:\n%s\ncommit the source changes first or use --force", strings.Join(lines, "\n"))
}

func UncommittedSourcePaths(repoRoot string, planDirectories, ignore []string) ([]string, error) {
	repository, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return nil, err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := worktree.Status()
	if err != nil {
		return nil, err
	}
	paths := []string{}
	ignorePatterns := compileIgnorePatterns(ignore)
	for path, fileStatus := range status {
		if fileStatus == nil || (fileStatus.Staging == git.Unmodified && fileStatus.Worktree == git.Unmodified) {
			continue
		}
		if !isGeneratedPlanPath(repoRoot, planDirectories, path) && !isPlanDraftPath(repoRoot, path) && !matchesIgnorePatterns(path, ignorePatterns) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func isPlanDraftPath(repoRoot, relativePath string) bool {
	clean := filepath.ToSlash(filepath.Clean(relativePath))
	if clean == ".planr" || strings.HasPrefix(clean, ".planr/") {
		return true
	}
	if !strings.EqualFold(filepath.Ext(relativePath), ".md") {
		return false
	}
	raw, err := vfs.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		return false
	}
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		return false
	}
	if name, ok := front["plan_name"].(string); ok && name != "" {
		return true
	}
	if value, ok := front["planr_new"].(string); ok && value == "phase" {
		return true
	}
	_, ok := front["planr_edit"]
	return ok
}

type ignorePattern struct {
	raw        string
	expression *regexp.Regexp
}

func compileIgnorePatterns(patterns []string) []ignorePattern {
	compiled := make([]ignorePattern, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		pattern = strings.TrimPrefix(pattern, "./")
		if pattern == "" {
			continue
		}
		compiled = append(compiled, ignorePattern{raw: pattern, expression: globPathExpression(pattern)})
	}
	return compiled
}

func (p ignorePattern) match(path string) bool {
	if p.expression != nil && p.expression.MatchString(path) {
		return true
	}
	return !strings.ContainsAny(p.raw, "*?") && (path == p.raw || strings.HasPrefix(path, strings.TrimSuffix(p.raw, "/")+"/"))
}

func IsIgnoredPath(relativePath string, patterns []string) bool {
	return matchesIgnorePatterns(relativePath, compileIgnorePatterns(patterns))
}

func matchesIgnorePatterns(relativePath string, patterns []ignorePattern) bool {
	path := filepath.ToSlash(filepath.Clean(relativePath))
	for _, pattern := range patterns {
		if pattern.match(path) {
			return true
		}
	}
	return false
}

func globPathExpression(pattern string) *regexp.Regexp {
	pattern = filepath.ToSlash(pattern)
	var expression strings.Builder
	expression.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				expression.WriteString(".*")
				index++
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[index])))
		}
	}
	expression.WriteString("$")
	compiled, err := regexp.Compile(expression.String())
	if err != nil {
		return nil
	}
	return compiled
}

func isGeneratedPlanPath(repoRoot string, planDirectories []string, relativePath string) bool {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if filepath.Clean(absPath) == filepath.Join(repoRoot, ".planr.yaml") {
		return true
	}
	for _, planDirectory := range planDirectories {
		relative, err := filepath.Rel(planDirectory, absPath)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative) {
			return true
		}
	}
	return false
}
