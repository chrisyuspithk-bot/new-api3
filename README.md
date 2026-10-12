<div align="center">

![new-api](/web/public/logo.png)

# New API

**An AI gateway for models, applications, and agents**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <strong>English</strong> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/Calcium-Ion/new-api/main/LICENSE">
    <img src="https://img.shields.io/github/license/Calcium-Ion/new-api?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/Calcium-Ion/new-api/releases/latest">
    <img src="https://img.shields.io/github/v/release/Calcium-Ion/new-api?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://hub.docker.com/r/CalciumIon/new-api">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
  <a href="https://atomgit.com/QuantumNous/new-api" target="_blank">
    <img alt="AtomGit G-Star" src="https://atomgit.com/QuantumNous/new-api/star/badge.svg"/>
  </a>
</p>

<p align="center">
  <a href="https://trendshift.io/repositories/20180" target="_blank">
    <img src="https://trendshift.io/api/badge/repositories/20180" alt="QuantumNous%2Fnew-api | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/>
  </a>
  <br>
  <a href="https://hellogithub.com/repository/QuantumNous/new-api" target="_blank">
    <img src="https://api.hellogithub.com/v1/widgets/recommend.svg?rid=539ac4217e69431684ad4a0bab768811&claim_uid=tbFPfKIDHpc4TzR" alt="Featured｜HelloGitHub" style="width: 250px; height: 54px;" width="250" height="54" />
  </a><!--
  -->
  <a href="https://atomgit.com/QuantumNous/new-api" target="_blank">
    <img alt="AtomGit G-Star" src="https://atomgit.com/QuantumNous/new-api/star/new_badge.svg" width="250" height="55" />
  </a>
</p>

<p align="center">
  <a href="#screenshots">Screenshots</a> •
  <a href="#capabilities">Capabilities</a> •
  <a href="#quick-start">Quick start</a> •
  <a href="#deployment">Deployment</a> •
  <a href="#development">Development</a> •
  <a href="#documentation">Documentation</a>
</p>

</div>

---

## 📝 Project Description

New API is a self-hosted AI gateway for applications, agents, and teams. Connect upstream model services, expose a consistent API to your clients, and manage routing, access, usage, and costs in one place.

**This branch (`main` / `feat/b2b2c-phase1`) includes B2B2C Phase 1**: reseller roles, dual-leg billing, admin/reseller consoles, customer markups, and monthly invoices. See [docs/b2b2c-phase1-STATUS.md](./docs/b2b2c-phase1-STATUS.md).

Use it to share authorized model access across a team, switch providers without configuring every client again, or operate a private multi-model / reseller service with a web console. Upstreams include OpenAI, Anthropic, Google Gemini, Azure OpenAI, AWS Bedrock, Vertex AI, DeepSeek, Qwen, and other compatible services.

> [!IMPORTANT]
> - This project is intended solely for lawful and authorized AI API gateway, organization-level authentication, multi-model management, usage analytics, cost accounting, and private deployment scenarios.
> - Users must lawfully obtain upstream API keys, accounts, model services, and interface permissions, and must comply with upstream terms of service and applicable laws and regulations.
> - Users should ensure their use complies with upstream terms of service and applicable laws and regulations.
> - When providing generative AI services to the public, users should comply with applicable regulatory requirements and fulfill all filing, licensing, content safety, real-name verification, log retention, tax, and upstream authorization obligations required by their jurisdiction.

<!-- -->

> [!WARNING]
> When operating this project as a public generative AI service or API resale service, users should first complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations.

---

<a id="screenshots"></a>

## Screenshots

Accounts, channels, plugin installation states, usage, prices, and costs shown below are simulated data. The marketplace displays the official plugin catalog. Click an image to view it at full size.

