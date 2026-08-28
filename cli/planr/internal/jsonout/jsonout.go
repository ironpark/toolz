// Package jsonout holds the `--json` output schema for every planr command.
// The types here are a public contract: field names, JSON tags, field order and
// the empty-slice-vs-null shape are what consumers parse, so they live in one
// place instead of being spread across the command layer.
package jsonout

import (
	"encoding/json"
	"fmt"
	"os"
)

// Write encodes value as a single JSON line on stdout.
func Write(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}
