# new-api B2B2C Phase 1 — STATUS

Branch: `feat/b2b2c-phase1` (local clone /workspace/new-api; pushed to github.com/chrisyuspithk-bot/new-api3).
Design followed: /workspace/new-api-b2b2c-phase1-datamodel.md

## What landed
- **Roles**: `common.RoleResellerUser = 5`; `IsValidateRole` accepts it; `middleware.ResellerAuth()` (role must be exactly 5). Generic `POST /api/user/` rejects role=5 (use the reseller API so a profile exists).
- **Schema** (GORM AutoMigrate): `users.reseller_id` (+ `idx_users_reseller_id`, `idx_users_reseller_status`), `users.markup decimal(18,6) NULL`; `users.quota/used_quota` → bigint (explicit idempotent ALTER for MySQL/PG in `migrateUserQuotaColumnsToBigint`, SQLite no-op); new tables `reseller_profiles`, `reseller_groups`, `reseller_model_prices`, `reseller_model_markups`, `customer_model_markups`, `reseller_ledger`; `logs.reseller_id`, `logs.reseller_cost_quota` (+ `idx_logs_reseller_created`), ClickHouse `ADD COLUMN IF NOT EXISTS`; `redemptions.reseller_id`. LogTypes 8 (`LogTypeResellerCredit`) / 9 (`LogTypeResellerAdminCredit`). User cache schema bumped to 3 (carries `ResellerId`).
- **Pricing** (`service/reseller_pricing.go`): `ApplyResellerPricing` is called from `relay/helper.HandleGroupRatio` (single choke point for text/image/tiered/task billing) and folds `costFactor × markup` into GroupRatio, so every engine path produces the **customer charge**. Reseller cost = `ceil(charge / markup)`. Markup precedence: customer×model → customer → reseller×model → reseller default. Cost override: `cost_multiplier`, else `model_price/base_price`, else `model_ratio/base_ratio`.
- **Dual-leg BillingSession** (`service/billing_session.go`): reseller leg reserved first via atomic `TryReserveUserQuota` (never trust-bypassed, no overdraft at reserve), released on any later failure; `Reserve()` top-ups also cover reseller leg; Settle commits reseller leg first (may go negative once only on post-settle top-up, per design §5); Refund returns both legs. Disabled reseller / pricing failure / group outside whitelist → 403. Customers forced to `wallet_only` (no subscriptions). Settle stores cost for consume logs (`logs.reseller_id`, `reseller_cost_quota`).
- **Groups ⊆ reseller_groups**: enforced on customer create/update, token create/update (incl. auto groups), at TokenAuth (403), and at billing time after auto-group selection. Shrinking the whitelist moves offending customers to a fallback group.
- **APIs** (`controller/reseller.go`, `router/api-router.go`):
  - Admin (`AdminAuth`): `GET/POST /api/admin/resellers`, `GET/PUT/DELETE /api/admin/resellers/:id`, `POST …/:id/credit|debit` (quota or money_usd 1:1), `GET/PUT …/:id/groups`, `GET/PUT/DELETE …/:id/model_prices[/:model]`, `GET …/:id/ledger`, `GET /api/admin/reports/gross_profit?reseller_id&from&to&group_by=model|customer`.
  - Reseller (`ResellerAuth`, all scoped `reseller_id = me`, foreign ids → 404): `/api/reseller/self`, `PUT /self/markup`, `/ledger`, `/customers` CRUD + `/status` + `/credit|debit`, `/model_markups`, `/customers/:id/model_markups`, `/redemptions` (reseller-scoped codes), `/logs` (charge/cost/profit), `/reports/gross_profit`.
- **Customer restrictions**: `BlockResellerCustomer` on subscription purchase, aff transfer, check-in (NOT on online wallet top-up — see Product change below). Redemption requires `redemption.reseller_id == user.reseller_id`. `/api/user/self` adds `reseller_id`, `is_reseller`, and `reseller` block (customers see only reseller display name, never cost/markup; `online_topup_enabled: true`).
- **Cache invalidation**: in-process reseller/customer pricing caches (TTL 30s) invalidated on every write; status changes bump `auth_version` + publish user cache.
- **UI**: Phase 1c frontend (see below).

