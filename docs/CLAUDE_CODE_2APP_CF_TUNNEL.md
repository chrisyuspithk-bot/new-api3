# Claude Code playbook — two-app HTTPS deploy (Cloudflare Tunnel)

Deploy **New API** (this repo) and **SP Chat** as **two separate apps**, linked over
**public HTTPS**, fronted by **Cloudflare Tunnel** (Cloudflare Free + a free/always-on VPS).

This is the **new free/easy 2-app target** pattern. It does **not** replace the live
New API / Credits stack on `api.spit.hk`. Do **not** restart or disrupt live spit
`:8092` / existing New API containers unless Chris explicitly asks.

| Repo | Guide |
|---|---|
| This repo (`new-api3` `main`) | This file |
| Companion SP Chat | https://github.com/chrisyuspithk-bot/sp-chat/blob/main/docs/CLAUDE_CODE_2APP_CF_TUNNEL.md |
| Live spit New API rebuild | [`CLAUDE_CODE_DEPLOY.md`](./CLAUDE_CODE_DEPLOY.md) |

---

## 1. Architecture

```
Internet (HTTPS)
  │
  ├─ https://api.<domain>   ──Cloudflare Tunnel──► 127.0.0.1:3000     App A: New API (this repo, Docker)
  │                                                      /v1
  │
  └─ https://chat.<domain>  ──Cloudflare Tunnel──► 127.0.0.1:<ui>     App B: SP Chat (backend + frontend-8092)
                                                           │
                                                           └─ HTTPS → https://api.<domain>/v1
```

**Linking**

1. **SP Chat → New API:** OpenAI-compatible base URL `https://api.<domain>/v1` +
   company/customer token issued by New API.
2. **New API → SP Chat:** `ProvisionSpChatOrg` (`service/sp_chat_provision.go`) calls
   SP Chat admin API at `SP_CHAT_BASE_URL=https://chat.<domain>`.

Suggested DNS (Cloudflare, proxied): `api.<domain>`, `chat.<domain>`.

---

## 2. Free + easy path

| Use | Avoid |
|---|---|
| Always-on free VPS (Oracle Always Free, etc.) | Cloudflare Containers (not free / costly always-on) |
| Cloudflare Free + `cloudflared` Tunnel | Opening VPS ports without CF |
| Docker image from this tree | Touching live spit `:3000` / `:8092` without an explicit ask |

One VPS can run both apps (two local ports, one tunnel, two hostnames).

---

## 3. Prerequisites

- Cloudflare account; domain on Cloudflare DNS.
- API token with Tunnel + Zone DNS edit. On the deploy host only:

```bash
mkdir -p ~/.secrets && chmod 700 ~/.secrets
chmod 600 ~/.secrets/cloudflare_api_token
export CLOUDFLARE_API_TOKEN="$(cat ~/.secrets/cloudflare_api_token)"
test -f ~/.secrets/cloudflare_api_token && wc -c ~/.secrets/cloudflare_api_token
```

Also useful on multi-user hosts: `/root/.secrets/cloudflare_api_token` (mode 600,
dir 700). **Never** commit or print the token.

- Tools: Docker, `cloudflared`, git.
- Clones: **`new-api3` `main`** (this repo) and **`sp-chat` `main`**.

---

## 4. Steps (Claude Code copy-paste)

### 4.1 Install `cloudflared`

```bash
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared $(lsb_release -cs) main" \
  | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt-get update && sudo apt-get install -y cloudflared
cloudflared version
```

### 4.2 Create tunnel + DNS routes

```bash
export CLOUDFLARE_API_TOKEN="$(cat ~/.secrets/cloudflare_api_token)"
TUNNEL_NAME="sp-2app"
DOMAIN="<domain>"

cloudflared tunnel create "$TUNNEL_NAME"
cloudflared tunnel list
TUNNEL_UUID="<uuid-from-create>"

cloudflared tunnel route dns "$TUNNEL_UUID" "api.${DOMAIN}"
cloudflared tunnel route dns "$TUNNEL_UUID" "chat.${DOMAIN}"
```

Keep `~/.cloudflared/<TUNNEL_UUID>.json` mode 600; do not commit.

### 4.3 Run New API (App A) on loopback

