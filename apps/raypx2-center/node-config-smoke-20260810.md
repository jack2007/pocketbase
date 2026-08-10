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

Overall result after the fix: automated coverage passes for ignoring startup-only
rates while preserving writable peer updates and untouched peers. A fresh manual
Step 3 re-smoke remains open because no local center/node process was running;
repeat it with a writable field such as `socks_listen` or `enabled`.
