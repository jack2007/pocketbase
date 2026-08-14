# coturn third_party 同机集成 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 pocketbase 钉住 `jack2007/coturn`，独立编出 `turnserver`，并落地与 raypx2-center 同机、独立进程的 UDP-only TURN REST 契约。

**Architecture:** submodule 只存在于 pocketbase `third_party/coturn`。`go build` 不编译 coturn。CMake 脚本产出 `build-coturn/bin/turnserver`。`deploy/coturn` 提供配置、secret wrapper 与独立 systemd unit。`internal/turncred` 实现 secret 权限校验、TURN URL 契约和 REST username/password；完整签发 API 仍属 ICE 阶段，本计划不把它挂进 `serve`。

**Tech Stack:** git submodule、CMake、coturn `turnserver`、Go 1.25、systemd；Linux。

**Spec:** [docs/superpowers/specs/2026-08-14-coturn-third-party-integration-design.md](../specs/2026-08-14-coturn-third-party-integration-design.md)

## Global Constraints

- 只在 pocketbase 钉一份 `https://github.com/jack2007/coturn`；gitlink 固定为 `7c24c88a4c13ef79edce9e645bef578eb7e5a6ad`。
- `go build` / `go test ./apps/raypx2-center/...` 不依赖 submodule 已初始化。
- Center 不 spawn `turnserver`；两个 systemd unit 不得 `Requires=` / `PartOf=` 互相绑定。
- 只启用 UDP：`no-tcp`、`no-tls`、`no-tcp-relay`；不启 `dtls`；禁止设置 `no-udp`。
- 生产 URL 为公网 `stun:host:3478` 与 `turn:host:3478?transport=udp`；禁止对远端 Agent 下发 `127.0.0.1` / `::1`。
- secret 单文件默认 `/run/secrets/coturn-rest-secret`，mode `0640`；others 可读则拒绝。
- `username = "{expiry_unix}:{session_id}_{connection_id}_{epoch}"`；`password = base64(hmac-sha1(shared_secret, username))`。
- `realm=raypx2`；`user-quota=2`；`listening-port=3478`。
- 不实现 ICE 信令、grant、libjuice、MsQuic patch。
- 提交信息使用 `feat(center):` / `docs(center):` / `test(center):` 前缀。

## File Structure

| Path | Responsibility |
| --- | --- |
| `third_party/coturn/` | `jack2007/coturn` submodule |
| `.gitmodules` | submodule URL |
| `.gitignore` | 忽略 `build-coturn/` |
| `scripts/build-coturn.sh` | 唯一官方 CMake 构建入口 |
| `deploy/coturn/turnserver.conf` | UDP-only + REST 模板，不含 secret |
| `deploy/coturn/turnserver-start.sh` | 读 secret 文件后 exec `turnserver` |
| `deploy/coturn/coturn.service` | 独立 systemd unit |
| `deploy/coturn/README.md` | 部署、防火墙、Refresh 结论 |
| `apps/raypx2-center/internal/turncred/config.go` | URL / realm 校验与 `EvaluateTURN` |
| `apps/raypx2-center/internal/turncred/secret.go` | secret 文件权限与读取 |
| `apps/raypx2-center/internal/turncred/rest.go` | REST username/password |
| `apps/raypx2-center/internal/turncred/*_test.go` | 契约、模板审计、REST 向量 |
| `apps/raypx2-center/internal/turncred/refresh_compat_test.go` | `//go:build coturn` Refresh 门禁 |

---

### Task 1: 钉住 coturn submodule 并保持 Go 隔离

**Files:**
- Create: `.gitmodules`
- Create: `third_party/coturn/`（gitlink）
- Modify: `.gitignore`
- Modify: `docs/superpowers/specs/2026-08-14-coturn-third-party-integration-design.md`（header 已含 plan 链接则跳过）

**Interfaces:**
- Consumes: 无
- Produces: submodule 路径 `third_party/coturn`，commit `7c24c88a4c13ef79edce9e645bef578eb7e5a6ad`，远程 `https://github.com/jack2007/coturn.git`

- [ ] **Step 1: 确认未初始化 submodule 时 Center 测试已通过**

Run:

```bash
go test ./apps/raypx2-center/...
```

Expected: PASS（现有测试，不依赖 coturn）

- [ ] **Step 2: 添加 submodule 并钉 SHA**

```bash
git submodule add https://github.com/jack2007/coturn.git third_party/coturn
git -C third_party/coturn checkout --detach 7c24c88a4c13ef79edce9e645bef578eb7e5a6ad
```

