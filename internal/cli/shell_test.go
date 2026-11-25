package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindShell(t *testing.T) {
	notFound := func(string) (string, error) { return "", errors.New("not found") }
	// layout creates a fake Git for Windows installation with sh in sub.
	layout := func(t *testing.T, sub string) (root, execPath string) {
		root = t.TempDir()
		execPath = filepath.Join(root, "mingw64", "libexec", "git-core")
		for _, d := range []string{execPath, filepath.Join(root, filepath.FromSlash(sub))} {
			if err := os.MkdirAll(d, 0o777); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(sub), "sh.exe"), nil, 0o666); err != nil {
			t.Fatal(err)
		}
		// git prints its exec path with forward slashes and a newline.
		return root, filepath.ToSlash(execPath) + "\n"
	}
	for _, tc := range []struct {
		name string
		sub  string
		path func(string) (string, error)
		git  bool
		want string
	}{
		{"on PATH", "usr/bin", func(string) (string, error) { return "/somewhere/sh.exe", nil }, true, "/somewhere/sh.exe"},
		{"usr/bin", "usr/bin", notFound, true, "usr/bin/sh.exe"},
		{"bin", "bin", notFound, true, "bin/sh.exe"},
		{"no git", "usr/bin", notFound, false, ""},
		{"no sh", "elsewhere", notFound, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, execPath := layout(t, tc.sub)
			gitExecPath := func() (string, error) { return execPath, nil }
			if !tc.git {
				gitExecPath = func() (string, error) { return "", errors.New("git: not found") }
			}
			got, err := findShell(tc.path, gitExecPath, "sh.exe")
			switch {
			case tc.want == "":
				if err == nil || !strings.Contains(err.Error(), "the sh that ships with Git for Windows") {
					t.Fatalf("found %q, error %v", got, err)
				}
			case strings.HasPrefix(tc.want, "/"):
				if got != tc.want || err != nil {
					t.Fatalf("found %q, error %v; want %q", got, err, tc.want)
				}
			default:
				if want := filepath.Join(root, filepath.FromSlash(tc.want)); got != want || err != nil {
					t.Fatalf("found %q, error %v; want %q", got, err, want)
				}
			}
		})
	}
}
