package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/evcc-io/evcc/core/session"
	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useClientTools makes the PostgreSQL client tools available to the test. Without the tools on the PATH
// they run in the container image named by EVCC_TEST_PG_TOOLS_IMAGE on the host network.
func useClientTools(t *testing.T) {
	t.Helper()

	if image := os.Getenv("EVCC_TEST_PG_TOOLS_IMAGE"); image != "" {
		orig := toolRunner
		t.Cleanup(func() { toolRunner = orig })

		toolRunner = func(ctx context.Context, name string, args, env []string, stdin io.Reader, stdout io.Writer) error {
			dockerArgs := []string{"run", "--rm", "-i", "--network", "host"}
			for _, e := range env {
				k, _, _ := strings.Cut(e, "=")
				dockerArgs = append(dockerArgs, "-e", k)
			}
			dockerArgs = append(dockerArgs, image, name)

			cmd := exec.CommandContext(ctx, "docker", append(dockerArgs, args...)...)
			cmd.Env = append(os.Environ(), env...)
			cmd.Stdin, cmd.Stdout = stdin, stdout

			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return &toolError{err: err, msg: stderr.String()}
			}
			return nil
		}
		return
	}

	// CI must run these tests
	skip := t.Skip
	if os.Getenv("CI") != "" {
		skip = t.Fatal
	}

	out, err := exec.Command("pg_dump", "--version").Output()
	if err != nil {
		skip("pg_dump not installed")
	}

	// pg_dump refuses servers newer than itself
	conn, err := db.New(db.Postgres, os.Getenv(dbtest.Env))
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, conn) })

	var serverMajor int
	require.NoError(t, conn.Raw("SELECT current_setting('server_version_num')::int / 10000").Scan(&serverMajor).Error)

	var clientMajor int
	_, _ = fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(string(out)), "pg_dump (PostgreSQL) "), "%d", &clientMajor)
	if clientMajor < serverMajor {
		skip(fmt.Sprintf("pg_dump %d is older than the server %d", clientMajor, serverMajor))
	}
}

type toolError struct {
	err error
	msg string
}

func (e *toolError) Error() string { return e.err.Error() + ": " + strings.TrimSpace(e.msg) }

// seedPostgres fills a database with the schema and some data, optionally marked by a stale session
func seedPostgres(t *testing.T, dsn string, sessions int) {
	t.Helper()

	require.NoError(t, db.NewInstance(db.Postgres, dsn))
	conn := db.Instance
	t.Cleanup(func() { closeDB(t, conn) })

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	for i := range sessions {
		require.NoError(t, conn.Create(&session.Session{Loadpoint: "carport", Created: now.Add(time.Duration(i) * time.Hour), ChargedEnergy: 5}).Error)
	}

	require.NoError(t, conn.Exec(`INSERT INTO entities (id, "group", name, title, is_temp) VALUES (2, 'pv', 'pv1', 'Roof', false)`).Error)
	require.NoError(t, conn.Exec(`INSERT INTO meters (meter, ts, energy, return_energy, recovered)
		SELECT 2, 900 * i, 0.5, 0, false FROM generate_series(0, 99) i`).Error)
}

func hypertableCount(t *testing.T, conn *gorm.DB) int64 {
	t.Helper()

	if !db.HasTimescale(conn) {
		return 0
	}

	var n int64
	require.NoError(t, conn.Raw("SELECT count(*) FROM timescaledb_information.hypertables WHERE hypertable_schema = current_schema()").Scan(&n).Error)
	return n
}

