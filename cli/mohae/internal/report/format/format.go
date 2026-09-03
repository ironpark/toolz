// Package format defines the report formats understood by configuration,
// command flags, and renderers.
package format

import "slices"

const (
	Terminal = "terminal"
	JSON     = "json"
	Markdown = "markdown"
	HTML     = "html"
)

var known = [...]string{HTML, JSON, Markdown, Terminal}

// All returns every supported format in stable diagnostic order.
func All() []string {
	return slices.Clone(known[:])
}

// IsKnown reports whether name is a supported report format.
func IsKnown(name string) bool {
	return slices.Contains(known[:], name)
}
