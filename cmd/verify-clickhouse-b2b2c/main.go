// verify-clickhouse-b2b2c exercises model.InitLogDB → migrateClickHouseLogDB
// for B2B2C logs columns (reseller_id, reseller_cost_quota).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func main() {
	logDSN := strings.TrimSpace(os.Getenv("LOG_SQL_DSN"))
	if logDSN == "" {
		fmt.Fprintln(os.Stderr, "LOG_SQL_DSN is required (clickhouse://...)")
		os.Exit(2)
	}
	// Primary DB: SQLite scratch (ClickHouse cannot be main DB)
	_ = os.Setenv("SQLITE_PATH", os.Getenv("SQLITE_PATH"))
	if os.Getenv("SQLITE_PATH") == "" {
		_ = os.Setenv("SQLITE_PATH", "/tmp/verify-ch-b2b2c-main.db")
	}
	_ = os.Unsetenv("SQL_DSN")

	os.Args = []string{os.Args[0]}
	common.InitEnv()
	common.IsMasterNode = true

	fmt.Println("=== InitDB (SQLite primary) ===")
	if err := model.InitDB(); err != nil {
		fatalf("InitDB: %v", err)
	}
	fmt.Println("PASS InitDB")

	fmt.Println("=== InitLogDB pass 1 (ClickHouse migrate) ===")
	if err := model.InitLogDB(); err != nil {
		fatalf("InitLogDB pass1: %v", err)
	}
	fmt.Println("PASS InitLogDB pass1")
	if err := assertResellerCols("after fresh/create migrate"); err != nil {
		fatalf("%v", err)
	}

	// Idempotency
	if sqlDB, err := model.LOG_DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	fmt.Println("=== InitLogDB pass 2 (idempotency) ===")
	if err := model.InitLogDB(); err != nil {
		fatalf("InitLogDB pass2: %v", err)
	}
	fmt.Println("PASS InitLogDB pass2")
	if err := assertResellerCols("after idempotent migrate"); err != nil {
		fatalf("%v", err)
	}

	fmt.Println("OK ClickHouse B2B2C logs columns verified")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL "+format+"\n", args...)
	os.Exit(1)
}

func assertResellerCols(label string) error {
	type row struct {
		Name string `gorm:"column:name"`
		Type string `gorm:"column:type"`
		DefaultKind string `gorm:"column:default_kind"`
		DefaultExpression string `gorm:"column:default_expression"`
	}
	var rows []row
	err := model.LOG_DB.Raw(`
		SELECT name, type, default_kind, default_expression
		FROM system.columns
		WHERE database = currentDatabase() AND table = 'logs'
		  AND name IN ('reseller_id', 'reseller_cost_quota')
		ORDER BY name`).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("query system.columns %s: %w", label, err)
	}
	got := map[string]row{}
	for _, r := range rows {
		got[r.Name] = r
		fmt.Printf("  column %s type=%s default_kind=%s default_expression=%s\n",
			r.Name, r.Type, r.DefaultKind, r.DefaultExpression)
	}
	want := map[string]string{
		"reseller_id":         "Int32",
		"reseller_cost_quota": "Int64",
	}
	for name, typ := range want {
		r, ok := got[name]
		if !ok {
			return fmt.Errorf("missing column logs.%s (%s)", name, label)
		}
		if r.Type != typ {
			return fmt.Errorf("logs.%s type %q want %s (%s)", name, r.Type, typ, label)
		}
		fmt.Printf("  PASS logs.%s = %s (%s)\n", name, r.Type, label)
	}
	return nil
}
