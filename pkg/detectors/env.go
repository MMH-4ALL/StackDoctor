package detectors

import (
	"context"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

type Env struct{}

func (Env) ID() string { return "env" }

var templateName = regexp.MustCompile(`^\.env(?:\.[a-zA-Z0-9_-]+)?\.(?:example|sample|template)$`)
var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// EnvKeys retains only key names. It never includes right-hand-side values.
func EnvKeys(data []byte) map[string]bool {
	keys := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if ok && envKey.MatchString(key) {
			keys[key] = true
		}
	}
	return keys
}

func (Env) Detect(_ context.Context, c *diagnostic.Context) []diagnostic.Finding {
	names, err := c.Files.List(".")
	if err != nil {
		return []diagnostic.Finding{finding("env.scan.unavailable", diagnostic.Environment, diagnostic.Warning, "Environment template listing unavailable", "The selected directory cannot be listed safely or contains too many entries.", "")}
	}
	sort.Strings(names)
	f := []diagnostic.Finding{}
	count := 0
	for _, name := range names {
		if !templateName.MatchString(name) {
			continue
		}
		count++
		if count > 16 {
			f = append(f, finding("env.templates.limit", diagnostic.Environment, diagnostic.Warning, "Environment template limit reached", "Only the first 16 matching templates are checked.", ""))
			break
		}
		b, err := c.Files.Read(name)
		if err != nil {
			f = append(f, unreadable("env.template."+name, name))
			continue
		}
		target := name[:strings.LastIndex(name, ".")]
		keys := EnvKeys(b)
		if !diagnostic.Exists(c.Files, target) {
			command := "cp -n " + name + " " + target
			if runtime.GOOS == "windows" {
				command = "if (!(Test-Path '" + target + "')) { Copy-Item '" + name + "' '" + target + "' }"
			}
			f = append(f, finding("env.file."+name, diagnostic.Environment, diagnostic.Warning, "Environment file may be required: "+target, "A matching template exists. Confirm the framework's loading rules, then fill the file locally. Template values and secret values are never exported.", name, repair("Create the local file if required; this command preserves an existing file.", command)))
			continue
		}
		actual, err := c.Files.Read(target)
		if err != nil {
			f = append(f, unreadable("env.target."+name, target))
			continue
		}
		present := EnvKeys(actual)
		missing := []string{}
		for key := range keys {
			if !present[key] {
				missing = append(missing, key)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			f = append(f, finding("env.keys."+name, diagnostic.Environment, diagnostic.Warning, "Template keys are absent from "+target, "Key names only: "+namesSummary(missing)+". A template can include optional keys; confirm which ones the project needs.", target, repair("Set required keys locally, using the template and project documentation.", "")))
		} else {
			f = append(f, finding("env.keys."+name, diagnostic.Environment, diagnostic.Pass, "Environment template keys are present", "All key names from "+name+" exist in "+target+". Values, validity, and inherited process variables are not evaluated.", target))
		}
	}
	if count == 0 {
		f = append(f, finding("env.templates.none", diagnostic.Environment, diagnostic.Info, "No environment template found", "Recognized templates include .env.example and .env.local.example. Secret values and source code are not searched for inferred requirements.", ""))
	}
	return f
}
