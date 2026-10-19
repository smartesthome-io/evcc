package db

import (
	"fmt"
	"sync"
	"time"

	"github.com/evcc-io/evcc/util"
	"gorm.io/gorm"
)

var warnExtension sync.Once

// timescaleSchema returns the schema of the timescaledb extension, empty if it is not installed
func timescaleSchema(conn *gorm.DB) (string, error) {
	var res []string
	if err := conn.Raw("SELECT extnamespace::regnamespace::text FROM pg_extension WHERE extname = 'timescaledb'").Scan(&res).Error; err != nil {
		return "", err
	}

	if len(res) == 0 {
		return "", nil
	}

	return res[0], nil
}

// HasTimescale reports whether the timescaledb extension is installed in the database
func HasTimescale(conn *gorm.DB) bool {
	if Dialect(conn) != Postgres {
		return false
	}

	schema, err := timescaleSchema(conn)

	return err == nil && schema != ""
}

// EnableHypertable turns a table with an integer time column into a TimescaleDB hypertable.
// Without the extension, or if the conversion fails, the plain table stays in use.
// It reports whether the table is a hypertable afterwards.
func EnableHypertable(conn *gorm.DB, table, column string, chunk time.Duration) bool {
	if Dialect(conn) != Postgres {
		return false
	}

	log := util.NewLogger("db")

	schema, err := timescaleSchema(conn)
	if err != nil {
		log.WARN.Printf("cannot check for timescaledb extension, keeping the plain table %s: %v", table, err)
		return false
	}

	if schema == "" {
		warnExtension.Do(func() {
			log.WARN.Println("enable timescaledb extension on your postgres db to remove this warning")
		})
		return false
	}

	// the schema comes quoted from the catalog
	err = conn.Exec(
		fmt.Sprintf("SELECT %s.create_hypertable(?::regclass, ?::name, chunk_time_interval => ?::bigint, migrate_data => true, if_not_exists => true)", schema),
		table, column, int64(chunk.Seconds()),
	).Error
	if err != nil {
		log.WARN.Printf("cannot convert %s to a timescaledb hypertable, keeping the plain table: %v", table, err)
		return false
	}

	return true
}

// SetTimescaleRestoring prepares TimescaleDB for pg_restore (on) and returns it to normal operation afterwards (off).
// Without the extension it does nothing.
func SetTimescaleRestoring(conn *gorm.DB, on bool) error {
	schema, err := timescaleSchema(conn)
	if err != nil || schema == "" {
		return err
	}

	fn := "timescaledb_post_restore"
	if on {
		fn = "timescaledb_pre_restore"
	}

	if err := conn.Exec(fmt.Sprintf("SELECT %s.%s()", schema, fn)).Error; err != nil {
		return fmt.Errorf("%s (superuser or database owner rights are required): %w", fn, err)
	}

	return nil
}
