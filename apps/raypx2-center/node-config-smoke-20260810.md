# Node configuration manual smoke (2026-08-10)

Environment: current `master` center on `127.0.0.1:18091` with temporary
local raypx2 client and server agents. Authentication used PocketBase's
`_superusers/auth-with-password` endpoint and a temporary smoke superuser.

- PASS — Both client and server enrolled and reported `online=true`.
- PASS — Server ACL changed to `10.0.0.0/8`; `listen` was reported in
  `ignored_fields`, remained `127.0.0.1:19443`, and the latest desired
  revision had `source=manual_edit`.
- FAIL — Updating only client peer `primary`'s
  `connection.max_send_rate_kbps` returned `400 admin_rejected` from the
  raypx2 Admin API (`bad_request`). The rate was not changed, so preservation
  of the second peer and the edited peer's encryption/compression could not
  be validated through a successful save.
- PASS — Payloads containing `tls.key` and peer `enroll_secret` each returned
  `400 secret_field_forbidden`.
- PASS — After stopping the client agent and observing `online=false`, Save
  returned `409 node_offline`.

Overall result: **FAIL** because the client rate-edit path did not apply.