## How to migrate
1. Deploy; startup AutoMigrate adds columns/tables (MySQL/PG also widen users.quota/used_quota to bigint — table rewrite on big tables, run in a window). Existing users get `reseller_id=0` (direct).
2. Admin settings: for upgrades that still have option rows with registration on, turn off `RegisterEnabled` / `PasswordRegisterEnabled` and invitation quotas (new installs default both off in code + setup wizard).
3. Create reseller: `POST /api/admin/resellers {username,password,groups:[…],group,default_markup}` or `{user_id}` to upgrade a direct user; fund via `/credit {money_usd}`.
4. Rollback: deploy previous binary; new columns/tables are ignored.

## Tests run
- `go build ./...` OK; `go vet` on controller/model/service/middleware/router OK.
- `go test ./model ./service ./middleware ./controller ./relay/helper ./relay ./router -count=1` → all ok.
- New `service/reseller_billing_test.go`: dual-leg reserve/settle/refund, reseller-insufficient (nothing charged), customer-insufficient releases reseller leg, disabled reseller 403, group outside whitelist, markup precedence + cost multiplier, ceil rounding.
- SQLite fresh DB: started the server twice on the same DB — no errors; all new tables/indexes/columns present.
- **MySQL(MariaDB) / PostgreSQL**: verified 2026-10-12 (fresh + upgrade-from-int32) — see DB verification section. **ClickHouse**: verified 2026-10-12 18:50 HKT (fresh CREATE + upgrade ADD COLUMN + idempotent re-run) — see DB verification section.

## Phase 1b (leftovers pass, 2026-10-12)

### Done
1. **Async task completion + Midjourney reseller leg**
   - `TaskBillingContext` now freezes `reseller_id`, `reseller_markup`, `reseller_cost_factor` at submit (JSON in `private_data`, no schema change).
   - `RefundTaskQuota` returns `ceil(q/markup)` to the reseller; `RecalculateTaskQuota` moves the reseller leg by `cost(actual) - cost(pre)` (positive delta may take reseller below zero once, same as sync settle, design §5). `RecalculateTaskQuotaByTokens` re-applies `costFactor × markup` so the recomputed quota stays the customer charge. Refund/delta logs carry `reseller_id` + `reseller_cost_quota`.
   - Midjourney: `CheckMidjourneyResellerBilling` fails closed **before** the upstream call (pricing error / reseller balance < cost); `SettleMidjourneyTaskBilling` charges the reseller leg; `RefundMidjourneyQuota` refunds it. New columns `midjourneys.reseller_id`, `midjourneys.reseller_cost_quota` (AutoMigrate, default 0). Consume logs carry the reseller leg.
   - Realtime WSS (`PreWssConsumeQuota`) was also bypassing reseller pricing: now folds `costFactor × markup` and atomically reserves the reseller leg per chunk (mirrors the customer-leg pattern of that path).
   - Gross-profit report now nets `LogTypeRefund` rows (charge and cost) and counts requests from consume rows only.
   - Tasks submitted **before** this deploy have no reseller fields → completion refund/delta does not touch the reseller leg for them (in-flight only; one-time).
