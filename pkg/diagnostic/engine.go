package diagnostic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Context struct {
	Files         Files
	Tools         ToolProvider
	Ports         PortProvider
	ExplicitPorts []int
	SkipPorts     bool
}

// Detector implementations are compiled in and registered explicitly. No
// project-supplied or downloaded executable plugins are loaded.
type Detector interface {
	ID() string
	Detect(context.Context, *Context) []Finding
}

type Registry struct {
	detectors []Detector
	ids       map[string]bool
}

func (r *Registry) Register(d Detector) error {
	if d == nil || d.ID() == "" {
		return errors.New("detector needs a stable ID")
	}
	if r.ids == nil {
		r.ids = make(map[string]bool)
	}
	if r.ids[d.ID()] {
		return errors.New("duplicate detector ID")
	}
	r.ids[d.ID()] = true
	r.detectors = append(r.detectors, d)
	return nil
}

type Options struct {
	Path         string
	Version      string
	Ports        []int
	SkipPorts    bool
	Tools        ToolProvider
	PortProvider PortProvider
}

func (r *Registry) Scan(ctx context.Context, opts Options) (Report, error) {
	start := time.Now()
	fs, path, err := OpenProject(opts.Path)
	if err != nil {
		return Report{}, err
	}
	defer fs.Close()
	tools := opts.Tools
	if tools == nil {
		tools = &System{ProjectRoot: path}
	}
	ports := opts.PortProvider
	if ports == nil {
		ports = LocalPorts{}
	}
	c := &Context{Files: fs, Tools: tools, Ports: ports, ExplicitPorts: opts.Ports, SkipPorts: opts.SkipPorts}
	report := Report{SchemaVersion: 1, Version: opts.Version, Project: Text(filepath.Base(path)), Stacks: []string{}, GeneratedAt: start.UTC(), Findings: []Finding{}}
	for _, marker := range []struct {
		stack string
		files []string
	}{
		{"Node.js", []string{"package.json"}}, {"Python", []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg"}},
		{"Go", []string{"go.mod", "go.work"}}, {"Rust", []string{"Cargo.toml"}},
	} {
		for _, name := range marker.files {
			_, statErr := fs.Stat(name)
			if statErr == nil {
				report.Stacks = append(report.Stacks, marker.stack)
				break
			}
			if !errors.Is(statErr, os.ErrNotExist) {
				report.Findings = append(report.Findings, Finding{ID: "project.marker." + name, Detector: "engine", Category: Configuration, Severity: Error, Title: "Project marker cannot be inspected safely", Detail: "The named marker is inaccessible or a symbolic link. No external target was read.", File: name})
			}
		}
	}
	for _, detector := range r.detectors {
		if ctx.Err() != nil {
			return Report{}, errors.New("scan cancelled or timed out")
		}
		findings := detector.Detect(ctx, c)
		for i := range findings {
			findings[i].Detector = detector.ID()
			findings[i].Title = Text(findings[i].Title)
			findings[i].Detail = Text(findings[i].Detail)
			findings[i].File = Text(findings[i].File)
			for j := range findings[i].Repairs {
				findings[i].Repairs[j].Description = Text(findings[i].Repairs[j].Description)
				findings[i].Repairs[j].Command = Text(findings[i].Repairs[j].Command)
			}
		}
		report.Findings = append(report.Findings, findings...)
	}
	if ctx.Err() != nil {
		return Report{}, errors.New("scan cancelled or timed out")
	}
	if len(report.Stacks) == 0 {
		report.Findings = append(report.Findings, Finding{ID: "project.unsupported", Detector: "engine", Category: Configuration, Severity: Info, Title: "No supported project manifest at this path", Detail: "Select the folder containing package.json, pyproject.toml, requirements.txt, go.mod, go.work, or Cargo.toml. Subdirectories are not scanned automatically."})
	}
	report.DurationMS = time.Since(start).Milliseconds()
	report.Finalize()
	return report, nil
}
