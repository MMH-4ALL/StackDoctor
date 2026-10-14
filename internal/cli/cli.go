package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MMH-4ALL/StackDoctor/internal/tui"
	"github.com/MMH-4ALL/StackDoctor/pkg/detectors"
	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
	"github.com/MMH-4ALL/StackDoctor/pkg/report"
	"golang.org/x/term"
)

const Help = `StackDoctor — find what is blocking your development environment

Usage:
  stackdoctor check [path] [options]       Scan and print real diagnostics
  stackdoctor doctor [path] [options]      Open the interactive dashboard
  stackdoctor explain [finding-id]         Explain findings and review repairs
  stackdoctor report [path] [options]      Export JSON or Markdown
  stackdoctor --help                      Show this help
  stackdoctor --version                   Show the build version

Options:
  --path <directory>          Project directory (default: current directory)
  --port <number[,number]>    Check explicit loopback TCP ports (repeatable, max 16)
  --no-ports                 Skip all loopback TCP probes
  --format <format>          text, json, or markdown (report defaults to markdown)
  --output <file>            Write a new report file; never overwrite a file
  --strict                   check exits 1 for warnings as well as errors
  --timeout <duration>       Scan time limit, 1s to 2m (default: 30s)

Dashboard: Tab switches category/issues; ↑/↓ or j/k move; Enter views an issue;
           / searches; Esc clears; a shows passed checks; r rescans; ? helps;
           PageUp/PageDown scroll details; q quits.

No repairs run automatically. No telemetry, API keys, or required accounts.
Only the selected project's files are inspected. External runtime tools receive
version-only probes. No package imports, build scripts, or global caches are read.
`

type portsFlag []int

func (p *portsFlag) String() string { return fmt.Sprint([]int(*p)) }
func (p *portsFlag) Set(value string) error {
	for _, raw := range strings.Split(value, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 || n > 65535 {
			return errors.New("ports must be numbers between 1 and 65535")
		}
		if len(*p) >= 16 {
			return errors.New("at most 16 ports may be checked")
		}
		*p = append(*p, n)
	}
	return nil
}

// normalizeFlags allows conventional `check . --format json` syntax while
// preserving -- and flag values. It does not interpret or execute shell syntax.
func normalizeFlags(args []string) []string {
	flags, positionals := []string{}, []string{}
	valueFlags := map[string]bool{"--path": true, "-path": true, "--port": true, "-port": true, "--format": true, "-format": true, "--output": true, "-output": true, "--timeout": true, "-timeout": true}
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(s, "-") {
			flags = append(flags, s)
			if valueFlags[s] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positionals = append(positionals, s)
		}
	}
	return append(append(flags, "--"), positionals...)
}

