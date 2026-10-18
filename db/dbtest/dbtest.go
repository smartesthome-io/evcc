// Package dbtest provides databases for tests, in memory SQLite by default
// or PostgreSQL when EVCC_TEST_DB holds a postgres dsn.
package dbtest

import (
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"testing"

	"github.com/evcc-io/evcc/db"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Env is the environment variable that selects the test database
const Env = "EVCC_TEST_DB"

// IsPostgres reports whether tests run against PostgreSQL
func IsPostgres() bool {
	return db.DriverFromDSN(db.SQLite, os.Getenv(Env)) == db.Postgres
}

// RequireTimescale skips tests that need the timescaledb extension, unless EVCC_TEST_TIMESCALE is set
func RequireTimescale(t testing.TB) {
	t.Helper()
	if !IsPostgres() {
		t.Skip("postgres only")
	}
	installed := db.HasTimescale(New(t))
	if os.Getenv("EVCC_TEST_TIMESCALE") != "" {
		require.True(t, installed, "timescaledb extension missing")
		return
	}
	if !installed {
		t.Skip("timescaledb only")
	}
}

// SkipPostgres skips tests that depend on SQLite behavior
func SkipPostgres(t testing.TB) {
	t.Helper()
	if IsPostgres() {
		t.Skip("sqlite only")
	}
}

// New returns an empty database without registering it as db.Instance
func New(t testing.TB) *gorm.DB {
	t.Helper()

	conn, err := db.New(Target(t))
	require.NoError(t, err)

	if sqlDB, err := conn.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	return conn
}

// NewInstance sets up an empty database as db.Instance including all registered schemas
func NewInstance(t testing.TB) {
	t.Helper()

	driver, dsn := Target(t)
	require.NoError(t, db.NewInstance(driver, dsn))

	// connections must be released to drop the schema; tests may still use an in-memory instance afterwards
	if conn := db.Instance; IsPostgres() {
		t.Cleanup(func() {
			if sqlDB, err := conn.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})
	}
}

// Target returns driver and dsn of an empty database
func Target(t testing.TB) (string, string) {
	t.Helper()

	dsn := os.Getenv(Env)
	if !IsPostgres() {
		return db.SQLite, ":memory:"
	}

	// every test gets its own schema, dropped on cleanup
	schema := fmt.Sprintf("test_%08x", rand.Uint32())

	admin, err := db.New(db.Postgres, dsn)
	require.NoError(t, err)
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)

	t.Cleanup(func() {
		defer func() {
			if sqlDB, err := admin.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()
		_ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error
	})

	u, err := url.Parse(dsn)
	require.NoError(t, err)

	q := u.Query()
	// public holds the timescaledb functions
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()

	return db.Postgres, u.String()
}
