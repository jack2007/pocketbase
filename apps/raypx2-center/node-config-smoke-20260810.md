# Node configuration manual smoke (2026-08-10)

Environment: current `master` center on `127.0.0.1:18091` with temporary
local raypx2 client and server agents. Authentication used PocketBase's
`_superusers/auth-with-password` endpoint and a temporary smoke superuser.

- PASS — Both client and server enrolled and reported `online=true`.
- PASS — Server ACL changed to `10.0.0.0/8`; `listen` was reported in
  `ignored_fields`, remained `127.0.0.1:19443`, and the latest desired
  revision had `source=manual_edit`.
- EXPECTED REJECTION — Updating only client peer `primary`'s
  `connection.max_send_rate_kbps` returned `400 admin_rejected` from the
  raypx2 Admin API (`bad_request`). This established that peer send-rate
  defaults are startup-only. Center now excludes these paths from the client
  writable surface: Config submissions report them in `ignored_fields`, and
  template merge rejects them.
- PASS — Payloads containing `tls.key` and peer `enroll_secret` each returned
  `400 secret_field_forbidden`.
- PASS — After stopping the client agent and observing `online=false`, Save
  returned `409 node_offline`.

Automated coverage after the fix passed for ignoring startup-only rates while
preserving writable peer updates and untouched peers. The manual Step 3 gap was
then closed by the re-smoke below.

## Step 3 re-smoke after rate remediation

- PASS — A fresh current-`master` center ran on `127.0.0.1:18091` with a
  temporary two-peer local client enrolled and online.
- PASS — Config PUT changed only peer `primary`'s `socks_listen` from
  `127.0.0.1:19380` to `127.0.0.1:19382`; Admin returned 200.
- PASS — Peer `secondary` remained present with its original SOCKS listener,
  compression settings, encryption, and rates. Both peers retained
  `encryption=enabled`.
- PASS — Submitted `min_send_rate_kbps=1234` and
  `max_send_rate_kbps=5678` appeared in `ignored_fields`; live values remained
  0/0 for `primary` and 1000/9000 for `secondary`.
- PASS — The focused `TrimForRole` rate-ignore test passed.

The remaining manual Step 3 gap is closed.