func Run(ctx context.Context, args []string, out, errOut io.Writer, version string) int {
	command := "check"
	if len(args) == 0 && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		command = "doctor"
	}
	if len(args) > 0 {
		if args[0] == "--version" || args[0] == "version" {
			fmt.Fprintln(out, "StackDoctor "+version)
			return 0
		}
		if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
			fmt.Fprint(out, Help)
			return 0
		}
		if !strings.HasPrefix(args[0], "-") {
			command = args[0]
			args = args[1:]
		}
	}
	if command != "check" && command != "doctor" && command != "explain" && command != "report" {
		fmt.Fprintln(errOut, "Unknown command. Use stackdoctor --help.")
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errOut)
	path := fs.String("path", ".", "project directory")
	format := fs.String("format", "text", "report format")
	output := fs.String("output", "", "new report file")
	skipPorts := fs.Bool("no-ports", false, "skip TCP probes")
	strict := fs.Bool("strict", false, "warnings fail checks")
	timeout := fs.Duration("timeout", 30*time.Second, "scan timeout")
	var ports portsFlag
	fs.Var(&ports, "port", "explicit loopback TCP port")
	fs.Usage = func() { fmt.Fprint(out, Help) }
	if err := fs.Parse(normalizeFlags(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *timeout < time.Second || *timeout > 2*time.Minute {
		fmt.Fprintln(errOut, "Timeout must be between 1s and 2m.")
		return 2
	}
	if len(fs.Args()) > 1 {
		fmt.Fprintln(errOut, "Provide one project path or one finding ID. Use --path with explain.")
		return 2
	}
	id := ""
	if len(fs.Args()) == 1 {
		if command == "explain" {
			id = fs.Args()[0]
		} else {
			if *path != "." {
				fmt.Fprintln(errOut, "Provide the project path once.")
				return 2
			}
			*path = fs.Args()[0]
		}
	}
	if command == "report" && *format == "text" {
		*format = "markdown"
	}
	if *format != "text" && *format != "json" && *format != "markdown" {
		fmt.Fprintln(errOut, "Format must be text, json, or markdown.")
		return 2
	}
	if *output != "" && *format == "text" {
		fmt.Fprintln(errOut, "Use --format json or --format markdown with --output.")
		return 2
	}
	options := diagnostic.Options{Path: *path, Version: version, Ports: ports, SkipPorts: *skipPorts}
	scan := func() (diagnostic.Report, error) {
		scanCtx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		return detectors.Builtins().Scan(scanCtx, options)
	}
	if command == "doctor" {
		if *output != "" || *format != "text" {
			fmt.Fprintln(errOut, "Use stackdoctor report for report export.")
			return 2
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			fmt.Fprintln(errOut, "The dashboard needs an interactive terminal. Use stackdoctor check or stackdoctor report.")
			return 2
		}
		if err := tui.Run(ctx, scan, version); err != nil {
			fmt.Fprintln(errOut, "Dashboard stopped:", diagnostic.Text(err.Error()))
			return 2
		}
		return 0
	}
	r, err := scan()
	if err != nil {
		fmt.Fprintln(errOut, "Scan failed:", diagnostic.Text(err.Error()))
		return 2
	}
	if command == "explain" && id != "" {
		match := []diagnostic.Finding{}
		for _, f := range r.Findings {
			if f.ID == id {
				match = append(match, f)
			}
		}
		if len(match) == 0 {
			fmt.Fprintln(errOut, "Finding ID was not found in this scan. Run stackdoctor check for current IDs.")
			return 2
		}
		r.Findings = match
		r.Finalize()
	}
	writer := out
	var file *os.File
	if *output != "" {
		file, err = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			fmt.Fprintln(errOut, "Cannot create report file. Choose a new writable path; existing files are never overwritten.")
			return 2
		}
		writer = file
	}
	switch *format {
	case "json":
		err = report.JSON(writer, r)
	case "markdown":
		err = report.Markdown(writer, r)
	default:
		err = Text(writer, r, command == "explain")
	}
	if file != nil {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		fmt.Fprintln(errOut, "Could not write the complete report.")
		return 2
	}
	if *output != "" {
		fmt.Fprintln(out, "Report saved. Secret values are not included.")
	}
	if command == "check" && (r.Summary.Errors > 0 || (*strict && r.Summary.Warnings > 0)) {
		return 1
	}
	return 0
}

func Text(w io.Writer, r diagnostic.Report, explain bool) error {
	var b strings.Builder
	fmt.Fprintf(&b, "StackDoctor · %s · %s\n", r.Project, strings.Join(r.Stacks, " + "))
	fmt.Fprintf(&b, "Health: %s | %d errors, %d warnings, %d passed, %d info | %dms\n\n", r.Summary.Health, r.Summary.Errors, r.Summary.Warnings, r.Summary.Passed, r.Summary.Info, r.DurationMS)
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "[%s] %s\n  %s · %s\n", strings.ToUpper(string(f.Severity)), f.Title, f.ID, f.Category)
		if f.Severity != diagnostic.Pass || explain {
			fmt.Fprintf(&b, "  %s\n", f.Detail)
		}
		for _, repair := range f.Repairs {
			fmt.Fprintf(&b, "  → %s\n", repair.Description)
			if repair.Command != "" {
				fmt.Fprintf(&b, "    $ %s\n", repair.Command)
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("Repair commands are for review; none were executed. Clear means no detected blockers, not guaranteed startup.\n")
	_, err := io.WriteString(w, b.String())
	return err
}
