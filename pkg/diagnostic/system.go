package diagnostic

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Tool struct {
	Name      string
	Available bool
	Version   string
	Problem   string
}

type ToolProvider interface {
	Probe(context.Context, string) Tool
}
type PortProvider interface {
	Listening(context.Context, int) (bool, error)
}

// System probes only a fixed allowlist of external tools. It never runs a shell.
type System struct {
	ProjectRoot string
	mu          sync.Mutex
	cache       map[string]Tool
}

var versionPattern = regexp.MustCompile(`(?:^|[^0-9A-Za-z])(?:v|go)?([0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[a-zA-Z0-9.]+)?)`)

func parseVersion(output string) string {
	if m := versionPattern.FindStringSubmatch(output); len(m) > 1 {
		return m[1]
	}
	return ""
}

var probes = map[string][]string{
	"node": {"--version"}, "npm": {"--version"}, "pnpm": {"--version"}, "yarn": {"--version"}, "bun": {"--version"},
	"python": {"-I", "-S", "--version"}, "python3": {"-I", "-S", "--version"}, "py": {"-3", "-I", "-S", "--version"},
	"go": {"version"}, "rustc": {"--version"}, "cargo": {"--version"}, "uv": {"--version"}, "poetry": {"--version"}, "pipenv": {"--version"},
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 8192 {
		_, _ = b.Buffer.Write(p[:min(len(p), 8192-b.Len())])
	}
	return n, nil
}

func (s *System) Probe(ctx context.Context, name string) Tool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.cache[name]; ok {
		return t
	}
	if s.cache == nil {
		s.cache = make(map[string]Tool)
	}
	t := s.probe(ctx, name)
	s.cache[name] = t
	return t
}

func (s *System) probe(ctx context.Context, name string) Tool {
	t := Tool{Name: name}
	args, ok := probes[name]
	if !ok {
		t.Problem = "tool is not on the safe probe list"
		return t
	}
	path, err := exec.LookPath(name)
	if err != nil {
		t.Problem = "not found on PATH"
		return t
	}
	path, err = filepath.Abs(path)
	if err != nil {
		t.Problem = "cannot resolve executable"
		return t
	}
	// Check both the discovered path and its canonical target. Windows may
	// represent the same directory with an 8.3 short name and a long name.
	if Within(s.ProjectRoot, path) {
		t.Problem = "project-local executables are not run"
		return t
	}
	if s.ProjectRoot != "" {
		root, rootErr := filepath.EvalSymlinks(s.ProjectRoot)
		if rootErr != nil {
			t.Problem = "cannot resolve the selected project boundary"
			return t
		}
		s.ProjectRoot = root
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Problem = "cannot resolve executable"
		return t
	}
	if Within(s.ProjectRoot, path) {
		t.Problem = "project-local executables are not run"
		return t
	}
	// Windows npm/yarn often resolve to .cmd files, which require a shell.
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".cmd" || ext == ".bat" {
		t.Available = true
		t.Problem = "found; version probe skipped for a shell wrapper"
		return t
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, path, args...)
	cmd.Dir = os.TempDir()
	cmd.WaitDelay = 150 * time.Millisecond
	cmd.Env = probeEnv(os.Environ())
	var out cappedBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Problem = "version probe failed"
		if probeCtx.Err() != nil {
			t.Problem = "version probe timed out or was cancelled"
		}
		return t
	}
	t.Available = true
	t.Version = parseVersion(out.String())
	if t.Version == "" {
		t.Problem = "version could not be determined"
	}
	return t
}

func probeEnv(env []string) []string {
	out := make([]string, 0, len(env)+5)
	for _, item := range env {
		key := strings.ToUpper(strings.SplitN(item, "=", 2)[0])
		if key == "NODE_OPTIONS" || key == "NODE_PATH" || strings.HasPrefix(key, "PYTHON") || strings.HasPrefix(key, "GO") || key == "RUSTUP_TOOLCHAIN" || key == "RUSTUP_AUTO_INSTALL" || strings.HasPrefix(key, "NPM_CONFIG_") || strings.HasPrefix(key, "YARN_") || key == "COREPACK_ENABLE_NETWORK" {
			continue
		}
		out = append(out, item)
	}
	return append(out, "GOTOOLCHAIN=local", "GOWORK=off", "GOENV=off", "COREPACK_ENABLE_NETWORK=0", "RUSTUP_AUTO_INSTALL=0", "NO_UPDATE_NOTIFIER=1")
}

func Within(root, path string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

type LocalPorts struct{}

// Listening makes only loopback TCP connections. It does not bind ports, kill
// processes, inspect process arguments, or connect to project-configured hosts.
func (LocalPorts) Listening(ctx context.Context, port int) (bool, error) {
	if port < 1 || port > 65535 {
		return false, errors.New("invalid port")
	}
	for _, host := range []string{"127.0.0.1", "::1"} {
		conn, err := (&net.Dialer{Timeout: 120 * time.Millisecond}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			_ = conn.Close()
			return true, nil
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// A refused/unreachable loopback connection is not evidence of a conflict.
	}
	return false, nil
}
