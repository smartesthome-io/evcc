package charger

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/loadpoint"
	"github.com/evcc-io/evcc/util/request"
	"github.com/stretchr/testify/require"
)

// statusErr builds the error a plugin returns for the given response
func statusErr(t *testing.T, code int, body string) error {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "http://vehicle.test", nil)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}

	_, err = request.ReadBody(resp)
	require.Error(t, err)

	return err
}

func TestAsleep(t *testing.T) {
	sleeping := `{"response":{"result":false,"reason":"vehicle is sleeping"}}`

	// TeslaBleHttpProxy sleeping response
	require.ErrorIs(t, asleep(statusErr(t, http.StatusServiceUnavailable, sleeping)), api.ErrAsleep)

	// 503 for any other reason is not asleep
	other := statusErr(t, http.StatusServiceUnavailable, `{"response":{"reason":"vehicle is offline"}}`)
	require.NotErrorIs(t, asleep(other), api.ErrAsleep)

	// non-503 with sleeping body is not asleep
	require.NotErrorIs(t, asleep(statusErr(t, http.StatusInternalServerError, sleeping)), api.ErrAsleep)

	// non-json body and nil error pass through
	require.NotErrorIs(t, asleep(statusErr(t, http.StatusServiceUnavailable, "sleeping")), api.ErrAsleep)
	require.NoError(t, asleep(nil))
}

func TestAsleepTeslaMate(t *testing.T) {
	unavailable := `{"error":"vehicle unavailable: {error: \"vehicle unavailable:\"}","error_description":"","response":null}`
	require.ErrorIs(t, asleep(statusErr(t, http.StatusRequestTimeout, unavailable)), api.ErrAsleep)

	for _, err := range []error{
		statusErr(t, http.StatusServiceUnavailable, unavailable),
		statusErr(t, http.StatusRequestTimeout, `{"error":"request timed out"}`),
		statusErr(t, http.StatusRequestTimeout, `{"response":{"reason":"vehicle is sleeping"}}`),
		statusErr(t, http.StatusRequestTimeout, "vehicle unavailable"),
	} {
		require.Same(t, err, asleep(err))
	}
}

type statusLoadpoint struct {
	loadpoint.API
	vehicle api.Vehicle
}

func (lp *statusLoadpoint) GetVehicle() api.Vehicle { return lp.vehicle }

type statusVehicle struct {
	api.Vehicle
	status api.ChargeStatus
	err    error
}

func (v *statusVehicle) Status() (api.ChargeStatus, error) { return v.status, v.err }

func TestVehicleApiStatus(t *testing.T) {
	boom := errors.New("boom")

	type step struct {
		status  api.ChargeStatus
		err     error
		want    api.ChargeStatus
		wantErr bool
	}

	tc := []struct {
		name  string
		steps []step
	}{
		{"asleep at startup is connected", []step{
			{err: api.ErrAsleep, want: api.StatusB},
		}},
		{"asleep keeps connected", []step{
			{status: api.StatusB, want: api.StatusB},
			{err: api.ErrAsleep, want: api.StatusB},
		}},
		{"asleep keeps charging", []step{
			{status: api.StatusC, want: api.StatusC},
			{err: api.ErrAsleep, want: api.StatusC},
		}},
		{"asleep keeps disconnected", []step{
			{status: api.StatusA, want: api.StatusA},
			{err: api.ErrAsleep, want: api.StatusA},
		}},
		{"error is no disconnect", []step{
			{status: api.StatusB, want: api.StatusB},
			{err: boom, wantErr: true},
			{err: api.ErrNotAvailable, wantErr: true},
		}},
		{"explicit disconnect", []step{
			{status: api.StatusB, want: api.StatusB},
			{status: api.StatusA, want: api.StatusA},
			{status: api.StatusB, want: api.StatusB},
		}},
	}

	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			v := &statusVehicle{}
			c := &VehicleApi{lp: &statusLoadpoint{vehicle: v}}

			for i, st := range tc.steps {
				v.status, v.err = st.status, st.err

				got, err := c.Status()
				if st.wantErr {
					require.Error(t, err, "step %d", i)
					require.NotEqual(t, api.StatusA, got, "step %d", i)
					continue
				}

				require.NoError(t, err, "step %d", i)
				require.Equal(t, st.want, got, "step %d", i)
			}
		})
	}
}
