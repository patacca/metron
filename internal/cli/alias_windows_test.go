//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// alias returns another spelling of the directory dir: a symbolic link to it
// where the account may create one, else its short (8.3) name, the spelling
// Windows gives temporary directories.
func alias(t *testing.T, dir string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "alias")
	if os.Symlink(dir, link) == nil {
		return link
	}
	long, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, syscall.MAX_LONG_PATH)
	n, err := syscall.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		t.Fatalf("no other spelling of %s: %v", dir, err)
	}
	return syscall.UTF16ToString(buf[:n])
}
