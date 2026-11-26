package ociimage

import (
	"context"
	"errors"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"path/filepath"
)

// Only called on the fresh verified staging root; no external filesystem
// mode changes. All regular files stay nonexecutable except the pinned ELF.
func normalizeDelegatedStaging(ctx context.Context, root *os.Root, entry string) error {
	f, e := root.Open(".")
	if e != nil {
		return e
	}
	mount, e := stagedMount(f)
	f.Close()
	if e != nil {
		return e
	}
	entries := 0
	var walk func(string) error
	walk = func(name string) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		entries++
		if entries > MaxLayerEntries+4 {
			return errors.New("delegated staging quota exceeded")
		}
		i, e := root.Lstat(name)
		if e != nil {
			return e
		}
		var f *os.File
		mode := os.FileMode(0444)
		if i.IsDir() {
			if privateStoreInfo(i) != nil {
				return errors.New("unsafe staged directory")
			}
			f, e = root.Open(name)
			mode = 0755
		} else {
			if name == entry {
				if i.Mode().Perm() != 0500 {
					return errors.New("entrypoint mode changed")
				}
				mode = 0555
			} else if !safeStagedFile(i) {
				return errors.New("unsafe staged data")
			}
			f, e = regularinput.Open(root, name, i)
		}
		if e != nil {
			return e
		}
		defer f.Close()
		a, e := f.Stat()
		if e != nil || !os.SameFile(i, a) {
			return errors.New("staged identity changed")
		}
		m, e := stagedMount(f)
		if e != nil || m != mount {
			return errors.New("staged mount changed")
		}
		if i.IsDir() {
			names, e := f.Readdirnames(MaxLayerEntries + 5)
			if e != nil && e != io.EOF {
				return e
			}
			if len(names) > MaxLayerEntries+4 {
				return errors.New("staged directory quota exceeded")
			}
			for _, n := range names {
				if e = walk(filepath.Join(name, n)); e != nil {
					return e
				}
			}
		}
		current, e := root.Lstat(name)
		if e != nil || !os.SameFile(i, current) || i.Mode() != current.Mode() {
			return errors.New("staged entry changed")
		}
		if e = f.Chmod(mode); e != nil {
			return e
		}
		return f.Sync()
	}
	return walk(".")
}
