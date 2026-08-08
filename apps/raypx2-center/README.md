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

The compiled UI is committed under `ui/dist`, so Node.js is not required to
serve the center. To change the UI:

```bash
cd apps/raypx2-center/ui
npm install
npm test
npm run build
```

Commit the updated `dist` files together with the source changes.

## M1 manual smoke check

Real raypx2 Agent wiring is deferred for this milestone.

1. Run `npm test && npm run build` in `ui/`.
2. Start the center and verify `GET /app/` returns `200`.
3. Sign in at `/app/` as a PocketBase superuser.
4. Open **Nodes**, create a node, and save the one-time enrollment secret.
5. Verify the new node is listed and `GET /api/center/nodes` succeeds with the
   superuser token.
