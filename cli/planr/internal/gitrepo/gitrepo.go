package gitrepo

import (
	"errors"
	"fmt"

	git "github.com/go-git/go-git/v5"
)

// EnsureRepository fails fast when planr is run outside a git repository.
// Completion records are stored as git notes and the done-check reads the
// worktree status, so every plan operation assumes a repository is present.
func EnsureRepository(start string) error {
	_, err := git.PlainOpenWithOptions(start, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err == nil {
		return nil
	}
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return fmt.Errorf("planr requires a git repository, but %s is not inside one; run `git init` at your project root first", start)
	}
	return fmt.Errorf("cannot open the git repository for %s: %w", start, err)
}
