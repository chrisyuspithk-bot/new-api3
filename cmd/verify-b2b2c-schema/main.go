// verify-b2b2c-schema runs model.InitDB/InitLogDB (AutoMigrate) against SQL_DSN
// and asserts B2B2C Phase 1 columns/tables. Intended for MySQL/MariaDB and PostgreSQL.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func main() {
	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "SQL_DSN is required")
		os.Exit(2)
	}
	pass := 0
	os.Args = []string{os.Args[0]} // avoid flag.Parse eating harness args
	common.InitEnv()
	common.IsMasterNode = true

	fmt.Printf("=== migrate pass 1 dialect=%s ===\n", dialectOf(dsn))
	if err := model.InitDB(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL InitDB: %v\n", err)
		os.Exit(1)
	}
	if err := model.InitLogDB(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL InitLogDB: %v\n", err)
		os.Exit(1)
	}
	if err := assertB2B2C(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL assert after pass 1: %v\n", err)
		os.Exit(1)
	}
	pass++
	fmt.Println("PASS assert after pass 1")

	// Idempotency: close and re-init
	if sqlDB, err := model.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	fmt.Println("=== migrate pass 2 (idempotency) ===")
	if err := model.InitDB(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL InitDB pass2: %v\n", err)
		os.Exit(1)
	}
	if err := model.InitLogDB(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL InitLogDB pass2: %v\n", err)
		os.Exit(1)
	}
	if err := assertB2B2C(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL assert after pass 2: %v\n", err)
		os.Exit(1)
	}
	pass++
	fmt.Println("PASS assert after pass 2")
	fmt.Printf("OK dialect=%s passes=%d\n", dialectOf(dsn), pass)
}

func dialectOf(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return "postgres"
	}
	return "mysql"
}

func assertB2B2C() error {
	db := model.DB
	dialector := db.Dialector.Name()

	requiredTables := []string{
		"reseller_profiles",
		"reseller_groups",
		"reseller_model_prices",
		"reseller_model_markups",
		"customer_model_markups",
		"reseller_ledger",
		"revenue_records",
		"revenue_monthly_snapshots",
		"quota_refund_journal",
	}
	for _, t := range requiredTables {
		if !db.Migrator().HasTable(t) {
			return fmt.Errorf("missing table %s", t)
		}
		fmt.Printf("  table OK: %s\n", t)
	}

	type colNeed struct {
		table, column string
	}
	cols := []colNeed{
		{"users", "reseller_id"},
		{"users", "markup"},
		{"users", "quota"},
		{"users", "used_quota"},
		{"logs", "reseller_id"},
		{"logs", "reseller_cost_quota"},
		{"redemptions", "reseller_id"},
		{"midjourneys", "reseller_id"},
		{"midjourneys", "reseller_cost_quota"},
	}
	for _, c := range cols {
		if !db.Migrator().HasColumn(c.table, c.column) {
			return fmt.Errorf("missing column %s.%s", c.table, c.column)
		}
		fmt.Printf("  column OK: %s.%s\n", c.table, c.column)
	}

	// quota / used_quota must be bigint on MySQL/PG
	for _, column := range []string{"quota", "used_quota"} {
		var dataType string
		var err error
		switch dialector {
		case "postgres":
			err = db.Raw(`SELECT data_type FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = ?`, column).Scan(&dataType).Error
		case "mysql":
			err = db.Raw(`SELECT DATA_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = 'users' AND column_name = ?`, column).Scan(&dataType).Error
		default:
			return fmt.Errorf("unexpected dialector %s", dialector)
		}
		if err != nil {
			return err
		}
		norm := strings.ToLower(strings.TrimSpace(dataType))
		ok := norm == "bigint" || norm == "int8"
		fmt.Printf("  type users.%s = %s (bigint required)\n", column, dataType)
		if !ok {
			return fmt.Errorf("users.%s type %q is not bigint", column, dataType)
		}
	}

	// logs.reseller_cost_quota bigint
	{
		var dataType string
		var err error
		switch dialector {
		case "postgres":
			err = db.Raw(`SELECT data_type FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = 'logs' AND column_name = 'reseller_cost_quota'`).Scan(&dataType).Error
		case "mysql":
			err = db.Raw(`SELECT DATA_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = 'logs' AND column_name = 'reseller_cost_quota'`).Scan(&dataType).Error
		}
		if err != nil {
			return err
		}
		norm := strings.ToLower(strings.TrimSpace(dataType))
		fmt.Printf("  type logs.reseller_cost_quota = %s\n", dataType)
		if !(norm == "bigint" || norm == "int8") {
			return fmt.Errorf("logs.reseller_cost_quota type %q is not bigint", dataType)
		}
	}

	// Dump exact column defs for STATUS docs
	fmt.Println("  --- column dump ---")
	dumpSQL := map[string]string{
		"mysql": `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_KEY
			FROM information_schema.columns
			WHERE table_schema = DATABASE() AND (
				(TABLE_NAME='users' AND COLUMN_NAME IN ('reseller_id','markup','quota','used_quota'))
				OR (TABLE_NAME='logs' AND COLUMN_NAME IN ('reseller_id','reseller_cost_quota'))
				OR (TABLE_NAME='redemptions' AND COLUMN_NAME='reseller_id')
				OR (TABLE_NAME='midjourneys' AND COLUMN_NAME IN ('reseller_id','reseller_cost_quota'))
			) ORDER BY TABLE_NAME, COLUMN_NAME`,
		"postgres": `SELECT table_name, column_name, data_type, udt_name, is_nullable, column_default
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND (
				(table_name='users' AND column_name IN ('reseller_id','markup','quota','used_quota'))
				OR (table_name='logs' AND column_name IN ('reseller_id','reseller_cost_quota'))
				OR (table_name='redemptions' AND column_name='reseller_id')
				OR (table_name='midjourneys' AND column_name IN ('reseller_id','reseller_cost_quota'))
			) ORDER BY table_name, column_name`,
	}
	rows, err := db.Raw(dumpSQL[dialector]).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	colsMeta, _ := rows.Columns()
	for rows.Next() {
		vals := make([]any, len(colsMeta))
		ptrs := make([]any, len(colsMeta))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		parts := make([]string, len(colsMeta))
		for i, v := range vals {
			switch t := v.(type) {
			case []byte:
				parts[i] = string(t)
			case nil:
				parts[i] = "NULL"
			default:
				parts[i] = fmt.Sprint(t)
			}
		}
		fmt.Printf("  %s\n", strings.Join(parts, " | "))
	}
	return nil
}
