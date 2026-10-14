package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, err bytes.Buffer
	code := Run(context.Background(), args, &out, &err, "test")
	return code, out.String(), err.String()
}

func TestHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"check", "--help"}, {"doctor", "--help"}, {"--version"}} {
		code, out, _ := run(t, args...)
		if code != 0 || out == "" {
			t.Fatalf("%v failed", args)
		}
	}
}

func TestFlagValidation(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"check", "--format", "xml"}, {"check", "--port", "0"}, {"check", "--port", "65536"}, {"check", "--timeout", "0s"}, {"check", "--timeout", "3m"}, {"check", "a", "b"}, {"check", "--output", "foo"}, {"check", "--path", "/definitely-not-a-project"}} {
		code, _, _ := run(t, args...)
		if code != 2 {
			t.Errorf("%v code=%d", args, code)
		}
	}
}

func TestJSONAndExitCodes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, err := run(t, "check", root, "--format", "json", "--no-ports")
	if code != 1 || err != "" {
		t.Fatalf("code=%d err=%s", code, err)
	}
	var data map[string]any
	if json.Unmarshal([]byte(out), &data) != nil {
		t.Fatal("invalid JSON")
	}
	if code, _, _ := run(t, "report", root, "--format", "json"); code != 0 {
		t.Fatal("report must succeed despite diagnostic errors")
	}
	if code, out, _ := run(t, "explain", "node.manifest.invalid", "--path", root); code != 0 || !strings.Contains(out, "package.json cannot be parsed") {
		t.Fatal("explain failed")
	}
	if code, _, _ := run(t, "explain", "missing-id", "--path", root); code != 2 {
		t.Fatal("unknown finding accepted")
	}
}

func TestExportNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "report.json")
	code, _, err := run(t, "report", root, "--format", "json", "--output", target)
	if code != 0 {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(target)
	code, _, _ = run(t, "report", root, "--format", "json", "--output", target)
	if code != 2 {
		t.Fatal("existing report overwritten")
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("report changed")
	}
}

func TestStrictWarnings(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("TOKEN=hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run(t, "check", root, "--no-ports"); code != 0 {
		t.Fatal("warning failed default check")
	}
	if code, _, _ := run(t, "check", root, "--strict", "--no-ports"); code != 1 {
		t.Fatal("strict warning did not fail")
	}
}

func TestNormalizeFlags(t *testing.T) {
	want := []string{"--path", "space path", "--format=json", "--", "id"}
	got := normalizeFlags([]string{"id", "--path", "space path", "--format=json"})
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%v", got)
	}
}

func TestSecretSentinelsStayOutOfEveryCLIFormat(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{".env.example": "TOKEN=template-value-secret\nREQUIRED=another-template-secret", ".env": "TOKEN=live-value-secret", "package.json": "{\"private\":\"manifest-value-secret\""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, format := range []string{"text", "json", "markdown"} {
		_, out, err := run(t, "check", root, "--no-ports", "--format", format)
		for _, value := range []string{"template-value-secret", "another-template-secret", "live-value-secret", "manifest-value-secret", root} {
			if strings.Contains(out+err, value) {
				t.Fatalf("%s leaked a private value", format)
			}
		}
	}
}