func TestBackupRestorePostgres(t *testing.T) {
	if !dbtest.IsPostgres() {
		t.Skip("postgres only")
	}
	useClientTools(t)

	for _, tc := range []struct {
		name                 string
		source, target       []string
		hypertables, wantErr string
	}{
		{name: "plain to plain"},
		{name: "timescale to timescale", source: []string{"timescaledb"}, target: []string{"timescaledb"}, hypertables: "2"},
		{name: "plain to timescale", target: []string{"timescaledb"}},
		{name: "timescale to plain", source: []string{"timescaledb"}, wantErr: "TimescaleDB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.source)+len(tc.target) > 0 {
				dbtest.RequireTimescale(t)
			}

			srcDSN := dbtest.NewDatabase(t, tc.source...)
			tgtDSN := dbtest.NewDatabase(t, tc.target...)

			seedPostgres(t, srcDSN, 3)
			seedPostgres(t, tgtDSN, 1) // existing data that the restore replaces

			file := filepath.Join(t.TempDir(), "evcc.dump")
			require.NoError(t, backupDatabase(t.Context(), db.Postgres, srcDSN, file))

			fi, err := os.Stat(file)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

			// the seeding connections are evcc instances, the restore must not run next to them
			closeDB(t, db.Instance)

			err = restoreDatabaseFile(t.Context(), db.Postgres, tgtDSN, file, restoreOptions{yes: true, force: true})
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)

				// the target is untouched
				conn, err := db.New(db.Postgres, tgtDSN)
				require.NoError(t, err)
				t.Cleanup(func() { closeDB(t, conn) })
				assert.EqualValues(t, 1, count(t, conn, "sessions"))
				return
			}
			require.NoError(t, err)

			conn, err := db.New(db.Postgres, tgtDSN)
			require.NoError(t, err)
			t.Cleanup(func() { closeDB(t, conn) })

			assert.EqualValues(t, 3, count(t, conn, "sessions"), "replaced the existing data")
			assert.EqualValues(t, 100, count(t, conn, "meters"))
			assert.EqualValues(t, 2, count(t, conn, "entities"))

			want := int64(0)
			if tc.hypertables != "" {
				want = 2
			}
			assert.Equal(t, want, hypertableCount(t, conn))

			// evcc converts restored plain tables when it starts on a timescale database
			if len(tc.target) > 0 && len(tc.source) == 0 {
				require.NoError(t, db.NewInstance(db.Postgres, tgtDSN))
				t.Cleanup(func() { closeDB(t, db.Instance) })
				assert.EqualValues(t, 2, hypertableCount(t, db.Instance))
			}
		})
	}
}

func TestRestorePostgresGuardsRunningInstance(t *testing.T) {
	if !dbtest.IsPostgres() {
		t.Skip("postgres only")
	}
	useClientTools(t)

	srcDSN := dbtest.NewDatabase(t)
	tgtDSN := dbtest.NewDatabase(t)

	seedPostgres(t, srcDSN, 3)
	file := filepath.Join(t.TempDir(), "evcc.dump")
	require.NoError(t, backupDatabase(t.Context(), db.Postgres, srcDSN, file))
	closeDB(t, db.Instance)

	// a running evcc is connected to the target
	running, err := db.New(db.Postgres, tgtDSN)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, running) })
	require.NoError(t, running.Exec("SELECT 1").Error)

	err = restoreDatabaseFile(t.Context(), db.Postgres, tgtDSN, file, restoreOptions{yes: true})
	require.ErrorContains(t, err, "evcc connection")

	require.NoError(t, restoreDatabaseFile(t.Context(), db.Postgres, tgtDSN, file, restoreOptions{yes: true, force: true}))
}

func TestRestorePostgresInvalidArchive(t *testing.T) {
	if !dbtest.IsPostgres() {
		t.Skip("postgres only")
	}
	useClientTools(t)

	tgtDSN := dbtest.NewDatabase(t)
	seedPostgres(t, tgtDSN, 1)
	closeDB(t, db.Instance)

	file := filepath.Join(t.TempDir(), "notadump.dump")
	require.NoError(t, os.WriteFile(file, []byte("this is not a dump"), 0o600))

	err := restoreDatabaseFile(t.Context(), db.Postgres, tgtDSN, file, restoreOptions{yes: true})
	require.ErrorContains(t, err, "invalid backup file")

	conn, err := db.New(db.Postgres, tgtDSN)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, conn) })
	assert.EqualValues(t, 1, count(t, conn, "sessions"), "target untouched")
}

func TestBackupDoesNotPassPasswordAsArgument(t *testing.T) {
	var gotArgs, gotEnv []string

	orig := toolRunner
	t.Cleanup(func() { toolRunner = orig })
	toolRunner = func(_ context.Context, _ string, args, env []string, _ io.Reader, _ io.Writer) error {
		gotArgs, gotEnv = args, env
		return nil
	}

	file := filepath.Join(t.TempDir(), "evcc.dump")
	require.NoError(t, backupDatabase(t.Context(), db.Postgres, "postgres://user:s3cr3t@dbhost:5433/evcc?sslmode=require", file))

	assert.NotContains(t, strings.Join(gotArgs, " "), "s3cr3t")
	assert.Contains(t, gotEnv, "PGPASSWORD=s3cr3t")
	assert.Contains(t, gotEnv, "PGHOST=dbhost")
	assert.Contains(t, gotEnv, "PGPORT=5433")
	assert.Contains(t, gotEnv, "PGUSER=user")
	assert.Contains(t, gotEnv, "PGDATABASE=evcc")
	assert.Contains(t, gotEnv, "PGSSLMODE=require")
}

