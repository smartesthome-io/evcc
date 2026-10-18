//go:build !release

package assets

import (
	"os"
)

func init() {
	Web = os.DirFS("ui/classic/dist")
	I18n = os.DirFS("i18n")
}
