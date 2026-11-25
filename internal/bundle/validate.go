// Package bundle checks an intentionally restricted, rootless OCI bundle profile.
// This is defense-in-depth, NOT a substitute for an audited OCI runtime or sandbox.
package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/jsonkey"
	"github.com/GoreeCloud/containers/internal/regularinput"
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

// ValidateRuntimeLayout rejects an incomplete on-disk OCI guest rootfs
// before the external runtime can be called or its mutation journal written.
// This checks minimum paths, NOT OCI confinement or proc/dev mount security.
func ValidateRuntimeLayout(dir string, spec *Spec) error {
	if spec == nil || spec.Root == nil || spec.Process == nil ||
		len(spec.Process.Args) == 0 || !filepath.IsAbs(spec.Process.Args[0]) ||
		filepath.Clean(spec.Process.Args[0]) != spec.Process.Args[0] ||
		!filepath.IsAbs(spec.Process.Cwd) || filepath.Clean(spec.Process.Cwd) != spec.Process.Cwd ||
		!filepath.IsLocal(spec.Root.Path) || spec.Root.Path == "." ||
		filepath.Clean(spec.Root.Path) != spec.Root.Path {
		return errors.New("invalid OCI guest rootfs path")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return errors.New("unable to open OCI bundle for layout preflight")
	}
	defer root.Close()
	needed := []string{"dev", "proc"}
	for _, m := range spec.Mounts {
		if m.Destination == "/dev/pts" && m.Type == "devpts" {
			needed = append(needed, "dev/pts")
		}
	}
	for _, name := range needed {
		path := filepath.Join(spec.Root.Path, name)
		info, err := root.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("OCI guest rootfs requires a real %s directory", name)
		}
	}
	// The preview process requires an absolute entrypoint. Check its
	// presence and executable mode through an os.Root anchored inside the
	// guest: a host-path symlink must not make an empty rootfs look runnable.
	// This is a preflight, not an immutable bundle snapshot or an attestation
	// that the dynamic linker / interpreter is available at OCI start time.
	guest, err := root.OpenRoot(spec.Root.Path)
	if err != nil {
		return errors.New("OCI guest rootfs unavailable for process preflight")
	}
	defer guest.Close()
	executable := strings.TrimPrefix(spec.Process.Args[0], "/")
	binaryInfo, err := guest.Stat(executable)
	if err != nil || !binaryInfo.Mode().IsRegular() || binaryInfo.Mode().Perm()&0111 == 0 {
		return errors.New("OCI guest executable missing, nonregular or not executable")
	}
	if err := validateGuestEntrypoint(guest, executable); err != nil {
		return err
	}
	workdir := strings.TrimPrefix(spec.Process.Cwd, "/")
	if workdir == "" {
		workdir = "."
	}
	dirInfo, err := guest.Stat(workdir)
	if err != nil || !dirInfo.IsDir() {
		return errors.New("OCI guest working directory missing or not a directory")
	}
	return nil
}

// Validate checks the restricted bundle profile without running a container.
func Validate(dir string) error { _, err := Load(dir); return err }

