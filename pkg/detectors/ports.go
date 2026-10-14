package detectors

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

type Ports struct{}

func (Ports) ID() string { return "ports" }

var scriptPort = regexp.MustCompile(`(?:^|\s)(?:--port(?:=|\s+)|-p\s+)([0-9]{1,5})(?:\s|$)`)
var scriptEnvPort = regexp.MustCompile(`(?:^|\s)PORT=([0-9]{1,5})(?:\s|$)`)

func validPort(raw string) int {
	i, err := strconv.Atoi(raw)
	if err != nil || i < 1 || i > 65535 {
		return 0
	}
	return i
}

func inferredPorts(c *diagnostic.Context) map[int]string {
	ports := map[int]string{}
	for _, p := range c.ExplicitPorts {
		if p >= 1 && p <= 65535 {
			ports[p] = "explicit --port"
		}
	}
	if len(ports) > 0 {
		return ports
	}
	for _, file := range []string{".env", ".env.local"} {
		b, err := c.Files.Read(file)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export ")), "=")
			if !ok || strings.TrimSpace(key) != "PORT" {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			if p := validPort(value); p != 0 {
				ports[p] = "numeric PORT in " + file
			}
		}
	}
	if diagnostic.Exists(c.Files, "package.json") {
		if p, err := readNode(c); err == nil {
			for _, name := range []string{"dev", "start"} {
				for _, pattern := range []*regexp.Regexp{scriptPort, scriptEnvPort} {
					for _, m := range pattern.FindAllStringSubmatch(p.Scripts[name], -1) {
						if n := validPort(m[1]); n != 0 {
							ports[n] = "explicit port in a Node start/dev script"
						}
					}
				}
			}
			if len(ports) == 0 {
				for _, framework := range []struct {
					name string
					port int
				}{{"vite", 5173}, {"next", 3000}, {"react-scripts", 3000}, {"astro", 4321}, {"nuxt", 3000}} {
					if _, ok := p.Dependencies[framework.name]; ok {
						ports[framework.port] = framework.name + " default (inferred)"
					}
					if _, ok := p.DevDependencies[framework.name]; ok {
						ports[framework.port] = framework.name + " default (inferred)"
					}
				}
			}
		}
	}
	return ports
}

func (Ports) Detect(ctx context.Context, c *diagnostic.Context) []diagnostic.Finding {
	if c.SkipPorts {
		return []diagnostic.Finding{finding("ports.skipped", diagnostic.Ports, diagnostic.Info, "Port checks disabled", "No loopback TCP connections were made.", "")}
	}
	ports := inferredPorts(c)
	if len(ports) == 0 {
		return []diagnostic.Finding{finding("ports.unknown", diagnostic.Ports, diagnostic.Info, "No development port declared", "Use --port 3000 to check a known port. Only IPv4 and IPv6 loopback TCP listeners are checked.", "")}
	}
	numbers := []int{}
	for port := range ports {
		numbers = append(numbers, port)
	}
	sort.Ints(numbers)
	f := []diagnostic.Finding{}
	for i, port := range numbers {
		if i >= 16 {
			f = append(f, finding("ports.limit", diagnostic.Ports, diagnostic.Warning, "Port check limit reached", "Only the first 16 ports are checked.", ""))
			break
		}
		listening, err := c.Ports.Listening(ctx, port)
		id := fmt.Sprintf("ports.%d", port)
		if err != nil {
			f = append(f, finding(id, diagnostic.Ports, diagnostic.Warning, fmt.Sprintf("Port %d could not be checked", port), "The loopback probe was cancelled or unavailable.", ""))
			continue
		}
		if listening {
			f = append(f, finding(id, diagnostic.Ports, diagnostic.Warning, fmt.Sprintf("Port %d has a TCP listener", port), "Source: "+ports[port]+". This may be your already-running development server. A different server on this port could prevent startup.", "", repair("Identify the listener locally, then stop it yourself or choose another port. StackDoctor never terminates processes.", "")))
		} else {
			f = append(f, finding(id, diagnostic.Ports, diagnostic.Pass, fmt.Sprintf("Port %d has no loopback TCP listener", port), "Source: "+ports[port]+". This is a point-in-time check; bind permissions, non-loopback listeners, UDP, and future conflicts are not checked.", ""))
		}
	}
	return f
}
