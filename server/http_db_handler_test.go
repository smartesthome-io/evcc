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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFileDB opens a file database with one session and returns its path
func newFileDB(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "evcc.db")
	require.NoError(t, db.NewInstance("sqlite", path))
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now()
	require.NoError(t, db.Instance.Create(&session.Session{Created: now, Finished: now.Add(time.Hour), ChargedEnergy: 1.0}).Error)

	return path
}

// sessionCount reopens the database file and counts sessions
func sessionCount(t *testing.T, path string) int64 {
	t.Helper()

	conn, err := db.New("sqlite", path)
	require.NoError(t, err)

	var count int64
	require.NoError(t, conn.Model(new(session.Session)).Count(&count).Error)

	sqlDB, err := conn.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	return count
}

func TestBackupHandler(t *testing.T) {
	newFileDB(t)

	rec := httptest.NewRecorder()
	getBackup()(rec, httptest.NewRequest(http.MethodGet, "/backup", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="evcc-backup-`))
	assert.True(t, strings.HasPrefix(rec.Body.String(), "SQLite format 3\x00"), "backup is a sqlite file")
}

func TestResetHandler(t *testing.T) {
	path := newFileDB(t)

	var stopped bool
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reset", strings.NewReader(`{"sessions":true}`))
	resetDatabase(func() { stopped = true })(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, stopped, "shutdown requested")

	_, err := os.Stat(path + ".bak")
	assert.NoError(t, err, "local backup created before the reset")

	assert.Zero(t, sessionCount(t, path), "sessions deleted")
}

func TestResetHandlerKeepsUnselected(t *testing.T) {
	path := newFileDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reset", strings.NewReader(`{"settings":true}`))
	resetDatabase(func() {})(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.EqualValues(t, 1, sessionCount(t, path), "sessions untouched")
}

func TestRestoreHandler(t *testing.T) {
	path := newFileDB(t)

	// snapshot with one session, then drop it from the live database
	snapshot := filepath.Join(t.TempDir(), "snapshot.db")
	require.NoError(t, db.Backup(t.Context(), snapshot))
	require.NoError(t, db.Instance.Exec("DELETE FROM sessions").Error)

	data, err := os.ReadFile(snapshot)
	require.NoError(t, err)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "evcc-backup.db")
	require.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader(data))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	var stopped bool
	req := httptest.NewRequest(http.MethodPost, "/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	restoreDatabase(func() { stopped = true })(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, stopped, "shutdown requested")
	assert.EqualValues(t, 1, sessionCount(t, path), "session restored")
}

func TestRestoreHandlerMissingFile(t *testing.T) {
	newFileDB(t)

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