Expected: `third_party/coturn/CMakeLists.txt` 与 `third_party/coturn/LICENSE` 存在；`git -C third_party/coturn rev-parse HEAD` 输出 `7c24c88a4c13ef79edce9e645bef578eb7e5a6ad`。

`.gitmodules` 必须是：

```ini
[submodule "third_party/coturn"]
	path = third_party/coturn
	url = https://github.com/jack2007/coturn.git
```

- [ ] **Step 3: 忽略构建目录**

在 `.gitignore` 末尾追加：

```gitignore

# coturn CMake build (scripts/build-coturn.sh)
/build-coturn/
```

- [ ] **Step 4: 再次运行 Go 测试，确认 submodule 不进入 Go 构建**

Run:

```bash
go test ./apps/raypx2-center/...
```

Expected: PASS。`go list -deps ./apps/raypx2-center` 的输出不得包含 `third_party/coturn`。

- [ ] **Step 5: Commit**

```bash
git add .gitmodules third_party/coturn .gitignore docs/superpowers/specs/2026-08-14-coturn-third-party-integration-design.md docs/superpowers/plans/2026-08-14-coturn-third-party-integration.md
git commit -m "$(cat <<'EOF'
feat(center): pin jack2007/coturn as third_party submodule

EOF
)"
```

---

### Task 2: 独立构建 `turnserver`

**Files:**
- Create: `scripts/build-coturn.sh`

**Interfaces:**
- Consumes: `third_party/coturn/CMakeLists.txt`（Task 1）
- Produces: 可执行文件 `build-coturn/bin/turnserver`；脚本在 submodule 缺失时以非 0 退出并打印 `git submodule update --init third_party/coturn`

- [ ] **Step 1: 写构建脚本**

Create `scripts/build-coturn.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${ROOT}/third_party/coturn"
BUILD="${ROOT}/build-coturn"

if [[ ! -f "${SRC}/CMakeLists.txt" ]]; then
  echo "coturn submodule missing. Run: git submodule update --init third_party/coturn" >&2
  exit 1
fi

cmake -S "${SRC}" -B "${BUILD}" \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_TESTING=OFF \
  -DFUZZER=OFF \
  -DWITH_MYSQL=OFF
cmake --build "${BUILD}" --target turnserver --parallel

BIN="${BUILD}/bin/turnserver"
if [[ ! -x "${BIN}" ]]; then
  echo "turnserver binary missing: ${BIN}" >&2
  exit 1
fi

echo "${BIN}"
```

```bash
chmod +x scripts/build-coturn.sh
```

- [ ] **Step 2: 验证 submodule 探测失败路径**

Run:

```bash
bash -c 'SRC=third_party/coturn/CMakeLists.txt; mv "$SRC" "$SRC.bak"; ./scripts/build-coturn.sh; status=$?; mv "$SRC.bak" "$SRC"; exit $status'
```

Expected: 退出码 1，stderr 含 `git submodule update --init third_party/coturn`。

- [ ] **Step 3: 完整构建并冒烟**

若缺依赖，先安装：`sudo apt-get install -y cmake libevent-dev libmicrohttpd-dev libssl-dev pkg-config`。

Run:

```bash
./scripts/build-coturn.sh
"$(./scripts/build-coturn.sh)" --help | head
```

Expected: 脚本打印 `.../build-coturn/bin/turnserver`；`--help` 退出 0，stdout 含 `turnserver`。

- [ ] **Step 4: Commit**

```bash
git add scripts/build-coturn.sh
git commit -m "$(cat <<'EOF'
feat(center): add standalone coturn CMake build script

EOF
)"
```

---

### Task 3: UDP-only 部署模板与配置审计

**Files:**
- Create: `deploy/coturn/turnserver.conf`
- Create: `deploy/coturn/turnserver-start.sh`
- Create: `deploy/coturn/coturn.service`
- Create: `deploy/coturn/README.md`
- Create: `apps/raypx2-center/internal/turncred/conftemplate_test.go`

**Interfaces:**
- Consumes: 无
- Produces: 提交的四份 `deploy/coturn/*` 文件；测试包 `turncred` 通过读仓库根下 `deploy/coturn/turnserver.conf` 断言必选/禁选指令

- [ ] **Step 1: 写失败的配置审计测试**

Create `apps/raypx2-center/internal/turncred/conftemplate_test.go`:

```go
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
		"denied-peer-ip=0.0.0.0-0.0.0.0",
		"denied-peer-ip=127.0.0.0-127.255.255.255",
		"denied-peer-ip=169.254.0.0-169.254.255.255",
		"denied-peer-ip=::",
		"denied-peer-ip=::1",
	}
	for _, want := range required {
		if !hasLine(lines, want) {
			t.Errorf("missing %q", want)
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
```

- [ ] **Step 2: 运行测试，确认因缺少模板而失败**

Run:

```bash
go test ./apps/raypx2-center/internal/turncred -run TestTurnserverTemplateUDPOnly -v
```

Expected: FAIL，`open .../deploy/coturn/turnserver.conf: no such file or directory` 或 `missing "listening-port=3478"`。

- [ ] **Step 3: 写入模板、wrapper、unit 与 README**

Create `deploy/coturn/turnserver.conf`:

```conf
# raypx2 coturn UDP-only + TURN REST template.
# Do not put static-auth-secret in this file. The unit wrapper passes
# --static-auth-secret from /run/secrets/coturn-rest-secret.
#
# Fill listening-ip / relay-ip / external-ip on the target host.
# Set external-ip when the host has only a private address behind 1:1 NAT.

listening-port=3478
#listening-ip=0.0.0.0
#relay-ip=0.0.0.0
#external-ip=203.0.113.10

use-auth-secret
realm=raypx2

no-tcp
no-tls
no-tcp-relay

user-quota=2
#total-quota=0
#bps-capacity=0
#min-port=49152
#max-port=65535

no-multicast-peers
denied-peer-ip=0.0.0.0-0.0.0.0
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=::
denied-peer-ip=::1

# CLI stays disabled: do not set cli-password.
# Do not enable web-admin or prometheus on a public address.
```

Create `deploy/coturn/turnserver-start.sh`:

```bash
#!/bin/sh
set -eu

SECRET_FILE="${COTURN_REST_SECRET_FILE:-/run/secrets/coturn-rest-secret}"
CONF="${COTURN_CONFIG:-/etc/raypx2/turnserver.conf}"
TURNSERVER_BIN="${TURNSERVER_BIN:-/usr/local/bin/turnserver}"

if [ ! -f "$SECRET_FILE" ]; then
  echo "coturn secret file missing: $SECRET_FILE" >&2
  exit 1
fi

MODE=$(stat -c '%a' "$SECRET_FILE")
OTHERS="${MODE#"${MODE%?}"}"
if [ "$OTHERS" -ge 4 ]; then
  echo "coturn secret file is world-readable: $SECRET_FILE mode $MODE" >&2
  exit 1
fi

if [ ! -f "$CONF" ]; then
  echo "coturn config missing: $CONF" >&2
  exit 1
fi

SECRET=$(tr -d '\n' < "$SECRET_FILE")
if [ -z "$SECRET" ]; then
  echo "coturn secret file is empty: $SECRET_FILE" >&2
  exit 1
fi

exec "$TURNSERVER_BIN" -c "$CONF" --static-auth-secret="$SECRET"
```

```bash
chmod +x deploy/coturn/turnserver-start.sh
```

Create `deploy/coturn/coturn.service`:

```ini
[Unit]
Description=raypx2 coturn STUN/TURN (UDP)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=turnserver
Group=turnserver
ExecStart=/usr/local/libexec/raypx2/turnserver-start.sh
Restart=on-failure
RestartSec=2
LimitNOFILE=65536
AmbientCapabilities=CAP_NET_BIND_SERVICE
# Do not add Requires= or PartOf= for raypx2-center.service.

[Install]
WantedBy=multi-user.target
```

Create `deploy/coturn/README.md`:

```markdown
# raypx2 coturn (same host as Center)

Center is signaling only. coturn is a separate STUN/TURN process on the
same machine. Do not start `turnserver` from the Center process.

## Build

From the pocketbase repository root:

```bash
git submodule update --init third_party/coturn
./scripts/build-coturn.sh
```

Install `build-coturn/bin/turnserver` to `/usr/local/bin/turnserver`.
Install `deploy/coturn/turnserver-start.sh` to
`/usr/local/libexec/raypx2/turnserver-start.sh`.
Install `deploy/coturn/coturn.service` to
`/etc/systemd/system/coturn.service`.
Copy `deploy/coturn/turnserver.conf` to `/etc/raypx2/turnserver.conf`
and set `listening-ip` / `relay-ip` / `external-ip`.

## Secret

Create `/run/secrets/coturn-rest-secret` (or a persistent path bind-mounted
there) with mode `0640`, owner `root`, group readable by both `turnserver`
and the Center service user. Never commit the secret. Never put
`static-auth-secret` in the conf file.

## URLs issued to remote agents

Use the server public DNS or public IP:

```text
stun:turn.example.com:3478
turn:turn.example.com:3478?transport=udp
```

Do not issue `127.0.0.1` or `::1` to remote agents. Same host means Center
and coturn share a machine, not that agents connect to loopback.

## Firewall

Allow inbound UDP `3478` and the relay range (`49152-65535/udp` unless
`min-port`/`max-port` are narrowed). Do not open TCP 3478 or TLS 5349.

## Refresh compatibility

Record the result for gitlink `7c24c88a4c13ef79edce9e645bef578eb7e5a6ad`
after Task 5:

```text
(pending Task 5)
```
```

- [ ] **Step 4: 运行配置审计测试**

Run:

```bash
go test ./apps/raypx2-center/internal/turncred -run TestTurnserverTemplateUDPOnly -v
```

Expected: PASS

- [ ] **Step 5: 用模板启动并确认只有 UDP 3478（本机回环、高权限不需要则用 3478）**

```bash
SECRET=$(mktemp)
CONF=$(mktemp)
chmod 640 "$SECRET"
printf 'test-rest-secret' > "$SECRET"
sed 's/^#listening-ip=0.0.0.0/listening-ip=127.0.0.1/' deploy/coturn/turnserver.conf > "$CONF"
COTURN_REST_SECRET_FILE="$SECRET" COTURN_CONFIG="$CONF" TURNSERVER_BIN="$PWD/build-coturn/bin/turnserver" \
  deploy/coturn/turnserver-start.sh &
pid=$!
sleep 1
ss -ulnp | grep -E '3478' || true
ss -tlnp | grep -E '3478|5349' || true
kill "$pid"
wait "$pid" || true
rm -f "$SECRET" "$CONF"
```

Expected: `ss -ulnp` 出现 `127.0.0.1:3478` 或 `*:3478`；`ss -tlnp` 无 3478/5349。若 `CAP_NET_BIND_SERVICE` 不足导致 bind 失败，改用 root 重跑同一命令，或把临时 conf 的 `listening-port` 换成 `34780` 后再确认 UDP 监听存在、TCP 不存在。

- [ ] **Step 6: Commit**

```bash
git add deploy/coturn apps/raypx2-center/internal/turncred/conftemplate_test.go
git commit -m "$(cat <<'EOF'
feat(center): add UDP-only coturn deploy templates

EOF
)"
```

---

### Task 4: secret 文件与 TURN URL 契约

**Files:**
- Create: `apps/raypx2-center/internal/turncred/config.go`
- Create: `apps/raypx2-center/internal/turncred/secret.go`
- Create: `apps/raypx2-center/internal/turncred/config_test.go`
- Create: `apps/raypx2-center/internal/turncred/secret_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `const DefaultRealm = "raypx2"`
  - `const DefaultSecretFile = "/run/secrets/coturn-rest-secret"`
  - `type Config struct { STUNURLs []string; TURNURLs []string; SharedSecretFile string; CredentialTTLSeconds int; Realm string }`
  - `func ValidateConfig(cfg Config) error` — TURN URL 缺少 `transport=udp` 则返回 error；STUN/TURN 主机为 `127.0.0.1` 或 `::1` 则返回 error
  - `func LoadSharedSecret(path string) ([]byte, error)` — 文件不存在、为空、others 可读则 error
  - `func EvaluateTURN(cfg Config) (enabled bool, secret []byte, reason string, err error)` — `ValidateConfig` 失败时 `err != nil`；secret 不可用时 `enabled==false` 且 `err==nil`

- [ ] **Step 1: 写失败测试**

Create `apps/raypx2-center/internal/turncred/config_test.go`:

```go
package turncred

import "testing"

func TestValidateConfigRequiresUDPTransport(t *testing.T) {
	err := ValidateConfig(Config{
		TURNURLs: []string{"turn:turn.example.com:3478"},
	})
	if err == nil {
		t.Fatal("expected error for TURN URL without transport=udp")
	}
}