| Usage dashboard | Plugin marketplace |
| --- | --- |
| [![Usage dashboard with simulated requests, costs, and performance metrics](assets/screenshots/dashboard.en.jpg)](assets/screenshots/dashboard.en.jpg) | [![Official plugin marketplace with simulated installation status](assets/screenshots/plugin-marketplace.en.jpg)](assets/screenshots/plugin-marketplace.en.jpg) |
| **Model square** | **Usage logs** |
| [![Model square with simulated model pricing and availability](assets/screenshots/models.en.jpg)](assets/screenshots/models.en.jpg) | [![Usage logs with simulated tokens, costs, and response times](assets/screenshots/usage-logs.en.jpg)](assets/screenshots/usage-logs.en.jpg) |

---

## 🤝 Trusted Partners

<p align="center">
  <em>No particular order</em>
</p>

<p align="center">
  <a href="https://www.cherry-ai.com/" target="_blank">
    <img src="./docs/images/cherry-studio.png" alt="Cherry Studio" height="80" />
  </a><!--
  --><a href="https://github.com/iOfficeAI/AionUi/" target="_blank">
    <img src="./docs/images/aionui.png" alt="Aion UI" height="80" />
  </a><!--
  --><a href="https://bda.pku.edu.cn/" target="_blank">
    <img src="./docs/images/pku.png" alt="Peking University" height="80" />
  </a><!--
  --><a href="https://www.aliyun.com/" target="_blank">
    <img src="./docs/images/aliyun.png" alt="Alibaba Cloud" height="80" />
  </a><!--
  --><a href="https://io.net/" target="_blank">
    <img src="./docs/images/io-net.png" alt="IO.NET" height="80" />
  </a>
</p>

---

## 🙏 Special Thanks

<p align="center">
  <a href="https://www.jetbrains.com/?from=new-api" target="_blank">
    <img src="https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.png" alt="JetBrains Logo" width="120" />
  </a>
</p>

<p align="center">
  <strong>Thanks to <a href="https://www.jetbrains.com/?from=new-api">JetBrains</a> for providing free open-source development license for this project</strong>
</p>

---

<a id="capabilities"></a>

## Capabilities

| Area | What you can do |
| --- | --- |
| Model access | Use OpenAI Chat Completions, Responses, Anthropic Messages, and Gemini APIs; stream responses and use tools, reasoning, and multimodal inputs where supported |
| Routing | Configure model mappings, channel priorities and weights, retries, channel affinity, and multiple upstream keys |
| Usage and costs | Manage quotas, subscriptions, usage logs, cache accounting, and expression-based pricing for different usage tiers |
| Access control | Manage users, groups, fine-grained permissions, and API key restrictions; use OAuth/OIDC, passkeys, two-factor authentication, and login session management |
| Asynchronous tasks | Extend image, video, and other task APIs with JavaScript plugins, including task status and output retrieval |
| Web console | Configure channels and models, inspect usage and audit logs, and try models in the playground; available in English, Simplified Chinese, Traditional Chinese, French, Japanese, Russian, and Vietnamese |
| B2B2C (this branch) | Admin resellers, reseller console (customers/tokens/markups/redemptions/logs/reports/invoices), dual-leg billing (customer charge + reseller cost), registration default-off (`POST /api/setup` closes public signup) |

### Protocols and endpoints

| Interface | Common endpoints |
| --- | --- |
| OpenAI Chat / Responses | `POST /v1/chat/completions`, `POST /v1/responses` |
| Anthropic Messages | `POST /v1/messages` |
| Gemini | `POST /v1beta/models/{model}:generateContent`, `POST /v1beta/models/{model}:streamGenerateContent` |
| Realtime / Responses WebSocket | `GET /v1/realtime`, `GET /v1/responses` (WebSocket upgrade) |
| Images / audio | `/v1/images/generations`, `/v1/images/edits`, `/v1/audio/speech`, `/v1/audio/transcriptions`, `/v1/audio/translations` |
| Embeddings / rerank | `POST /v1/embeddings`, `POST /v1/rerank` |
| Task plugins | `POST /v1/tasks/{pluginKey}`, `GET /v1/tasks/{taskId}`, plus routes declared by each plugin |

