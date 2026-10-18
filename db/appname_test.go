package db_test

import (
	"net/url"
	"testing"

	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func applicationName(t *testing.T, dsn string) string {
	t.Helper()

	conn, err := db.New(db.Postgres, dsn)
	require.NoError(t, err)

	var name string
	require.NoError(t, conn.Raw("SHOW application_name").Scan(&name).Error)

	sqlDB, err := conn.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	return name
}

func TestPostgresApplicationName(t *testing.T) {
	if !dbtest.IsPostgres() {
		t.Skip("postgres only")
	}

	_, dsn := dbtest.Target(t)
	assert.Equal(t, db.ApplicationName, applicationName(t, dsn))

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	q.Set("application_name", "custom")
	u.RawQuery = q.Encode()

	assert.Equal(t, "custom", applicationName(t, u.String()), "explicit name wins")
}
