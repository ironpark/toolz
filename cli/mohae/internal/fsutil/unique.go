// Package fsutil contains focused filesystem operations shared by mohae's
// output producers.
package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WriteFileUnique writes data to stem+suffix in dir without replacing an
// existing path. A collision appends -2, -3, and so on before the suffix.
// A file that cannot be written completely is removed.
func WriteFileUnique(dir, stem, suffix string, data []byte, perm fs.FileMode) (string, error) {
	return writeFileUnique(dir, stem, suffix, perm, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

func writeFileUnique(dir, stem, suffix string, perm fs.FileMode, write func(*os.File) error) (string, error) {
	return claimUniqueName(dir, stem, suffix, func(path string) (result error) {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if err != nil {
			return err
		}
		defer func() {
			result = errors.Join(result, file.Close())
			if result != nil {
				if removeErr := os.Remove(path); removeErr != nil {
					result = errors.Join(result, fmt.Errorf("removing incomplete file: %w", removeErr))
				}
			}
		}()
		return write(file)
	})
}

// MkdirUnique creates stem+suffix in dir without reusing an existing path. A
// collision appends -2, -3, and so on before the suffix.
func MkdirUnique(dir, stem, suffix string, perm fs.FileMode) (string, error) {
	return claimUniqueName(dir, stem, suffix, func(path string) error {
		return os.Mkdir(path, perm)
	})
}

func claimUniqueName(dir, stem, suffix string, create func(path string) error) (string, error) {
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

// SanitizeName keeps a config name usable as a filename stem or directory
// prefix. Report files and workspace directories are both named from the same
// config name, so the mapping lives here with the naming helpers above rather
// than once per package, where the two could drift into spelling the same
// config differently.
func SanitizeName(name string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(name))
	if mapped == "" {
		return "trial"
	}
	return mapped
}
