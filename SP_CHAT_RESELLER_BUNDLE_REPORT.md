# SP Chat reseller bundle — Create Customer → SP Chat org provision

Target tree: `/root/new-api3` only. Not touched: `/root/new-api`, live SP Chat `/root/open-webui-mvp` (:8088/:8080), image `new-api-b2b2c:prev-spchat`.

## Why this is a patch bundle
The agent ran as `claudeuser` (uid 1000). `/root/new-api3` is root-owned and read-only for that user except `SP_CHAT_PROVISION.md` and `service/sp_chat_provision*.go`. `sudo` is denied. The host has no `go` or `bun`, and the agent cannot use docker. So:
- `SP_CHAT_PROVISION.md` was edited in place (reseller path documented).
- `service/sp_chat_provision.go` was left unchanged. It already uses `common.Marshal`/`common.Unmarshal` and the module path `github.com/QuantumNous/new-api`. Every model/common API it calls exists in new-api3 with matching signatures. Its only missing dependency was the `User` link fields, which this patch adds.
- Everything else is in `sp-chat-reseller.patch` (15 files). It applies cleanly (`patch --dry-run` against a copy of the current files).
- **Nothing has been compiled or tested yet.** Run the build and test steps below before swapping the container.

## Changes in the patch
| File | Change |
|---|---|
| `model/user.go` | `CompanyName`, `OwuiGroupId`, `OwuiAdminUserId`, `NewapiTokenId`, `SpChatProvisioned`, `SpChatProvisionedAt`, with the same tags/columns as `/root/new-api/model/user.go`. Username/DisplayName stay `max=20`. |
| `controller/reseller.go` | `resellerCustomerRequest` gets `email` and `company_name`. `createResellerCustomer` (both reseller and admin create paths) requires a valid email (≤50 chars, not used by another user) and a company name. It stores both, then calls `service.ProvisionSpChatOrg(id, plaintextPassword, company)`. On failure the New API customer is **kept** and the response is `success:true` with `sp_chat_provisioned:false` and `sp_chat_warning:"Customer created, but SP Chat provisioning failed"`. `customerView` now includes `email`, `company_name` and `sp_chat_provisioned`. |
| `controller/user.go` | Admin `UpdateUser` with a password: sync to SP Chat if linked. If not linked but the user has a company name and email, provisioning runs (this is the retry path). The plaintext is captured before `EditWithTx` hashes it in place. `UpdateSelf` password change: background sync. |
| `controller/misc.go` | Email password reset (`/api/user/reset`): background sync of the generated password. |
| `web/.../reseller-console/{api,types}.ts`, `components/customers-page.tsx` | The create form gets required **Email** (`type=email`) and **Company name** fields, sent in the POST body. A warning toast appears when `sp_chat_warning` is returned. `maxLength` matches the server limits (username/display name 20, email 50, company 128). |
| `web/.../__tests__/customers.test.tsx` | The create test now asserts `email` and `company_name` in the body. New test for the warning toast. |
| `web/src/i18n/locales/*.json` (7) | `Company name`, `Customer created, but SP Chat provisioning failed`. |

Design choices:
- **No rollback on provision failure.** `ProvisionSpChatOrg` already deletes the token, OWUI user and group it created, and leaves `sp_chat_provisioned=0`. To retry, an admin resets the customer's password in Users.
- **No new reseller password-reset endpoint.** Resellers cannot set customer passwords today. Adding that ability would be a new authentication capability and needs its own review. Reseller customers' passwords sync through the three existing paths: self change, admin reset, email reset.
- **Email uniqueness** is checked in New API, so email password reset stays unambiguous. The reseller is an authenticated operator, so the "email is already in use" reply is not a public account-enumeration oracle. SP Chat also rejects duplicate emails, which shows up as a provision warning.

## 1. Apply (as root)
```bash
B=/home/claudeuser/sp-chat-reseller-bundle
cd /root/new-api3
patch -p1 --dry-run < $B/sp-chat-reseller.patch && patch -p1 < $B/sp-chat-reseller.patch
cp $B/SP_CHAT_RESELLER_BUNDLE_REPORT.md /root/new-api3/
```

## 2. Build the web UI (required: TSX changed and `web/dist` is embedded in the binary)
The host has no bun and `web/node_modules` does not exist. Build in the pinned bun image and keep `node_modules` plus the bun cache in a throwaway directory. Check `df -h / /tmp` first; about 1 GB is needed. Use `/tmp` (tmpfs) if it fits, otherwise a directory on `/`.
```bash
mkdir -p /tmp/na3-nm /tmp/na3-bun-cache
docker run --rm \
  -v /root/new-api3/web:/build/web -v /root/new-api3/VERSION:/build/VERSION:ro \
  -v /tmp/na3-nm:/build/web/node_modules -v /tmp/na3-bun-cache:/root/.bun/install/cache \
  -w /build/web oven/bun:1.4.0 sh -c '
    bun install --frozen-lockfile &&
    bun run test src/features/reseller-console &&
    bun run typecheck &&
    DISABLE_ESLINT_PLUGIN=true VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build'
ls /root/new-api3/web/dist/index.html
```

