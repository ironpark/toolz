// Package mdoc reads and writes the Markdown-with-YAML-Split documents
// that make up a plan.
package mdoc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"
)

func Split(input string) (map[string]any, string, error) {
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

func Render(front map[string]any, body string) (string, error) {
	header, err := yaml.Marshal(PruneEmptyMeta(front))
	if err != nil {
		return "", err
	}
	return "---\n" + string(header) + "---\n" + body, nil
}

func Title(contents string) string {
	for _, line := range strings.Split(contents, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return "unnamed phase"
}

func Strings(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func WriteFile(path string, front map[string]any, body string) error {
	contents, err := Render(front, body)
	if err != nil {
		return fmt.Errorf("encode %s frontmatter: %w", filepath.Base(path), err)
	}
	return WriteAtomically(path, contents)
}

// WriteAtomically rewrites a document that may already be tracked in git,
// so the contents are staged next to the target and renamed into place: an
// interrupted write leaves the previous contents rather than a truncated
// document.
func WriteAtomically(path, contents string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.WriteString(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(temporary.Name(), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func Body(raw []byte) string {
	_, body, err := Split(string(raw))
	if err != nil {
		return string(raw)
	}
	return body
}

// WithBody replaces only the body of a Split document. Edits to
// a derived or prose section must not reserialize unrelated Split and
// accidentally create a noisy change.
func WithBody(raw, body string) (string, error) {
	if !strings.HasPrefix(raw, "---\n") {
		return body, nil
	}
	_, currentBody, err := Split(raw)
	if err != nil {
		return "", err
	}
	offset := len(raw) - len(currentBody)
	if offset < 0 || offset > len(raw) {
		return "", fmt.Errorf("could not locate document body")
	}
	return raw[:offset] + body, nil
}

func CopyFront(front map[string]any) map[string]any {
	result := make(map[string]any, len(front))
	for key, value := range front {
		result[key] = value
	}
	return result
}

func Hash(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func FrontString(front map[string]any, key string) string {
	return StringValue(front[key])
}

// StringValue is the shared coercion of a Split value into a string; a
// non-string value reads as empty.
func StringValue(value any) string {
	text, _ := value.(string)
	return text
}

func PruneEmptyMeta(front map[string]any) map[string]any {
	for key, value := range front {
		if IsEmptyMeta(value) {
			delete(front, key)
		}
	}
	return front
}

func IsEmptyMeta(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case *string:
		return typed == nil || strings.TrimSpace(*typed) == ""
	}
	switch reflected := reflect.ValueOf(value); reflected.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return reflected.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return reflected.IsNil()
	}
	return false
}
