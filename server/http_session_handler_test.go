package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evcc-io/evcc/core/session"
	"github.com/evcc-io/evcc/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionHandlerTimezoneFilter(t *testing.T) {
	require.NoError(t, db.NewInstance("sqlite", ":memory:"))
	require.NoError(t, db.Instance.AutoMigrate(new(session.Session)))

	// Build in the process zone SQLite's 'localtime' uses (baked at startup).
	// Just past local midnight May 1 is still April 30 UTC east of UTC.
	ts := time.Date(2026, 5, 1, 0, 1, 0, 0, time.Local)
	require.NoError(t, db.Instance.Create(&session.Session{
		Created:       ts,
		Finished:      ts.Add(time.Hour),
		ChargedEnergy: 1.0,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/?year=2026&month=5", nil)
	rec := httptest.NewRecorder()
	sessionHandler(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got session.Sessions
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 1)
}

func TestSessionHandlerFilter(t *testing.T) {
	require.NoError(t, db.NewInstance("sqlite", ":memory:"))
	require.NoError(t, db.Instance.AutoMigrate(new(session.Session)))

	for _, ts := range []struct {
		created time.Time
		energy  float64
	}{
		{time.Date(2025, 12, 31, 23, 30, 0, 0, time.Local), 1.0},
		{time.Date(2026, 1, 1, 0, 30, 0, 0, time.Local), 2.0},
		{time.Date(2026, 1, 15, 12, 0, 0, 0, time.Local), 0.04}, // below the 0.05 kWh cutoff
		{time.Date(2026, 12, 1, 12, 0, 0, 0, time.Local), 3.0},
	} {
		require.NoError(t, db.Instance.Create(&session.Session{
			Created:       ts.created,
			Finished:      ts.created.Add(time.Hour),
			ChargedEnergy: ts.energy,
		}).Error)
	}

	energies := func(query string) []float64 {
		rec := httptest.NewRecorder()
		sessionHandler(rec, httptest.NewRequest(http.MethodGet, query, nil))
		require.Equal(t, http.StatusOK, rec.Code)

		var got session.Sessions
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

		res := make([]float64, 0, len(got))
		for _, s := range got {
			res = append(res, s.ChargedEnergy)
		}
		return res
	}

	// newest first
	assert.Equal(t, []float64{3.0, 2.0, 1.0}, energies("/"), "cutoff excludes short sessions")
	assert.Equal(t, []float64{3.0, 2.0}, energies("/?year=2026"))
	assert.Equal(t, []float64{1.0}, energies("/?year=2025"))
	assert.Equal(t, []float64{1.0}, energies("/?year=2025&month=12"))
	assert.Equal(t, []float64{2.0}, energies("/?year=2026&month=1"), "month rollover")
	assert.Equal(t, []float64{3.0}, energies("/?year=2026&month=12"))
	assert.Empty(t, energies("/?year=2026&month=2"))
}
