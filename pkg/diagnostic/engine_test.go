package diagnostic

import (
	"context"
	"testing"
)

type stubDetector struct{ id string }

func (s stubDetector) ID() string { return s.id }
func (s stubDetector) Detect(context.Context, *Context) []Finding {
	return []Finding{{ID: "stub.warning", Category: Configuration, Severity: Warning, Title: "safe\x1b[2J", Detail: "test"}}
}

func TestRegistryAndScan(t *testing.T) {
	r := &Registry{}
	if err := r.Register(stubDetector{"stub"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(stubDetector{"stub"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := r.Register(nil); err == nil {
		t.Fatal("nil accepted")
	}
	report, err := r.Scan(context.Background(), Options{Path: t.TempDir(), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Warnings != 1 || report.Summary.Health != "needs attention" {
		t.Fatalf("summary: %+v", report.Summary)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Scan(ctx, Options{Path: t.TempDir()}); err == nil {
		t.Fatal("cancelled scan succeeded")
	}
}

func TestHealthReflectsCounts(t *testing.T) {
	for _, tc := range []struct {
		severity Severity
		health   string
	}{{Pass, "clear"}, {Info, "clear"}, {Warning, "needs attention"}, {Error, "blocked"}} {
		r := Report{Stacks: []string{"Node.js"}, Findings: []Finding{{Severity: tc.severity}}}
		r.Finalize()
		if r.Summary.Health != tc.health {
			t.Fatalf("%s: %s", tc.severity, r.Summary.Health)
		}
	}
	r := Report{}
	r.Finalize()
	if r.Summary.Health != "unknown" {
		t.Fatal("unsupported project must be unknown")
	}
}
