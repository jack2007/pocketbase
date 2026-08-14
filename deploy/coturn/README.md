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
Pinned gitlink `7c24c88a4c13ef79edce9e645bef578eb7e5a6ad`: authenticated Refresh after credential timestamp expiry succeeded. Existing allocations remain usable until Delete or idle timeout.
```
