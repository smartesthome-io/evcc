package db

import (
	"sync"
	"time"

	"github.com/evcc-io/evcc/util"
	"gorm.io/gorm"
)

var warnExtension sync.Once

// HasTimescale reports whether the timescaledb extension is installed in the database
func HasTimescale(conn *gorm.DB) bool {
	if Dialect(conn) != Postgres {
		return false
	}

	var installed bool
	err := conn.Raw("SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')").Scan(&installed).Error

	return err == nil && installed
}

// EnableHypertable turns a table with an integer time column into a TimescaleDB hypertable.
// Without the extension, or if the conversion fails, the plain table stays in use.
// It reports whether the table is a hypertable afterwards.
func EnableHypertable(conn *gorm.DB, table, column string, chunk time.Duration) bool {
	if Dialect(conn) != Postgres {
		return false
	}

	log := util.NewLogger("db")

	if !HasTimescale(conn) {
		warnExtension.Do(func() {
			log.WARN.Println("enable timescaledb extension on your postgres db to remove this warning")
		})
		return false
	}

	err := conn.Exec(
		"SELECT create_hypertable(?::regclass, ?::name, chunk_time_interval => ?::bigint, migrate_data => true, if_not_exists => true)",
		table, column, int64(chunk.Seconds()),
	).Error
	if err != nil {
		log.WARN.Printf("cannot convert %s to a timescaledb hypertable, keeping the plain table: %v", table, err)
		return false
	}

	return true
}
