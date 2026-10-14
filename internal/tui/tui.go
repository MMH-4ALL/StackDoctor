// Package tui presents live diagnostics in a responsive Bubble Tea dashboard.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	bg      = lipgloss.Color("#0B1017")
	panelBG = lipgloss.Color("#101822")
	fg      = lipgloss.Color("#DAE5F0")
	muted   = lipgloss.Color("#8092A8")
	teal    = lipgloss.Color("#67DDCB")
	amber   = lipgloss.Color("#EAB676")
	red     = lipgloss.Color("#FB7789")
	violet  = lipgloss.Color("#ABB5FA")
	line    = lipgloss.Color("#263747")
	normal  = lipgloss.NewStyle().Foreground(fg)
	dim     = lipgloss.NewStyle().Foreground(muted)
	accent  = lipgloss.NewStyle().Foreground(teal).Bold(true)
)

var categories = []struct {
	name     string
	category diagnostic.Category
}{
	{"Overview", ""}, {"Runtimes", diagnostic.Runtime}, {"Dependencies", diagnostic.Dependencies},
	{"Environment", diagnostic.Environment}, {"Ports", diagnostic.Ports}, {"Configuration", diagnostic.Configuration},
}

type Scanner func() (diagnostic.Report, error)
type scanMsg struct {
	report diagnostic.Report
	err    error
}

type Model struct {
	scan                                    Scanner
	version                                 string
	report                                  diagnostic.Report
	err                                     error
	loading                                 bool
	width, height                           int
	category, selected, focus               int
	showPassed, help, detailMode, searching bool
	spinner                                 spinner.Model
	search                                  textinput.Model
	viewport                                viewport.Model
}

func New(scan Scanner, version string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = accent
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "Search findings"
	input.CharLimit = 80
	input.TextStyle = normal
	input.PromptStyle = accent
	input.PlaceholderStyle = dim
	return Model{scan: scan, version: version, loading: true, width: 100, height: 30, spinner: s, search: input, viewport: viewport.New(30, 20)}
}

func (m Model) scanCmd() tea.Cmd {
	return func() tea.Msg { r, err := m.scan(); return scanMsg{r, err} }
}
func (m Model) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.scanCmd()) }

func (m Model) filtered() []diagnostic.Finding {
	result := []diagnostic.Finding{}
	query := strings.ToLower(m.search.Value())
	for _, f := range m.report.Findings {
		if !m.showPassed && f.Severity == diagnostic.Pass {
			continue
		}
		if category := categories[m.category].category; category != "" && f.Category != category {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(f.Title+" "+f.ID+" "+f.Detail), query) {
			continue
		}
		result = append(result, f)
	}
	return result
}

func (m *Model) syncDetail(reset bool) {
	items := m.filtered()
	m.selected = max(0, min(m.selected, len(items)-1))
	if len(items) == 0 {
		m.viewport.SetContent("No findings in this view.\n\nPress a to include passed checks, or Esc to clear search.")
		return
	}
	if reset {
		m.viewport.GotoTop()
	}
	m.viewport.SetContent(m.detail(items[m.selected]))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		m.syncDetail(false)
	case scanMsg:
		m.report, m.err, m.loading = msg.report, msg.err, false
		m.selected = 0
		m.syncDetail(true)
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.searching {
			if key == "esc" || key == "enter" {
				m.searching = false
				m.search.Blur()
				if key == "esc" {
					m.search.SetValue("")
				}
				m.selected = 0
				m.syncDetail(true)
				return m, nil
			}
			m.search, cmd = m.search.Update(msg)
			m.selected = 0
			m.syncDetail(true)
			return m, cmd
		}
		switch key {
		case "q":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "/":
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink
		case "esc":
			m.help = false
			m.detailMode = false
			m.search.SetValue("")
			m.syncDetail(true)
		case "tab":
			m.focus = (m.focus + 1) % 2
		case "shift+tab":
			m.focus = (m.focus + 1) % 2
		case "[", "left":
			m.category = (m.category + len(categories) - 1) % len(categories)
			m.selected = 0
			m.syncDetail(true)
		case "]", "right":
			m.category = (m.category + 1) % len(categories)
			m.selected = 0
			m.syncDetail(true)
		case "a":
			m.showPassed = !m.showPassed
			m.selected = 0
			m.syncDetail(true)
		case "r":
			if !m.loading {
				m.loading = true
				m.err = nil
				return m, tea.Batch(m.spinner.Tick, m.scanCmd())
			}
		case "enter":
			m.focus = 1
			m.detailMode = !m.detailMode
			m.resize()
			m.syncDetail(true)
		case "j", "down":
			if m.focus == 0 {
				m.category = (m.category + 1) % len(categories)
				m.selected = 0
			} else {
				m.selected++
			}
			m.syncDetail(true)
		case "k", "up":
			if m.focus == 0 {
				m.category = (m.category + len(categories) - 1) % len(categories)
				m.selected = 0
			} else {
				m.selected--
			}
			m.syncDetail(true)
		case "pgdown", "ctrl+d":
			m.viewport.HalfViewDown()
		case "pgup", "ctrl+u":
			m.viewport.HalfViewUp()
		case "home":
			m.selected = 0
			m.syncDetail(true)
		case "end":
			m.selected = len(m.filtered()) - 1
			m.syncDetail(true)
		}
	default:
		m.spinner, cmd = m.spinner.Update(msg)
	}
	return m, cmd
}

