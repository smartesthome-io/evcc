package db_test

import (
	"testing"
	"time"

	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const chunk = 30 * 24 * time.Hour

func hypertableInterval(t *testing.T, conn *gorm.DB, table string) (int64, bool) {
	t.Helper()

	var res []int64
	require.NoError(t, conn.Raw(`SELECT integer_interval FROM timescaledb_information.dimensions
		WHERE hypertable_schema = current_schema() AND hypertable_name = ?`, table).Scan(&res).Error)

	if len(res) == 0 {
		return 0, false
	}
	return res[0], true
}

func TestEnableHypertable(t *testing.T) {
	dbtest.RequireTimescale(t)
	conn := dbtest.New(t)

	require.NoError(t, conn.Exec("CREATE TABLE series (ts bigint NOT NULL, v real)").Error)
	require.NoError(t, conn.Exec("CREATE UNIQUE INDEX series_ts ON series (ts)").Error)
	require.NoError(t, conn.Exec("INSERT INTO series SELECT 1000000 * i, i FROM generate_series(1, 500) i").Error)

	_, ok := hypertableInterval(t, conn, "series")
	require.False(t, ok, "plain table before")

	require.True(t, db.EnableHypertable(conn, "series", "ts", chunk))

	interval, ok := hypertableInterval(t, conn, "series")
	require.True(t, ok)
	assert.EqualValues(t, chunk.Seconds(), interval)

	var count int64
	require.NoError(t, conn.Table("series").Count(&count).Error)
	assert.EqualValues(t, 500, count, "existing rows are kept")

	// repeated start
	assert.True(t, db.EnableHypertable(conn, "series", "ts", chunk))

	// conflicting inserts keep working on the hypertable
	require.NoError(t, conn.Exec("INSERT INTO series VALUES (1000000, 99) ON CONFLICT DO NOTHING").Error)
}

func TestEnableHypertableFailureKeepsTable(t *testing.T) {
	dbtest.RequireTimescale(t)
	conn := dbtest.New(t)

	require.NoError(t, conn.Exec("CREATE TABLE series (ts bigint NOT NULL, v real)").Error)

	assert.False(t, db.EnableHypertable(conn, "series", "missing", chunk), "unknown column")
	assert.False(t, db.EnableHypertable(conn, "nonexistent", "ts", chunk), "unknown table")

	var count int64
	require.NoError(t, conn.Table("series").Count(&count).Error)
	assert.Zero(t, count)
}

func TestEnableHypertableWithoutExtension(t *testing.T) {
	if !dbtest.IsPostgres() || db.HasTimescale(dbtest.New(t)) {
		t.Skip("plain postgres only")
	}
	conn := dbtest.New(t)

	require.NoError(t, conn.Exec("CREATE TABLE series (ts bigint NOT NULL, v real)").Error)

	assert.False(t, db.EnableHypertable(conn, "series", "ts", chunk))
	assert.True(t, conn.Migrator().HasTable("series"), "plain table stays")
}

func TestEnableHypertableSQLite(t *testing.T) {
	dbtest.SkipPostgres(t)
	conn := dbtest.New(t)

	assert.False(t, db.HasTimescale(conn))
	assert.False(t, db.EnableHypertable(conn, "series", "ts", chunk))
}
