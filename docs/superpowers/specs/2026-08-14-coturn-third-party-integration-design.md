# coturn third_party submodule 与 Center 同机部署设计

**日期：** 2026-08-14  
**状态：** 已批准  
**实现计划：** [docs/superpowers/plans/2026-08-14-coturn-third-party-integration.md](../plans/2026-08-14-coturn-third-party-integration.md)  
**范围仓库：** `/home/jack/src/pocketbase`  
**对照规格：** `/home/jack/src/raypx2/docs/superpowers/specs/2026-08-14-udp-ice-p2p-nat-traversal-design.md`  
**决策摘要：** 方案 1 + 同机独立进程（A）。只在 pocketbase 钉一份 `jack2007/coturn`；Center 与 coturn 同机、两个 systemd unit、互不父子。

## 1. 背景与目标

raypx2 ICE/P2P 规格要求 Center 签发 TURN REST 临时凭据，coturn 提供 STUN/UDP 与 TURN/UDP。目标部署中，**raypx2-center（信令）与 coturn（STUN/TURN）在同一台服务器上**，但崩溃域必须分开：Center 重启不得杀掉 relay allocation。

pocketbase 当前没有 `third_party` 依赖。本设计把 `https://github.com/jack2007/coturn` 以 git submodule 钉进本仓库，独立编出 `turnserver`，并锁定同机配置、secret 契约与测试门禁。

**目标：**

1. 在 pocketbase 以 submodule 固定 `jack2007/coturn` 的具体 commit。
2. `go build` / Center 测试不依赖该 submodule。
3. 独立脚本用 CMake 编出 `turnserver`。
4. 提供 UDP-only + TURN REST 配置模板；Center 与 coturn 读同一 secret 文件。
5. 向远端 Agent 下发该机公网 `stun:` / `turn:?transport=udp`，不下发 `127.0.0.1`。
6. raypx2 系统测试引用同一 SHA，不再复制一份 submodule。

### 1.1 决策摘要

| 项 | 决定 |
| --- | --- |
| 集成深度 | 同机独立进程（A），不是 Center 子进程 |
| 钉版本位置 | 只在 pocketbase（方案 1） |
| 构建 | `scripts/build-coturn.sh` + CMake；不进入 `go build` |
| 认证 | `use-auth-secret` + HMAC-SHA1 REST；无静态用户表 |
| 传输 | 只启用 UDP；`no-tcp` / `no-tls` / `no-tcp-relay`；不启 DTLS |
| 地址 | 公网 DNS 或公网 IP；同机 ≠ localhost |
| 规格落点 | 本文件；raypx2 ICE 规格交叉引用本文件 |

### 1.2 非目标

- 不实现 ICE 信令、grant、libjuice 或 MsQuic patch。
- 不把 coturn 链进 Go，不在 Center 进程内 spawn `turnserver`。
- 不支持 TURN/TCP、TURN/TLS、DTLS。
- 不在 pocketbase 与 raypx2 各钉一份 submodule。
- 不把 `127.0.0.1` / `::1` 当作生产 TURN URL。
- 不做多 Center 区域、双边 server relay、UPnP。

## 2. 范围、仓库边界与同机架构

本规格只覆盖：钉版本、独立构建、UDP-only 配置、secret 契约、同机独立进程。ICE 状态机与 Center 信令/grant 实现仍归 raypx2 ICE 规格。

| 仓库 | 做什么 | 不做什么 |
| --- | --- | --- |
| pocketbase | 钉 `third_party/coturn`；独立编出 `turnserver`；提供配置模板与 unit；Center 读取同一 secret 并下发公网 URL | 不把 coturn 链进 Go；不 `add_subdirectory` 进 `go build`；Center 不 spawn `turnserver` |
| raypx2 | Agent 用 libjuice 做 ICE/TURN 客户端；系统测试检出 **同一 coturn commit** | 不再复制一份 submodule |

```text
远端 Agent                         目标服务器（同一台）
  |                                +------------------+
  |  HTTPS/WSS 信令/凭据            | raypx2-center    |
  +------------------------------->| (PocketBase)     |
  |                                +--------+---------+
  |                                         | 只共享
  |                                         | REST secret 文件
  |                                +--------v---------+
  |  STUN/UDP、TURN/UDP             | coturn/turnserver|
  +------------------------------->| 3478/udp         |
                                   +------------------+
```