2. **completion_ratio override applied**: `service.ResellerCompletionRatio` replaces the completion ratio in `ModelPriceHelper` (text path via `PriceData.CompletionRatio`) and in the WSS/audio post-settle paths (`completionRatioFor`). Exact because group ratio already carries `costFactor`. Not applied to tiered-expr models (use `cost_multiplier`) or `calculateAudioQuota` pre-WSS chunk estimate (still base completion ratio).
3. **Cost derivation order**: kept engine order (charge computed by engine with `costFactor × markup` folded into group ratio; `cost = ceil(charge/markup)`). Changing to design's `charge = ceil(cost × markup)` would require rewriting every billing engine path — not done. Exact bound documented + tested (`TestResellerCostDerivationMaxError`, 200k random cases): for markup ≥ 1, `cost − round(base×costFactor) ∈ {0, +1}` quota and `charge ≥ cost` (reseller never under-collects); for markup < 1 the error is ≤ `0.5/markup + 1.5` quota. Customer charge vs design differs by < markup + 1 quota (1 quota = $0.000002).
4. **/api/pricing**: for reseller customers, `model_ratio`/`model_price` are multiplied by `costFactor × markup` (their actual price), completion ratio override shown, `billing_expr` / plugin variants / usage schema removed, groups limited to reseller whitelist. Resellers see their own cost (`costFactor` only). Direct/anonymous users unchanged. **Admin action**: set the pricing nav module to "require login" and keep `ExposeRatioEnabled` off (`/api/ratio_config`), otherwise a logged-out customer can still see base prices.
5. **Reseller customer delete + token CRUD**: `DELETE /api/reseller/customers/:id` = disable user + revoke sessions + disable all its tokens (rows kept for logs/ledger). `GET/POST /api/reseller/customers/:id/tokens`, `PUT/DELETE /api/reseller/customers/:id/tokens/:token_id` — scoped to own customers (404 otherwise), group must be in reseller whitelist (`auto` rejected), max-user-tokens enforced; full key returned once on create.
6. **Transactions**: `AdjustQuotaWithLedger` = conditional `UPDATE users SET quota = quota + ?` (debit guarded by `quota >= ?`) + ledger insert in one DB transaction; Redis user cache dropped after commit. `CreateResellerCustomer` does profile row lock (`FOR UPDATE` on MySQL/PG) + count + insert + zero quota in one transaction. Tests: `model/reseller_tx_test.go`.
7. **Registration**: first-time setup wizard (`POST /api/setup`) persists `RegisterEnabled=false` and `PasswordRegisterEnabled=false`. Code defaults in `common/constants.go` are now also `false` (new installs / no option row start closed). OAuth/OIDC/social signup is gated by `RegisterEnabled` in `controller/oauth.go` (and WeChat). Existing users can still login/bind. **Existing deploys with option rows still set to true must toggle both off in admin settings.** Reseller customer creation does not depend on these flags.
8. **Reconciliation**: `GET /api/admin/reports/reseller_reconcile?from=<unix>` (AdminAuth, read-only) flags resellers with negative balance, consume logs with charge but no reseller cost, and logs where cost > charge.
9. **Cache**: reseller/customer pricing cache TTL 30s → 10s (writes still invalidate locally). No Redis pub/sub cross-node invalidation.
10. New reseller/admin-reseller routes declared as session-only in `middleware/access_token_routes.go` (access tokens rejected) — fixes `TestAccessTokenRouteRulesCoverEveryDashboardRoute`, which was failing since phase 1.

### Tests (Phase 1b)
- `go build ./...` OK; `go vet` model/service/controller/router/relay/relay/helper OK.
- `go test ./model ./service ./middleware ./controller ./relay/helper ./relay ./router -count=1` → all ok.
- New: `service/reseller_phase1b_test.go` (task refund/recalc reseller leg, MJ charge+refund+precheck, completion_ratio override, cost-derivation max-error), `model/reseller_tx_test.go` (ledger atomicity, max_customers).

## Phase 1c (frontend + consistency, 2026-10-12)

### Done
1. **Admin Resellers UI** (`web/src/features/resellers`, routes `/resellers`, `/resellers/$id`)
   - List/create/edit/disable; credit/debit (USD → cost balance); detail tabs: overview, groups whitelist, model cost overrides, ledger.
   - Embedded gross-profit + reconcile panels on the list page (wired to `/api/admin/reports/gross_profit` and `/api/admin/reports/reseller_reconcile`).
2. **Reseller console** (`web/src/features/reseller-console`, role===5 only)
   - `/reseller` overview (balance, default markup, ledger)
   - `/reseller/customers` + `/reseller/customers/$id` (CRUD-ish, status, credit, tokens, per-customer model markups)
   - `/reseller/markups`, `/reseller/redemptions`, `/reseller/logs`, `/reseller/reports`
