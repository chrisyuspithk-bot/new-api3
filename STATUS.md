# STATUS

Canonical B2B2C Phase 1 status lives in [`docs/b2b2c-phase1-STATUS.md`](docs/b2b2c-phase1-STATUS.md).

**Online self top-up (2026-10-12 ~19:10 HKT): DONE** — customers + resellers may use existing payment gateways; consume still requires reseller cost stock (dual-leg). See Product change section in the canonical STATUS.

**ClickHouse logs migrate (2026-10-12 18:50 HKT): PASS** — `logs.reseller_id` Int32, `logs.reseller_cost_quota` Int64 via `migrateClickHouseLogDB` (fresh + upgrade ADD COLUMN + idempotent).