- 同机不等于 Agent 连本机回环。Center 下发该机公网主机名或 `external-ip` 对应地址。
- 两个 systemd unit，独立启停。Center 重启不影响 coturn allocation；coturn 重启只影响 relay，不影响 Center 和已建立的 direct QUIC。
- Center 继续只听 loopback HTTP，TLS 由反代终止（现有 `apps/raypx2-center/README.md`）。coturn 不走该反代，Agent 直连主机 UDP。

## 3. 目录、构建与版本钉扎

### 3.1 目录

```text
third_party/coturn/                 # submodule → https://github.com/jack2007/coturn
scripts/build-coturn.sh             # 唯一官方构建入口
deploy/coturn/turnserver.conf       # UDP-only 模板，不含 secret
deploy/coturn/turnserver-start.sh   # 读 secret 文件后 exec turnserver
deploy/coturn/coturn.service        # 独立 systemd unit 模板
deploy/coturn/README.md             # 公网地址、防火墙、secret 权限、Refresh 结论
docs/superpowers/specs/2026-08-14-coturn-third-party-integration-design.md
.gitmodules
```

`build-coturn/` 加入 `.gitignore`。submodule 缺失时，`go test ./apps/raypx2-center/...` 与 `go build ./apps/raypx2-center` 必须仍通过。

### 3.2 版本钉扎

- `.gitmodules` 记录 URL；gitlink 钉 **具体 commit**，不钉浮动 `master`。
- 该 SHA 是全系统唯一真相源。raypx2 NAT/TURN 系统测试必须检出同一 commit。
- 升级流程：先在 `jack2007/coturn` 合入或快进 → 跑凭据过期后 authenticated Refresh 兼容门禁 → 再更新 pocketbase gitlink。
- 许可证：coturn 为 BSD-3-Clause，保留 submodule 内 `LICENSE`，不把源码摊进 Go 树。

### 3.3 构建

官方入口：

```bash
./scripts/build-coturn.sh
```

行为：`third_party/coturn/CMakeLists.txt` 不存在则失败并提示 `git submodule update --init third_party/coturn`；然后：

```bash
cmake -S third_party/coturn -B build-coturn \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_TESTING=OFF \
  -DFUZZER=OFF \
  -DWITH_MYSQL=OFF
cmake --build build-coturn --target turnserver --parallel
```

硬依赖：`libevent2`、`libmicrohttpd`（上游 CMake 缺此库会失败）、`OpenSSL`（STUN `MESSAGE-INTEGRITY`）。SQL/Mongo/Redis 用户库首期不启用——REST `use-auth-secret` 不需要用户数据库。

保留 Prometheus 导出（依赖 libmicrohttpd），供 allocation/quota 告警。不在编译期关掉 TLS 代码；UDP-only 由运行时配置保证。

必交付二进制：`turnserver`。`turnutils_stunclient` 可作为可选冒烟工具，不链进 Center。

## 4. 配置、REST 凭据与同机地址

### 4.1 配置落点

提交：

```text
deploy/coturn/turnserver.conf
deploy/coturn/turnserver-start.sh
deploy/coturn/coturn.service
deploy/coturn/README.md
```

运行时（目标机，不入库）：

```text
/etc/raypx2/turnserver.conf            # 由模板安装，填入 listening-ip / external-ip
/run/secrets/coturn-rest-secret        # 唯一 shared secret；mode 0640
```

coturn 没有稳定的 secret 文件选项。约定：Center 直接读该文件；`turnserver` 由 `turnserver-start.sh` 读同一文件后传入 `--static-auth-secret`。secret 不得写入已提交的 `turnserver.conf`。

### 4.2 UDP-only 模板