3. **Customer-facing**
   - Profile: `ResellerContactCard` (brand/display name + top-up guidance)
   - Dashboard overview banner when `user.reseller.role === 'customer'`
   - Wallet: online top-up / Creem / Waffo enabled for reseller customers (same gateways); subscriptions / affiliate still hidden; reseller contact kept on profile
4. **Roles / sidebar**
   - Frontend `ROLE.RESELLER = 5`; users role filter includes 5; auth store carries `reseller_id` / `reseller`
   - Sidebar admin item "Resellers"; new `reseller` nav group gated to exact role 5
   - Backend default sidebar configs + `calculateUserPermissions` include reseller modules; login/refresh/`buildSelfUserData` now attach the `reseller` block (not only GetSelf)
5. **Admin generic user manage consistency**
   - `ManageUser` promote/demote/delete reject `RoleResellerUser`; promote also rejects `reseller_id > 0` customers (must use reseller APIs)

### UI routes
| Audience | Path | Purpose |
|---|---|---|
| Admin | `/resellers` | Reseller list + create + credit/debit + reports |
| Admin | `/resellers/$id` | Detail: overview / groups / model prices / ledger |
| Reseller | `/reseller` | Overview + markup + ledger |
| Reseller | `/reseller/customers` | Customer list |
| Reseller | `/reseller/customers/$id` | Customer detail, tokens, markups |
| Reseller | `/reseller/markups` | Default per-model markups |
| Reseller | `/reseller/redemptions` | Reseller-scoped codes |
| Reseller | `/reseller/logs` | Charge / cost / profit logs |
| Reseller | `/reseller/reports` | Gross profit |
| Customer | `/profile`, `/dashboard/overview`, `/wallet` | Reseller name/contact + online self top-up |

### DB verification (this environment)
- See **DB verification (2026-10-12)** section below (MariaDB + PostgreSQL + ClickHouse PASS).
- **SQLite**: already verified in Phase 1 (fresh DB + restart).

### Tests (Phase 1c)
- `go build ./...` OK; `go vet` controller/model/service/middleware/router OK.
- `go test ./model ./service ./middleware ./controller ./relay/helper ./relay ./router -count=1` → all ok.
- Frontend: `pnpm install` + `pnpm run build` OK (routeTree regenerates reseller routes).

## DB verification (2026-10-12; ClickHouse 18:50 HKT)

Environment: Debian 13 (trixie) box; installed via apt (no Docker):
- **MariaDB** `11.8.6-MariaDB-0+deb13u1` (MySQL-compatible; `mysql-server` package unavailable on Trixie — used `mariadb-server`)
- **PostgreSQL** `17.11 (Debian 17.11-0+deb13u1)`
- **ClickHouse** `26.9.16.11` (official `packages.clickhouse.com/deb stable`; not in Debian stock apt)
- Harness: `cmd/verify-b2b2c-schema` (MySQL/PG) and `cmd/verify-clickhouse-b2b2c` (ClickHouse logs) call `model.InitDB` + `model.InitLogDB` (full migrate path) twice for idempotency, then assert columns/types.

### Fresh migrate — MariaDB **PASS** (2/2 passes)
DSN: `newapi:***@tcp(127.0.0.1:3306)/newapi?parseTime=true&charset=utf8mb4`

| Check | Result |
|---|---|
| `users.reseller_id` | PASS — `bigint(20) NOT NULL DEFAULT 0`, indexes `idx_users_reseller_id`, `idx_users_reseller_status(reseller_id,status)` |
| `users.markup` | PASS — `decimal(18,6) NULL` |
| `users.quota` / `used_quota` | PASS — `bigint(20) DEFAULT 0` |
| tables `reseller_profiles`, `reseller_groups`, `reseller_model_prices`, `reseller_model_markups`, `customer_model_markups`, `reseller_ledger` | PASS |
| `logs.reseller_id`, `logs.reseller_cost_quota` | PASS — `bigint(20) DEFAULT 0`; index `idx_logs_reseller_created(reseller_id,created_at)` |
| `redemptions.reseller_id` | PASS — `bigint(20) NOT NULL DEFAULT 0`, index |
| `midjourneys.reseller_id`, `reseller_cost_quota` | PASS — `bigint(20) DEFAULT 0` |
| Idempotent second startup | PASS |

