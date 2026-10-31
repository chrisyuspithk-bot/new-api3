# Claude Code deploy guide — New API (SP Chat B2B2C)

How to rebuild / swap the **New API** gateway used by SP Chat on a host like
`api.spit.hk`, using **Claude Code**. Companion SP Chat copy-stack guide:
https://github.com/chrisyuspithk-bot/sp-chat/blob/copy-8092/docs/CLAUDE_CODE_DEPLOY.md

**Repo:** https://github.com/chrisyuspithk-bot/new-api3

---

## 0. Safety

| Do | Do not |
|---|---|
| Tag the running image as `new-api-b2b2c:prev-spchat` (or a dated `pre-*` tag) **before** swapping | Rebuild/restart New API during a live demo without a rollback tag |
| Keep SP Chat live `:8088` / `:5174` / `:8092` units alone unless asked | Point `SP_CHAT_BASE_URL` at a broken copy |
| Prefer copy ports for SP Chat experiments (`:8089` provision / `:8092` migration) | Commit `*.bak*` or secrets |

Reference host listeners: New API Docker on `:3000`, public `https://api.spit.hk/v1`.

---

## 1. What this tree provides for SP Chat

- B2B2C reseller / customer wallet APIs
- `service/sp_chat_provision.go` — `ProvisionSpChatOrg` creates the SP Chat company/org + per-customer keys after reseller **create-customer**
- Default provision target on the reference host: SP Chat **:8089** (`owui-copy`), **not** the :8092 migration copy (dual-target not wired)

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
- Do NOT restart owui.service (:8088), public :5174, owui-5174-copy (:8092), or
  sp-chat-ui units unless I explicitly ask.
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
   Confirm SP Chat :8088 / :8089 / :8092 / :5174 still 200 and MainPIDs unchanged
   for owui / owui-copy / caddy.
6) Report image digests for :local and :prev-spchat.
```

---

## 4. Reseller → SP Chat provision flow

1. Reseller (or admin) **create-customer** in New API (B2B2C).
2. New API calls **`ProvisionSpChatOrg`** (`service/sp_chat_provision.go`) against
   `SP_CHAT_BASE_URL` (reference host: the **:8089** SP Chat copy).
3. SP Chat receives company/org + stores per-customer LLM key material under
   `group.data.sp_company` (used when `SP_COMPANY_ISOLATION=true` on the chat side).
4. Chat completions go to `https://api.spit.hk/v1`; wallet reads may use loopback
   `NEW_API_BASE=http://127.0.0.1:3000` from the SP Chat env.

For redeploying the **:8092** migration UI/backend (not the provision target), follow the
sp-chat `docs/CLAUDE_CODE_DEPLOY.md` on branch `copy-8092`.

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

- SP Chat :8092 copy deploy: https://github.com/chrisyuspithk-bot/sp-chat/blob/copy-8092/docs/CLAUDE_CODE_DEPLOY.md
- SP Chat :8092 feature notes: https://github.com/chrisyuspithk-bot/sp-chat/blob/copy-8092/docs/FEATURES_8092_DONE.md