func TestValidateConfigAcceptsUDPTurnAndPublicSTUN(t *testing.T) {
	err := ValidateConfig(Config{
		STUNURLs: []string{"stun:turn.example.com:3478"},
		TURNURLs: []string{"turn:turn.example.com:3478?transport=udp"},
		Realm:    DefaultRealm,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateConfigRejectsLoopbackHost(t *testing.T) {
	err := ValidateConfig(Config{
		TURNURLs: []string{"turn:127.0.0.1:3478?transport=udp"},
	})
	if err == nil {
		t.Fatal("expected error for loopback TURN host")
	}
}

func TestEvaluateTURNDisablesWhenSecretMissing(t *testing.T) {
	enabled, secret, reason, err := EvaluateTURN(Config{
		TURNURLs:         []string{"turn:turn.example.com:3478?transport=udp"},
		SharedSecretFile: "/no/such/coturn-rest-secret",
	})
	if err != nil {
		t.Fatalf("missing secret must not be fatal: %v", err)
	}
	if enabled || secret != nil || reason == "" {
		t.Fatalf("enabled=%v secret=%v reason=%q", enabled, secret, reason)
	}
}
```

Create `apps/raypx2-center/internal/turncred/secret_test.go`:

```go
package turncred

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSharedSecretRejectsWorldReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("shared-secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSharedSecret(path); err == nil {
		t.Fatal("expected error for 0644 secret")
	}
}

func TestLoadSharedSecretAccepts0640(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("shared-secret\n"), 0640); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSharedSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "shared-secret" {
		t.Fatalf("got %q", got)
	}
}

func TestEvaluateTURNEnablesWithValidSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("shared-secret"), 0640); err != nil {
		t.Fatal(err)
	}
	enabled, secret, reason, err := EvaluateTURN(Config{
		TURNURLs:         []string{"turn:turn.example.com:3478?transport=udp"},
		SharedSecretFile: path,
	})
	if err != nil || !enabled || string(secret) != "shared-secret" || reason != "" {
		t.Fatalf("enabled=%v secret=%q reason=%q err=%v", enabled, secret, reason, err)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./apps/raypx2-center/internal/turncred -count=1
```

Expected: FAIL，`undefined: ValidateConfig` / `LoadSharedSecret` / `EvaluateTURN` / `DefaultRealm`。

- [ ] **Step 3: 写最小实现**

Create `apps/raypx2-center/internal/turncred/config.go`:

```go
package turncred

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultRealm      = "raypx2"
	DefaultSecretFile = "/run/secrets/coturn-rest-secret"
)

type Config struct {
	STUNURLs             []string
	TURNURLs             []string
	SharedSecretFile     string
	CredentialTTLSeconds int
	Realm                string
}

func ValidateConfig(cfg Config) error {
	for _, raw := range cfg.STUNURLs {
		if err := validateURI(raw, "stun", false); err != nil {
			return err
		}
	}
	for _, raw := range cfg.TURNURLs {
		if err := validateURI(raw, "turn", true); err != nil {
			return err
		}
	}
	return nil
}

func EvaluateTURN(cfg Config) (enabled bool, secret []byte, reason string, err error) {
	if err := ValidateConfig(cfg); err != nil {
		return false, nil, "", err
	}
	path := cfg.SharedSecretFile
	if path == "" {
		return false, nil, "shared_secret_file empty", nil
	}
	secret, err = LoadSharedSecret(path)
	if err != nil {
		return false, nil, err.Error(), nil
	}
	return true, secret, "", nil
}

func validateURI(raw, wantScheme string, requireUDP bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid %s URL %q: %w", wantScheme, raw, err)
	}
	if !strings.EqualFold(u.Scheme, wantScheme) {
		return fmt.Errorf("URL %q must use scheme %s", raw, wantScheme)
	}
	host := u.Hostname()
	if host == "" || host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return fmt.Errorf("URL %q must use a public host, not loopback", raw)
	}
	if requireUDP {
		if u.Query().Get("transport") != "udp" {
			return fmt.Errorf("TURN URL %q must include transport=udp", raw)
		}
	}
	return nil
}
```

Create `apps/raypx2-center/internal/turncred/secret.go`:

```go
package turncred

import (
	"fmt"
	"os"
	"strings"
)

func LoadSharedSecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o004 != 0 {
		return nil, fmt.Errorf("secret file %s is world-readable (mode %04o)", path, info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	secret := strings.TrimRight(string(raw), "\n")
	if secret == "" {
		return nil, fmt.Errorf("secret file %s is empty", path)
	}
	return []byte(secret), nil
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run:

```bash
go test ./apps/raypx2-center/internal/turncred -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/internal/turncred/config.go apps/raypx2-center/internal/turncred/secret.go apps/raypx2-center/internal/turncred/config_test.go apps/raypx2-center/internal/turncred/secret_test.go
git commit -m "$(cat <<'EOF'
feat(center): add coturn secret and TURN URL contract helpers

EOF
)"
```

---

### Task 5: REST 凭据向量与 Refresh 兼容记录

**Files:**
- Create: `apps/raypx2-center/internal/turncred/rest.go`
- Create: `apps/raypx2-center/internal/turncred/rest_test.go`
- Create: `apps/raypx2-center/internal/turncred/refresh_compat_test.go`
- Modify: `deploy/coturn/README.md`（Refresh 结论段落）

**Interfaces:**
- Consumes: Task 4 的 `LoadSharedSecret` / `EvaluateTURN`
- Produces:
  - `func RESTUsername(expiryUnix int64, sessionID, connectionID string, epoch uint64) string` → `"{expiryUnix}:{sessionID}_{connectionID}_{epoch}"`
  - `func RESTPassword(secret []byte, username string) string` → `base64.StdEncoding.EncodeToString(hmac-sha1(secret, username))`
  - `func IssueREST(secret []byte, sessionID, connectionID string, epoch uint64, nowUnix, ttlSeconds int64) (username, password string)`
  - Refresh 测试：`go test -tags coturn ./apps/raypx2-center/internal/turncred -run TestRefreshAfterExpiry`

- [ ] **Step 1: 写失败的 REST 向量测试**

Create `apps/raypx2-center/internal/turncred/rest_test.go`:

```go
package turncred

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRESTUsernameFormat(t *testing.T) {
	got := RESTUsername(1710000000, "sessA", "conn-7", 2)
	if got != "1710000000:sessA_conn-7_2" {
		t.Fatalf("got %q", got)
	}
	if strings.Count(got, ":") != 1 {
		t.Fatalf("userid must not contain extra colons: %q", got)
	}
}

func TestRESTPasswordVector(t *testing.T) {
	username := RESTUsername(1710000000, "sessA", "conn-7", 2)
	got := RESTPassword([]byte("shared-secret"), username)
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil || len(decoded) != 20 {
		t.Fatalf("password %q is not 20-byte base64 hmac: %v", got, err)
	}
	if got != RESTPassword([]byte("shared-secret"), username) {
		t.Fatal("password must be deterministic")
	}
	user, pass := IssueREST([]byte("shared-secret"), "sessA", "conn-7", 2, 1710000000-86400, 86400)
	if user != username || pass != got {
		t.Fatalf("IssueREST=%q,%q", user, pass)
	}
}
```

- [ ] **Step 2: 运行 REST 测试，确认失败**

Run:

```bash
go test ./apps/raypx2-center/internal/turncred -run 'TestREST' -count=1
```

Expected: FAIL，`undefined: RESTUsername`

- [ ] **Step 3: 写 REST 实现**

Create `apps/raypx2-center/internal/turncred/rest.go`:

```go
package turncred

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
)

func RESTUsername(expiryUnix int64, sessionID, connectionID string, epoch uint64) string {
	return fmt.Sprintf("%d:%s_%s_%d", expiryUnix, sessionID, connectionID, epoch)
}

func RESTPassword(secret []byte, username string) string {
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func IssueREST(secret []byte, sessionID, connectionID string, epoch uint64, nowUnix, ttlSeconds int64) (username, password string) {
	username = RESTUsername(nowUnix+ttlSeconds, sessionID, connectionID, epoch)
	password = RESTPassword(secret, username)
	return username, password
}
```

- [ ] **Step 4: 运行 REST 测试，确认通过**

Run:

```bash
go test ./apps/raypx2-center/internal/turncred -run 'TestREST' -count=1
```

Expected: PASS

- [ ] **Step 5: 写 Refresh 兼容测试**

Create `apps/raypx2-center/internal/turncred/refresh_compat_test.go`:

```go
//go:build coturn

package turncred

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	stunMagic          = 0x2112A442
	stunAllocate       = 0x0003
	stunRefresh        = 0x0004
	stunAttrUsername   = 0x0006
	stunAttrErrorCode  = 0x0009
	stunAttrRealm      = 0x0014
	stunAttrNonce      = 0x0015
	stunAttrLifetime   = 0x000D
	stunAttrIntegrity  = 0x0008
	stunAttrReqTrans   = 0x0019
)

func TestRefreshAfterExpiry(t *testing.T) {
	root := repoRoot(t)
	bin := os.Getenv("TURNSERVER_BIN")
	if bin == "" {
		bin = filepath.Join(root, "build-coturn", "bin", "turnserver")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("turnserver not built; run ./scripts/build-coturn.sh")
	}

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "secret")
	confPath := filepath.Join(dir, "turnserver.conf")
	if err := os.WriteFile(secretPath, []byte("compat-secret"), 0640); err != nil {
		t.Fatal(err)
	}
	conf := `listening-ip=127.0.0.1
listening-port=34780
use-auth-secret
realm=raypx2
no-tcp
no-tls
no-tcp-relay
user-quota=2
`
	if err := os.WriteFile(confPath, []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "-c", confPath, "--static-auth-secret=compat-secret")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	time.Sleep(400 * time.Millisecond)

	expiry := time.Now().Add(2 * time.Second).Unix()
	user := RESTUsername(expiry, "sessR", "conn-1", 1)
	pass := RESTPassword([]byte("compat-secret"), user)

	conn, err := net.Dial("udp", "127.0.0.1:34780")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	realm, nonce := mustAllocateUnauth(t, conn)
	realm, nonce, err = allocateAuth(t, conn, user, pass, realm, nonce)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	time.Sleep(3 * time.Second)
	err = refreshAuth(t, conn, user, pass, realm, nonce)
	if err != nil {
		t.Logf("REFRESH_COMPAT=fail sha=7c24c88a4c13ef79edce9e645bef578eb7e5a6ad err=%v", err)
		t.Fatalf("authenticated Refresh after expiry failed: %v", err)
	}
	t.Log("REFRESH_COMPAT=ok sha=7c24c88a4c13ef79edce9e645bef578eb7e5a6ad")
}

func mustAllocateUnauth(t *testing.T, conn net.Conn) (realm, nonce string) {
	t.Helper()
	msg := stunRequest(stunAllocate, []stunAttr{{stunAttrReqTrans, []byte{17, 0, 0, 0}}})
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	resp := readSTUN(t, conn)
	code := stunErrorCode(resp)
	if code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
	return string(stunAttr(resp, stunAttrRealm)), string(stunAttr(resp, stunAttrNonce))
}

func allocateAuth(t *testing.T, conn net.Conn, user, pass, realm, nonce string) (string, string, error) {
	t.Helper()
	for attempt := 0; attempt < 2; attempt++ {
		attrs := []stunAttr{
			{stunAttrReqTrans, []byte{17, 0, 0, 0}},
			{stunAttrUsername, []byte(user)},
			{stunAttrRealm, []byte(realm)},
			{stunAttrNonce, []byte(nonce)},
		}
		msg := stunRequestWithIntegrity(stunAllocate, attrs, user, realm, pass)
		if _, err := conn.Write(msg); err != nil {
			return realm, nonce, err
		}
		resp := readSTUN(t, conn)
		code := stunErrorCode(resp)
		if code == 0 {
			return realm, nonce, nil
		}
		if code == 438 {
			realm, nonce = string(stunAttr(resp, stunAttrRealm)), string(stunAttr(resp, stunAttrNonce))
			continue
		}
		return realm, nonce, fmt.Errorf("allocate error %d", code)
	}
	return realm, nonce, fmt.Errorf("allocate stale nonce retry exhausted")
}

func refreshAuth(t *testing.T, conn net.Conn, user, pass, realm, nonce string) error {
	t.Helper()
	lifetime := make([]byte, 4)
	binary.BigEndian.PutUint32(lifetime, 600)
	for attempt := 0; attempt < 2; attempt++ {
		attrs := []stunAttr{
			{stunAttrLifetime, lifetime},
			{stunAttrUsername, []byte(user)},
			{stunAttrRealm, []byte(realm)},
			{stunAttrNonce, []byte(nonce)},
		}
		msg := stunRequestWithIntegrity(stunRefresh, attrs, user, realm, pass)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write(msg); err != nil {
			return err
		}
		resp := readSTUN(t, conn)
		code := stunErrorCode(resp)
		if code == 0 {
			return nil
		}
		if code == 438 {
			if v := stunAttr(resp, stunAttrRealm); len(v) > 0 {
				realm = string(v)
			}
			if v := stunAttr(resp, stunAttrNonce); len(v) > 0 {
				nonce = string(v)
			}
			continue
		}
		return fmt.Errorf("refresh error %d", code)
	}
	return fmt.Errorf("refresh stale nonce retry exhausted")
}

type stunAttr struct {
	typ uint16
	val []byte
}

func stunRequest(typ uint16, attrs []stunAttr) []byte {
	var body bytes.Buffer
	for _, a := range attrs {
		writeAttr(&body, a.typ, a.val)
	}
	return stunHeader(typ, body.Bytes(), nil)
}

func stunRequestWithIntegrity(typ uint16, attrs []stunAttr, user, realm, pass string) []byte {
	var body bytes.Buffer
	for _, a := range attrs {
		writeAttr(&body, a.typ, a.val)
	}
	writeAttr(&body, stunAttrIntegrity, make([]byte, 20))
	raw := stunHeader(typ, body.Bytes(), nil)
	key := md5.Sum([]byte(user + ":" + realm + ":" + pass))
	mac := hmac.New(sha1.New, key[:])
	_, _ = mac.Write(raw[:len(raw)-20])
	copy(raw[len(raw)-20:], mac.Sum(nil))
	return raw
}

func stunHeader(typ uint16, body, tid []byte) []byte {
	if tid == nil {
		tid = make([]byte, 12)
		_, _ = rand.Read(tid)
	}
	raw := make([]byte, 20+len(body))
	binary.BigEndian.PutUint16(raw[0:2], typ)
	binary.BigEndian.PutUint16(raw[2:4], uint16(len(body)))
	binary.BigEndian.PutUint32(raw[4:8], stunMagic)
	copy(raw[8:20], tid)
	copy(raw[20:], body)
	return raw
}

func writeAttr(buf *bytes.Buffer, typ uint16, val []byte) {
	var hdr [4]byte
	binary.BigEndian.PutUint16(hdr[0:2], typ)
	binary.BigEndian.PutUint16(hdr[2:4], uint16(len(val)))
	buf.Write(hdr[:])
	buf.Write(val)
	if pad := (4 - len(val)%4) % 4; pad > 0 {
		buf.Write(make([]byte, pad))
	}
}

func readSTUN(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

func stunAttr(msg []byte, typ uint16) []byte {
	for i := 20; i+4 <= len(msg); {
		at := binary.BigEndian.Uint16(msg[i : i+2])
		al := int(binary.BigEndian.Uint16(msg[i+2 : i+4]))
		start := i + 4
		end := start + al
		if end > len(msg) {
			return nil
		}
		if at == typ {
			return msg[start:end]
		}
		i = end + (4-al%4)%4
	}
	return nil
}

func stunErrorCode(msg []byte) int {
	val := stunAttr(msg, stunAttrErrorCode)
	if len(val) < 4 {
		return 0
	}
	return int(val[2])*100 + int(val[3])
}
```

- [ ] **Step 6: 运行 Refresh 门禁并写入 README 结论**

Run:

```bash
go test -tags coturn ./apps/raypx2-center/internal/turncred -run TestRefreshAfterExpiry -v -count=1
```

Expected: 若 `turnserver` 已构建，测试打印 `REFRESH_COMPAT=ok` 或 `REFRESH_COMPAT=fail`。

把 `deploy/coturn/README.md` 中的 `(pending Task 5)` 换成对应段落，二选一，不得留 pending：

成功：

```markdown
Pinned gitlink `7c24c88a4c13ef79edce9e645bef578eb7e5a6ad`: authenticated Refresh after credential timestamp expiry succeeded. Existing allocations remain usable until Delete or idle timeout.
```

失败：

```markdown
Pinned gitlink `7c24c88a4c13ef79edce9e645bef578eb7e5a6ad`: authenticated Refresh after credential timestamp expiry failed. Follow the ICE spec replacement-connection drain: create a temporary replacement child before expiry, then drain the old slot. Long-lived tunnels may drop at forced expiry.
```

- [ ] **Step 7: 全量 Center 测试**

Run:

```bash
go test ./apps/raypx2-center/...
```

Expected: PASS（不含 `-tags coturn` 的 Refresh 测试，默认构建必须仍通过）

- [ ] **Step 8: Commit**

```bash
git add apps/raypx2-center/internal/turncred/rest.go apps/raypx2-center/internal/turncred/rest_test.go apps/raypx2-center/internal/turncred/refresh_compat_test.go deploy/coturn/README.md
git commit -m "$(cat <<'EOF'
feat(center): add TURN REST credential helpers and refresh gate

EOF
)"
```