### Fresh migrate — PostgreSQL **PASS** (2/2 passes)
DSN: `postgres://newapi:***@127.0.0.1:5432/newapi?sslmode=disable`

| Check | Result |
|---|---|
| `users.reseller_id` | PASS — `bigint NOT NULL DEFAULT 0`, indexes `idx_users_reseller_id`, `idx_users_reseller_status` |
| `users.markup` | PASS — `numeric` / `numeric(18,6)` NULL |
| `users.quota` / `used_quota` | PASS — `bigint` (`int8`) DEFAULT 0 |
| six reseller tables | PASS (unique indexes `uniq_reseller_group`, `uniq_rmp`, `uniq_rmm`, `uniq_cmm`) |
| `logs.reseller_id`, `logs.reseller_cost_quota` | PASS — `bigint` DEFAULT 0; `idx_logs_reseller_created` |
| `redemptions.reseller_id` | PASS |
| `midjourneys.reseller_id`, `reseller_cost_quota` | PASS |
| Idempotent second startup | PASS |

### Upgrade-from-int32 (representative pre-B2B2C schema) — MariaDB & PG
Minimal legacy tables with `INT`/`INTEGER` `users.quota`/`used_quota` and **no** B2B2C columns; one row preserved (`quota=100`, `used_quota=10`).

1. **Without** `SKIP_64BIT_QUOTA_SCHEMA_CHECK`: **FAIL as designed** — `ensureUserQuotaColumns` rejects before migrate:
   - MySQL: `users.quota uses int; 32-bit is not supported`
   - PG: `users.quota uses int4; 32-bit is not supported`
2. **With** `SKIP_64BIT_QUOTA_SCHEMA_CHECK=true`: **PASS** on both — `migrateUserQuotaColumnsToBigint` widens columns, AutoMigrate adds B2B2C columns/tables; row data preserved (`reseller_id=0`, `markup=NULL`).

**Exact SQL issued by `migrateUserQuotaColumnsToBigint`** (when column is not already bigint):
```sql
-- MySQL / MariaDB
ALTER TABLE users MODIFY COLUMN quota bigint DEFAULT 0;
ALTER TABLE users MODIFY COLUMN used_quota bigint DEFAULT 0;

-- PostgreSQL
ALTER TABLE users ALTER COLUMN quota TYPE bigint;
ALTER TABLE users ALTER COLUMN used_quota TYPE bigint;
```
All other B2B2C columns/tables come from GORM `AutoMigrate` (no hand-written DDL beyond the bigint widen + ClickHouse `ADD COLUMN IF NOT EXISTS` below).

### ClickHouse logs — **PASS** (2026-10-12 18:50 HKT)
Environment: Debian 13 (trixie) box; installed ClickHouse from official apt (`packages.clickhouse.com/deb stable`):
- **clickhouse-server / clickhouse-client** `26.9.16.11`
- Primary DB: SQLite scratch (`SQLITE_PATH`); log DB: `LOG_SQL_DSN=clickhouse://default:@127.0.0.1:9000/new_api_logs`
- Path exercised: `model.InitLogDB` → `migrateLOGDB` → `migrateClickHouseLogDB` (via `cmd/verify-clickhouse-b2b2c`)

**SQL issued by `migrateClickHouseLogDB`** (after `CREATE TABLE IF NOT EXISTS logs (... reseller_id Int32 ..., reseller_cost_quota Int64 ...)`):
```sql
ALTER TABLE logs ADD COLUMN IF NOT EXISTS reseller_id Int32 DEFAULT 0;
ALTER TABLE logs ADD COLUMN IF NOT EXISTS reseller_cost_quota Int64 DEFAULT 0;
```

