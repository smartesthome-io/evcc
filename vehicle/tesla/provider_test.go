package tesla

import (
	"testing"

	"github.com/evcc-io/evcc/api"
	"github.com/stretchr/testify/assert"
)

func TestProviderStatus(t *testing.T) {
	tc := []struct {
		state string
		want  api.ChargeStatus
		err   error
	}{
		{"Disconnected", api.StatusA, nil},
		{"Stopped", api.StatusB, nil},
		{"NoPower", api.StatusB, nil},
		{"Complete", api.StatusB, nil},
		{"Starting", api.StatusB, nil},
		{"Charging", api.StatusC, nil},
		{"", api.StatusNone, api.ErrNotAvailable},
		{"Unknown", api.StatusNone, api.ErrNotAvailable},
	}

	for _, tc := range tc {
		t.Run(tc.state, func(t *testing.T) {
			var data VehicleData
			data.Response.ChargeState.ChargingState = tc.state

			p := &Provider{dataG: func() (*VehicleData, error) { return &data, nil }}

			got, err := p.Status()
			assert.Equal(t, tc.want, got)
			assert.ErrorIs(t, err, tc.err)
		})
	}
}

func TestProviderStatusError(t *testing.T) {
	p := &Provider{dataG: func() (*VehicleData, error) { return nil, api.ErrAsleep }}

	got, err := p.Status()
	assert.Equal(t, api.StatusNone, got)
	assert.ErrorIs(t, err, api.ErrAsleep)
}
