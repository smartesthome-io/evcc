package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evcc-io/evcc/core/session"
	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/db/cache"
	"github.com/evcc-io/evcc/db/dbtest"
	"github.com/evcc-io/evcc/db/settings"
	"github.com/evcc-io/evcc/hems/smartgrid"
	"github.com/evcc-io/evcc/util/config"
	"github.com/evcc-io/evcc/util/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const unreachableDSN = "postgres://user:secret@127.0.0.1:1/evcc?sslmode=disable&connect_timeout=1"

func closeDB(t *testing.T, conn *gorm.DB) {
	t.Helper()
	sqlDB, err := conn.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// seedSQLite creates a SQLite database file with data in every table
func seedSQLite(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "evcc.db")
	require.NoError(t, db.NewInstance(db.SQLite, path))

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	for i := range 3 {
		require.NoError(t, db.Instance.Create(&session.Session{
			Loadpoint:     "carport",
			Created:       now.Add(time.Duration(i) * time.Hour),
			Finished:      now.Add(time.Duration(i)*time.Hour + 30*time.Minute),
			ChargedEnergy: 5,
		}).Error)
	}

	// open session keeps the zero finished time
	require.NoError(t, db.Instance.Create(&session.Session{Loadpoint: "carport", Created: now}).Error)

	power := 4200.0
	require.NoError(t, db.Instance.Create(&smartgrid.GridSession{Created: now, Type: "consumption", GridPower: &power, LimitPower: 4200}).Error)

	require.NoError(t, db.Instance.Create(&config.Config{
		Class:      templates.Charger,
		Properties: config.Properties{Type: "template", Title: "Wallbox"},
		Data:       map[string]any{"host": "192.0.2.1", "factor": 1.5},
	}).Error)

	settings.SetString("foo", "bar")
	require.NoError(t, settings.Persist())
	require.NoError(t, cache.Put("answer", 42))

	for _, stmt := range []string{
		`INSERT INTO entities (id, "group", name, title, is_temp) VALUES (2, 'pv', 'pv1', 'Roof', false)`,
		// spans three batches
		`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 2499)
			INSERT INTO meters (meter, ts, energy, return_energy, recovered)
			SELECT 2, 900 * i, 0.5, 0.1, i = 7 FROM n`,
		`INSERT INTO tariffs (ts, grid, temperature) VALUES (900, 0.3, 11.5), (1800, 0.35, NULL)`,
	} {
		require.NoError(t, db.Instance.Exec(stmt).Error, stmt)
	}

	closeDB(t, db.Instance)

	return path
}

func count(t *testing.T, conn *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, conn.Table(table).Count(&n).Error)
	return n
}

func TestMigrateDatabase(t *testing.T) {
	if !dbtest.IsPostgres() {
		t.Skip("postgres only")
	}

	path := seedSQLite(t)
	_, dsn := dbtest.Target(t)

	require.NoError(t, migrateDatabase(path, dsn, false))

	conn, err := db.New(db.Postgres, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, conn) })

	assert.EqualValues(t, 4, count(t, conn, "sessions"))
	assert.EqualValues(t, 1, count(t, conn, "grid_sessions"))
	assert.EqualValues(t, 1, count(t, conn, "configs"))
	assert.EqualValues(t, 2, count(t, conn, "entities"), "home and pv1")
	assert.EqualValues(t, 2500, count(t, conn, "meters"))
	assert.EqualValues(t, 2, count(t, conn, "tariffs"))
	assert.EqualValues(t, 1, count(t, conn, "caches"))

	var kv struct{ Value string }
	require.NoError(t, conn.Table("settings").Select("value").Where("key = ?", "foo").Scan(&kv).Error)
	assert.Equal(t, "bar", kv.Value)

	var cfg config.Config
	require.NoError(t, conn.First(&cfg).Error)
	assert.Equal(t, "Wallbox", cfg.Title)
	assert.Equal(t, 1.5, cfg.Data["factor"])

	var energy float64
	var recovered int64
	require.NoError(t, conn.Table("meters").Select("SUM(energy)").Scan(&energy).Error)
	require.NoError(t, conn.Table("meters").Where("recovered").Count(&recovered).Error)
	assert.InDelta(t, 1250, energy, 1e-6)
	assert.EqualValues(t, 1, recovered)

	// open session survives with its zero finished time
	var open int64
	require.NoError(t, conn.Model(new(session.Session)).Where(map[string]any{"finished": time.Time{}}).Count(&open).Error)
	assert.EqualValues(t, 1, open)

	// sequences continue behind the copied ids
	next := session.Session{Loadpoint: "carport", Created: time.Now()}
	require.NoError(t, conn.Create(&next).Error)
	assert.EqualValues(t, 5, next.ID)

	t.Run("refuses non-empty target", func(t *testing.T) {
		err := migrateDatabase(path, dsn, false)
		assert.ErrorContains(t, err, "not empty")
		assert.EqualValues(t, 4+1, count(t, conn, "sessions"), "target untouched")
	})

	t.Run("force replaces target", func(t *testing.T) {
		require.NoError(t, migrateDatabase(path, dsn, true))
		assert.EqualValues(t, 4, count(t, conn, "sessions"))
		assert.EqualValues(t, 2500, count(t, conn, "meters"))
	})
}

func TestMigrateDatabaseLeavesSourceUntouched(t *testing.T) {
	if !dbtest.IsPostgres() {
		t.Skip("postgres only")
	}

	path := seedSQLite(t)
	before, err := filepathModTime(path)
	require.NoError(t, err)

	_, dsn := dbtest.Target(t)
	require.NoError(t, migrateDatabase(path, dsn, false))

	after, err := filepathModTime(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestMigrateDatabaseInvalid(t *testing.T) {
	good := seedSQLite(t)

	legacy := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := db.New(db.SQLite, legacy)
	require.NoError(t, err)
	require.NoError(t, conn.Exec(`CREATE TABLE meters (meter integer, ts datetime, val real, neg real)`).Error)
	closeDB(t, conn)

	for _, tc := range []struct {
		name, from, to, want string
	}{
		{"target not postgres", good, filepath.Join(t.TempDir(), "x.db"), "postgres dsn"},
		{"source missing", filepath.Join(t.TempDir(), "missing.db"), unreachableDSN, "source database"},
		{"source outdated", legacy, unreachableDSN, "outdated"},
		{"target unreachable", good, unreachableDSN, "connect to postgres"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := migrateDatabase(tc.from, tc.to, false)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func filepathModTime(path string) (time.Time, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return fi.ModTime(), nil
}
