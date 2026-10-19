package cmd

import (
	"testing"

	"github.com/evcc-io/evcc/db/dbtest"
	"github.com/evcc-io/evcc/db/settings"
	"github.com/stretchr/testify/require"
)

func TestMigrateYaml(t *testing.T) {
	dbtest.NewInstance(t)

	const key = "foo"

	type T struct {
		Key string
		Val []string
	}

	expect := T{
		Key: "foo",
		Val: []string{"a", "b"},
	}

	require.NoError(t, settings.SetYaml(key, expect))
	require.False(t, settings.IsJson(key))

	var res T
	require.NoError(t, migrateYamlToJson(key, &res))
	require.True(t, settings.IsJson(key))

	require.Equal(t, expect, res)
}
