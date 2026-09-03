package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Manifest is the file that makes a directory a skill. The name comes from the
// convention the agent CLIs already share, so a repository written for them
// needs nothing added to be usable here.
const Manifest = "SKILL.md"

// searchDirs are where a repository keeps its skills, in the order a shallower
// location shadows a deeper one. They are the conventional locations rather
// than a full walk of the tree: a repository that vendors its dependencies
// would otherwise offer every skill its dependencies happen to contain.
var searchDirs = []string{
	"skills",
	"skills/.curated",
	"skills/.experimental",
	"skills/.system",
	".claude/skills",
	".codex/skills",
	".agent/skills",
}

// Found is one skill located inside a fetched tree.
type Found struct {
	// Name is the directory name, which is what the skill is installed as.
	Name string
	// Dir is the absolute path to the skill's directory.
	Dir string
}

// discover locates the skills in a fetched tree. A tree that is itself a skill
// yields exactly that one; otherwise the conventional directories are searched
// and everything found is returned, so pointing a configuration at a repository
// of skills installs the set rather than making the author list each one.
func discover(root string) ([]Found, error) {
	if isSkill(root) {
		return []Found{{Name: filepath.Base(root), Dir: root}}, nil
	}
	byName := map[string]Found{}
	for _, relative := range searchDirs {
		directory := filepath.Join(root, filepath.FromSlash(relative))
		entries, err := os.ReadDir(directory)
		if err != nil {
			continue // Not every repository uses every convention.
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(directory, entry.Name())
			if !isSkill(candidate) {
				continue
			}
			// First writer wins: searchDirs is ordered shallowest first, so a
			// skill published at the top level shadows a copy vendored deeper.
			if _, taken := byName[entry.Name()]; !taken {
				byName[entry.Name()] = Found{Name: entry.Name(), Dir: candidate}
			}
		}
	}
	if len(byName) == 0 {
		return nil, fmt.Errorf("no %s found: looked in the repository root and in %v", Manifest, searchDirs)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	// Sorted so the same tree installs the same skills in the same order on
	// every machine: an install order that varied would be one more way two
	// runs of one configuration could differ.
	sort.Strings(names)
	found := make([]Found, 0, len(names))
	for _, name := range names {
		found = append(found, byName[name])
	}
	return found, nil
}

func isSkill(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, Manifest))
	return err == nil && info.Mode().IsRegular()
}
