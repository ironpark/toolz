// Package fileutil contains small filesystem operations shared across output
// producers without coupling them to the runner or report packages.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClaimUniqueName finds a name built from stem in dir that create can claim,
// appending a counter when the candidate already exists.
func ClaimUniqueName(dir, stem, suffix string, create func(path string) error) (string, error) {
	for attempt := 0; ; attempt++ {
		candidate := stem
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", stem, attempt+1)
		}
		path := filepath.Join(dir, candidate+suffix)
		err := create(path)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		return path, nil
	}
}
