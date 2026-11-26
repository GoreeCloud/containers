package stack

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/bundle"
	"github.com/GoreeCloud/containers/internal/engine"
	"github.com/GoreeCloud/containers/internal/ociimage"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ExerciseInput struct {
	ID               string `json:"id"`
	ManifestSize     int64  `json:"manifestSize"`
	ConfigFile       string `json:"configFile"`
	ConfigSHA256     string `json:"configSHA256"`
	EntrypointSHA256 string `json:"entrypointSHA256"`
	Whiteouts        bool   `json:"whiteouts"`
}
type ExerciseAdmission struct {
	PlanSHA256 string          `json:"planSHA256"`
	Instances  []ExerciseInput `json:"instances"`
}
type ExerciseReview struct {
	PlanSHA256           string `json:"planSHA256"`
	AdmissionSHA256      string `json:"admissionSHA256"`
	Instances            int    `json:"instances"`
	Waves                int    `json:"waves"`
	ReadOnly             bool   `json:"readOnly"`
	DeploymentAuthorized bool   `json:"deploymentAuthorized"`
}
type ExerciseReport struct {
	ExerciseReview
	SessionDirectory          string `json:"sessionDirectory,omitempty"`
	Prepared                  int    `json:"prepared"`
	CompletedWaves            int    `json:"completedWaves"`
	CompletedStopWaves        int    `json:"completedStopWaves"`
	AllRunningObserved        bool   `json:"allRunningObserved"`
	CleanupVerified           bool   `json:"cleanupVerified"`
	RecoveryReviewRequired    bool   `json:"recoveryReviewRequired"`
	HealthVerified            bool   `json:"healthVerified"`
	MIPSAuthorizationVerified bool   `json:"mipsAuthorizationVerified"`
}

func digestBytes(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
func parseExercise(s Spec, raw []byte, pin string) (ExerciseAdmission, InstancePlan, error) {
	var a ExerciseAdmission
	var p InstancePlan
	if len(raw) == 0 || len(raw) > 64<<10 || !imagePattern.MatchString(pin) || digestBytes(raw) != pin {
		return a, p, errors.New("exercise admission pin mismatch")
	}
	if e := rejectDuplicate(raw); e != nil {
		return a, p, e
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&a); e != nil {
		return a, p, e
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return a, p, errors.New("trailing exercise admission")
	}
	p, e := ReviewInstancePlan(s, a.PlanSHA256)
	if e != nil {
		return a, p, e
	}
	if p.InstanceCount < 1 || p.InstanceCount > 4 || p.SecretReferenceCount != 0 || len(a.Instances) != p.InstanceCount {
		return a, p, errors.New("exercise requires 1–4 instances and no secret references")
	}
	expected := map[string]bool{}
	var cpu, mem int64
	for _, w := range p.StartWaves {
		for _, i := range w.Instances {
			expected[i.ID] = true
			cpu += i.CPUMilli
			mem += i.MemoryBytes
		}
	}
	if cpu > 2000 || mem > 256<<20 {
		return a, p, errors.New("exercise aggregate resource budget exceeded")
	}
	for _, i := range a.Instances {
		if !expected[i.ID] || i.ManifestSize <= 0 || i.ManifestSize > 4<<20 || !filepath.IsAbs(i.ConfigFile) || filepath.Clean(i.ConfigFile) != i.ConfigFile || !imagePattern.MatchString(i.ConfigSHA256) || !imagePattern.MatchString(i.EntrypointSHA256) {
			return a, p, errors.New("invalid or duplicate exercise instance admission")
		}
		delete(expected, i.ID)
	}
	return a, p, nil
}
func LoadExercise(s Spec, name, pin string) (ExerciseAdmission, InstancePlan, ExerciseReview, error) {
	var empty ExerciseReview
	abs, e := filepath.Abs(name)
	if e != nil {
		return ExerciseAdmission{}, InstancePlan{}, empty, e
	}
	parent := filepath.Dir(abs)
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return ExerciseAdmission{}, InstancePlan{}, empty, errors.New("unsafe admission parent")
	}
	r, e := os.OpenRoot(parent)
	if e != nil {
		return ExerciseAdmission{}, InstancePlan{}, empty, e
	}
	defer r.Close()
	base := filepath.Base(abs)
	i, e := r.Lstat(base)
	if e != nil || !i.Mode().IsRegular() || i.Size() <= 0 || i.Size() > 64<<10 {
		return ExerciseAdmission{}, InstancePlan{}, empty, errors.New("invalid admission file")
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || i.Mode().Perm() != 0600 && i.Mode().Perm() != 0400 {
		return ExerciseAdmission{}, InstancePlan{}, empty, errors.New("private admission file required")
	}
	f, e := regularinput.Open(r, base, i)
	if e != nil {
		return ExerciseAdmission{}, InstancePlan{}, empty, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 64<<10+1))
	if e != nil || int64(len(b)) != i.Size() {
		return ExerciseAdmission{}, InstancePlan{}, empty, errors.New("admission input changed")
	}
	if e = verifyStackInputIdentity(r, f, i, base); e != nil {
		return ExerciseAdmission{}, InstancePlan{}, empty, e
	}
	a, p, e := parseExercise(s, b, pin)
	if e != nil {
		return a, p, empty, e
	}
	return a, p, ExerciseReview{PlanSHA256: p.PlanSHA256, AdmissionSHA256: pin, Instances: p.InstanceCount, Waves: len(p.StartWaves), ReadOnly: true}, nil
}

