// Package bundle checks an intentionally restricted, rootless OCI bundle profile.
// This is defense-in-depth, NOT a substitute for an audited OCI runtime or sandbox.
package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxConfigBytes = 1 << 20

type Spec struct {
	OCIVersion  string            `json:"ociVersion"`
	Process     *Process          `json:"process"`
	Root        *Root             `json:"root"`
	Linux       *Linux            `json:"linux"`
	Mounts      []Mount           `json:"mounts"`
	Hooks       json.RawMessage   `json:"hooks"`
	Hostname    string            `json:"hostname"`
	Annotations map[string]string `json:"annotations"`
}
type Process struct {
	Args []string `json:"args"`
	Cwd  string   `json:"cwd"`
	Env  []string `json:"env"`
	User struct {
		UID uint32 `json:"uid"`
		GID uint32 `json:"gid"`
	} `json:"user"`
	Capabilities    *Capabilities `json:"capabilities"`
	NoNewPrivileges bool          `json:"noNewPrivileges"`
	Terminal        bool          `json:"terminal"`
}
type Capabilities struct {
	Bounding    []string `json:"bounding"`
	Effective   []string `json:"effective"`
	Inheritable []string `json:"inheritable"`
	Permitted   []string `json:"permitted"`
	Ambient     []string `json:"ambient"`
}
type Root struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly"`
}
type Linux struct {
	Namespaces  []Namespace       `json:"namespaces"`
	UIDMappings []IDMap           `json:"uidMappings"`
	GIDMappings []IDMap           `json:"gidMappings"`
	Seccomp     *Seccomp          `json:"seccomp"`
	Devices     []json.RawMessage `json:"devices"`
}
type Namespace struct {
	Type string `json:"type"`
	Path string `json:"path"`
}
type IDMap struct {
	ContainerID uint32 `json:"containerID"`
	HostID      uint32 `json:"hostID"`
	Size        uint32 `json:"size"`
}
type Seccomp struct {
	DefaultAction string            `json:"defaultAction"`
	Architectures []string          `json:"architectures"`
	Syscalls      []json.RawMessage `json:"syscalls"`
	Flags         []string          `json:"flags"`
}
type Mount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Source      string   `json:"source"`
	Options     []string `json:"options"`
}

// Validate reads no more than 1 MiB and never executes a bundle.
func Validate(dir string) error {
	bundleDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("bundle directory: %w", err)
	}
	fi, err := os.Stat(bundleDir)
	if err != nil || !fi.IsDir() {
		return errors.New("bundle must be a directory")
	}
	path := filepath.Join(bundleDir, "config.json")
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("OCI config: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("OCI config must be a regular file (not a symlink)")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("OCI config: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err != nil {
		return errors.New("unable to read OCI config")
	}
	if len(b) > MaxConfigBytes {
		return errors.New("OCI config exceeds 1 MiB limit")
	}
	if err := rejectDuplicateKeys(b); err != nil {
		return fmt.Errorf("OCI JSON: %w", err)
	}
	var spec Spec
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&spec); err != nil {
		return fmt.Errorf("unsupported or invalid OCI config: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("OCI config has trailing data")
	}
	return validateSpec(&spec, bundleDir)
}