| Scenario | Result |
|---|---|
| Upgrade: pre-create legacy `logs` **without** reseller columns, then InitLogDB | **PASS** — `ADD COLUMN IF NOT EXISTS` added `reseller_id Int32 DEFAULT 0`, `reseller_cost_quota Int64 DEFAULT 0` |
| Fresh: drop `logs`, InitLogDB `CREATE TABLE IF NOT EXISTS` | **PASS** — both columns present in `system.columns` / `SHOW CREATE TABLE` |
| Idempotent second InitLogDB | **PASS** (no error) |
| Smoke INSERT/SELECT of reseller cols | **PASS** — wrote `reseller_id=42`, `reseller_cost_quota=1000`, read back |

Asserted types (`system.columns`): `reseller_id` = `Int32`, `reseller_cost_quota` = `Int64`, both `DEFAULT 0`.

### How to re-run
```bash
# MariaDB/MySQL
SQL_DSN='user:pass@tcp(127.0.0.1:3306)/dbname?parseTime=true&charset=utf8mb4' \
  go run ./cmd/verify-b2b2c-schema/

# PostgreSQL
SQL_DSN='postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable' \
  go run ./cmd/verify-b2b2c-schema/

# Legacy int32 wallet upgrade
SKIP_64BIT_QUOTA_SCHEMA_CHECK=true SQL_DSN='...' go run ./cmd/verify-b2b2c-schema/

# ClickHouse logs (needs clickhouse-server; empty default password OK for local)
SQLITE_PATH=/tmp/verify-ch-b2b2c-main.db \
LOG_SQL_DSN='clickhouse://default:@127.0.0.1:9000/new_api_logs' \
SKIP_64BIT_QUOTA_SCHEMA_CHECK=true \
  go run ./cmd/verify-clickhouse-b2b2c/
```

### Errors observed
- Fresh MySQL/PG: none.
- Upgrade without skip flag: expected `ensureUserQuotaColumns` rejection (operators must set `SKIP_64BIT_QUOTA_SCHEMA_CHECK=true` once to allow the bigint widen, or pre-ALTER manually).
- ClickHouse: none on migrate path (CREATE + ADD COLUMN + idempotent re-run). Inline `INSERT ... VALUES (..., toUnixTimestamp(now()), ...)` via native client hung once in this environment; HTTP insert and `FORMAT Values` succeeded — not an app migrate issue.


## Product change — online self top-up (2026-10-12 ~19:10 HKT)

Chris product decision: re-enable customer online self top-up; allow reseller self top-up; keep consume gated on reseller stock.

### Done
1. **Customer online top-up re-enabled** — removed `BlockResellerCustomer` from wallet pay/amount routes: epay (`/pay`, `/amount`), Stripe, Creem, Waffo, Waffo-Pancake. Subscriptions, aff transfer, and check-in remain blocked for reseller customers.
2. **Reseller self top-up** — resellers already have sidebar `personal.topup` and `reseller_id=0`, so the same payment pipelines credit `users.quota` (cost balance) 1:1 USD via existing `Recharge*` paths. Overview UI adds **Top up cost balance** → `/wallet`.
3. **Stock policy (prefer A)** — customer top-up credits the customer wallet only; **consume still requires reseller cost credit** via existing dual-leg `BillingSession` (no change). Documented in `/api/user/self` (`online_topup_enabled: true`) and profile reseller contact copy. Top-up is not gated on reseller stock.
4. **UI**
   - Wallet: removed "managed by reseller" banner and frontend gating that forced `enable_*_topup=false` for customers.
   - Profile `ResellerContactCard`: keep contact; copy now says online top-up is available and consume needs reseller stock.
   - Dashboard overview: "You can top up online; contact them for support."
   - Reseller overview: top-up entry button.
5. **Tests**: `controller/reseller_topup_policy_test.go` — `online_topup_enabled` for customer+reseller; `BlockResellerCustomer` still blocks non-topup; pay routes no longer mount the middleware.

## Product change — public registration default-off (2026-10-12 ~19:20 HKT)

Chris product decision: hard-disable / default-off public registration and OAuth/OIDC/social signup; existing users may still login/bind.

