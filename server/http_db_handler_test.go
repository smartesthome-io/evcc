package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evcc-io/evcc/core/session"
	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDB opens a database with one session, a file for SQLite so it can be backed up
func newTestDB(t *testing.T) (driver, dsn string) {
	t.Helper()

	if dbtest.IsPostgres() {
		driver, dsn = dbtest.Target(t)
	} else {
		driver, dsn = db.SQLite, filepath.Join(t.TempDir(), "evcc.db")
	}

	require.NoError(t, db.NewInstance(driver, dsn))
	conn := db.Instance
	t.Cleanup(func() {
		if sqlDB, err := conn.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	now := time.Now()
	require.NoError(t, db.Instance.Create(&session.Session{Created: now, Finished: now.Add(time.Hour), ChargedEnergy: 1.0}).Error)

	return driver, dsn
}

// sessionCount reopens the database and counts sessions
func sessionCount(t *testing.T, driver, dsn string) int64 {
	t.Helper()

	conn, err := db.New(driver, dsn)
	require.NoError(t, err)

	var count int64
	require.NoError(t, conn.Model(new(session.Session)).Count(&count).Error)

	sqlDB, err := conn.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	return count
}

func restoreRequest(t *testing.T, data []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "evcc-backup.db")
	require.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader(data))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestBackupHandler(t *testing.T) {
	dbtest.SkipPostgres(t)
	newTestDB(t)

	rec := httptest.NewRecorder()
	getBackup()(rec, httptest.NewRequest(http.MethodGet, "/backup", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="evcc-backup-`))
	assert.True(t, strings.HasPrefix(rec.Body.String(), "SQLite format 3\x00"), "backup is a sqlite file")
}

func TestBackupRestoreUnsupportedOnPostgres(t *testing.T) {
	if !dbtest.IsPostgres() {
		t.Skip("postgres only")
	}
	driver, dsn := newTestDB(t)

	rec := httptest.NewRecorder()
	getBackup()(rec, httptest.NewRequest(http.MethodGet, "/backup", nil))
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
	assert.Contains(t, rec.Body.String(), "pg_dump")

	rec = httptest.NewRecorder()
	restoreDatabase(func() { t.Fatal("must not shut down") })(rec, restoreRequest(t, []byte("data")))
	assert.Equal(t, http.StatusNotImplemented, rec.Code)

	assert.EqualValues(t, 1, sessionCount(t, driver, dsn), "data untouched")
}

func TestResetHandler(t *testing.T) {
	driver, dsn := newTestDB(t)

	var stopped bool
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reset", strings.NewReader(`{"sessions":true}`))
	resetDatabase(func() { stopped = true })(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, stopped, "shutdown requested")

	if !dbtest.IsPostgres() {
		_, err := os.Stat(dsn + ".bak")
		assert.NoError(t, err, "local backup created before the reset")
	}

	assert.Zero(t, sessionCount(t, driver, dsn), "sessions deleted")
}

func TestResetHandlerKeepsUnselected(t *testing.T) {
	driver, dsn := newTestDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reset", strings.NewReader(`{"settings":true}`))
	resetDatabase(func() {})(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.EqualValues(t, 1, sessionCount(t, driver, dsn), "sessions untouched")
}

func TestRestoreHandler(t *testing.T) {
	dbtest.SkipPostgres(t)
	driver, dsn := newTestDB(t)

	// snapshot with one session, then drop it from the live database
	snapshot := filepath.Join(t.TempDir(), "snapshot.db")
	require.NoError(t, db.Backup(t.Context(), snapshot))
	require.NoError(t, db.Instance.Exec("DELETE FROM sessions").Error)

	data, err := os.ReadFile(snapshot)
	require.NoError(t, err)

	var stopped bool
	rec := httptest.NewRecorder()
	restoreDatabase(func() { stopped = true })(rec, restoreRequest(t, data))

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, stopped, "shutdown requested")
	assert.EqualValues(t, 1, sessionCount(t, driver, dsn), "session restored")
}

func TestRestoreHandlerMissingFile(t *testing.T) {
	dbtest.SkipPostgres(t)
	newTestDB(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("other", "x"))
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	restoreDatabase(func() { t.Fatal("must not shut down") })(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