```bash
cd /path/to/new-api3
docker build -t new-api-b2b2c:local .
# Host-local env file (SP_CHAT_BASE_URL=https://chat.<domain>, DB, admin secrets…)
docker run -d --name new-api \
  -p 127.0.0.1:3000:3000 \
  --env-file /path/to/new-api.env \
  new-api-b2b2c:local

curl -sf -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3000/api/status
```

On a **greenfield** VPS this is a new container. On **spit**, do not recreate the
live New API unless Chris asks — use [`CLAUDE_CODE_DEPLOY.md`](./CLAUDE_CODE_DEPLOY.md)
for that host and tag `prev-spchat` first.

### 4.4 Run SP Chat (App B)

Follow the twin guide in sp-chat:

https://github.com/chrisyuspithk-bot/sp-chat/blob/main/docs/CLAUDE_CODE_2APP_CF_TUNNEL.md

Use `sp-chat` **`main`**: `backend/` + `frontend-8092/`, dedicated `DATA_DIR`,
dedicated units. Point chat’s New API base at `https://api.<domain>/v1`.

### 4.5 Tunnel config

```yaml
tunnel: <TUNNEL_UUID>
credentials-file: /home/<user>/.cloudflared/<TUNNEL_UUID>.json

ingress:
  - hostname: api.<domain>
    service: http://127.0.0.1:3000
  - hostname: chat.<domain>
    service: http://127.0.0.1:15175
  - service: http_status:404
```

```bash
cloudflared tunnel run <TUNNEL_UUID>
# or: sudo cloudflared service install && sudo systemctl enable --now cloudflared
```

### 4.6 Wire + smoke

```bash
DOMAIN=<domain>
curl -sf -o /dev/null -w "api=%{http_code}\n" "https://api.${DOMAIN}/api/status" \
  || curl -sf -o /dev/null -w "api_root=%{http_code}\n" "https://api.${DOMAIN}/"
curl -sf -o /dev/null -w "chat=%{http_code}\n" "https://chat.${DOMAIN}/"

# Provision smoke: reseller create-customer → ProvisionSpChatOrg → chat org exists.
# Tokens/passwords stay on the host only.
```

New API env must include `SP_CHAT_BASE_URL=https://chat.<domain>`.
SP Chat env must use `https://api.<domain>/v1` plus the customer/company token.

### 4.7 Claude Code prompt recipe

```text
Deploy New API (this tree) + SP Chat as TWO apps on a free VPS, HTTPS via
Cloudflare Tunnel (cloudflared). Do NOT use Cloudflare Containers.

Token: export CLOUDFLARE_API_TOKEN="$(cat ~/.secrets/cloudflare_api_token)"
Never print or commit it.

Hostnames: api.<domain>, chat.<domain>.
App A: Docker New API on 127.0.0.1:3000 from new-api3 main.
App B: sp-chat main backend + frontend-8092.
SP_CHAT_BASE_URL=https://chat.<domain>; chat OpenAI base=https://api.<domain>/v1.

Do NOT touch live api.spit.hk Credits :8092 or existing spit New API unless I ask.
No secrets / *.bak* in git.

Steps: cloudflared install → tunnel create → DNS route → run apps → ingress →
wire URLs → smoke both public hosts → provision smoke. Report UUID, DNS, ports, codes.
```

---

## 5. Safety vs live spit

| This guide | Live spit |
|---|---|
| New free VPS + new tunnel hostnames | New API `:3000` / `https://api.spit.hk/v1` |
| Leave spit Credits alone | Product UI http://api.spit.hk:8092/ |

Live rebuilds → [`CLAUDE_CODE_DEPLOY.md`](./CLAUDE_CODE_DEPLOY.md).

---

## 6. Cross-links

- SP Chat twin: https://github.com/chrisyuspithk-bot/sp-chat/blob/main/docs/CLAUDE_CODE_2APP_CF_TUNNEL.md
- SP Chat live Credits: https://github.com/chrisyuspithk-bot/sp-chat/blob/main/docs/CLAUDE_CODE_DEPLOY.md
- This repo live rebuild: [`CLAUDE_CODE_DEPLOY.md`](./CLAUDE_CODE_DEPLOY.md)
