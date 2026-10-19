package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseWebDir(t *testing.T) {
	orig := Web
	t.Cleanup(func() { Web = orig })

	dir := t.TempDir()
	assert.Error(t, UseWebDir(dir), "missing index.html")
	assert.Error(t, UseWebDir(filepath.Join(dir, "nope")), "missing dir")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644))
	require.NoError(t, UseWebDir(dir))

	b, err := fs.ReadFile(Web, "index.html")
	require.NoError(t, err)
	assert.Equal(t, "x", string(b))
}
