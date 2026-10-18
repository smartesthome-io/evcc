package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/evcc-io/evcc/api/globalconfig"
	"github.com/evcc-io/evcc/server/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeUI(t *testing.T, dir, name, manifest string, files map[string]string) {
	t.Helper()
	root := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(root, 0o755))
	if manifest != "" {
		require.NoError(t, os.WriteFile(filepath.Join(root, uiManifest), []byte(manifest), 0o644))
	}
	for f, content := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, f), []byte(content), 0o644))
	}
}

func TestLoadCustomUIs(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		dir := t.TempDir()
		writeUI(t, dir, "simple", `{"name":"simple","title":"Simple"}`, map[string]string{"index.html": "<html>"})
		writeUI(t, dir, "no-manifest", "", map[string]string{"index.html": "<html>"})

		res, err := loadCustomUIs(dir)
		require.NoError(t, err)
		require.Len(t, res, 1)
		assert.Equal(t, "simple", res[0].Name)
		assert.Equal(t, "Simple", res[0].Title)
		assert.Equal(t, "index.html", res[0].Entry)
	})

	t.Run("missing dir", func(t *testing.T) {
		_, err := loadCustomUIs(filepath.Join(t.TempDir(), "missing"))
		require.Error(t, err)
	})

	for name, tc := range map[string]struct {
		dirName  string
		manifest string
	}{
		"reserved name":   {"classic", `{"name":"classic"}`},
		"invalid name":    {"Bad_Name", `{"name":"Bad_Name"}`},
		"name mismatch":   {"simple", `{"name":"other"}`},
		"entry traversal": {"simple", `{"name":"simple","entry":"../index.html"}`},
		"entry missing":   {"simple", `{"name":"simple","entry":"app.html"}`},
		"broken manifest": {"simple", `{`},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeUI(t, dir, tc.dirName, tc.manifest, map[string]string{"index.html": "<html>"})
			_, err := loadCustomUIs(dir)
			require.Error(t, err)
		})
	}
}

func TestNewUIRegistry(t *testing.T) {
	classic := fstest.MapFS{"index.html": {Data: []byte("classic")}}

	t.Run("default classic", func(t *testing.T) {
		r, err := newUIRegistry(globalconfig.UI{}, classic)
		require.NoError(t, err)
		assert.Equal(t, UIClassic, r.def)
		assert.Equal(t, UIClassic, r.resolve("unknown"))
	})

	t.Run("unknown default", func(t *testing.T) {
		_, err := newUIRegistry(globalconfig.UI{Default: "nope"}, classic)
		require.Error(t, err)
	})

	t.Run("cookie and default", func(t *testing.T) {
		dir := t.TempDir()
		writeUI(t, dir, "simple", `{"name":"simple"}`, map[string]string{"index.html": "<html>"})

		r, err := newUIRegistry(globalconfig.UI{CustomDir: dir, Default: "simple"}, classic)
		require.NoError(t, err)
		assert.Equal(t, "simple", r.resolve(""))
		assert.Equal(t, UIClassic, r.resolve(UIClassic))
		assert.Equal(t, "simple", r.resolve("../etc"))
	})
}

func newTestHTTPd(t *testing.T, cfg globalconfig.UI) *HTTPd {
	t.Helper()

	old := assets.Web
	t.Cleanup(func() { assets.Web = old })
	assets.Web = fstest.MapFS{
		"index.html":     {Data: []byte("classic [[.Version]]")},
		"assets/app.js":  {Data: []byte("classic-js")},
		"meta/icon.png":  {Data: []byte("png")},
		"assets/a/b.css": {Data: []byte("css")},
	}
	if assets.I18n == nil {
		assets.I18n = fstest.MapFS{}
	}

	srv := NewHTTPd(":0", NewSocketHub(), Customization{}, cfg)
	srv.Router().HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	return srv
}