type exerciseRuntime interface {
	CreateDelegated(context.Context, string, string) error
	Start(context.Context, string) error
	State(context.Context, string) ([]byte, error)
	Stop(context.Context, string) error
	Delete(context.Context, string) error
}
type exerciseEvent struct {
	Phase string   `json:"phase"`
	Wave  int      `json:"wave,omitempty"`
	IDs   []string `json:"ids,omitempty"`
}
type eventWriter func(exerciseEvent) error

// No new wave after failure; allow already-started siblings to finish their
// bounded calls. No automatic rollback when ownership or state is uncertain.
func parallelInstances(ctx context.Context, ids []string, op func(context.Context, string) error) error {
	if len(ids) < 1 || len(ids) > 4 {
		return errors.New("invalid wave width")
	}
	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	for n, id := range ids {
		wg.Add(1)
		go func(n int, id string) {
			defer wg.Done()
			if ctx.Err() != nil {
				errs[n] = ctx.Err()
				return
			}
			bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			errs[n] = op(bounded, id)
		}(n, id)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// A private runtime root alone does not namespace cgroup names. Use
// per-session random runtime IDs, not deterministic plan IDs, for mutations.
type isolatedRuntime struct {
	rt  *engine.Engine
	ids map[string]string
}

func (r isolatedRuntime) CreateDelegated(ctx context.Context, id, dir string) error {
	return r.rt.CreateDelegated(ctx, r.ids[id], dir)
}
func (r isolatedRuntime) Start(ctx context.Context, id string) error {
	return r.rt.Start(ctx, r.ids[id])
}
func (r isolatedRuntime) Stop(ctx context.Context, id string) error { return r.rt.Stop(ctx, r.ids[id]) }
func (r isolatedRuntime) Delete(ctx context.Context, id string) error {
	return r.rt.Delete(ctx, r.ids[id])
}
func (r isolatedRuntime) State(ctx context.Context, id string) ([]byte, error) {
	raw, e := r.rt.State(ctx, r.ids[id])
	if e != nil {
		return nil, e
	}
	s, e := engine.DecodeRuntimeState(raw, r.ids[id])
	if e != nil {
		return nil, e
	}
	s.ID = id
	return json.Marshal(s)
}
func sessionRuntimeIDs(p InstancePlan) (map[string]string, error) {
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return nil, e
	}
	ids := map[string]string{}
	seen := map[string]bool{}
	for _, w := range p.StartWaves {
		for _, i := range w.Instances {
			sum := sha256.Sum256(append(append([]byte("goreecloud-stack-exercise-v1\x00"), nonce[:]...), []byte(i.ID)...))
			id := fmt.Sprintf("gc-ex-%x", sum[:20])
			if seen[id] {
				return nil, errors.New("runtime identity collision")
			}
			seen[id] = true
			ids[i.ID] = id
		}
	}
	return ids, nil
}
func running(ctx context.Context, r exerciseRuntime, id string) error {
	raw, e := r.State(ctx, id)
	if e != nil {
		return e
	}
	s, e := engine.DecodeRuntimeState(raw, id)
	if e != nil || s.Status != "running" || s.PID <= 0 {
		return errors.New("running observation not verified")
	}
	return nil
}
func executeWaves(ctx context.Context, r exerciseRuntime, p InstancePlan, dirs map[string]string, record eventWriter, report *ExerciseReport) error {
	if ctx == nil || r == nil || record == nil || report == nil {
		return errors.New("exercise context required")
	}
	report.RecoveryReviewRequired = true
	for _, w := range p.StartWaves {
		ids := []string{}
		for _, i := range w.Instances {
			if dirs[i.ID] == "" {
				return errors.New("missing prepared instance")
			}
			for _, dep := range i.Requires {
				if e := running(ctx, r, dep); e != nil {
					return e
				}
			}
			ids = append(ids, i.ID)
		}
		if e := record(exerciseEvent{"create-intent", w.Number, ids}); e != nil {
			return e
		}
		if e := parallelInstances(ctx, ids, func(c context.Context, id string) error { return r.CreateDelegated(c, id, dirs[id]) }); e != nil {
			return errors.New("create wave incomplete; preserve session")
		}
		if e := record(exerciseEvent{"create-observed", w.Number, ids}); e != nil {
			return e
		}
		if e := record(exerciseEvent{"start-intent", w.Number, ids}); e != nil {
			return e
		}
		if e := parallelInstances(ctx, ids, func(c context.Context, id string) error {
			if e := r.Start(c, id); e != nil {
				return e
			}
			return running(c, r, id)
		}); e != nil {
			return errors.New("start wave incomplete; preserve session")
		}
		if e := record(exerciseEvent{"running-observed", w.Number, ids}); e != nil {
			return e
		}
		report.CompletedWaves++
	}
	for _, w := range p.StartWaves {
		for _, i := range w.Instances {
			if e := running(ctx, r, i.ID); e != nil {
				return e
			}
		}
	}
	report.AllRunningObserved = true
	if e := record(exerciseEvent{Phase: "all-running-observed"}); e != nil {
		return e
	}
	for n, ids := range p.StopWaves {
		if e := record(exerciseEvent{"stop-intent", n + 1, ids}); e != nil {
			return e
		}
		if e := parallelInstances(ctx, ids, func(c context.Context, id string) error { return r.Stop(c, id) }); e != nil {
			return errors.New("stop wave incomplete; preserve session")
		}
		if e := record(exerciseEvent{"stop-observed", n + 1, ids}); e != nil {
			return e
		}
		if e := record(exerciseEvent{"delete-intent", n + 1, ids}); e != nil {
			return e
		}
		if e := parallelInstances(ctx, ids, func(c context.Context, id string) error { return r.Delete(c, id) }); e != nil {
			return errors.New("delete wave incomplete; preserve session")
		}
		if e := record(exerciseEvent{"delete-observed", n + 1, ids}); e != nil {
			return e
		}
		report.CompletedStopWaves++
	}
	return nil
}
func privateExerciseDir(path string) (os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("absolute private exercise path required")
	}
	real, e := filepath.EvalSymlinks(path)
	if e != nil || real != path {
		return nil, errors.New("unsafe exercise path")
	}
	i, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || !i.IsDir() || i.Mode().Perm() != 0700 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, errors.New("private exercise directory required")
	}
	return i, nil
}
func checkExerciseBundle(s *bundle.Spec, i Instance) (uint32, uint32, error) {
	if s == nil || s.Process == nil || s.Linux == nil || s.Linux.Resources == nil || s.Linux.Resources.CPU == nil || s.Linux.Resources.Memory == nil || s.Linux.Resources.Pids == nil {
		return 0, 0, errors.New("explicit resource limits required")
	}
	r := s.Linux.Resources
	if r.CPU.Quota != i.CPUMilli*100 || r.CPU.Period != 100000 || r.Memory.Limit != i.MemoryBytes || r.Pids.Limit != 32 {
		return 0, 0, errors.New("exercise resource mismatch")
	}
	if len(s.Process.Args) != 1 || !reflect.DeepEqual(s.Process.Env, []string{"PATH=/bin"}) || s.Process.Cwd != "/" || len(s.Annotations) != 0 || s.Hostname != "" {
		return 0, 0, errors.New("restricted exercise process policy")
	}
	// Exact subset of the reviewed isolated fixture's syscall allowlist.
	// No socket, IPC, signals to other processes or process creation.
	allowed := strings.Fields("execve read write close exit exit_group brk mmap mprotect munmap arch_prctl set_tid_address set_robust_list rseq prlimit64 getrandom readlink readlinkat openat newfstatat fstat fstatfs getdents64 lseek getpid getuid getgid geteuid getegid prctl capget rt_sigaction rt_sigprocmask rt_sigreturn clock_nanosleep nanosleep clock_gettime gettid futex fcntl getcwd statx access faccessat2")
	known := map[string]bool{}
	for _, n := range allowed {
		known[n] = true
	}
	if s.Linux.Seccomp == nil || s.Linux.Seccomp.DefaultAction != "SCMP_ACT_ERRNO" || !reflect.DeepEqual(s.Linux.Seccomp.Architectures, []string{"SCMP_ARCH_X86_64"}) {
		return 0, 0, errors.New("restricted exercise seccomp policy")
	}
	for _, raw := range s.Linux.Seccomp.Syscalls {
		var rule struct {
			Names  []string
			Action string
		}
		if e := json.Unmarshal(raw, &rule); e != nil || rule.Action != "SCMP_ACT_ALLOW" {
			return 0, 0, errors.New("unsupported exercise syscall rule")
		}
		for _, n := range rule.Names {
			if !known[n] {
				return 0, 0, errors.New("exercise syscall outside reviewed subset")
			}
		}
	}
	var u, g uint32
	for _, m := range s.Linux.UIDMappings {
		if m.ContainerID == 1000 {
			u = m.HostID
		}
	}
	for _, m := range s.Linux.GIDMappings {
		if m.ContainerID == 1000 {
			g = m.HostID
		}
	}
	if u == 0 || g == 0 {
		return 0, 0, errors.New("delegated identity missing")
	}
	return u, g, nil
}