func (m *Model) resize() {
	_, _, detailW := m.columns()
	m.viewport.Width = max(10, detailW-4)
	m.viewport.Height = max(3, m.height-12)
	m.search.Width = max(8, m.width-8)
}

func (m Model) columns() (int, int, int) {
	if m.width >= 110 {
		return 22, 36, m.width - 58
	}
	if m.width >= 70 {
		if m.detailMode {
			return 20, 0, m.width - 20
		}
		return 20, m.width - 20, m.width - 20
	}
	return 0, m.width, m.width
}

func clip(s string, w int) string { return ansi.Truncate(s, max(0, w), "…") }

func severity(s diagnostic.Severity) lipgloss.Style {
	switch s {
	case diagnostic.Error:
		return lipgloss.NewStyle().Foreground(red)
	case diagnostic.Warning:
		return lipgloss.NewStyle().Foreground(amber)
	case diagnostic.Pass:
		return lipgloss.NewStyle().Foreground(teal)
	default:
		return lipgloss.NewStyle().Foreground(violet)
	}
}
func icon(s diagnostic.Severity) string {
	switch s {
	case diagnostic.Error:
		return "✕"
	case diagnostic.Warning:
		return "!"
	case diagnostic.Pass:
		return "✓"
	default:
		return "·"
	}
}