func TestDSNSettings(t *testing.T) {
	assert.Equal(t, "/ca.pem", dsnSettings("postgres://u:p@h/db?sslmode=verify-full&sslrootcert=/ca.pem")["sslrootcert"])
	assert.Equal(t, "/ca.pem", dsnSettings("host=h sslrootcert='/ca.pem' sslmode=require")["sslrootcert"])
}

func TestPgEnvKeyValueDSN(t *testing.T) {
	env, database, err := pgEnv("host=localhost port=5432 user=evcc password='pw' dbname=home sslmode=disable")
	require.NoError(t, err)

	assert.Equal(t, "home", database)
	assert.Contains(t, env, "PGPASSWORD=pw")
	assert.Contains(t, env, "PGSSLMODE=disable")
}

func TestBackupToolErrors(t *testing.T) {
	err := runTool(t.Context(), "evcc-missing-tool", nil, nil, nil, nil)
	require.ErrorContains(t, err, "not found, install the PostgreSQL client tools")

	// unreachable server, the tool fails and the partial file is removed, no password in the message
	orig := toolRunner
	t.Cleanup(func() { toolRunner = orig })
	toolRunner = func(_ context.Context, name string, _, _ []string, _ io.Reader, _ io.Writer) error {
		return runTool(t.Context(), "sh", []string{"-c", "echo 'pg_dump: error: connection refused' >&2; exit 1"}, nil, nil, nil)
	}

	file := filepath.Join(t.TempDir(), "evcc.dump")
	err = backupDatabase(t.Context(), db.Postgres, "postgres://user:s3cr3t@127.0.0.1:1/evcc", file)
	require.ErrorContains(t, err, "connection refused")
	assert.NotContains(t, err.Error(), "s3cr3t")
	assert.NoFileExists(t, file)
}

func TestBackupRefusesExistingFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "evcc.dump")
	require.NoError(t, os.WriteFile(file, []byte("precious"), 0o600))

	orig := toolRunner
	t.Cleanup(func() { toolRunner = orig })
	toolRunner = func(context.Context, string, []string, []string, io.Reader, io.Writer) error {
		t.Fatal("must not run")
		return nil
	}

	require.Error(t, backupDatabase(t.Context(), db.Postgres, "postgres://u:p@h/db", file))

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "precious", string(data))
}

func TestRestoreNeedsConfirmation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "evcc.db")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	// stdin is not a terminal in tests
	in, err := os.Open(os.DevNull)
	require.NoError(t, err)
	t.Cleanup(func() { in.Close() })

	err = restoreDatabaseFile(t.Context(), db.SQLite, filepath.Join(t.TempDir(), "evcc.db"), file, restoreOptions{in: in})
	require.ErrorContains(t, err, "--yes")

	require.ErrorContains(t, restoreDatabaseFile(t.Context(), db.SQLite, "x", filepath.Join(t.TempDir(), "missing"), restoreOptions{yes: true}), "backup file")
}

