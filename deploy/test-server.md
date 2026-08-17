# 远程测试服务器（信令 + coturn）

后续在真实网络上验证 raypx2-center 信令与 coturn STUN/TURN 时，使用这台机器。
Center 与 coturn 同机、独立进程。

## 主机

| 项 | 值 |
| --- | --- |
| 公网 IP | `64.176.42.49` |
| SSH | `ssh vultr` 或 `ssh root@64.176.42.49` |
| 仓库路径 | `/opt/src/pocketbase` |
| 本机二进制 | `/opt/src/pocketbase/bin/raypx2-center` |

本机 `~/.ssh/config` 中 `Host vultr` / `64.176.42.49` 使用 `~/.ssh/id_ed25519`。

仓库部署在 `31d5fe19`。开机由 systemd 拉起（与 supervisord 里旧的 `8743` coturn 并存，互不影响）：

```bash
systemctl status raypx2-center raypx2-coturn
systemctl restart raypx2-center
systemctl restart raypx2-coturn
```

## raypx2-center（信令）

监听 `0.0.0.0:8443`，数据目录 `/opt/src/pocketbase/bin/pb_data`：

```bash
/opt/src/pocketbase/bin/raypx2-center serve --http=0.0.0.0:8443 --dir=/opt/src/pocketbase/bin/pb_data
```

- 控制台：`http://64.176.42.49:8443/app/`
- Agent 信令：`http://64.176.42.49:8443`（WebSocket `/api/agent/ws`）

这是测试环境直出 HTTP，不是生产反代 HTTPS/WSS 部署。

## coturn（STUN/TURN）

与仓库模板默认 `3478` / `49152-65535` 不同，本机测试端口为：

| 项 | 值 |
| --- | --- |
| 控制端口（listening-port） | `8744` |
| Relay 端口段 | `40000-40500` |

下发给远端 Agent 的 URL 应使用公网 IP，不要用 `127.0.0.1`：

```text
stun:64.176.42.49:8744
turn:64.176.42.49:8744?transport=udp
```

对应 `turnserver.conf` 覆盖：

```text
listening-port=8744
min-port=40000
max-port=40500
```

`listening-ip` / `relay-ip` / `external-ip` 按主机网卡填写。secret 仍走 `/run/secrets/coturn-rest-secret`，不要写入已提交的配置。

## 防火墙

至少放行：

- TCP `8443`（Center HTTP / WebSocket）
- UDP `8744`（STUN/TURN 控制）
- UDP `40000-40500`（TURN relay）

不要对远端 Agent 开放 loopback 地址。通用部署说明见 [coturn/README.md](coturn/README.md)。
