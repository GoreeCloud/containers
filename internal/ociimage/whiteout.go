package ociimage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

type whiteoutMarker struct {
	target string
	opaque bool
}

func parseWhiteout(n string) (whiteoutMarker, error) {
	if !validLayerName(n) {
		return whiteoutMarker{}, errors.New("invalid whiteout path")
	}
	base := path.Base(n)
	if base == ".wh..wh..opq" {
		return whiteoutMarker{path.Dir(n), true}, nil
	}
	if !strings.HasPrefix(base, ".wh.") {
		return whiteoutMarker{}, errors.New("invalid whiteout marker")
	}
	name := strings.TrimPrefix(base, ".wh.")
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".wh.") {
		return whiteoutMarker{}, errors.New("invalid whiteout target")
	}
	target := path.Join(path.Dir(n), name)
	if !validLayerName(target) || target == "." {
		return whiteoutMarker{}, errors.New("invalid whiteout target")
	}
	return whiteoutMarker{target, false}, nil
}
func verifiedWhiteouts(ctx context.Context, l LayerInput) ([]whiteoutMarker, error) {
	seeker, ok := l.Reader.(io.ReadSeeker)
	if !ok {
		return nil, errors.New("whiteout assembly requires seekable pinned layers")
	}
	if _, e := seeker.Seek(0, io.SeekStart); e != nil {
		return nil, e
	}
	tracked := &trackedLayerReader{input: seeker, hash: sha256.New()}
	decoded := sha256.New()
	markers := []whiteoutMarker{}
	_, e := inspectLayerPolicy(ctx, tracked, l.Media, nil, decoded, func(n string) error {
		m, e := parseWhiteout(n)
		if e != nil {
			return e
		}
		markers = append(markers, m)
		return nil
	})
	if e != nil {
		return nil, e
	}
	if tracked.bytes != l.Size || fmt.Sprintf("sha256:%x", tracked.hash.Sum(nil)) != l.Digest || l.DiffID != "" && fmt.Sprintf("sha256:%x", decoded.Sum(nil)) != l.DiffID {
		return nil, errors.New("whiteout layer pin mismatch")
	}
	if _, e = seeker.Seek(0, io.SeekStart); e != nil {
		return nil, e
	}
	return markers, nil
}
func stagedMount(f *os.File) (uint64, error) {
	info, e := os.Open(fmt.Sprintf("/proc/self/fdinfo/%d", f.Fd()))
	if e != nil {
		return 0, e
	}
	defer info.Close()
	raw, e := io.ReadAll(io.LimitReader(info, 65537))
	if e != nil || len(raw) > 65536 {
		return 0, errors.New("staging mount identity unavailable")
	}
	var id uint64
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "mnt_id:") {
			n, e := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "mnt_id:")), 10, 64)
			if e != nil || n == 0 || id != 0 {
				return 0, errors.New("invalid staging mount identity")
			}
			id = n
		}
	}
	if id == 0 {
		return 0, errors.New("staging mount identity missing")
	}
	return id, nil
}

// Markers affect lower layers only, independent of archive member order.
// Remove only tracked entries in the fresh tree, leaf-first; never recursively
// delete an untracked path, follow a link or cross an observed mount.
func applyWhiteouts(ctx context.Context, root *os.Root, dirs map[string]bool, dirIDs, files map[string]os.FileInfo, markers []whiteoutMarker) error {
	if len(markers) == 0 {
		return nil
	}
	rf, e := root.Open(".")
	if e != nil {
		return e
	}
	mount, e := stagedMount(rf)
	rf.Close()
	if e != nil {
		return e
	}
	selected := map[string]bool{}
	for _, m := range markers {
		if m.opaque && files[m.target] != nil {
			return errors.New("opaque marker targets lower regular file")
		}
		for p := path.Dir(m.target); p != "." && p != "/"; p = path.Dir(p) {
			if files[p] != nil {
				return errors.New("whiteout parent is a lower regular file")
			}
		}
		matches := func(n string) bool {
			if n == "." {
				return false
			}
			if m.opaque {
				return m.target == "." || strings.HasPrefix(n, m.target+"/")
			}
			return n == m.target || strings.HasPrefix(n, m.target+"/")
		}
		for n := range dirs {
			if matches(n) {
				selected[n] = true
			}
		}
		for n := range files {
			if matches(n) {
				selected[n] = true
			}
		}
	}
	names := make([]string, 0, len(selected))
	for n := range selected {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := strings.Count(names[i], "/"), strings.Count(names[j], "/")
		if a != b {
			return a > b
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		if e = ctx.Err(); e != nil {
			return e
		}
		info, e := root.Lstat(n)
		if e != nil {
			return e
		}
		var f *os.File
		if dirs[n] {
			if privateStoreInfo(info) != nil || dirIDs[n] == nil || !os.SameFile(dirIDs[n], info) {
				return errors.New("staged directory identity changed")
			}
			f, e = root.Open(n)
		} else {
			if !safeStagedFile(info) || files[n] == nil || !os.SameFile(files[n], info) {
				return errors.New("staged file identity changed")
			}
			f, e = regularinput.Open(root, n, info)
		}
		if e != nil {
			return e
		}
		actual, e := f.Stat()
		if e != nil || !os.SameFile(info, actual) {
			f.Close()
			return errors.New("staged entry changed")
		}
		m, e := stagedMount(f)
		f.Close()
		if e != nil || m != mount {
			return errors.New("whiteout mount boundary crossed")
		}
		current, e := root.Lstat(n)
		if e != nil || !os.SameFile(info, current) {
			return errors.New("whiteout entry changed")
		}
		if e = root.Remove(n); e != nil {
			return e
		}
		delete(dirs, n)
		delete(dirIDs, n)
		delete(files, n)
	}
	return nil
}
