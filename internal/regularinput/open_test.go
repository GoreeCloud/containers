package regularinput

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenRejectsSubstitution(t *testing.T) {
	for _, kind := range []string{"regular", "fifo", "symlink", "replacement"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "input")
			if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			expected, err := root.Lstat("input")
			if err != nil {
				t.Fatal(err)
			}
			if kind != "regular" {
				if err := os.Rename(name, name+".old"); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "fifo":
					err = syscall.Mkfifo(name, 0600)
				case "symlink":
					err = os.Symlink("input.old", name)
				case "replacement":
					err = os.WriteFile(name, []byte("replaced"), 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			f, err := Open(root, "input", expected)
			if f != nil {
				defer f.Close()
			}
			if (err == nil) != (kind == "regular") {
				t.Fatalf("unexpected admission: %v", err)
			}
		})
	}
}
