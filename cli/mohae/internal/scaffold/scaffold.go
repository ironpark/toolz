package scaffold

import (
	"embed"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ironpark/toolz/cli/mohae/internal/config"
)

//go:embed templates/*
var templateFiles embed.FS

// Templates differ in what they put under test, not in how a trial is run.
var Templates = []string{"basic", "mcp-server", "cli-skill", "multi-agent"}

var supportTemplates = map[string]string{
	"init.sh":           "templates/init.sh",
	"verify.sh":         "templates/verify.sh",
	"AGENTS.md":         "templates/AGENTS.md",
	"PROMPT.md":         "templates/PROMPT.md",
	"fixture/README.md": "templates/fixture.README.md",
	"mcp.json":          "templates/mcp.json",
}

type Options struct {
	Template    string
	Target      string
	WithScripts bool
	WithAgentMD bool
	WithPrompt  bool
	WithFixture bool
	WithMCP     bool
	All         bool
	Force       bool
	Out         io.Writer
}

// Write creates the selected project scaffold.
func Write(options Options) error {
	if !slices.Contains(Templates, options.Template) {
		return fmt.Errorf("unknown template %q (one of: %s)", options.Template, strings.Join(Templates, ", "))
	}
	target := options.Target
	if target == "" {
		target = config.DefaultConfigName
	}
	// A target that is an existing directory, or that has no extension and so
	// names one, holds the configuration rather than being it.
	var directory string
	info, err := os.Stat(target)
	if (err == nil && info.IsDir()) || filepath.Ext(target) == "" {
		directory = target
		target = filepath.Join(target, config.DefaultConfigName)
	} else {
		directory = filepath.Dir(target)
	}

	files := map[string]string{target: configTemplate(options.Template)}
	if options.All || options.WithScripts {
		addTemplate(files, directory, "init.sh")
		addTemplate(files, directory, "verify.sh")
	}
	if options.All || options.WithAgentMD {
		addTemplate(files, directory, "AGENTS.md")
	}
	if options.All || options.WithPrompt {
		addTemplate(files, directory, "PROMPT.md")
	}
	if options.All || options.WithFixture {
		addTemplate(files, directory, "fixture/README.md")
	}
	if options.WithMCP || (options.All && options.Template == "mcp-server") {
		addTemplate(files, directory, "mcp.json")
	}

	if !options.Force {
		for path := range files {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
		}
	}
	// Each file's parent is created below, which covers directory itself.
	for _, path := range slices.Sorted(maps.Keys(files)) {
		if parent := filepath.Dir(path); parent != "" && parent != "." {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return err
			}
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(files[path]), mode); err != nil {
			return err
		}
		if options.Out != nil {
			fmt.Fprintf(options.Out, "created %s\n", path)
		}
	}
	return nil
}

func configTemplate(name string) string {
	base := strings.Replace(templateContent("templates/config.yaml"), "{{template}}", name, 1)
	if name == "basic" {
		return base
	}
	return base + templateContent("templates/config_"+name+".yaml")
}

func addTemplate(files map[string]string, directory, destination string) {
	files[filepath.Join(directory, filepath.FromSlash(destination))] = templateContent(supportTemplates[destination])
}

func templateContent(path string) string {
	data, err := templateFiles.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("embedded template %s: %v", path, err))
	}
	return string(data)
}
