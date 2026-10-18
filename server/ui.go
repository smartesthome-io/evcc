package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/evcc-io/evcc/api/globalconfig"
	"github.com/gorilla/mux"
)

const (
	// UIClassic is the first-party frontend, always available
	UIClassic = "classic"

	uiCookie    = "evcc-ui"
	uiManifest  = "ui.json"
	uiMountPath = "/ui/"
)

var (
	uiNameRegexp = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

	// reserved names cannot be used by custom UIs
	uiReserved = []string{UIClassic, "api", "ws", "assets", "meta", "i18n"}

	// backend endpoints a mounted UI can reach with relative urls
	uiAliasRegexp = regexp.MustCompile(`^/ui/([a-z0-9-]+)/((?:api|i18n|providerauth)(?:/.*)?|ws|globals\.js|custom\.css|custom-logo-(?:light|dark))$`)
)

// uiManifestFile is the ui.json of a custom UI
type uiManifestFile struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Entry string `json:"entry"`
}

// uiEntry is an installed frontend
type uiEntry struct {
	Name  string
	Title string
	Entry string
	FS    fs.FS
}

// uiRegistry holds the installed frontends and the server default
type uiRegistry struct {
	entries map[string]uiEntry
	def     string
}

// newUIRegistry assembles the first-party and custom frontends from configuration
func newUIRegistry(cfg globalconfig.UI, classic fs.FS) (*uiRegistry, error) {
	r := &uiRegistry{
		entries: map[string]uiEntry{
			UIClassic: {Name: UIClassic, Title: "evcc", Entry: "index.html", FS: classic},
		},
		def: UIClassic,
	}

	if cfg.CustomDir != "" {
		custom, err := loadCustomUIs(cfg.CustomDir)
		if err != nil {
			return nil, err
		}
		for _, e := range custom {
			r.entries[e.Name] = e
		}
	}

	if cfg.Default != "" {
		if _, ok := r.entries[cfg.Default]; !ok {
			return nil, fmt.Errorf("ui.default: unknown ui %q", cfg.Default)
		}
		r.def = cfg.Default
	}

	return r, nil
}

// loadCustomUIs reads all custom UIs below dir. Directories without manifest are ignored.
func loadCustomUIs(dir string) ([]uiEntry, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("ui.customDir: %w", err)
	}

	children, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("ui.customDir: %w", err)
	}

	var res []uiEntry
	for _, child := range children {
		if !child.IsDir() {
			continue
		}

		name := child.Name()
		if !uiNameRegexp.MatchString(name) {
			return nil, fmt.Errorf("ui.customDir: invalid ui name %q (expected lowercase letters, digits and dashes)", name)
		}
		if slices.Contains(uiReserved, name) {
			return nil, fmt.Errorf("ui.customDir: ui name %q is reserved", name)
		}

		root, err := os.OpenRoot(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("ui %s: %w", name, err)
		}

		if _, err := fs.Stat(root.FS(), uiManifest); errors.Is(err, fs.ErrNotExist) {
			continue
		}

		entry, err := readUIManifest(name, root.FS())
		if err != nil {
			return nil, fmt.Errorf("ui %s: %w", name, err)
		}

		res = append(res, entry)
	}

	return res, nil
}

// readUIManifest validates the ui.json at the root of the given file system
func readUIManifest(name string, fsys fs.FS) (uiEntry, error) {
	b, err := fs.ReadFile(fsys, uiManifest)
	if err != nil {
		return uiEntry{}, err
	}

	var m uiManifestFile
	if err := json.Unmarshal(b, &m); err != nil {
		return uiEntry{}, fmt.Errorf("%s: %w", uiManifest, err)
	}

	if m.Name != name {
		return uiEntry{}, fmt.Errorf("%s: name %q does not match directory", uiManifest, m.Name)
	}
	if m.Entry == "" {
		m.Entry = "index.html"
	}
	if !fs.ValidPath(m.Entry) || m.Entry == "." {
		return uiEntry{}, fmt.Errorf("%s: invalid entry %q", uiManifest, m.Entry)
	}
	if _, err := fs.Stat(fsys, m.Entry); err != nil {
		return uiEntry{}, fmt.Errorf("entry %q: %w", m.Entry, err)
	}
	if m.Title == "" {
		m.Title = name
	}

	return uiEntry{Name: name, Title: m.Title, Entry: m.Entry, FS: fsys}, nil
}

