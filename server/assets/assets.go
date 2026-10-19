package assets

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	// Web is the embedded dist file system
	Web fs.FS

	// I18n is the embedded i18n file system
	I18n fs.FS
)

// Live indicates assets are passed-through from filesystem
func Live() bool {
	return Web != nil
}

// UseWebDir replaces the web file system with the given directory, which must contain an index.html
func UseWebDir(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return fmt.Errorf("ui dir: %w", err)
	}
	Web = os.DirFS(dir)
	return nil
}