func validateSpec(s *Spec, dir string) error {
	if s.OCIVersion == "" || s.Process == nil || s.Root == nil || s.Linux == nil {
		return errors.New("OCI version, process, root and linux are required")
	}
	if len(s.Hooks) != 0 && string(s.Hooks) != "null" {
		return errors.New("OCI hooks are prohibited in the preview profile")
	}
	if s.Process.Terminal || !s.Process.NoNewPrivileges || len(s.Process.Args) == 0 || s.Process.Args[0] == "" {
		return errors.New("noninteractive process, arguments and noNewPrivileges=true required")
	}
	c := s.Process.Capabilities
	if c == nil || len(c.Bounding)+len(c.Effective)+len(c.Inheritable)+len(c.Permitted)+len(c.Ambient) != 0 {
		return errors.New("all process capability sets must be explicitly empty")
	}
	if !s.Root.Readonly || !filepath.IsLocal(s.Root.Path) {
		return errors.New("root.path must be local and root.readonly must be true")
	}
	rootPath, err := filepath.EvalSymlinks(filepath.Join(dir, s.Root.Path))
	if err != nil {
		return errors.New("rootfs directory not available")
	}
	rel, err := filepath.Rel(dir, rootPath)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return errors.New("rootfs must resolve inside the OCI bundle")
	}
	rootInfo, err := os.Stat(rootPath)
	if err != nil || !rootInfo.IsDir() {
		return errors.New("rootfs must be a directory")
	}
	required := map[string]bool{"user": false, "pid": false, "mount": false, "network": false, "ipc": false, "uts": false}
	seen := map[string]bool{}
	for _, ns := range s.Linux.Namespaces {
		if ns.Path != "" || seen[ns.Type] {
			return errors.New("joined or duplicate namespaces prohibited")
		}
		if _, ok := required[ns.Type]; !ok && ns.Type != "cgroup" {
			return errors.New("unsupported namespace")
		}
		required[ns.Type] = true
		seen[ns.Type] = true
	}
	for ns, ok := range required {
		if !ok {
			return fmt.Errorf("private %s namespace required", ns)
		}
	}
	if !validMaps(s.Linux.UIDMappings) || !validMaps(s.Linux.GIDMappings) {
		return errors.New("nonzero rootless UID/GID mappings are required")
	}
	if len(s.Linux.Devices) != 0 {
		return errors.New("custom devices are prohibited")
	}
	if s.Linux.Seccomp == nil {
		return errors.New("seccomp profile is required")
	}
	switch s.Linux.Seccomp.DefaultAction {
	case "SCMP_ACT_ERRNO", "SCMP_ACT_KILL", "SCMP_ACT_KILL_PROCESS", "SCMP_ACT_KILL_THREAD":
	default:
		return errors.New("seccomp default must deny unlisted syscalls")
	}

	destinations := map[string]bool{}
	for _, m := range s.Mounts {
		if destinations[m.Destination] {
			return errors.New("duplicate mount destination")
		}
		destinations[m.Destination] = true
		if !strings.HasPrefix(m.Destination, "/") || filepath.Clean(m.Destination) != m.Destination {
			return errors.New("invalid mount destination")
		}
		switch m.Type {
		case "proc":
			if m.Destination != "/proc" || m.Source != "proc" || !onlyOptions(m.Options, "ro", "nosuid", "noexec", "nodev") {
				return errors.New("unsupported proc mount policy")
			}
		case "tmpfs":
			if m.Destination != "/tmp" || m.Source != "tmpfs" || !onlyOptions(m.Options, "nosuid", "noexec", "nodev", "size=67108864", "mode=1777") {
				return errors.New("unsupported tmpfs mount policy")
			}
			if !hasOption(m.Options, "size=67108864") || !hasOption(m.Options, "nosuid") || !hasOption(m.Options, "nodev") {
				return errors.New("tmpfs must be memory-limited and protected")
			}
		default:
			return errors.New("unsupported mount type")
		}
	}
	if len(s.Linux.Seccomp.Flags) > 0 {
		return errors.New("seccomp flags must be reviewed before support")
	}
	for _, raw := range s.Linux.Seccomp.Syscalls {
		var rule struct {
			Names  []string          `json:"names"`
			Action string            `json:"action"`
			Args   []json.RawMessage `json:"args"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rule); err != nil {
			return errors.New("invalid seccomp rule")
		}
		if len(rule.Names) == 0 || len(rule.Args) != 0 {
			return errors.New("empty or argument-filtered seccomp rule is unsupported")
		}
		switch rule.Action {
		case "SCMP_ACT_ALLOW", "SCMP_ACT_ERRNO", "SCMP_ACT_KILL", "SCMP_ACT_KILL_PROCESS", "SCMP_ACT_KILL_THREAD":
		default:
			return errors.New("unsupported seccomp rule action")
		}
		for _, name := range rule.Names {
			if name == "" || strings.ContainsAny(name, " /\\") {
				return errors.New("invalid syscall name")
			}
			if rule.Action == "SCMP_ACT_ALLOW" && forbiddenSyscall(name) {
				return errors.New("dangerous syscall cannot be allowed by preview profile")
			}
		}
	}
	return nil
}

func hasOption(options []string, wanted string) bool {
	for _, o := range options {
		if o == wanted {
			return true
		}
	}
	return false
}
func onlyOptions(options []string, allowed ...string) bool {
	seen := map[string]bool{}
	for _, o := range options {
		if seen[o] {
			return false
		}
		seen[o] = true
		found := false
		for _, a := range allowed {
			if o == a {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func forbiddenSyscall(name string) bool {
	switch name {
	case "mount", "umount2", "pivot_root", "ptrace", "bpf", "userfaultfd",
		"open_by_handle_at", "io_uring_setup", "clone3", "unshare", "setns",
		"init_module", "finit_module", "delete_module", "kexec_load",
		"kexec_file_load", "perf_event_open", "keyctl", "add_key", "request_key",
		"swapon", "swapoff", "reboot", "iopl", "ioperm", "syslog":
		return true
	default:
		return false
	}
}

func validMaps(maps []IDMap) bool {
	if len(maps) == 0 || len(maps) > 5 {
		return false
	}
	rootMapped := false
	for i, m := range maps {
		if m.Size == 0 || m.HostID == 0 {
			return false
		}
		hc := uint64(m.HostID) + uint64(m.Size)
		cc := uint64(m.ContainerID) + uint64(m.Size)
		if hc > 1<<32 || cc > 1<<32 {
			return false
		}
		if m.ContainerID == 0 {
			rootMapped = true
		}
		for j := 0; j < i; j++ {
			prior := maps[j]
			if rangesOverlap(uint64(m.HostID), hc, uint64(prior.HostID), uint64(prior.HostID)+uint64(prior.Size)) ||
				rangesOverlap(uint64(m.ContainerID), cc, uint64(prior.ContainerID), uint64(prior.ContainerID)+uint64(prior.Size)) {
				return false
			}
		}
	}
	return rootMapped
}
func rangesOverlap(aStart, aEnd, bStart, bEnd uint64) bool {
	return aStart < bEnd && bStart < aEnd
}

func rejectDuplicateKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := walkJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func walkJSON(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting too deep")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key must be a string")
			}
			if keys[key] {
				return errors.New("duplicate JSON key")
			}
			keys[key] = true
			if err := walkJSON(d, depth+1); err != nil {
				return err
			}
		}
		_, err = d.Token()
	case '[':
		for d.More() {
			if err := walkJSON(d, depth+1); err != nil {
				return err
			}
		}
		_, err = d.Token()
	default:
		return errors.New("invalid JSON delimiter")
	}
	return err
}
