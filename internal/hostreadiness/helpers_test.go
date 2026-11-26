package hostreadiness

import (
	"context"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"
)

type helperInfo struct {
	mode os.FileMode
	uid  uint32
	rdev uint64
}

func (i helperInfo) Name() string       { return "fixture" }
func (i helperInfo) Size() int64        { return 1 }
func (i helperInfo) Mode() os.FileMode  { return i.mode }
func (i helperInfo) ModTime() time.Time { return time.Time{} }
func (i helperInfo) IsDir() bool        { return i.mode.IsDir() }
func (i helperInfo) Sys() any           { return &syscall.Stat_t{Uid: i.uid, Rdev: i.rdev} }
func TestHelpersReportDoesNotAuthorize(t *testing.T) {
	f := map[string]helperInfo{"/": {mode: os.ModeDir | 0755}, "/usr": {mode: os.ModeDir | 0755}, "/usr/bin": {mode: os.ModeDir | 0755}, "/usr/bin/newuidmap": {mode: os.ModeSetuid | 0755}, "/usr/bin/newgidmap": {mode: os.ModeSetuid | 0755}, "/usr/bin/pasta": {mode: 0755}, "/usr/bin/slirp4netns": {mode: 0777}, "/usr/bin/fuse-overlayfs": {mode: 0755, uid: 1000}, "/dev/net/tun": {mode: os.ModeDevice | os.ModeCharDevice | 0666, rdev: 10<<8 | 200}}
	inspect := func(n string) (fs.FileInfo, error) {
		if i, ok := f[n]; ok {
			return i, nil
		}
		return nil, os.ErrNotExist
	}
	r, e := inspectHelpers(context.Background(), inspect, true)
	if e != nil || !r.Helpers[0].OwnerModeVerified || !r.Helpers[2].OwnerModeVerified || r.Helpers[3].OwnerModeVerified || r.Helpers[4].OwnerModeVerified || !r.TunDevicePresent || r.FuseDevicePresent || r.NetworkingQualified || r.DelegationQualified || r.DeploymentAuthorized {
		t.Fatal(r, e)
	}
}
