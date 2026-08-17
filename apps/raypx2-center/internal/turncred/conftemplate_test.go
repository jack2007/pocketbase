package turncred

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func parseActiveConf(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func hasLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func TestTurnserverTemplateUDPOnly(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy/coturn/turnserver.conf")
	lines := parseActiveConf(t, path)
	required := []string{
		"listening-port=3478",
		"use-auth-secret",
		"realm=raypx2",
		"no-tcp",
		"no-tls",
		"no-tcp-relay",
		"user-quota=2",
		"no-multicast-peers",
		"denied-peer-ip=127.0.0.0-127.255.255.255",
		"denied-peer-ip=169.254.0.0-169.254.255.255",
		"denied-peer-ip=::1",
		"denied-peer-ip=fe80::-febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
	}
	for _, want := range required {
		if !hasLine(lines, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, banned := range []string{
		"denied-peer-ip=::",
		"denied-peer-ip=0.0.0.0-0.0.0.0",
	} {
		if hasLine(lines, banned) {
			t.Errorf("%s matches every IPv4 peer in coturn 4.17", banned)
		}
	}
	for _, line := range lines {
		if line == "no-udp" || line == "dtls" || strings.HasPrefix(line, "static-auth-secret=") {
			t.Errorf("forbidden active line %q", line)
		}
		if strings.HasPrefix(line, "tls-listening-port=") {
			t.Errorf("forbidden tls listener %q", line)
		}
	}
}
