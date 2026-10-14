// Package report renders the same real diagnostic report as JSON or Markdown.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

func JSON(w io.Writer, r diagnostic.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func md(s string) string {
	s = diagnostic.Text(s)
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "|", "\\|", "#", "\\#").Replace(s)
}

func Markdown(w io.Writer, r diagnostic.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# StackDoctor report\n\nProject: **%s**  \nStacks: %s  \nHealth: **%s**  \nGenerated: %s  \nVersion: %s\n\n", md(r.Project), md(strings.Join(r.Stacks, ", ")), md(r.Summary.Health), r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), md(r.Version))
	fmt.Fprintf(&b, "%d errors · %d warnings · %d passed · %d informational checks\n\n", r.Summary.Errors, r.Summary.Warnings, r.Summary.Passed, r.Summary.Info)
	b.WriteString("Repair commands are suggestions for review. StackDoctor did not run them. Clear means no detected blockers, not a guarantee that the project will run.\n\n")
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "## %s: %s\n\nID: %s · Category: %s\n\n%s\n\n", md(string(f.Severity)), md(f.Title), md(f.ID), md(string(f.Category)), md(f.Detail))
		if f.File != "" {
			fmt.Fprintf(&b, "File: %s\n\n", md(f.File))
		}
		for _, repair := range f.Repairs {
			fmt.Fprintf(&b, "- %s\n", md(repair.Description))
			if repair.Command != "" {
				fmt.Fprintf(&b, "\n    %s\n\n", diagnostic.Text(repair.Command))
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