## 3. Build the Go binary to `/tmp/new-api-bin-out/new-api`
```bash
mkdir -p /tmp/new-api-bin-out /tmp/na3-gomod
docker run --rm \
  -v /root/new-api3:/build -v /tmp/na3-gomod:/go/pkg/mod -v /tmp/new-api-bin-out:/out \
  -w /build -e CGO_ENABLED=0 -e GOWORK=off -e GOEXPERIMENT=greenteagc \
  golang:1.26.1-alpine sh -c '
    gofmt -l model controller service;   # must print nothing; else run gofmt -w on the listed files
    go vet ./model ./service ./controller &&
    go test ./service -run SpChat -count=1 &&
    go test ./controller ./model -count=1 &&
    go build -pgo="$(go list -m -f "{{.Dir}}" github.com/Calcium-Ion/moejs)/default.pgo" \
      -ldflags "-s -w -X github.com/QuantumNous/new-api/common.Version=$(cat VERSION)" -o /out/new-api'
ls -la /tmp/new-api-bin-out/new-api
```

## 4. Binary-only image (prev-spchat stays as rollback)
```bash
cat > /tmp/new-api-bin-out/Dockerfile <<'EOF'
FROM new-api-b2b2c:prev-spchat
COPY --chmod=0755 new-api /new-api
EOF
docker build -t new-api-b2b2c:local /tmp/new-api-bin-out
docker image inspect new-api-b2b2c:prev-spchat >/dev/null && echo "rollback image intact"
```

## 5. Swap the B2B2C container
Recreate it with its current run arguments and only the image changed. Get them with `docker inspect <b2b2c-container>` (ports, `-v .../data:/data`, `--env-file`). The env must include:
`SP_CHAT_PROVISION_ENABLED=true`, `SP_CHAT_BASE_URL=http://172.17.0.1:8089`, `SP_CHAT_ADMIN_EMAIL`, `SP_CHAT_ADMIN_PASSWORD` (and optionally `SP_CHAT_ADMIN_TOKEN`).
```bash
docker stop <b2b2c-container> && docker rename <b2b2c-container> <b2b2c-container>-prev
docker run -d --name <b2b2c-container> <same args as before> new-api-b2b2c:local
docker logs --tail 50 <b2b2c-container>     # AutoMigrate adds the 6 users columns if missing
```
Rollback: `docker rm -f <b2b2c-container> && docker rename <b2b2c-container>-prev <b2b2c-container> && docker start <b2b2c-container>`.

## 6. Smoke test
1. Log in as a reseller, go to `/reseller/customers` → Add Customer, and fill in username, email, company name and password (8+ chars). A "Customer created" toast should appear.
2. `curl -s http://127.0.0.1:8089/api/v1/auths/signin -H 'Content-Type: application/json' -d '{"email":"<email>","password":"<password>"}'` should return a token.
3. Stop the SP Chat COPY (or set a wrong `SP_CHAT_BASE_URL`) and create another customer. You should see a warning toast, the customer should exist in New API with `sp_chat_provisioned=false`, and there should be no orphan `sp-chat-<id>` token.
4. As the customer, change the password in New API, then sign into SP Chat with the new password.

## Not verified (blockers)
- No compile, `go vet`, `go test`, bun test, typecheck or web build has been run. They need steps 2–3 above.
- AGENTS.md requires real SQLite + MySQL + PostgreSQL checks, fresh and upgrade, run twice, for model changes. This was **not run**. The change only adds nullable/defaulted columns (`text`, `int`, `bigint`, `not null default 0`), identical to the live `/root/new-api` tree, and they already exist on the live SQLite DB. MySQL/PostgreSQL remain unverified.
- No new Go controller test was added. The service path is covered by the existing `service/sp_chat_provision_test.go`, and the reseller create/response contract is covered by the frontend test.
- OWASP: the reseller sets the initial password (8+ chars, hashed by the existing `InsertWithTx`). The plaintext is sent only to SP Chat over the internal `172.17.0.1:8089` link and is never logged. SP Chat errors are truncated in server logs and never shown to resellers. The SP Chat link is plain HTTP on the docker bridge; this gap is unchanged from the live tree.