| 项 | 值 | 原因 |
| --- | --- | --- |
| `listening-port` | `3478` | STUN/TURN 控制口 |
| `use-auth-secret` | 开启 | TURN REST，不用静态用户表 |
| `realm` | `raypx2` | 与 Center 签发一致 |
| `rest-api-separator` | `:`（默认） | 只拆第一个 `:` |
| `no-tcp` / `no-tls` / `no-tcp-relay` | 开启 | 只允许 UDP |
| `dtls` | 不开启 | 无 DTLS |
| `no-udp` | 禁止设置 | 必须有 UDP 监听 |
| `user-quota` | `2` | 旧 allocation 与重建可短暂重叠 |
| `no-multicast-peers` | 开启 | 防中继打到组播 |
| `denied-peer-ip` | loopback、link-local、unspecified | 不默认封 RFC1918，避免实验室/内网对端失败 |
| CLI / web-admin | 关闭 | 空 `cli-password` 即关 CLI；不启 web-admin |
| `total-quota` / `bps-capacity` / `min-port`–`max-port` | 部署填写 | 默认中继口 `49152-65535/udp`，防火墙必须放行 |

`listening-ip` / `relay-ip` / `external-ip` 按网卡填写。机器只有内网 IP、前面有 1:1 公网映射时，必须设 `external-ip`，否则 XOR-RELAYED-ADDRESS 会把私网地址发给远端 Agent。

### 4.3 Center 下发的地址

```yaml
p2p:
  stun_urls:
    - stun:turn.example.com:3478
  turn:
    urls:
      - turn:turn.example.com:3478?transport=udp
    shared_secret_file: /run/secrets/coturn-rest-secret
    credential_ttl_seconds: 86400
    realm: raypx2
```

- URL 主机名是该服务器的公网 DNS 或公网 IP，禁止对远端 Agent 下发 `127.0.0.1` / `::1`。
- Center 启动时拒绝不含 `transport=udp` 的 TURN URL。
- secret 文件缺失、不可读或权限过宽：禁用 TURN 签发，保留 host/srflx ICE，并告警。本规格锁定这条契约；签发 API 由后续 ICE 阶段实现。

### 4.4 REST 凭据格式

与 coturn TURN REST API 一致：

```text
username = "{expiry_unix}:{session_id}_{connection_id}_{epoch}"
password = base64(hmac-sha1(shared_secret, username))
```

- `expiry_unix` 为 Unix 秒；默认 TTL 86400。
- userid 用 `_` 连接，避免再出现 `:`，防止和 separator 歧义。
- 每个 `session_id + connection_id + epoch` 独立一套凭据；新 epoch 必须新 username，不得把旧 password 续期。
- 发布前对该钉扎版本记录明确结论：timestamp 过期后，已建立 allocation 的 authenticated Refresh 是否继续成功。若该版本拒绝 Refresh，沿用 ICE 规格的 replacement connection / 排空边界，并写入 `deploy/coturn/README.md`。

## 5. systemd、故障边界与安全

### 5.1 两个独立 unit

`deploy/coturn/coturn.service` 要点：

- `Type=simple`。即使 coturn 编进了 libsystemd，模板也不依赖 `notify`，避免强绑 systemd 库。
- `ExecStart=/usr/local/libexec/raypx2/turnserver-start.sh`（源文件为 `deploy/coturn/turnserver-start.sh`）：读 `/run/secrets/coturn-rest-secret`，再 `exec turnserver -c /etc/raypx2/turnserver.conf --static-auth-secret=...`。secret 不进 unit 文件、不进环境转储。
- `Restart=on-failure`，与 Center 的 restart 策略独立。
- `After=network-online.target`；不 `Requires=` Center，不 `PartOf=` Center。
- 以非 root 专用用户运行（`turnserver`），只对 secret 文件有组读权限。
- 不绑定 80/443/5349；只使用 UDP 3478 与中继端口范围。

`raypx2-center.service` 已有或另立。本规格不把 coturn 写进 Center `ExecStart`。

### 5.2 故障边界

| 事件 | Center | coturn | 已建立 direct QUIC | 已建立 relay |
| --- | --- | --- | --- | --- |
| Center 重启 | 中断信令 | 不受影响 | 保持 | 保持（allocation 仍在 coturn） |
| coturn 重启 | 不受影响 | 中断 | 保持 | 中断，由 Agent 按 slot restart |
| secret 文件丢失 | 停发 TURN 凭据并告警 | 启动失败或拒绝新 Allocate | 保持 | 已有 allocation 视进程是否还在 |
| 仅 3478/udp 被丢 | 信令正常 | 进程可在 | host/srflx 仍可能成功 | 新 TURN 失败，按 ICE 规格 10s 内终止 |

