# raypx2 center

PocketBase-based control plane with an embedded React SPA at `/app/`.

## Start

From the repository root:

```bash
# Create a superuser once (password must be at least 10 characters).
go run ./apps/raypx2-center superuser create admin@example.com change-this-password

# Start the center.
go run ./apps/raypx2-center serve --http=127.0.0.1:8090
```

Open <http://127.0.0.1:8090/app/> and sign in with the superuser account.

Remote signaling + coturn tests use `64.176.42.49` (repo at
`/opt/src/pocketbase`). Start Center with:

```bash
/opt/src/pocketbase/bin/raypx2-center serve --http=0.0.0.0:8443
```

Host, ports, and coturn relay range are in
[deploy/test-server.md](../../deploy/test-server.md).

The compiled UI is committed under `ui/dist`, so Node.js is not required to
serve the center. To change the UI:

```bash
cd apps/raypx2-center/ui
npm install
npm test
npm run build
```

Commit the updated `dist` files together with the source changes.

## Node configuration

Open a node → **Config**. Edit via **Form** or **JSON**, then Save.
The center whitelists writable Admin fields, writes the node over the
agent tunnel, and stores a `manual_edit` revision. Offline nodes are
read-only. On client nodes, use **Add peer** in the Config form (enter a
`peer_id`) to create peers; saving upserts and keeps peers omitted from
the submitted list. To delete a saved peer from the node, open **Ops**
and use **Delete** on the Client peers table
(`DELETE /api/center/nodes/{node_key}/peers/{peer_id}`).

## Production HTTPS and WSS

Do not expose the center's plain HTTP listener directly. Bind it to loopback
and terminate TLS at a reverse proxy:

```bash
go run ./apps/raypx2-center serve --http=127.0.0.1:8090
```

For example, a Caddy frontend can provide certificates and proxy both HTTP
and WebSocket traffic:

```caddyfile
center.example.com {
	reverse_proxy 127.0.0.1:8090
}
```

Agents must use `https://center.example.com` for enroll and refresh requests
and `wss://center.example.com/api/agent/ws` for the control channel. The proxy
must preserve WebSocket upgrade headers and the original client address. Use
a publicly trusted certificate, or install the private CA on every agent; do
not disable certificate verification.

Enrollment secrets are returned only when a node is created or its secret is
rotated. Store the returned value in the agent's secret store and never put it
in logs. The superuser-only node management endpoints are:

- `GET /api/center/nodes`: lists nodes and merges `health_status` from
  `node_status` (agent `status_summary`) into each item.
- `POST /api/center/nodes`: creates a node and returns a one-time
  `enroll_secret`.
- `DELETE /api/center/nodes/{node_key}`: permanently deletes the node, cleans
  related revisions/targets, cascades sessions/status, writes an audit log, and
  disconnects the agent with `bye {"reason":"deleted"}`.
- `POST /api/center/nodes/{node_key}/rotate-enroll`: returns a new one-time
  `enroll_secret`, invalidates the old secret and sessions, and disconnects the
  current WebSocket with `bye {"reason":"rotated"}`.
- `POST /api/center/nodes/{node_key}/revoke`: marks enrollment revoked,
  invalidates sessions, and disconnects the current WebSocket with
  `bye {"reason":"revoked"}`.
- `DELETE /api/center/nodes/{node_key}/peers/{peer_id}`: removes one
  client peer by rewriting Admin config over the agent tunnel, then
  stores a `peer_delete` revision and `node.peer.delete` audit entry.

After rotation, update the agent with the new secret before restarting it.
After revocation, enrollment remains blocked until an operator rotates the
secret, which reactivates enrollment.

The Nodes console auto-refreshes every 10 seconds, exposes Delete on each row,
and shows Health from the merged `health_status` field on the Overview tab.

Node detail includes a read-only **Tunnels** tab that proxies
`GET /api/v1/tunnels` (client) or `GET /api/v1/server/tunnels` (server) while
the node is online.

## M1 manual smoke check

1. Run `npm test && npm run build` in `ui/` when changing the SPA.
2. Start the center and verify `GET /app/` returns `200`.
3. Sign in at `/app/` as a PocketBase superuser.
4. Open **Nodes**, create a node, and save the one-time enrollment secret into the
   raypx2 `center.enroll_secret_file` JSON (`version` + `enroll_secret`).
5. Verify the new node is listed and `GET /api/center/nodes` succeeds with the
   superuser token.
6. On the node, set `center.enabled=true` with `url` / `node_key` / secret file
   (see raypx2 `docs/center-agent_cn.md`), ensure Admin is listening, then start
   raypx2. Confirm SPA shows `online=true` and Ops proxy works.
7. With an online agent reporting status, confirm Health shows a value such as
   `healthy` instead of Unknown.
8. Delete a node from the UI and confirm it disappears after refresh.
