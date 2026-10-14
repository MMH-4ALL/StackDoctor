// Package diagnostic defines StackDoctor's detector contract and scan engine.
package diagnostic

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

type Severity string

const (
	Pass    Severity = "pass"
	Info    Severity = "info"
	Warning Severity = "warning"
	Error   Severity = "error"
)

type Category string

const (
	Runtime       Category = "runtime"
	Dependencies  Category = "dependencies"
	Environment   Category = "environment"
	Ports         Category = "ports"
	Configuration Category = "configuration"
)

// Repair commands are display-only. The engine has no repair executor.
type Repair struct {
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
}

type Finding struct {
	ID       string   `json:"id"`
	Detector string   `json:"detector"`
	Category Category `json:"category"`
	Severity Severity `json:"severity"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	File     string   `json:"file,omitempty"`
	Repairs  []Repair `json:"repairs,omitempty"`
}

type Summary struct {
	Health   string `json:"health"`
	Errors   int    `json:"errors"`
	Warnings int    `json:"warnings"`
	Passed   int    `json:"passed"`
	Info     int    `json:"info"`
}

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	Version       string    `json:"stackdoctor_version"`
	Project       string    `json:"project"`
	Stacks        []string  `json:"stacks"`
	GeneratedAt   time.Time `json:"generated_at"`
	DurationMS    int64     `json:"duration_ms"`
	Summary       Summary   `json:"summary"`
	Findings      []Finding `json:"findings"`
}

func (r *Report) Finalize() {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		if rank(r.Findings[i].Severity) != rank(r.Findings[j].Severity) {
			return rank(r.Findings[i].Severity) < rank(r.Findings[j].Severity)
		}
		return r.Findings[i].ID < r.Findings[j].ID
	})
	r.Summary = Summary{Health: "clear"}
	for _, f := range r.Findings {
		switch f.Severity {
		case Error:
			r.Summary.Errors++
		case Warning:
			r.Summary.Warnings++
		case Pass:
			r.Summary.Passed++
		default:
			r.Summary.Info++
		}
	}
	if len(r.Stacks) == 0 {
		r.Summary.Health = "unknown"
	}
	if r.Summary.Warnings > 0 {
		r.Summary.Health = "needs attention"
	}
	if r.Summary.Errors > 0 {
		r.Summary.Health = "blocked"
	}
}

func rank(s Severity) int {
	switch s {
	case Error:
		return 0
	case Warning:
		return 1
	case Info:
		return 2
	default:
		return 3
	}
}

// Text removes terminal controls and limits untrusted display text.
// Detectors must also avoid returning secret values; escaping is not redaction.
func Text(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 1600 {
			b.WriteString("…")
			break
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
