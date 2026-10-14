package diagnostic

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestLocalPortsRealListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	ok, err := (LocalPorts{}).Listening(context.Background(), port)
	if err != nil || !ok {
		t.Fatalf("listener not detected: %v %v", ok, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	ok, err = (LocalPorts{}).Listening(context.Background(), port)
	if err != nil || ok {
		t.Fatalf("closed listener detected: %v %v", ok, err)
	}
	if _, err := (LocalPorts{}).Listening(context.Background(), 0); err == nil {
		t.Fatal("invalid port accepted")
	}
}

func TestSafeProbeEnv(t *testing.T) {
	env := probeEnv([]string{"NODE_OPTIONS=--require evil.js", "PYTHONPATH=evil", "GOTOOLCHAIN=auto", "RUSTUP_TOOLCHAIN=nightly", "NPM_CONFIG_USERCONFIG=secret", "PATH=/safe"})
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"evil", "nightly", "secret", "GOTOOLCHAIN=auto"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("unsafe probe environment: %s", bad)
		}
	}
	if !strings.Contains(joined, "GOTOOLCHAIN=local") {
		t.Fatal("toolchain downloading not disabled")
	}
	if !strings.Contains(joined, "RUSTUP_AUTO_INSTALL=0") {
		t.Fatal("Rustup auto-install not disabled")
	}
}

func TestProjectExecutableIsNotRun(t *testing.T) {
	root := t.TempDir()
	name := "node"
	if runtime.GOOS == "windows" {
		name = "node.exe"
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("this must never execute"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE")
	}
	s := &System{ProjectRoot: root}
	tool := s.Probe(context.Background(), "node")
	if tool.Available || tool.Problem != "project-local executables are not run" {
		t.Fatalf("unsafe probe: %+v", tool)
	}
	if s.Probe(context.Background(), "not-allowed").Available {
		t.Fatal("probe allowlist bypassed")
	}
}

func TestWithin(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{{".", true}, {"./child", true}, {"../outside", false}, {"child/../../outside", false}} {
		if Within(".", tc.path) != tc.want {
			t.Errorf("%s", tc.path)
		}
	}
}

func TestCappedOutput(t *testing.T) {
	var b cappedBuffer
	n, err := b.Write([]byte(strings.Repeat("a", 9000)))
	if n != 9000 || err != nil || b.Len() != 8192 {
		t.Fatal("output not bounded")
	}
	if got := parseVersion("node v22.1.0 secret=" + strconv.Itoa(999)); got != "22.1.0" {
		t.Fatal(got)
	}
}

func TestRuntimeVersionFormats(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"v22.1.0\n", "22.1.0"}, {"go version go1.26.9 linux/amd64", "1.26.9"}, {"Python 3.12.4", "3.12.4"}, {"rustc 1.90.0 (abc 2025)", "1.90.0"}, {"10.8.0", "10.8.0"}} {
		if got := parseVersion(tc.raw); got != tc.want {
			t.Errorf("%s: got %s", tc.raw, got)
		}
	}
}