func get(h http.Handler, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUIServing(t *testing.T) {
	dir := t.TempDir()
	writeUI(t, dir, "simple", `{"name":"simple","title":"Simple"}`, map[string]string{
		"index.html":    "<html>simple [[.Version]]</html>",
		"assets/app.js": "simple-js",
	})

	srv := newTestHTTPd(t, globalconfig.UI{CustomDir: dir})
	h := srv.Handler

	t.Run("root serves classic by default", func(t *testing.T) {
		rec := get(h, "/")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "classic")
		assert.Contains(t, rec.Header().Values("Vary"), "Cookie")
	})

	t.Run("root follows cookie", func(t *testing.T) {
		rec := get(h, "/", &http.Cookie{Name: uiCookie, Value: "simple"})
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Equal(t, "/ui/simple/", rec.Header().Get("Location"))
	})

	t.Run("root ignores unknown cookie", func(t *testing.T) {
		rec := get(h, "/", &http.Cookie{Name: uiCookie, Value: "../evil"})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("mount redirects without trailing slash", func(t *testing.T) {
		rec := get(h, "/ui/simple?x=1")
		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
		assert.Equal(t, "/ui/simple/?x=1", rec.Header().Get("Location"))
	})

	t.Run("mount serves raw index and sets cookie", func(t *testing.T) {
		rec := get(h, "/ui/simple/")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "<html>simple [[.Version]]</html>", rec.Body.String())

		cookies := rec.Result().Cookies()
		require.Len(t, cookies, 1)
		assert.Equal(t, "simple", cookies[0].Value)
		assert.True(t, cookies[0].HttpOnly)
		assert.Equal(t, "/", cookies[0].Path)
	})

	t.Run("mount of default clears cookie", func(t *testing.T) {
		rec := get(h, "/ui/classic/")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "classic")

		cookies := rec.Result().Cookies()
		require.Len(t, cookies, 1)
		assert.Equal(t, uiCookie, cookies[0].Name)
		assert.Negative(t, cookies[0].MaxAge)
	})

	t.Run("mount serves files", func(t *testing.T) {
		rec := get(h, "/ui/simple/assets/app.js")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "simple-js", rec.Body.String())
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.Empty(t, rec.Result().Cookies())

		rec = get(h, "/ui/classic/assets/app.js")
		assert.Equal(t, "classic-js", rec.Body.String())
	})

	t.Run("missing file with extension is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, get(h, "/ui/simple/missing.js").Code)
	})

	t.Run("extension-less path falls back to index", func(t *testing.T) {
		rec := get(h, "/ui/simple/some/route")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "simple")
	})

	t.Run("unknown ui", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, get(h, "/ui/other/").Code)
	})

	t.Run("only get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/ui/simple/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("backend endpoints stay reachable", func(t *testing.T) {
		assert.Equal(t, "pong", get(h, "/api/ping").Body.String())
		assert.Equal(t, "pong", get(h, "/ui/simple/api/ping").Body.String())
		assert.Equal(t, "pong", get(h, "/ui/classic/api/ping").Body.String())
		assert.Contains(t, get(h, "/ui/simple/ws").Body.String(), "WebSocket")
		assert.Equal(t, http.StatusNotFound, get(h, "/ui/other/api/ping").Code)
	})

	t.Run("alias does not leak other routes", func(t *testing.T) {
		assert.NotEqual(t, "pong", get(h, "/ui/simple/config").Body.String())
		assert.Equal(t, http.StatusNotFound, get(h, "/ui/simple/api/../../x").Code)
	})
}

func TestUITraversal(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "uis")
	writeUI(t, dir, "simple", `{"name":"simple"}`, map[string]string{"index.html": "<html>"})

	secret := filepath.Join(base, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "simple", "link.txt")))
	require.NoError(t, os.Symlink(base, filepath.Join(dir, "simple", "up")))

	h := newTestHTTPd(t, globalconfig.UI{CustomDir: dir}).Handler

	for _, target := range []string{
		"/ui/simple/link.txt",
		"/ui/simple/up/secret.txt",
		"/ui/simple/../secret.txt",
		"/ui/simple/%2e%2e/secret.txt",
		"/ui/simple/..%2fsecret.txt",
		"/ui/simple//etc/passwd",
	} {
		rec := get(h, target)
		assert.NotContains(t, rec.Body.String(), "secret", target)
	}
}