[RelayKit](./relaykit/README.md) provides request, response, and streaming conversion between the four text protocols. Available features depend on the channel, upstream model, and conversion path; protocol-specific tools and fields may not map exactly. WebSocket support also requires a compatible upstream and channel configuration.

This README describes the current source tree. Check the release notes for the version you deploy.

<a id="quick-start"></a>

## Quick start (B2B2C image from this repo)

> **Do not use `calciumion/new-api:latest` for this branch.** That Hub image does not include B2B2C. Build from the repository [Dockerfile](./Dockerfile) so the image matches this source tree.

### 1. Build the image

```bash
git clone https://github.com/chrisyuspithk-bot/new-api3.git
cd new-api3
# optional: git checkout feat/b2b2c-phase1   # same B2B2C code as main after merge

docker build -t new-api-b2b2c:local .
```

The Dockerfile builds the React console (Bun) then the Go binary, and exposes **port 3000**. Working directory inside the container is `/data`.

### 2. Run with SQLite (simplest)

Unset `SQL_DSN` → the server uses SQLite under `/data` (override path with `SQLITE_PATH` if needed). Persist `/data` with a volume:

```bash
mkdir -p data
docker run --name new-api-b2b2c -d --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -v "$(pwd)/data:/data" \
  new-api-b2b2c:local
```