func (m Model) detail(f diagnostic.Finding) string {
	w := max(10, m.viewport.Width)
	var b strings.Builder
	b.WriteString(severity(f.Severity).Bold(true).Render(strings.ToUpper(string(f.Severity))) + "  " + dim.Render(string(f.Category)) + "\n\n")
	b.WriteString(normal.Bold(true).Render(ansi.Wrap(f.Title, w, "")) + "\n\n")
	b.WriteString(normal.Render(ansi.Wrap(f.Detail, w, "")) + "\n\n")
	if f.File != "" {
		b.WriteString(dim.Render("FILE  "+f.File) + "\n\n")
	}
	if len(f.Repairs) > 0 {
		b.WriteString(accent.Render("REPAIR PLAN") + "\n" + dim.Render("Review before running") + "\n\n")
		for _, r := range f.Repairs {
			b.WriteString(normal.Render(ansi.Wrap(r.Description, w, "")) + "\n")
			if r.Command != "" {
				b.WriteString("\n" + accent.Render(ansi.Hardwrap("$ "+r.Command, w, true)) + "\n")
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString(dim.Render(ansi.Hardwrap("ID  "+f.ID, w, true)))
	return b.String()
}

func panel(s string, w, h int, color lipgloss.Color) string {
	return lipgloss.NewStyle().Background(color).Width(w).Height(h).MaxWidth(w).MaxHeight(h).Padding(1, 2).Render(s)
}

func (m Model) sidebar(w, h int) string {
	var b strings.Builder
	b.WriteString(dim.Render("DIAGNOSTIC MAP") + "\n\n")
	for i, cat := range categories {
		count := 0
		for _, f := range m.report.Findings {
			if (cat.category == "" || f.Category == cat.category) && f.Severity != diagnostic.Pass {
				count++
			}
		}
		label := fmt.Sprintf("%-13s %2d", cat.name, count)
		style := dim
		prefix := "  "
		if i == m.category {
			style = accent
			prefix = "› "
			if m.focus == 0 {
				style = style.Background(line)
			}
		}
		b.WriteString(style.Render(clip(prefix+label, w-4)) + "\n")
	}
	b.WriteString("\n" + dim.Render("──────────────") + "\n\n")
	b.WriteString(accent.Render("LOCAL / PRIVATE") + "\n" + dim.Render("Repairs for review") + "\n" + dim.Render("No telemetry") + "\n\n")
	b.WriteString(dim.Render("[ ] categories\nTab switch focus"))
	return panel(b.String(), w, h, panelBG)
}

func (m Model) issues(w, h int) string {
	items := m.filtered()
	var b strings.Builder
	b.WriteString(accent.Render(strings.ToUpper(categories[m.category].name)) + dim.Render(fmt.Sprintf(" / %d", len(items))) + "\n\n")
	if len(items) == 0 {
		b.WriteString(normal.Render("No active findings.") + "\n\n" + dim.Render("a  show passed checks"))
	}
	visible := max(1, (h-6)/3)
	start := max(0, m.selected-visible+1)
	for i := start; i < len(items) && i < start+visible; i++ {
		f := items[i]
		prefix := "  "
		style := normal
		if i == m.selected {
			prefix = "› "
			style = style.Bold(true).Foreground(fg).Background(line)
		}
		b.WriteString(style.Render(clip(prefix+icon(f.Severity)+" "+f.Title, w-4)) + "\n")
		b.WriteString("    " + severity(f.Severity).Render(string(f.Severity)) + " · " + dim.Render(string(f.Category)) + "\n\n")
	}
	if len(items) > visible {
		b.WriteString(dim.Render(fmt.Sprintf("%d–%d of %d", start+1, min(start+visible, len(items)), len(items))))
	}
	return panel(b.String(), w, h, bg)
}

func (m Model) View() string {
	if m.width < 35 || m.height < 12 {
		return panel("StackDoctor\n\nResize to at least 35 × 12.\nq quits", max(1, m.width), max(1, m.height), bg)
	}
	status := m.report.Summary.Health
	if m.loading {
		status = "scanning"
	}
	if m.err != nil {
		status = "scan failed"
	}
	brand := accent.Render("◈ STACK") + normal.Bold(true).Render("DOCTOR")
	right := dim.Render("v"+m.version+"  /  ") + accent.Render(strings.ToUpper(status))
	header := lipgloss.JoinHorizontal(lipgloss.Top, brand, strings.Repeat(" ", max(1, m.width-4-lipgloss.Width(brand)-lipgloss.Width(right))), right)
	project := m.report.Project
	if project == "" {
		project = "Selected project"
	}
	meta := normal.Render(project) + dim.Render("  /  "+strings.Join(m.report.Stacks, " + "))
	counts := fmt.Sprintf("%d errors   %d warnings   %d passed   %d info", m.report.Summary.Errors, m.report.Summary.Warnings, m.report.Summary.Passed, m.report.Summary.Info)
	top := panel(header+"\n"+clip(meta+"   "+dim.Render(counts), m.width-4), m.width, 4, bg)
	bodyH := max(4, m.height-8)
	sideW, listW, detailW := m.columns()
	var body string
	if m.loading {
		body = panel(m.spinner.View()+"  Inspecting local project configuration…\n\n"+dim.Render("Bounded file reads · version-only probes · review-only repairs"), m.width, bodyH, bg)
	} else if m.err != nil {
		body = panel(severity(diagnostic.Error).Render("SCAN COULD NOT COMPLETE")+"\n\n"+diagnostic.Text(m.err.Error())+"\n\nr rescan · q quit", m.width, bodyH, bg)
	} else if m.help {
		body = panel(accent.Render("KEYBOARD GUIDE")+"\n\nTab          Switch categories / findings\n↑ ↓ or j k   Move through the focused panel\n[ ]          Previous / next category\nEnter        Open or close the issue detail view\n/            Search titles, details, and finding IDs\nEsc          Clear search / close detail or help\na            Include or hide passed checks\nr            Rescan the project\nPgUp/PgDn    Scroll issue details\nq / Ctrl+C   Quit\n\n"+dim.Render("Reports: stackdoctor report --format json\nRepairs are displayed only. No commands are run."), m.width, bodyH, bg)
	} else {
		panels := []string{}
		if sideW > 0 {
			panels = append(panels, m.sidebar(sideW, bodyH))
		}
		if m.width >= 110 {
			panels = append(panels, m.issues(listW, bodyH), panel(accent.Render("ISSUE INSPECTOR")+"\n\n"+m.viewport.View(), detailW, bodyH, panelBG))
		} else if m.detailMode {
			panels = append(panels, panel(accent.Render("ISSUE INSPECTOR")+"\n\n"+m.viewport.View(), detailW, bodyH, bg))
		} else {
			panels = append(panels, m.issues(listW, bodyH))
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, panels...)
	}
	searchLine := dim.Render("/ search   a passed   r rescan   ? help   q quit")
	if m.searching || m.search.Value() != "" {
		searchLine = m.search.View()
	}
	focus := "CATEGORIES"
	if m.focus == 1 {
		focus = "FINDINGS"
	}
	bottom := panel(clip(searchLine, m.width-4)+"\n"+dim.Render(clip(fmt.Sprintf("%s  ·  %s  ·  %dms  ·  no commands executed", focus, categories[m.category].name, m.report.DurationMS), m.width-4)), m.width, 4, panelBG)
	return lipgloss.JoinVertical(lipgloss.Left, top, body, bottom)
}

func Run(ctx context.Context, scan Scanner, version string) error {
	_, err := tea.NewProgram(New(scan, version), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
