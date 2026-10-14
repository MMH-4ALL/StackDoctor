package detectors

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
	"github.com/Masterminds/semver/v3"
)

func finding(id string, category diagnostic.Category, severity diagnostic.Severity, title, detail, file string, repairs ...diagnostic.Repair) diagnostic.Finding {
	return diagnostic.Finding{ID: id, Category: category, Severity: severity, Title: title, Detail: detail, File: file, Repairs: repairs}
}

func repair(description, command string) diagnostic.Repair {
	return diagnostic.Repair{Description: description, Command: command}
}

func unreadable(id, file string) diagnostic.Finding {
	return finding(id, diagnostic.Configuration, diagnostic.Error, "Cannot safely read "+file, "The file is inaccessible, larger than 2 MiB, a symbolic link, or not a regular file. Inspect it locally; file contents are never included in this error.", file)
}

func runtimeFinding(ctx context.Context, c *diagnostic.Context, name, id string) (diagnostic.Tool, diagnostic.Finding) {
	t := c.Tools.Probe(ctx, name)
	if !t.Available {
		return t, finding(id+".missing", diagnostic.Runtime, diagnostic.Error, name+" is unavailable", t.Problem, "", repair("Install the runtime from its official distribution, then open a new terminal.", ""))
	}
	if t.Version == "" {
		return t, finding(id+".unknown", diagnostic.Runtime, diagnostic.Warning, name+" found; version unverified", t.Problem, "")
	}
	return t, finding(id+".available", diagnostic.Runtime, diagnostic.Pass, name+" "+t.Version+" available", "A version-only probe succeeded. Project code was not executed.", "")
}

var safeConstraint = regexp.MustCompile(`^[0-9vVxX*<>=~^|!, .+-]+$`)
var numericVersion = regexp.MustCompile(`^v?[0-9]+(?:\.[0-9]+){0,2}$`)

func compatible(version, constraint string) (bool, bool) {
	if len(constraint) > 180 || !safeConstraint.MatchString(constraint) {
		return false, false
	}
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, false
	}
	r, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, false
	}
	return r.Check(v), true
}

func versionFinding(id, tool, version, required, file string) diagnostic.Finding {
	ok, known := compatible(version, required)
	if !known {
		return finding(id, diagnostic.Runtime, diagnostic.Warning, tool+" version rule needs review", "This version rule is unsupported or the installed version is unknown. Inspect the requirement in the named file.", file)
	}
	if ok {
		return finding(id, diagnostic.Runtime, diagnostic.Pass, tool+" meets the project version rule", fmt.Sprintf("Detected %s; required %s.", version, required), file)
	}
	return finding(id, diagnostic.Runtime, diagnostic.Error, tool+" version does not match", fmt.Sprintf("Detected %s; required %s.", version, required), file, repair("Select a compatible runtime with your version manager, then run StackDoctor again.", ""))
}

func namesSummary(names []string) string {
	if len(names) > 12 {
		return strings.Join(names[:12], ", ") + fmt.Sprintf(" and %d more", len(names)-12)
	}
	return strings.Join(names, ", ")
}