// Validate reads no more than 1 MiB and never executes a bundle.
func Load(dir string) (*Spec, error) {
	bundleDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("bundle directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(bundleDir)
	if err != nil || resolved != bundleDir {
		return nil, errors.New("symlinked OCI bundle or parent is unsupported")
	}
	before, err := os.Lstat(bundleDir)
	if err != nil || !before.IsDir() {
		return nil, errors.New("bundle must be a directory")
	}
	root, err := os.OpenRoot(bundleDir)
	if err != nil {
		return nil, fmt.Errorf("open OCI bundle root: %w", err)
	}
	defer root.Close()
	opened, err := root.Lstat(".")
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("OCI bundle directory changed while opening")
	}
	info, err := root.Lstat("config.json")
	if err != nil {
		return nil, fmt.Errorf("OCI config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > MaxConfigBytes {
		return nil, errors.New("OCI config must be a regular bounded file, not a symlink")
	}
	f, err := regularinput.Open(root, "config.json", info)
	if err != nil {
		return nil, fmt.Errorf("OCI config: %w", err)
	}
	defer f.Close()
	opened, err = f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("OCI config changed while opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err != nil {
		return nil, errors.New("unable to read OCI config")
	}
	if len(b) > MaxConfigBytes || int64(len(b)) != info.Size() {
		return nil, errors.New("OCI config was oversized or changed size while reading")
	}
	if err := rejectDuplicateKeys(b); err != nil {
		return nil, fmt.Errorf("OCI JSON: %w", err)
	}
	var spec Spec
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return nil, fmt.Errorf("unsupported OCI config: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("OCI config has trailing data")
	}
	if err := validateSpec(&spec, root); err != nil {
		return nil, err
	}
	return &spec, nil
}

func validateSpec(s *Spec, root *os.Root) error {
	if s.OCIVersion == "" || s.Process == nil || s.Root == nil || s.Linux == nil {
		return errors.New("OCI version, process, root and linux are required")
	}
	if len(s.Hooks) != 0 && string(s.Hooks) != "null" {
		return errors.New("OCI hooks are prohibited in the preview profile")
	}
	if s.Process.Terminal || !s.Process.NoNewPrivileges || len(s.Process.Args) == 0 || s.Process.Args[0] == "" {
		return errors.New("noninteractive process, arguments and noNewPrivileges=true required")
	}
	if err := validateProcess(s.Process); err != nil {
		return err
	}
	c := s.Process.Capabilities
	if c == nil || len(c.Bounding)+len(c.Effective)+len(c.Inheritable)+len(c.Permitted)+len(c.Ambient) != 0 {
		return errors.New("all process capability sets must be explicitly empty")
	}
	if !s.Root.Readonly || !filepath.IsLocal(s.Root.Path) ||
		filepath.Clean(s.Root.Path) != s.Root.Path || s.Root.Path == "." {
		return errors.New("root.path must be a clean local directory and root.readonly must be true")
	}
	// Reject all symlink ancestors: even links that resolve inside the
	// bundle would create a mutable path-to-rootfs interpretation.
	parent := ""
	for _, segment := range strings.Split(s.Root.Path, string(filepath.Separator)) {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("unsafe OCI rootfs path component")
		}
		parent = filepath.Join(parent, segment)
		fi, err := root.Lstat(parent)
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return errors.New("OCI rootfs or ancestor is missing, non-directory or symlinked")
		}
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
			switch m.Destination {
			case "/tmp":
				if m.Source != "tmpfs" || !onlyOptions(m.Options, "nosuid", "noexec", "nodev", "size=67108864", "mode=1777") ||
					!hasOption(m.Options, "size=67108864") || !hasOption(m.Options, "nosuid") || !hasOption(m.Options, "nodev") {
					return errors.New("unsupported tmpfs mount policy")
				}
			case "/dev":
				// Rootless runc creates /dev symlinks in a private guest tmpfs.
				// Reject host binds, unbounded memory, and custom device access.
				if m.Source != "tmpfs" || !exactOptions(m.Options, "nosuid", "noexec", "nodev", "mode=755", "size=65536k") {
					return errors.New("unsupported private /dev tmpfs policy")
				}
			default:
				return errors.New("unsupported tmpfs destination")
			}
		case "devpts":
			if m.Destination != "/dev/pts" || m.Source != "devpts" ||
				!exactOptions(m.Options, "nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620") {
				return errors.New("unsupported private devpts policy")
			}
		default:
			return errors.New("unsupported mount type")
		}
	}
	// Private pseudo-terminals require a separately mounted bounded /dev.
	if destinations["/dev"] != destinations["/dev/pts"] {
		return errors.New("private /dev and /dev/pts mounts must be paired")
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
func exactOptions(options []string, required ...string) bool {
	return len(options) == len(required) && onlyOptions(options, required...)
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
			if keys[jsonkey.Fold(key)] {
				return errors.New("duplicate JSON key")
			}
			keys[jsonkey.Fold(key)] = true
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

// validateProcess bounds mutable process metadata in the strict preview
// profile. It does not certify a root filesystem or syscall allowlist.
func validateProcess(p *Process) error {
	if len(p.Args) > 64 || !strings.HasPrefix(p.Args[0], "/") || filepath.Clean(p.Args[0]) != p.Args[0] {
		return errors.New("preview process requires clean absolute executable and at most 64 arguments")
	}
	total := 0
	for _, arg := range p.Args {
		if len(arg) > 4096 || strings.IndexByte(arg, 0) >= 0 {
			return errors.New("invalid process argument")
		}
		total += len(arg)
	}
	if total > 64<<10 || len(p.Env) > 128 {
		return errors.New("process arguments/environment exceed preview quota")
	}
	if !strings.HasPrefix(p.Cwd, "/") || filepath.Clean(p.Cwd) != p.Cwd {
		return errors.New("process working directory must be clean and absolute")
	}
	seen := map[string]bool{}
	for _, entry := range p.Env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" || len(entry) > 4096 || strings.IndexByte(entry, 0) >= 0 {
			return errors.New("invalid process environment entry")
		}
		for i, ch := range key {
			if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch == '_' || i > 0 && ch >= '0' && ch <= '9') {
				return errors.New("invalid process environment key")
			}
		}
		if seen[key] {
			return errors.New("duplicate process environment key")
		}
		seen[key] = true
		total += len(entry)
	}
	if total > 128<<10 {
		return errors.New("process arguments/environment exceed total quota")
	}
	return nil
}

// RequireSingleCallerMapping is a deliberately narrow preview gate: container
// UID/GID 0 may map only to the invoking non-root host user and primary group.
// Subordinate ranges are deliberately unsupported until delegation is audited.
func RequireSingleCallerMapping(s *Spec, uid, gid uint32) error {
	if s == nil || s.Linux == nil || s.Process == nil || uid == 0 || gid == 0 {
		return errors.New("nonroot invoking user/group required")
	}
	if len(s.Linux.UIDMappings) != 1 || len(s.Linux.GIDMappings) != 1 ||
		s.Process.User.UID != 0 || s.Process.User.GID != 0 {
		return errors.New("preview requires one container-root mapping")
	}
	u := s.Linux.UIDMappings[0]
	g := s.Linux.GIDMappings[0]
	if u.ContainerID != 0 || u.HostID != uid || u.Size != 1 ||
		g.ContainerID != 0 || g.HostID != gid || g.Size != 1 {
		return errors.New("host mapping must match invoking user/group")
	}
	return nil
}