// RunExercise is an explicit finite local nonproduction controller, not
// deployment admission. Each run allocates a fresh private runtime root and
// never targets existing runtime IDs. Prepared bytes and evidence are kept.
func RunExercise(ctx context.Context, s Spec, admissionPath, pin, storePath, parent, runtimeBinary string) (ExerciseReport, error) {
	var report ExerciseReport
	if ctx == nil || os.Geteuid() == 0 || filepath.Base(runtimeBinary) != "runc" {
		return report, errors.New("rootless runc exercise required")
	}
	a, p, review, e := LoadExercise(s, admissionPath, pin)
	if e != nil {
		return report, e
	}
	report.ExerciseReview = review
	report.ReadOnly = false
	store, e := ociimage.OpenExistingStore(storePath)
	if e != nil {
		return report, e
	}
	parentID, e := privateExerciseDir(parent)
	if e != nil {
		return report, e
	}
	dir, e := os.MkdirTemp(parent, "stack-exercise-")
	if e != nil {
		return report, e
	}
	report.SessionDirectory = dir
	identity, e := privateExerciseDir(dir)
	if e != nil {
		return report, e
	}
	check := func() error {
		a, e := privateExerciseDir(parent)
		if e != nil || !os.SameFile(a, parentID) {
			return errors.New("exercise parent identity changed")
		}
		b, e := privateExerciseDir(dir)
		if e != nil || !os.SameFile(b, identity) {
			return errors.New("exercise directory identity changed")
		}
		return nil
	}
	if e = check(); e != nil {
		return report, e
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return report, e
	}
	defer root.Close()
	log, e := root.OpenFile("events.ndjson", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return report, e
	}
	defer log.Close()
	record := func(ev exerciseEvent) error {
		if e := check(); e != nil {
			return e
		}
		if e := json.NewEncoder(log).Encode(ev); e != nil {
			return e
		}
		return log.Sync()
	}
	if e = record(exerciseEvent{Phase: "session-created"}); e != nil {
		return report, e
	}
	if e = root.Mkdir("runtime", 0700); e != nil {
		return report, e
	}
	if e = root.Mkdir("bundles", 0700); e != nil {
		return report, e
	}
	base, e := root.Open(".")
	if e != nil {
		return report, e
	}
	e = errors.Join(base.Sync(), base.Close())
	if e != nil {
		return report, e
	}
	stateRoot, e := engine.PrepareStateRoot(filepath.Join(dir, "runtime"))
	if e != nil {
		return report, e
	}
	rt, e := engine.New(runtimeBinary, stateRoot, nil)
	if e != nil {
		return report, e
	}
	input := map[string]ExerciseInput{}
	for _, i := range a.Instances {
		input[i.ID] = i
	}
	dirs := map[string]string{}
	uids := map[uint32]bool{}
	gids := map[uint32]bool{}
	for _, w := range p.StartWaves {
		for _, i := range w.Instances {
			if e = check(); e != nil {
				return report, e
			}
			in := input[i.ID]
			b, e := store.PrepareDelegatedBundle(ctx, i.ImageDigest, in.ManifestSize, in.ConfigFile, in.ConfigSHA256, in.EntrypointSHA256, filepath.Join(dir, "bundles"), in.Whiteouts)
			if e != nil {
				return report, errors.New("pinned instance preparation failed")
			}
			spec, e := bundle.Load(b.Directory)
			if e != nil {
				return report, e
			}
			u, g, e := checkExerciseBundle(spec, i)
			if e != nil {
				return report, e
			}
			if uids[u] || gids[g] {
				return report, errors.New("exercise replicas require distinct subordinate UID/GID")
			}
			uids[u] = true
			gids[g] = true
			dirs[i.ID] = b.Directory
			report.Prepared++
		}
	}
	if e = record(exerciseEvent{Phase: "all-bundles-prepared"}); e != nil {
		return report, e
	}
	inv, e := rt.Inventory(ctx)
	if e != nil || len(inv.Entries) != 0 || inv.RecoveryRequired {
		return report, errors.New("fresh runtime root not empty")
	}
	ids, e := sessionRuntimeIDs(p)
	if e != nil {
		return report, e
	}
	receipt, e := root.OpenFile("receipt.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return report, e
	}
	e = json.NewEncoder(receipt).Encode(struct {
		Review     ExerciseReview    `json:"review"`
		RuntimeIDs map[string]string `json:"runtimeIDs"`
		Bundles    map[string]string `json:"bundles"`
	}{review, ids, dirs})
	if e == nil {
		e = receipt.Sync()
	}
	if e = errors.Join(e, receipt.Close()); e != nil {
		return report, e
	}
	base, e = root.Open(".")
	if e != nil {
		return report, e
	}
	if e = errors.Join(base.Sync(), base.Close()); e != nil {
		return report, e
	}
	if e = executeWaves(ctx, isolatedRuntime{rt: rt, ids: ids}, p, dirs, record, &report); e != nil {
		return report, e
	}
	inv, e = rt.Inventory(ctx)
	if e != nil || len(inv.Entries) != 0 || inv.RecoveryRequired {
		return report, errors.New("exercise cleanup inventory not empty")
	}
	if e = record(exerciseEvent{Phase: "cleanup-verified"}); e != nil {
		return report, e
	}
	report.CleanupVerified = true
	report.RecoveryReviewRequired = false
	return report, nil
}
