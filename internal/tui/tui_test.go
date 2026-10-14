package tui

import (
	"strings"
	"testing"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func sample() diagnostic.Report {
	r := diagnostic.Report{Project: "demo", Stacks: []string{"Node.js"}, Version: "test", Findings: []diagnostic.Finding{
		{ID: "node.missing", Category: diagnostic.Dependencies, Severity: diagnostic.Error, Title: "Missing packages", Detail: strings.Repeat("A long explanation that must wrap in narrow terminal layouts. ", 20), Repairs: []diagnostic.Repair{{Description: "Review", Command: "npm ci"}}},
		{ID: "env.file", Category: diagnostic.Environment, Severity: diagnostic.Warning, Title: "Missing environment file"},
		{ID: "node.runtime", Category: diagnostic.Runtime, Severity: diagnostic.Pass, Title: "Runtime found"},
	}}
	r.Finalize()
	return r
}

func ready(t *testing.T, width, height int) Model {
	t.Helper()
	m := New(func() (diagnostic.Report, error) { return sample(), nil }, "test")
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = next.(Model)
	next, _ = m.Update(scanMsg{sample(), nil})
	return next.(Model)
}

func key(m Model, k string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	switch k {
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "pgdown":
		msg = tea.KeyMsg{Type: tea.KeyPgDown}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestNavigationSearchAndPassedChecks(t *testing.T) {
	m := ready(t, 120, 30)
	if len(m.filtered()) != 2 {
		t.Fatal("passed checks must initially be hidden")
	}
	m = key(m, "tab")
	m = key(m, "down")
	if m.selected != 1 {
		t.Fatal("issue navigation failed")
	}
	m = key(m, "a")
	if len(m.filtered()) != 3 {
		t.Fatal("passed toggle failed")
	}
	m = key(m, "/")
	m = key(m, "Missing packages")
	if len(m.filtered()) != 1 {
		t.Fatal("search failed")
	}
	m = key(m, "esc")
	if m.searching || m.search.Value() != "" {
		t.Fatal("search reset failed")
	}
	m = key(m, "]")
	if m.category != 1 {
		t.Fatal("category navigation failed")
	}
}

func TestResponsiveBounds(t *testing.T) {
	for _, size := range []struct{ w, h int }{{140, 40}, {110, 28}, {90, 24}, {70, 20}, {55, 18}, {35, 12}, {20, 8}} {
		m := ready(t, size.w, size.h)
		for _, state := range []string{"list", "detail", "help"} {
			if state == "detail" {
				m = key(m, "enter")
			}
			if state == "help" {
				m = key(m, "?")
			}
			v := m.View()
			if lipgloss.Width(v) > size.w || lipgloss.Height(v) > size.h {
				t.Errorf("%dx%d %s: rendered %dx%d", size.w, size.h, state, lipgloss.Width(v), lipgloss.Height(v))
			}
		}
	}
}

func TestRescanAndQuit(t *testing.T) {
	m := ready(t, 120, 30)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !next.(Model).loading || cmd == nil {
		t.Fatal("rescan did not start")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("quit missing")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit did not emit QuitMsg")
	}
}

func TestDetailViewportScrolls(t *testing.T) {
	m := ready(t, 120, 25)
	m = key(m, "pgdown")
	if m.viewport.YOffset == 0 {
		t.Fatal("long detail does not scroll")
	}
}
