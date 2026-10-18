package cmd

import (
	"testing"

	"github.com/evcc-io/evcc/api/globalconfig"
	"github.com/stretchr/testify/assert"
)

func TestConfigureDatabaseInvalid(t *testing.T) {
	for _, tc := range []struct {
		name string
		conf globalconfig.DB
		want string
	}{
		{"postgres without dsn", globalconfig.DB{Type: "postgres"}, "missing database dsn"},
		{"unknown type", globalconfig.DB{Type: "mysql", Dsn: "x"}, "invalid database type"},
		{"postgres dsn overrides default type", globalconfig.DB{Type: "sqlite", Dsn: "postgres://u:secret@host:notaport/db"}, "invalid postgres dsn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := configureDatabase(tc.conf)
			assert.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}