禁止：Center 把 `turnserver` 当子进程；禁止两边共用一个 cgroup `PartOf`，以免 Center 升级杀掉 relay。

### 5.3 安全

- secret：单文件、0640、属主 root、组仅 `turnserver` 与 Center 运行用户；世界可读则 Center 拒绝签发并告警。
- 日志与 metrics label 禁止出现 secret、TURN password、完整 username 中的 session 明文（可用不可逆前缀）。
- 关闭 CLI、web-admin。若开启 Prometheus，只绑 loopback 或防火墙内网，不在公网暴露。
- `denied-peer-ip` 覆盖 loopback / link-local / unspecified；`no-multicast-peers` 开启。
- 本机防火墙：入站 UDP 3478 + `min-port`–`max-port`；不开放 TURN/TCP、TLS 5349。
- submodule 升级视为安全变更：必须重跑 Refresh 兼容门禁与 secret scan。

## 6. 测试与验收

本规格只验收钉版本、独立构建、UDP-only 配置、secret 契约、同机独立进程。完整 NAT/ICE/QUIC 矩阵仍归 raypx2 ICE 规格阶段 7。

### 6.1 pocketbase 测试

| 层 | 断言 |
| --- | --- |
| submodule | `.gitmodules` 指向 `https://github.com/jack2007/coturn`；gitlink 为固定 SHA；`LICENSE` 仍在树内 |
| Go 隔离 | 不 init submodule 时，`go test ./apps/raypx2-center/...` 与 `go build ./apps/raypx2-center` 通过 |
| 构建冒烟 | `./scripts/build-coturn.sh` 产出 `turnserver`；`turnserver --help` 退出 0 |
| 配置审计 | 用模板启动（测试 secret）后，监听只有 `3478/udp`；无 TCP 3478、无 5349、无 DTLS；进程参数含 `use-auth-secret`，不含静态用户 |
| secret 契约 | 缺文件或权限对 others 可读：Center 校验函数返回禁用 TURN；合法 0640 且属主/组正确则可读 |
| REST 向量 | 固定 secret + 固定 username，password 等于 `base64(hmac-sha1(secret, username))`；userid 不含额外 `:` |
| Refresh 门禁 | 对该 SHA：timestamp 过期后，已建立 allocation 的 authenticated Refresh 成功或失败必须记成明确结论，并写进 `deploy/coturn/README.md` |

### 6.2 raypx2 约定

- 系统测试检出 pocketbase `third_party/coturn` 的同一 SHA 再编 `turnserver`。
- 端到端门禁不得 stub coturn 凭据生成。
- raypx2 ICE 规格交叉引用本文件，避免两套 pin。

### 6.3 发布验收清单

- [ ] submodule 可 `git submodule update --init third_party/coturn`，Go 构建不依赖它
- [ ] 独立构建出 `turnserver`，模板为 UDP-only + REST
- [ ] Center 与 coturn 读同一 secret 文件；URL 为公网 `stun:` / `turn:?transport=udp`
- [ ] 两个 systemd unit 无 `Requires` / `PartOf` 互相绑定
- [ ] Refresh 兼容结论已记录
- [ ] secret / password 未进 git、日志样例或 metrics label

## 7. 实现拆分边界

后续实现计划按可独立验证的边界拆分：

1. 添加 `third_party/coturn` submodule、`.gitignore` 中的 `build-coturn/`、Go 隔离回归。
2. `scripts/build-coturn.sh` 与 `turnserver --help` 冒烟。
3. `deploy/coturn/turnserver.conf`、`turnserver-start.sh`、`coturn.service`、README。
4. Center 侧 secret 文件校验与 TURN URL 契约（预留 hook；完整签发 API 仍属 ICE 阶段）。
5. REST 向量测试与该 SHA 的 Refresh 兼容记录。

每一阶段都必须保持：不 init submodule 时 Go 测试仍通过。
