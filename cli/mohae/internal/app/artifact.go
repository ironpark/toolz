package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// captureArtifacts copies the configured pieces of the finished workspace
// into report.dir before the disposable workspace is removed. Paths in each
// result stay workspace-relative, so a report remains readable after the
// original temporary directory is gone.
func captureArtifacts(config *Config, workspace *Workspace, started time.Time) (string, []ArtifactResult, error) {
	if len(config.Artifacts) == 0 {
		return "", nil, nil
	}
	results := make([]ArtifactResult, len(config.Artifacts))
	selected := make([][]string, len(config.Artifacts))
	for index, pattern := range config.Artifacts {
		results[index].Pattern = pattern
	}

	err := filepath.WalkDir(workspace.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(workspace.Root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		relative = filepath.ToSlash(relative)
		for index, pattern := range config.Artifacts {
			if matchWorkspacePattern(pattern, relative) && !hasSelectedAncestor(selected[index], relative) {
				selected[index] = append(selected[index], relative)
			}
		}
		return nil
	})
	if err != nil {
		return "", results, fmt.Errorf("scanning artifacts: %w", err)
	}

	all := map[string]bool{}
	for index := range selected {
		results[index].Paths = selected[index]
		for _, relative := range selected[index] {
			all[relative] = true
		}
	}
	if len(all) == 0 {
		return "", results, nil
	}

	directory, err := makeArtifactDirectory(config.Resolve(config.Report.Dir), config.Name, started)
	if err != nil {
		return "", results, err
	}
	paths := make([]string, 0, len(all))
	for relative := range all {
		paths = append(paths, relative)
	}
	// A lexicographic sort already puts every ancestor directory before its
	// descendants: "plans" sorts before "plans/one" because they agree on the
	// shared prefix and the shorter string is the smaller one.
	sort.Strings(paths)
	copied := []string{}
	for _, relative := range paths {
		if hasSelectedAncestor(copied, relative) {
			continue
		}
		source := filepath.Join(workspace.Root, filepath.FromSlash(relative))
		target := filepath.Join(directory, filepath.FromSlash(relative))
		if err := copyArtifactPath(source, target); err != nil {
			return directory, results, fmt.Errorf("capturing artifact %s: %w", relative, err)
		}
		copied = append(copied, relative)
	}
	return directory, results, nil
}

func hasSelectedAncestor(selected []string, candidate string) bool {
	for _, parent := range selected {
		if candidate == parent || strings.HasPrefix(candidate, parent+"/") {
			return true
		}
	}
	return false
}

func makeArtifactDirectory(reportDir, name string, started time.Time) (string, error) {
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return "", fmt.Errorf("creating artifact report directory: %w", err)
	}
	stem := sanitizeName(name) + "-" + started.Format("20060102-150405") + ".artifacts"
	directory, err := claimUniqueName(reportDir, stem, "", func(path string) error {
		return os.Mkdir(path, 0o755)
	})
	if err != nil {
		return "", fmt.Errorf("creating artifact directory: %w", err)
	}
	return directory, nil
}

// copyArtifactPath preserves symlinks instead of following them. An agent may
// create a link outside its workspace; capturing that link must not copy host
// data into a report merely because the configured pattern matched it.
func copyArtifactPath(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return copySymlink(source, target)
	case info.IsDir():
		return copyTree(source, target)
	case info.Mode().IsRegular():
		return copyFile(source, target, info.Mode().Perm())
	default:
		return nil
	}
}
