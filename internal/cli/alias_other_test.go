//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// alias returns another spelling of the directory dir: a symbolic link to it,
// as macOS reaches its temporary directories (/var is a link to /private/var).
func alias(t *testing.T, dir string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	return link
}