func TestBackupRestoreSQLite(t *testing.T) {
	path := seedSQLite(t)
	file := filepath.Join(t.TempDir(), "evcc-backup.db")

	require.NoError(t, backupDatabase(t.Context(), db.SQLite, path, file))

	fi, err := os.Stat(file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	assert.Error(t, backupDatabase(t.Context(), db.SQLite, path, file), "existing file")
	assert.Error(t, backupDatabase(t.Context(), db.SQLite, filepath.Join(t.TempDir(), "missing.db"), filepath.Join(t.TempDir(), "x.db")), "missing database")

	sessions := func() int64 {
		conn, err := db.New(db.SQLite, path)
		require.NoError(t, err)
		defer closeDB(t, conn)
		return count(t, conn, "sessions")
	}

	require.EqualValues(t, 4, sessions())

	conn, err := db.New(db.SQLite, path)
	require.NoError(t, err)
	require.NoError(t, conn.Exec("DELETE FROM sessions").Error)
	closeDB(t, conn)
	require.Zero(t, sessions())

	require.NoError(t, restoreDatabaseFile(t.Context(), db.SQLite, path, file, restoreOptions{yes: true}))
	assert.EqualValues(t, 4, sessions())
}

func TestDefaultBackupName(t *testing.T) {
	now := time.Date(2026, 10, 19, 13, 5, 9, 0, time.UTC)
	assert.Equal(t, "evcc-backup-20261019-130509.dump", defaultBackupName(db.Postgres, now))
	assert.Equal(t, "evcc-backup-20261019-130509.db", defaultBackupName(db.SQLite, now))
}

func TestToolErrorsOmitStatementData(t *testing.T) {
	script := `echo 'pg_restore: error: could not execute query: ERROR:  duplicate key value violates unique constraint "settings_pkey"' >&2
echo 'DETAIL:  Key (key)=(token) already exists.' >&2
echo 'Command was: COPY public.settings (key, value) FROM stdin;' >&2
echo 'token	super-secret-token' >&2
exit 1`

	err := runTool(t.Context(), "sh", []string{"-c", script}, nil, nil, nil)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "duplicate key value violates unique constraint")
	assert.NotContains(t, err.Error(), "super-secret-token")
	assert.NotContains(t, err.Error(), "Key (key)")
}

func TestPgEnvMultipleHostsAndOptions(t *testing.T) {
	env, _, err := pgEnv("postgres://u:p@a:5432,b:5433/db?sslmode=disable&connect_timeout=5&target_session_attrs=read-write")
	require.NoError(t, err)

	assert.Contains(t, env, "PGHOST=a,b")
	assert.Contains(t, env, "PGPORT=5432,5433")
	assert.Contains(t, env, "PGCONNECT_TIMEOUT=5")
	assert.Contains(t, env, "PGTARGETSESSIONATTRS=read-write")

	_, _, err = pgEnv("postgres://u:p@h/db?sslmode=disable&sslpassword=secret")
	require.ErrorContains(t, err, "sslpassword")
	assert.NotContains(t, err.Error(), "secret")
}

func TestDSNSettingsQuoting(t *testing.T) {
	got := dsnSettings(`host=h user = evcc password='it\'s a pw' sslrootcert='/path with space/ca.pem' dbname=db`)

	assert.Equal(t, "h", got["host"])
	assert.Equal(t, "evcc", got["user"])
	assert.Equal(t, "/path with space/ca.pem", got["sslrootcert"])
	assert.Equal(t, "db", got["dbname"])
	assert.Equal(t, "it's a pw", got["password"])
}

// a restore that fails after the tables were dropped keeps the database usable
func TestRestoreTimescaleFailureLeavesRestoreMode(t *testing.T) {
	dbtest.RequireTimescale(t)
	useClientTools(t)

	srcDSN := dbtest.NewDatabase(t, "timescaledb")
	tgtDSN := dbtest.NewDatabase(t, "timescaledb")

	seedPostgres(t, srcDSN, 3)
	file := filepath.Join(t.TempDir(), "evcc.dump")
	require.NoError(t, backupDatabase(t.Context(), db.Postgres, srcDSN, file))
	closeDB(t, db.Instance)

	real := toolRunner
	t.Cleanup(func() { toolRunner = real })
	toolRunner = func(ctx context.Context, name string, args, env []string, stdin io.Reader, stdout io.Writer) error {
		if slices.Contains(args, "--single-transaction") {
			return errors.New("restore broke")
		}
		return real(ctx, name, args, env, stdin, stdout)
	}

	err := restoreDatabaseFile(t.Context(), db.Postgres, tgtDSN, file, restoreOptions{yes: true})
	require.ErrorContains(t, err, "restore again from the backup file")

	conn, err := db.New(db.Postgres, tgtDSN)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, conn) })

	var restoring string
	require.NoError(t, conn.Raw("SHOW timescaledb.restoring").Scan(&restoring).Error)
	assert.Equal(t, "off", restoring, "restore mode is left")

	// the next attempt succeeds, the connection above is an evcc instance for the guard
	toolRunner = real
	require.NoError(t, restoreDatabaseFile(t.Context(), db.Postgres, tgtDSN, file, restoreOptions{yes: true, force: true}))
	assert.EqualValues(t, 3, count(t, conn, "sessions"))
}