- **Host port**: map any host port → container `3000` (default app listen port; override inside the container with env `PORT` or flag `--port`).
- Open [http://localhost:3000](http://localhost:3000). On first boot the setup wizard runs (or use the API below).

### 3. First-time setup — `POST /api/setup`

Creates the root admin (if none) and marks the system initialized. On this B2B2C branch it also persists **`RegisterEnabled=false`** and **`PasswordRegisterEnabled=false`** (customers are created by resellers).

Check status:

```bash
curl -s http://localhost:3000/api/setup
```

Initialize (only works while setup is incomplete):

```bash
curl -s -X POST http://localhost:3000/api/setup \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "root",
    "password": "YourStrongPassword!",
    "confirmPassword": "YourStrongPassword!",
    "SelfUseModeEnabled": false,
    "DemoSiteEnabled": false
  }'
```

Then sign in at `/sign-in`, add channels, create resellers under **Resellers**, and let resellers provision customers.

### 4. Make your first request

1. Add a channel with your upstream API key, available models, and group assignment; run a channel test.
2. Configure model pricing and ensure the user has quota.
3. Create an API key in the console with access to the same group and models.
4. Set your client's base URL to `http://localhost:3000/v1` for OpenAI-compatible clients and use the **New API-issued key**.

```bash
export NEW_API_KEY=sk-...
curl --fail-with-body http://localhost:3000/v1/models \
  -H "Authorization: Bearer ${NEW_API_KEY}"

curl --fail-with-body http://localhost:3000/v1/responses \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"your-enabled-model","input":"Hello!"}'
```

<a id="deployment"></a>

## Deployment

### SQLite vs `SQL_DSN`

| Mode | How | Notes |
| --- | --- | --- |
| **SQLite** (default) | Leave `SQL_DSN` unset | DB file under `/data` (or `SQLITE_PATH`). Fine for single-node / demo. Mount `/data` for persistence. |
| **MySQL / MariaDB** | Set `SQL_DSN`, e.g. `user:pass@tcp(host:3306)/dbname?parseTime=true` | MySQL ≥ 5.7.8 / MariaDB. B2B2C AutoMigrate widens quota columns to bigint on first start. |
| **PostgreSQL** | Set `SQL_DSN`, e.g. `postgresql://user:pass@host:5432/dbname` | PostgreSQL ≥ 9.6. Same AutoMigrate behavior. |

Optional separate log DB: `LOG_SQL_DSN` (also supports ClickHouse). See [docs/b2b2c-phase1-STATUS.md](./docs/b2b2c-phase1-STATUS.md) for schema verification notes.

### Run with MySQL / PostgreSQL

```bash
docker run --name new-api-b2b2c -d --restart unless-stopped \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -e SQL_DSN='postgresql://user:pass@db-host:5432/new-api' \
  -e REDIS_CONN_STRING='redis://:pass@redis-host:6379/0' \
  -v "$(pwd)/data:/data" \
  new-api-b2b2c:local
```

### Docker Compose

The stock [docker-compose.yml](./docker-compose.yml) still references `calciumion/new-api:latest`. For **this B2B2C branch**, build locally and point Compose at that image (or use `build: .`):

```yaml
services:
  new-api:
    build: .
    image: new-api-b2b2c:local
    ports:
      - "3000:3000"
    volumes:
      - ./data:/data
    environment:
      - TZ=Asia/Shanghai
      - SESSION_SECRET=replace-with-openssl-rand-hex-32
      # omit SQL_DSN for SQLite, or set it for Postgres/MySQL:
      # - SQL_DSN=postgresql://root:CHANGE_ME@postgres:5432/new-api
      # - REDIS_CONN_STRING=redis://:CHANGE_ME@redis:6379
```

```bash
docker compose up -d --build
docker compose logs -f new-api
```

Replace example DB/Redis passwords in both services and connection strings. For an HTTPS console set `SESSION_COOKIE_SECURE=true` and `SESSION_COOKIE_TRUSTED_URL` to the exact public HTTPS origin(s).

### Ports and environment

| Item | Default / purpose |
| --- | --- |
| Container listen port | **3000** (`PORT` env or `--port`) |
| Host publish | e.g. `-p 3000:3000` or `-p 127.0.0.1:3000:3000` |
| Data dir | `/data` (SQLite + uploads); mount a host volume |

| Variable | Purpose |
| --- | --- |
| `SQL_DSN` | Main database connection; **unset → SQLite** |
| `SQLITE_PATH` | Optional SQLite file path (when not using `SQL_DSN`) |
| `LOG_SQL_DSN` | Optional separate log database / ClickHouse |
| `REDIS_CONN_STRING` | Redis connection string (recommended multi-node / rate limits) |
| `SESSION_SECRET` | Persistent auth secret; **set a random value**; all nodes must share it |
| `CRYPTO_SECRET` | Defaults to `SESSION_SECRET`; nodes sharing Redis must match |
| `SESSION_COOKIE_SECURE` | `true` for HTTPS console (Secure cookies + strict refresh/logout origin checks) |
| `SESSION_COOKIE_TRUSTED_URL` | Required in Secure mode: comma-separated exact HTTPS origins (no paths/wildcards) |
| `TRUSTED_PROXIES` | Trusted reverse-proxy IPs/CIDRs, or `none` |
| `TZ` | Timezone, e.g. `Asia/Shanghai` |
| `PORT` | Listen port inside the container (default 3000) |

See the [environment example](./.env.example), [environment reference](https://docs.newapi.ai/en/docs/installation/config-maintenance/environment-variables), and [authentication and session guide](./docs/authentication.md). Compose `environment` / `env_file` injects vars into the container; copying `.env.example` alone does not.

For production: HTTPS reverse proxy with streaming/WebSocket support; back up the DB and `/data`; multi-node must share the main DB and auth secrets.

<a id="development"></a>

## Development and extensions

The backend uses Go and Gin. The web console uses React 19, TypeScript, Rsbuild, TanStack, and Tailwind CSS 4. Use Bun for frontend dependencies and scripts; see [go.mod](./go.mod) for the Go language baseline and [Dockerfile](./Dockerfile) for the container build toolchain.

Build the frontend before starting the backend, which embeds `web/dist`:

```bash
# Repository root
cd web
bun install --frozen-lockfile
bun run build
cd ..
go run .
```

In a second terminal, start the frontend development server:

```bash
cd web
bun run dev -- --port 5173
```

Open [http://localhost:5173](http://localhost:5173); the development server proxies API requests to the backend on port 3000. For a containerized development backend, see [docker-compose.dev.yml](./docker-compose.dev.yml) and the `make dev` target in [makefile](./makefile).

| Location | Responsibility |
| --- | --- |
| `router/`, `middleware/`, `controller/` | HTTP routes, access checks, and API handlers |
| `relay/` | Upstream adapters and request routing |
| `service/`, `model/` | Business logic and persistence |
| [relaykit/](./relaykit/README.md) | Independently buildable Go module for protocol DTOs and conversions |
| [plugins/tasks/](./plugins/tasks/) | JavaScript task plugins; see [Task Plugin API v1](./docs/plugin-api/v1.md) for authoring and host boundaries |
| `web/` | Web console; see [frontend conventions](./web/AGENTS.md) |
| [electron/](./electron/README.md) | Desktop wrapper and packaging |

Read [AGENTS.md](./AGENTS.md) before contributing. Run checks appropriate to your change, including `make test` for the Go modules and `bun run typecheck`, `bun run lint`, `bun run test`, and `bun run build` in `web/` for frontend changes. Changes to RelayKit must also pass `GOWORK=off go build ./...` from `relaykit/`.

<a id="documentation"></a>

## Documentation and community

| Resource | Link |
| --- | --- |
| Official documentation | [Guides](https://docs.newapi.ai/en/docs) · [Installation](https://docs.newapi.ai/en/docs/installation) · [API reference](https://docs.newapi.ai/en/docs/api) |
| Project exploration | [DeepWiki](https://deepwiki.com/QuantumNous/new-api) |
| Questions and discussion | [FAQ](https://docs.newapi.ai/en/docs/support/faq) · [Community](https://docs.newapi.ai/en/docs/support/community-interaction) |
| Bugs and feature requests | [GitHub Issues](https://github.com/QuantumNous/new-api/issues) |
| Security reports | Follow the [security policy](./.github/SECURITY.md) for private reporting |

For bug reports, include the version, deployment method, reproduction steps, and redacted logs. Documentation, translations, provider integrations, and focused regression tests are all welcome contributions.

---

## 🔗 Related Projects

### Upstream Projects

| Project | Description |
|------|------|
| [One API](https://github.com/songquanpeng/one-api) | Original project base |
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney interface support |

### Supporting Tools

| Project | Description |
|------|------|
| [new-api-key-tool](https://github.com/Calcium-Ion/new-api-key-tool) | Key quota query tool |
| [new-api-horizon](https://github.com/Calcium-Ion/new-api-horizon) | New API high-performance optimized version |

---

## 📜 License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE).

Additional terms under AGPLv3 Section 7 apply. Modified versions must preserve
the author attribution notice `Frontend design and development by New API
contributors.` in the appropriate legal notices and in any prominent about,
legal, footer, or attribution location presented by the user interface.

Modified versions that present a user interface must also preserve a visible
link to the original project: <https://github.com/QuantumNous/new-api>.

This is an open-source project developed based on [One API](https://github.com/songquanpeng/one-api) (MIT License).

If your organization's policies do not permit the use of AGPLv3-licensed software, or if you wish to avoid the open-source obligations of AGPLv3, please contact us at: [support@quantumnous.com](mailto:support@quantumnous.com)

See [NOTICE](./NOTICE) and [third-party licenses](./THIRD-PARTY-LICENSES.md) for attribution and dependency notices.

---

## 🌟 Star History

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=Calcium-Ion/new-api&type=Date)](https://star-history.com/#Calcium-Ion/new-api&Date)

</div>

---

<div align="center">

### 💖 Thank you for using New API

If this project is helpful to you, welcome to give us a ⭐️ Star！

**[Official Documentation](https://docs.newapi.ai/en/docs)** • **[Issue Feedback](https://github.com/Calcium-Ion/new-api/issues)** • **[Latest Release](https://github.com/Calcium-Ion/new-api/releases)**

<sub>Built with ❤️ by QuantumNous</sub>

</div>
