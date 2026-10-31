# Claude Code deploy guide — New API (SP Chat B2B2C)

How to rebuild / swap the **New API** gateway used by SP Chat on a host like
`api.spit.hk`, using **Claude Code**. Companion SP Chat Credits-stack guide:
https://github.com/chrisyuspithk-bot/sp-chat/blob/copy-8092/docs/CLAUDE_CODE_DEPLOY.md

**Repo:** https://github.com/chrisyuspithk-bot/new-api3

---

## 0. Safety

| Do | Do not |
|---|---|
| Tag the running image as `new-api-b2b2c:prev-spchat` (or a dated `pre-*` tag) **before** swapping | Rebuild/restart New API during a live demo without a rollback tag |
| Leave SP Chat Credits units alone unless asked (`owui-5174-copy` / `sp-chat-ui-5174-copy` on `:8092`) | Start stopped old SP Chat (`owui` `:8088`, live `:5174`, `owui-copy` `:8089`) unless Chris asks |
| Prefer `:8092` as the **product / Credits** SP Chat target | Commit `*.bak*` or secrets |

### Current ops state (reference host)

- **Active SP Chat product:** http://api.spit.hk:8092/ (Credits + isolation)
- **Old SP Chat stopped:** `owui.service` (`:8088`) and `owui-copy` (`:8089`) inactive; live `:5174` UI killed
- New API Docker on `:3000`, public `https://api.spit.hk/v1`

---

## 1. What this tree provides for SP Chat

- B2B2C reseller / customer wallet APIs
- `service/sp_chat_provision.go` — `ProvisionSpChatOrg` creates the SP Chat company/org + per-customer keys after reseller **create-customer**
- **Product / Credits UI target:** SP Chat **`:8092`**
- **Provision URL note:** on the reference host `SP_CHAT_BASE_URL` may still historically point at **`:8089`** (`owui-copy`). That unit is currently **stopped**. Do not revive it just for a New API rebuild; dual-wire or retarget provision to `:8092` only if Chris asks

Image naming used on the reference host:

| Tag | Role |
|---|---|
| `new-api-b2b2c:local` | Current running build from `/root/new-api3` |
| `new-api-b2b2c:prev-spchat` | Rollback pin (create before swap) |
| `new-api-b2b2c:pre-*` | Dated pre-change snapshots |

---

## 2. Build from this tree

Working directory: clone of this repo (on reference host: `/root/new-api3`).

```bash
cd /root/new-api3
# optional: confirm clean of bak files
git status -sb   # do not add *.bak*

# Tag current local image for rollback
docker tag new-api-b2b2c:local new-api-b2b2c:prev-spchat

# Build (Dockerfile at repo root). Adjust if your compose file pins another tag.
docker build -t new-api-b2b2c:local .
```

If the host runs via `docker compose`, update the compose `image:` / build context to this tree, then:

```bash
docker compose up -d --force-recreate   # ONLY the new-api service
```

Exact compose service name / container name vary by host — inspect `docker ps` and the unit/compose file Claude Code finds under `/root` before recreating.

**Binary-only alternative** (if the host runs a Go binary instead of Docker): `go build` per project README, install beside the existing binary, restart the matching systemd/supervisor unit — still tag/backup the previous binary first.

---

## 3. Claude Code prompt recipe

```text
Rebuild New API for SP Chat from /root/new-api3 (github.com/chrisyuspithk-bot/new-api3).

Rules:
- Do NOT restart owui-5174-copy / sp-chat-ui-5174-copy (:8092) unless I ask.
- Do NOT start owui.service (:8088), public :5174, or owui-copy (:8089)
  unless I explicitly ask (they are stopped on purpose).
- Do NOT commit *.bak* files.
- Before swapping the running container/image, docker tag new-api-b2b2c:local
  new-api-b2b2c:prev-spchat (or a dated pre-* tag if prev-spchat already exists).

Steps:
1) cd /root/new-api3 && git status && git log -1
2) docker tag new-api-b2b2c:local new-api-b2b2c:prev-spchat
3) docker build -t new-api-b2b2c:local .
4) Recreate only the New API container/compose service that serves :3000 /
   https://api.spit.hk/v1
5) Smoke:
   curl -sf -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3000/api/status
   curl -sf -o /dev/null -w "%{http_code}\n" https://api.spit.hk/v1/models
   Confirm SP Chat Credits :8092 still 200 and MainPIDs unchanged for
   owui-5174-copy / sp-chat-ui-5174-copy / caddy.
   Confirm old units stay inactive: systemctl is-active owui owui-copy
6) Report image digests for :local and :prev-spchat.
```

---

## 4. Reseller → SP Chat provision flow

1. Reseller (or admin) **create-customer** in New API (B2B2C).
2. New API calls **`ProvisionSpChatOrg`** (`service/sp_chat_provision.go`) against
   `SP_CHAT_BASE_URL`.
3. SP Chat receives company/org + stores per-customer LLM key material under
   `group.data.sp_company` (used when `SP_COMPANY_ISOLATION=true` on the chat side).
4. Chat completions go to `https://api.spit.hk/v1`; wallet reads may use loopback
   `NEW_API_BASE=http://127.0.0.1:3000` from the SP Chat env (on `:8092`).

**Targets:**

| Role | URL / unit |
|---|---|
| Product / Credits SP Chat | **http://api.spit.hk:8092/** (`owui-5174-copy` + `sp-chat-ui-5174-copy`) |
| Historical provision base | `:8089` (`owui-copy`) — currently stopped; retarget/dual-wire only if Chris asks |

For redeploying the **:8092** Credits UI/backend, follow the sp-chat
`docs/CLAUDE_CODE_DEPLOY.md` on branch `copy-8092`.

---

## 5. Rollback

```bash
# Retag / run previous image
docker tag new-api-b2b2c:prev-spchat new-api-b2b2c:local
# recreate the same compose/service that serves :3000
docker compose up -d --force-recreate   # new-api service only

curl -sf -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3000/api/status
```

If a dated `pre-*` tag was used instead, retag that image to `:local` the same way.

---

## 6. Cross-links

- SP Chat Credits `:8092` deploy: https://github.com/chrisyuspithk-bot/sp-chat/blob/copy-8092/docs/CLAUDE_CODE_DEPLOY.md
- SP Chat `:8092` feature notes: https://github.com/chrisyuspithk-bot/sp-chat/blob/copy-8092/docs/FEATURES_8092_DONE.md
