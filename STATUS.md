# STATUS

Canonical B2B2C Phase 1 status lives in [`docs/b2b2c-phase1-STATUS.md`](docs/b2b2c-phase1-STATUS.md).

**ClickHouse logs migrate (2026-10-12 18:50 HKT): PASS** — `logs.reseller_id` Int32, `logs.reseller_cost_quota` Int64 via `migrateClickHouseLogDB` (fresh + upgrade ADD COLUMN + idempotent).
