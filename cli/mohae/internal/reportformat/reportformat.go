// Package reportformat defines the report formats shared by configuration
// validation, command flags, and renderers.
package reportformat

const (
	Terminal = "terminal"
	JSON     = "json"
	Markdown = "markdown"
	HTML     = "html"
)

// Known is stable for diagnostics and command output.
var Known = []string{HTML, JSON, Markdown, Terminal}
