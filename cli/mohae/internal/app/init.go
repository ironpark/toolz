package app

import (
	"context"
	"embed"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
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

func initAction(_ context.Context, cmd *cli.Command) error {
	template := cmd.String("template")
	if !slices.Contains(Templates, template) {
		return fmt.Errorf("unknown --template %q (one of: %s)", template, strings.Join(Templates, ", "))
	}
	target := cmd.Args().First()
	if target == "" {
		target = DefaultConfigName
	}
	directory := "."
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		directory = target
		target = filepath.Join(target, DefaultConfigName)
	} else if filepath.Ext(target) == "" {
		directory = target
		target = filepath.Join(target, DefaultConfigName)
	} else {
		directory = filepath.Dir(target)
	}

	files := map[string]string{target: configTemplate(template)}
	all := cmd.Bool("all")
	if all || cmd.Bool("with-scripts") {
		addTemplate(files, directory, "init.sh")
		addTemplate(files, directory, "verify.sh")
	}
	if all || cmd.Bool("with-agent-md") {
		addTemplate(files, directory, "AGENTS.md")
	}
	if all || cmd.Bool("with-prompt") {
		addTemplate(files, directory, "PROMPT.md")
	}
	if all || cmd.Bool("with-fixture") {
		addTemplate(files, directory, "fixture/README.md")
	}
	if cmd.Bool("with-mcp") || (all && template == "mcp-server") {
		addTemplate(files, directory, "mcp.json")
	}

	if !cmd.Bool("force") {
		for path := range files {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
		}
	}
	if directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
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
		fmt.Fprintf(cmd.Writer, "created %s\n", path)
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