// resolve returns the ui to show for the given cookie value
func (r *uiRegistry) resolve(cookie string) string {
	if _, ok := r.entries[cookie]; ok {
		return cookie
	}
	return r.def
}

// names returns the installed ui names
func (r *uiRegistry) names() []string {
	res := make([]string, 0, len(r.entries))
	for name := range r.entries {
		res = append(res, name)
	}
	slices.Sort(res)
	return res
}

// rootHandler serves the ui selected by cookie or server default at /
func (r *uiRegistry) rootHandler(classicIndex http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Vary", "Cookie")

		var cookie string
		if c, err := req.Cookie(uiCookie); err == nil {
			cookie = c.Value
		}

		name := r.resolve(cookie)
		if name == UIClassic {
			classicIndex(w, req)
			return
		}

		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		http.Redirect(w, req, uiMountPath+name+"/", http.StatusFound)
	}
}

// mountHandler serves a ui below /ui/{name}/
func (r *uiRegistry) mountHandler(classicIndex http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)
		entry, ok := r.entries[vars["name"]]
		if !ok {
			http.NotFound(w, req)
			return
		}

		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		// relative urls of the ui only work with a trailing slash
		rest := strings.TrimPrefix(req.URL.Path, uiMountPath+entry.Name)
		if rest == "" {
			target := uiMountPath + entry.Name + "/"
			if req.URL.RawQuery != "" {
				target += "?" + req.URL.RawQuery
			}
			http.Redirect(w, req, target, http.StatusMovedPermanently)
			return
		}

		file := strings.TrimPrefix(rest, "/")
		if file == "" || file == entry.Entry {
			r.setCookie(w, req, entry.Name)
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			if entry.Name == UIClassic {
				classicIndex(w, req)
				return
			}
			r.serveFile(w, req, entry, entry.Entry)
			return
		}

		// extension-less paths are client-side routes
		if path.Ext(file) == "" {
			r.setCookie(w, req, entry.Name)
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			r.serveFile(w, req, entry, entry.Entry)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		r.serveFile(w, req, entry, file)
	}
}

// serveFile serves a regular file from the ui file system
func (r *uiRegistry) serveFile(w http.ResponseWriter, req *http.Request, entry uiEntry, name string) {
	if !fs.ValidPath(name) {
		http.NotFound(w, req)
		return
	}

	b, err := fs.ReadFile(entry.FS, name)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	var modTime time.Time
	if stat, err := fs.Stat(entry.FS, name); err == nil {
		modTime = stat.ModTime()
	}

	http.ServeContent(w, req, name, modTime, bytes.NewReader(b))
}

// setCookie remembers a deviation from the server default. Visiting the default again clears it.
func (r *uiRegistry) setCookie(w http.ResponseWriter, req *http.Request, name string) {
	c := &http.Cookie{
		Name:     uiCookie,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https",
	}

	if name == r.def {
		c.MaxAge = -1
	} else {
		c.Value = name
		c.Expires = time.Now().Add(365 * 24 * time.Hour)
		c.MaxAge = 365 * 24 * 60 * 60
	}

	http.SetCookie(w, c)
}

// aliasHandler lets mounted uis reach backend endpoints with relative urls
func (r *uiRegistry) aliasHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if m := uiAliasRegexp.FindStringSubmatch(req.URL.Path); m != nil {
			if _, ok := r.entries[m[1]]; ok {
				target := "/" + m[2]
				if strings.Contains(target, "..") || strings.Contains(target, "//") {
					http.NotFound(w, req)
					return
				}

				req2 := req.Clone(req.Context())
				req2.URL = &url.URL{Path: target, RawQuery: req.URL.RawQuery}
				req2.RequestURI = req2.URL.RequestURI()
				next.ServeHTTP(w, req2)
				return
			}
		}

		next.ServeHTTP(w, req)
	})
}