### Done
1. **Code defaults** — `common.RegisterEnabled=false`, `common.PasswordRegisterEnabled=false` in `common/constants.go` (OptionMap + new installs inherit).
2. **OAuth/OIDC/social signup** — `findOrCreateOAuthUser` already refuses create when `!RegisterEnabled` (`OAuthRegistrationDisabledError`); WeChat signup path same gate. Login/bind for existing users unchanged. No separate per-provider register flags (provider Enabled only controls login availability).
3. **Status API** — `/api/status` exposes `oauth_register_enabled` (= `RegisterEnabled`) so the sign-up UI does not fall back to showing OAuth signup.
4. **Frontend** — admin auth settings defaults and sign-up OAuth block fall back to off / `register_enabled`.
5. **Setup wizard** — still persists both options false (idempotent with code defaults).

### Note for existing deploys
If `options` already has `RegisterEnabled=true` / `PasswordRegisterEnabled=true`, those rows win over code defaults until an admin turns them off.


## Monthly invoices (2026-10-12 ~20:23 HKT)

### Done
1. **Schema** — `invoices` table (AutoMigrate): unique `(period, scope, reseller_id, customer_id)`; stores requests, charge/cost/profit quota + USD (`quota / QuotaPerUnit`), line items JSON by model, generated_at/by.
2. **Generate** — `POST /api/admin/invoices/generate {period:"YYYY-MM"}` aggregates consume+refund logs for the Asia/Hong_Kong calendar month into:
   - one **customer** invoice per active customer with usage
   - one **reseller** invoice per reseller (sum of customers)
   - one **admin_rollup** for all resellers/customers
   Upsert on re-generate.
3. **APIs**
   - Admin: `GET /api/admin/invoices`, `GET /api/admin/invoices/:id` (`?format=csv`), `POST /api/admin/invoices/generate`
   - Reseller: `GET /api/reseller/invoices`, `GET /api/reseller/invoices/:id` (own + customers only; admin_rollup hidden)
   - Customer: `GET /api/user/invoices`, `GET /api/user/invoices/:id` (own customer scope only)
4. **UI** — Admin `/invoices`; Reseller `/reseller/invoices`; Customer wallet `InvoiceHistoryCard` (reseller customers).
5. **Tests** — `model/invoice_test.go` (period parse HKT, aggregate+refund net, upsert, reseller restrict). `go test ./model ./middleware ./controller ./router` OK.

### QA
- Traditional Chinese report: `docs/b2b2c-pro-qa/REPORT-invoices.md` / `REPORT-invoices.pdf` (2026-10-12 ~20:26 HKT); Admin/Reseller/Customer API+UI smoke Pass on SQLite `:3310`.

### UI routes
| Audience | Path | Purpose |
|---|---|---|
| Admin | `/invoices` | List/generate/detail + CSV |
| Reseller | `/reseller/invoices` | Own + customer invoices + CSV |
| Customer | `/wallet` | Invoice history card |

## Still open / blocked
- MySQL(MariaDB)/PostgreSQL/ClickHouse schema migrate verified 2026-10-12 — see DB verification section. No remaining DB-dialect blockers for Phase 1 schema.
- Design-exact pricing order (`charge = ceil(cost × markup)`) — deliberately not done; bounded error documented above.
- completion_ratio override for tiered-expr models; pre-WSS chunk estimate uses base completion ratio.
- Multi-node cache is TTL-based (≤10s); no Redis pub/sub.
- Reconciliation is report-only (no automatic job/repair); partial-settle detection relies on logs.
- Frontend i18n: new UI strings use English `t('…')` keys (same pattern as existing pages); no separate locale JSON entries added.
- Admin users table still lists resellers as role=5 rows (read-only manage via generic APIs is blocked for promote/demote/delete); prefer `/resellers` for commercial ops.

### Boss requirements QA (2026-10-12 ~21:08 HKT)
Traditional Chinese report: `docs/b2b2c-boss-qa/REPORT-boss.md` + `REPORT-boss.pdf`. Functional TCs mapped to boss checklist (Admin/Reseller/Customer/dual deduct/stock/register off/monthly invoices): **24 Pass / 0 Fail**; UI screenshots 17 Pass.
