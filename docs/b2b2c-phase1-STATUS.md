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
- **Customer restrictions**: `BlockResellerCustomer` on online pay/amount (epay/Stripe/Creem/Waffo/Waffo-Pancake), subscription purchase, aff transfer, check-in. Redemption requires `redemption.reseller_id == user.reseller_id`. `/api/user/self` adds `reseller_id`, `is_reseller`, and `reseller` block (customers see only reseller display name, never cost/markup).
- **Cache invalidation**: in-process reseller/customer pricing caches (TTL 30s) invalidated on every write; status changes bump `auth_version` + publish user cache.
- **UI**: Phase 1c frontend (see below).

## How to migrate
1. Deploy; startup AutoMigrate adds columns/tables (MySQL/PG also widen users.quota/used_quota to bigint — table rewrite on big tables, run in a window). Existing users get `reseller_id=0` (direct).
2. Admin settings: turn off `RegisterEnabled` / `PasswordRegisterEnabled` and invitation quotas (registration was NOT hard-disabled in code).
3. Create reseller: `POST /api/admin/resellers {username,password,groups:[…],group,default_markup}` or `{user_id}` to upgrade a direct user; fund via `/credit {money_usd}`.
4. Rollback: deploy previous binary; new columns/tables are ignored.

## Tests run
- `go build ./...` OK; `go vet` on controller/model/service/middleware/router OK.
- `go test ./model ./service ./middleware ./controller ./relay/helper ./relay ./router -count=1` → all ok.
- New `service/reseller_billing_test.go`: dual-leg reserve/settle/refund, reseller-insufficient (nothing charged), customer-insufficient releases reseller leg, disabled reseller 403, group outside whitelist, markup precedence + cost multiplier, ceil rounding.
- SQLite fresh DB: started the server twice on the same DB — no errors; all new tables/indexes/columns present.
- **NOT run**: MySQL / PostgreSQL / ClickHouse verification and upgrade-from-release test (required by AGENTS.md before claiming DB compatibility).

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
7. **Registration**: first-time setup wizard (`POST /api/setup`) now persists `RegisterEnabled=false` and `PasswordRegisterEnabled=false`. Code defaults in `common/constants.go` were NOT changed (an existing deploy with no option row would silently close registration). **Existing deploys must toggle both off in admin settings.** Reseller customer creation does not depend on these flags.
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
   - Wallet: online top-up / Creem / Waffo / subscriptions / affiliate hidden for reseller customers; redemption still available; notice banner
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
| Customer | `/profile`, `/dashboard/overview`, `/wallet` | Reseller name/contact + top-up restrictions |

### DB verification (this environment)
- **Docker / podman / nerdctl**: not installed; no docker.sock → cannot spin MySQL/Postgres/ClickHouse containers.
- **Host mysqld / postgres / clickhouse binaries**: not present; apt install requires root (unavailable).
- **SQLite**: already verified in Phase 1 (fresh DB + restart; AutoMigrate adds all B2B2C tables/columns). Reconfirmed by package tests using SQLite.
- **ClickHouse**: blocked (no server / no Docker).
- **MySQL / PostgreSQL upgrade-from-release**: still required by AGENTS.md before claiming cross-DB compatibility — **blocked here**.

### Tests (Phase 1c)
- `go build ./...` OK; `go vet` controller/model/service/middleware/router OK.
- `go test ./model ./service ./middleware ./controller ./relay/helper ./relay ./router -count=1` → all ok.
- Frontend: `pnpm install` + `pnpm run build` OK (routeTree regenerates reseller routes).

## Still open / blocked
- MySQL / PostgreSQL / ClickHouse migration verification and upgrade-from-release test (no DB servers / Docker here) — required by AGENTS.md before claiming DB compatibility. New in 1b: `midjourneys.reseller_id/reseller_cost_quota`, `FOR UPDATE` on `reseller_profiles`.
- Design-exact pricing order (`charge = ceil(cost × markup)`) — deliberately not done; bounded error documented above.
- completion_ratio override for tiered-expr models; pre-WSS chunk estimate uses base completion ratio.
- Multi-node cache is TTL-based (≤10s); no Redis pub/sub.
- Reconciliation is report-only (no automatic job/repair); partial-settle detection relies on logs.
- Frontend i18n: new UI strings use English `t('…')` keys (same pattern as existing pages); no separate locale JSON entries added.
- Admin users table still lists resellers as role=5 rows (read-only manage via generic APIs is blocked for promote/demote/delete); prefer `/resellers` for commercial ops.
