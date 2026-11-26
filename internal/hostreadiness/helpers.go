package hostreadiness

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"syscall"
)

type HelperFact struct {
	Name              string `json:"name"`
	Present           bool   `json:"present"`
	OwnerModeVerified bool   `json:"ownerModeVerified"`
}
type HelperReport struct {
	ReadOnly             bool         `json:"readOnly"`
	RootlessCaller       bool         `json:"rootlessCaller"`
	Helpers              []HelperFact `json:"helpers"`
	TunDevicePresent     bool         `json:"tunDevicePresent"`
	FuseDevicePresent    bool         `json:"fuseDevicePresent"`
	NetworkingQualified  bool         `json:"networkingQualified"`
	StorageQualified     bool         `json:"storageQualified"`
	DelegationQualified  bool         `json:"delegationQualified"`
	DeploymentAuthorized bool         `json:"deploymentAuthorized"`
}
type fileInspector func(string) (fs.FileInfo, error)

func trustedPath(inspect fileInspector, name string, special os.FileMode) (bool, bool) {
	i, e := inspect(name)
	if e != nil {
		return false, false
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || st.Gid != 0 || !i.Mode().IsRegular() || i.Mode().Perm()&0022 != 0 || i.Mode().Perm()&0111 == 0 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != special {
		return true, false
	}
	for p := path.Dir(name); ; p = path.Dir(p) {
		i, e = inspect(p)
		if e != nil {
			return true, false
		}
		st, ok = i.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || !i.IsDir() || i.Mode().Perm()&0022 != 0 {
			return true, false
		}
		if p == "/" {
			break
		}
	}
	return true, true
}
func inspectHelpers(ctx context.Context, inspect fileInspector, nonroot bool) (HelperReport, error) {
	var empty HelperReport
	if ctx == nil || inspect == nil {
		return empty, errors.New("context and inspector required")
	}
	r := HelperReport{ReadOnly: true, RootlessCaller: nonroot, Helpers: []HelperFact{}}
	for _, v := range []struct {
		name    string
		special os.FileMode
	}{{"newuidmap", os.ModeSetuid}, {"newgidmap", os.ModeSetuid}, {"pasta", 0}, {"slirp4netns", 0}, {"fuse-overlayfs", 0}} {
		if e := ctx.Err(); e != nil {
			return empty, e
		}
		present, verified := trustedPath(inspect, "/usr/bin/"+v.name, v.special)
		r.Helpers = append(r.Helpers, HelperFact{v.name, present, verified})
	}
	device := func(n string, major, minor uint64) bool {
		i, e := inspect(n)
		if e != nil || i.Mode()&(os.ModeDevice|os.ModeCharDevice) != (os.ModeDevice|os.ModeCharDevice) {
			return false
		}
		st, ok := i.Sys().(*syscall.Stat_t)
		if !ok {
			return false
		}
		d := uint64(st.Rdev)
		return (d>>8)&0xfff == major && (d&0xff) == minor
	}
	r.TunDevicePresent = device("/dev/net/tun", 10, 200)
	r.FuseDevicePresent = device("/dev/fuse", 10, 229)
	return r, ctx.Err()
}

// InspectHelpers never runs privileged helpers or opens device nodes.
func InspectHelpers(ctx context.Context) (HelperReport, error) {
	return inspectHelpers(ctx, os.Lstat, os.Geteuid() != 0)
}
