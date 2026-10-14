package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("failed") }

func TestExports(t *testing.T) {
	r := diagnostic.Report{Project: "<script>*x*", Stacks: []string{"Node.js"}, Findings: []diagnostic.Finding{{ID: "node.missing", Title: "Missing package", Severity: diagnostic.Error, Detail: "Review the project", Repairs: []diagnostic.Repair{{Description: "Install after review", Command: "npm ci"}}}}}
	r.Finalize()
	var j, m bytes.Buffer
	if err := JSON(&j, r); err != nil {
		t.Fatal(err)
	}
	var decoded diagnostic.Report
	if err := json.Unmarshal(j.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if err := Markdown(&m, r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.String(), "<script>") || !strings.Contains(m.String(), "npm ci") {
		t.Fatal("unsafe or incomplete Markdown")
	}
	if JSON(failingWriter{}, r) == nil || Markdown(failingWriter{}, r) == nil {
		t.Fatal("write failure ignored")
	}
}
