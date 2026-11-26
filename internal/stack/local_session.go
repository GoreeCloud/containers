package stack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"syscall"
	"time"
)

var localSessionName = regexp.MustCompile("^stack-exercise-[0-9]+$")
var localRuntimeName = regexp.MustCompile("^gc-ex-[a-f0-9]{40}$")

type localLease struct {
	root     *os.Root
	lock     *os.File
	parent   string
	identity os.FileInfo
}

func acquireLocalLease(ctx context.Context, parent string) (*localLease, error) {
	if ctx == nil || os.Geteuid() == 0 {
		return nil, errors.New("nonroot context required")
	}
	i, e := privateExerciseDir(parent)
	if e != nil {
		return nil, e
	}
	root, e := os.OpenRoot(parent)
	if e != nil {
		return nil, e
	}
	l := &localLease{root: root, parent: parent, identity: i}
	f, e := root.OpenFile("local-session.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		root.Close()
		return nil, e
	}
	l.lock = f
	info, e := f.Stat()
	if e != nil || !privateRecord(info, 0600) {
		l.close()
		return nil, errors.New("unsafe session lock")
	}
	for {
		if e = ctx.Err(); e != nil {
			l.close()
			return nil, e
		}
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			break
		}
		if e != syscall.EWOULDBLOCK {
			l.close()
			return nil, e
		}
		select {
		case <-ctx.Done():
			l.close()
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if e = l.check(); e != nil {
		l.close()
		return nil, e
	}
	return l, nil
}
func privateRecord(i os.FileInfo, mode os.FileMode) bool {
	if i == nil || !i.Mode().IsRegular() || i.Mode().Perm() != mode || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid()) && st.Nlink == 1
}
func (l *localLease) close() {
	if l.lock != nil {
		l.lock.Close()
	}
	l.root.Close()
}
func (l *localLease) check() error {
	i, e := privateExerciseDir(l.parent)
	if e != nil || !os.SameFile(i, l.identity) {
		return errors.New("session parent replaced")
	}
	j, e := l.root.Stat(".")
	if e != nil || !os.SameFile(i, j) {
		return errors.New("session parent anchor mismatch")
	}
	a, e := l.root.Lstat("local-session.lock")
	if e != nil || !privateRecord(a, 0600) {
		return errors.New("session lock replaced")
	}
	b, e := l.lock.Stat()
	if e != nil || !os.SameFile(a, b) {
		return errors.New("session lock identity changed")
	}
	return nil
}
func syncDirectory(r *os.Root) error {
	f, e := r.Open(".")
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func (l *localLease) requireVacant() error {
	if e := l.check(); e != nil {
		return e
	}
	if _, e := l.root.Lstat("active-session"); !os.IsNotExist(e) {
		return errors.New("active or uncertain local session; review required")
	}
	return nil
}
func (l *localLease) reserve(session string) error {
	if !localSessionName.MatchString(session) {
		return errors.New("invalid session directory name")
	}
	if e := l.requireVacant(); e != nil {
		return e
	}
	f, e := l.root.OpenFile("active-session", os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	_, e = f.WriteString(session + "\n")
	if e == nil {
		e = f.Sync()
	}
	if e = errors.Join(e, f.Close()); e != nil {
		return e
	}
	if e = syncDirectory(l.root); e != nil {
		return e
	}
	return l.checkActive(session)
}

// samePrivateRecord detects replacement and same-content rewrites with restored
// mtime using Linux ctime. This is a cooperative observation, not atomicity.
func samePrivateRecord(a, b os.FileInfo) bool {
	if !privateRecord(a, 0600) || !privateRecord(b, 0600) || !os.SameFile(a, b) || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	x, ok := a.Sys().(*syscall.Stat_t)
	y, ok2 := b.Sys().(*syscall.Stat_t)
	return ok && ok2 && x.Ctim == y.Ctim
}

func readPrivateRecord(r *os.Root, name string, max int64) ([]byte, os.FileInfo, error) {
	i, e := r.Lstat(name)
	if e != nil || !privateRecord(i, 0600) || i.Size() <= 0 || i.Size() > max {
		return nil, nil, errors.New("invalid private session record")
	}
	f, e := regularinput.Open(r, name, i)
	if e != nil {
		return nil, nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(b)) != i.Size() {
		return nil, nil, errors.New("session record changed")
	}
	opened, e := f.Stat()
	if e != nil || !samePrivateRecord(i, opened) {
		return nil, nil, errors.New("private session record changed during read")
	}
	current, e := r.Lstat(name)
	if e != nil || !samePrivateRecord(i, current) {
		return nil, nil, errors.New("private session record pathname changed")
	}
	return b, i, nil
}
func (l *localLease) checkActive(session string) error {
	if e := l.check(); e != nil {
		return e
	}
	raw, _, e := readPrivateRecord(l.root, "active-session", 128)
	if e != nil || string(raw) != session+"\n" {
		return errors.New("active session binding mismatch")
	}
	return nil
}
func (l *localLease) release(session string) error {
	if e := l.checkActive(session); e != nil {
		return e
	}
	if e := l.root.Remove("active-session"); e != nil {
		return e
	}
	return syncDirectory(l.root)
}

type localReceipt struct {
	Version         int                               `json:"version"`
	Session         string                            `json:"session"`
	PlanSHA256      string                            `json:"planSHA256"`
	AdmissionSHA256 string                            `json:"admissionSHA256"`
	Mapping         mappingReference                  `json:"mapping"`
	StopWaves       [][]string                        `json:"stopWaves"`
	Bindings        map[string]engine.InstanceBinding `json:"bindings"`
}

func writeLocalReceipt(root *os.Root, r localReceipt) (string, error) {
	raw, e := json.Marshal(r)
	if e != nil {
		return "", e
	}
	f, e := root.OpenFile("local-receipt.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return "", e
	}
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	if e = errors.Join(e, f.Close()); e != nil {
		return "", e
	}
	if e = syncDirectory(root); e != nil {
		return "", e
	}
	return digestBytes(raw), nil
}
func parseLocalReceipt(raw []byte, pin, session, dir string) (localReceipt, error) {
	var r localReceipt
	if len(raw) > 64<<10 || !imagePattern.MatchString(pin) || digestBytes(raw) != pin {
		return r, errors.New("local receipt pin mismatch")
	}
	if e := rejectDuplicate(raw); e != nil {
		return r, e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&r); e != nil {
		return r, e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return r, errors.New("trailing local receipt")
	}
	if r.Version != 1 || !validMappingReference(r.Mapping) || r.Session != session || !localSessionName.MatchString(session) || !imagePattern.MatchString(r.PlanSHA256) || !imagePattern.MatchString(r.AdmissionSHA256) || len(r.Bindings) < 1 || len(r.Bindings) > 4 || len(r.StopWaves) < 1 || len(r.StopWaves) > 4 {
		return r, errors.New("invalid local receipt")
	}
	seen := map[string]bool{}
	runtimeIDs := map[string]bool{}
	for _, w := range r.StopWaves {
		if len(w) == 0 || len(w) > 4 {
			return r, errors.New("invalid stop wave")
		}
		for _, id := range w {
			b, ok := r.Bindings[id]
			if !ok || seen[id] || engine.ValidateID(id) != nil || !localRuntimeName.MatchString(b.ID) || runtimeIDs[b.ID] || !imagePattern.MatchString(b.ProcessSHA256) || !imagePattern.MatchString(b.BootSHA256) || b.ProcessPID <= 0 || b.MetadataInode == 0 || filepath.Dir(b.Bundle) != filepath.Join(dir, "bundles") || filepath.Clean(b.Bundle) != b.Bundle {
				return r, errors.New("invalid bound instance receipt")
			}
			seen[id] = true
			runtimeIDs[b.ID] = true
		}
	}
	if len(seen) != len(r.Bindings) {
		return r, errors.New("incomplete local receipt")
	}
	return r, nil
}

type LocalSessionReport struct {
	ReadOnly                      bool   `json:"readOnly"`
	StopResumeChecksPassed        bool   `json:"stopResumeChecksPassed"`
	StartFinalizationChecksPassed bool   `json:"startFinalizationChecksPassed"`
	StartFinalized                bool   `json:"startFinalized"`
	ReceiptSHA256                 string `json:"receiptSHA256"`
	Expected                      int    `json:"expected"`
	Running                       int    `json:"running"`
	Stopped                       int    `json:"stopped"`
	Pending                       bool   `json:"pending"`
	BindingsObserved              bool   `json:"bindingsObserved"`
	MissingProcessAbsent          int    `json:"missingProcessAbsent"`
	CleanupVerified               bool   `json:"cleanupVerified"`
	RecoveryReviewRequired        bool   `json:"recoveryReviewRequired"`
	TerminalCleanupChecksPassed   bool   `json:"terminalCleanupChecksPassed"`
	HealthVerified                bool   `json:"healthVerified"`
	DeploymentAuthorized          bool   `json:"deploymentAuthorized"`
}

func StartLocal(ctx context.Context, s Spec, admission, pin, store, parent, binary, mappingStore string) (ExerciseReport, error) {
	return runExercise(ctx, s, admission, pin, store, parent, binary, true, mappingStore)
}

// validatePersistentEvents reconstructs the exact startup sequence from the
// independently pinned receipt. Stop waves retain the order within each
// corresponding start wave; only the order of waves is reversed.
func validatePersistentEvents(raw []byte, rec localReceipt) error {
	return validatePersistentPhase(raw, rec, false)
}

func validatePersistentPhase(raw []byte, rec localReceipt, finalization bool) error {
	p := InstancePlan{}
	for n := len(rec.StopWaves) - 1; n >= 0; n-- {
		w := InstanceWave{Number: len(p.StartWaves)}
		for _, id := range rec.StopWaves[n] {
			w.Instances = append(w.Instances, Instance{ID: id})
		}
		p.StartWaves = append(p.StartWaves, w)
	}
	phase, count, err := auditEvents(raw, p)
	expectedPhase, expectedCount := "persistent-running-verified", len(expectedExerciseEvents(p))+1
	if finalization {
		expectedPhase, expectedCount = "all-running-observed", expectedCount-1
	}
	if err != nil || phase != expectedPhase || count != expectedCount {
		return errors.New("persistent start evidence incomplete or inconsistent")
	}
	return nil
}

// Inspect or stop only the exact receipt selected by the caller. No automatic
// replay after an interrupted stop, no action on unmatched or pending IDs.
func ManageLocal(ctx context.Context, dir, pin, binary string, stop bool) (LocalSessionReport, error) {
	if stop {
		return manageLocal(ctx, dir, pin, binary, localStop)
	}
	return manageLocal(ctx, dir, pin, binary, localStatus)
}
func ResumeLocalStop(ctx context.Context, dir, pin, binary string) (LocalSessionReport, error) {
	return manageLocal(ctx, dir, pin, binary, localResumeStop)
}

// ReviewLocalStop reuses the exact resume preconditions but never mutates a workload or releases a lease.
func ReviewLocalStop(ctx context.Context, dir, pin, binary string) (LocalSessionReport, error) {
	return manageLocal(ctx, dir, pin, binary, localReviewStop)
}

// FinalizeLocalStart covers only a complete, independently pinned receipt
// with the exact durable all-running prefix and all original live bindings.
// It never launches or restarts a guest or clears a pending marker.
func FinalizeLocalStart(ctx context.Context, dir, pin, binary string, apply bool) (LocalSessionReport, error) {
	if apply {
		return manageLocal(ctx, dir, pin, binary, localFinalizeStart)
	}
	return manageLocal(ctx, dir, pin, binary, localReviewStart)
}

// RecoverTerminalCleanup handles the narrow crash window after terminal
// evidence was written but before either cooperating reservation was released.
func RecoverTerminalCleanup(ctx context.Context, dir, pin, binary string, apply bool) (LocalSessionReport, error) {
	if apply {
		return manageLocal(ctx, dir, pin, binary, localReleaseCleanup)
	}
	return manageLocal(ctx, dir, pin, binary, localReviewCleanup)
}

type localSessionAction uint8

const (
	localStatus localSessionAction = iota
	localStop
	localReviewStop
	localResumeStop
	localReviewStart
	localFinalizeStart
	localReviewCleanup
	localReleaseCleanup
)

func manageLocal(ctx context.Context, dir, pin, binary string, action localSessionAction) (LocalSessionReport, error) {
	stop := action == localStop || action == localResumeStop
	terminal := action == localReviewCleanup || action == localReleaseCleanup
	applyTerminal := action == localReleaseCleanup
	resume := action == localReviewStop || action == localResumeStop || terminal
	finalization := action == localReviewStart || action == localFinalizeStart
	applyFinalize := action == localFinalizeStart
	r := LocalSessionReport{ReadOnly: !stop && !applyFinalize && !applyTerminal, RecoveryReviewRequired: true}
	if ctx == nil || os.Geteuid() == 0 || filepath.Base(binary) != "runc" || !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return r, errors.New("private local session required")
	}
	session := filepath.Base(dir)
	if !localSessionName.MatchString(session) {
		return r, errors.New("invalid local session name")
	}
	lease, e := acquireLocalLease(ctx, filepath.Dir(dir))
	if e != nil {
		return r, e
	}
	defer lease.close()
	if e = lease.checkActive(session); e != nil {
		return r, e
	}
	i, e := privateExerciseDir(dir)
	if e != nil {
		return r, e
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return r, e
	}
	defer root.Close()
	check := func() error {
		if e := lease.checkActive(session); e != nil {
			return e
		}
		a, e := privateExerciseDir(dir)
		if e != nil || !os.SameFile(i, a) {
			return errors.New("local session replaced")
		}
		b, e := root.Stat(".")
		if e != nil || !os.SameFile(i, b) {
			return errors.New("local session anchor mismatch")
		}
		return nil
	}
	if e = check(); e != nil {
		return r, e
	}
	raw, receiptIdentity, e := readPrivateRecord(root, "local-receipt.json", 64<<10)
	if e != nil {
		return r, e
	}
	rec, e := parseLocalReceipt(raw, pin, session, dir)
	if e != nil {
		return r, e
	}
	mapping, e := acquireMappingStore(ctx, rec.Mapping.Store, filepath.Dir(dir))
	if e != nil {
		return r, e
	}
	defer mapping.close()
	if e = checkMapping(mapping, rec.Mapping, dir); e != nil {
		return r, e
	}
	r.ReceiptSHA256 = pin
	r.Expected = len(rec.Bindings)
	r.RecoveryReviewRequired = true
	var intentIdentity os.FileInfo
	if resume {
		raw, info, err := readPrivateRecord(root, "stop-intent", 128)
		intentIdentity = info
		if err != nil || string(raw) != pin+"\n" {
			return r, errors.New("exact prior stop intent required")
		}
	} else if _, e = root.Lstat("stop-intent"); !os.IsNotExist(e) {
		return r, errors.New("previous stop intent exists; recovery review required")
	}
	var terminalIdentity os.FileInfo
	if terminal {
		record, info, err := readPrivateRecord(root, "cleanup-verified", 128)
		if err != nil || string(record) != pin+"\n" {
			return r, errors.New("exact terminal cleanup record required")
		}
		terminalIdentity = info
	} else if _, e = root.Lstat("cleanup-verified"); !os.IsNotExist(e) {
		return r, errors.New("cleanup record already exists; manual reservation review required")
	}
	log, logIdentity, e := readPrivateRecord(root, "events.ndjson", 64<<10)
	if e != nil {
		return r, e
	}
	if e = validatePersistentPhase(log, rec, finalization); e != nil {
		return r, e
	}
	checkLog := func() error {
		current, currentIdentity, err := readPrivateRecord(root, "events.ndjson", 64<<10)
		if err != nil || !samePrivateRecord(logIdentity, currentIdentity) || !bytes.Equal(log, current) {
			return errors.New("persistent start evidence changed during operation")
		}
		return nil
	}
	stateRoot := filepath.Join(dir, "runtime", "goreecloud-containers")
	stateIdentity, e := privateExerciseDir(stateRoot)
	if e != nil {
		return r, e
	}
	checkFinalization := func() error {
		if !finalization {
			return nil
		}
		return checkStartFinalizationEvidence(root, dir, rec, i, stateIdentity, mapping.identity)
	}
	if e = checkFinalization(); e != nil {
		return r, e
	}
	rt, e := engine.New(binary, stateRoot, nil)
	if e != nil {
		return r, e
	}
	before, e := rt.Inventory(ctx)
	if e != nil {
		return r, e
	}
	r.Pending = before.RecoveryRequired
	if terminal && (before.RecoveryRequired || len(before.Entries) != 0) {
		return r, errors.New("terminal cleanup requires empty runtime inventory")
	}
	if before.RecoveryRequired || len(before.Entries) > len(rec.Bindings) || (!resume && len(before.Entries) != len(rec.Bindings)) {
		return r, errors.New("local inventory uncertain or incomplete")
	}
	expected := map[string]engine.InstanceBinding{}
	for _, b := range rec.Bindings {
		expected[b.ID] = b
	}
	present := map[string]bool{}
	for _, x := range before.Entries {
		present[x.ID] = true
		b, ok := expected[x.ID]
		if !ok || !x.MetadataPresent || x.PendingOperation != "" {
			return r, errors.New("unexpected runtime instance")
		}
		status, e := rt.CheckBinding(ctx, b)
		if e != nil || status != x.Status {
			return r, errors.New("runtime binding or state changed")
		}
		if status == "running" {
			r.Running++
		} else if status == "stopped" && !finalization {
			r.Stopped++
		} else {
			return r, errors.New("unexpected local state")
		}
	}
	if resume {
		// Missing entries must be from an already reached reverse wave; do not
		// permit a skipped dependency wave or assume a failed state query is absent.
		earlierPresent := false
		for _, wave := range rec.StopWaves {
			wavePresent := false
			for _, id := range wave {
				b := rec.Bindings[id]
				if present[b.ID] {
					wavePresent = true
					continue
				}
				if earlierPresent {
					return r, errors.New("missing instance outside completed reverse waves")
				}
				if e = rt.CheckAbsentBinding(ctx, b); e != nil {
					return r, e
				}
				r.MissingProcessAbsent++
			}
			earlierPresent = earlierPresent || wavePresent
		}
	}
	after, e := rt.Inventory(ctx)
	if e != nil || !reflect.DeepEqual(before, after) {
		return r, errors.New("local inventory changed")
	}
	if e = check(); e != nil {
		return r, e
	}
	if e = checkMapping(mapping, rec.Mapping, dir); e != nil {
		return r, e
	}
	// Re-read the exact receipt and prior stop intent after inventory observations.
	checkReceipt := func() error {
		confirmed, info, err := readPrivateRecord(root, "local-receipt.json", 64<<10)
		if err != nil || !samePrivateRecord(receiptIdentity, info) || !bytes.Equal(raw, confirmed) {
			return errors.New("local receipt changed during review")
		}
		return nil
	}
	checkIntent := func() error {
		intent, info, err := readPrivateRecord(root, "stop-intent", 128)
		if err != nil || !samePrivateRecord(intentIdentity, info) || string(intent) != pin+"\n" {
			return errors.New("stop intent changed")
		}
		return nil
	}
	if e = checkReceipt(); e != nil {
		return r, e
	}
	if resume {
		if e = checkIntent(); e != nil {
			return r, e
		}
	}
	if e = checkLog(); e != nil {
		return r, e
	}
	r.BindingsObserved = true
	if terminal {
		verifyTerminal := func() error {
			if err := check(); err != nil {
				return err
			}
			if err := checkMapping(mapping, rec.Mapping, dir); err != nil {
				return err
			}
			if err := checkReceipt(); err != nil {
				return err
			}
			if err := checkIntent(); err != nil {
				return err
			}
			if err := checkLog(); err != nil {
				return err
			}
			b, info, err := readPrivateRecord(root, "cleanup-verified", 128)
			if err != nil || !samePrivateRecord(terminalIdentity, info) || string(b) != pin+"\n" {
				return errors.New("terminal record changed")
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			return nil
		}
		if r.MissingProcessAbsent != r.Expected {
			return r, errors.New("all original processes must be absent")
		}
		if e = verifyTerminal(); e != nil {
			return r, e
		}
		r.TerminalCleanupChecksPassed = true
		if !applyTerminal {
			return r, nil
		}
		// No stop, delete, journal clearing, or runtime recreation.
		for _, b := range rec.Bindings {
			if e = rt.CheckAbsentBinding(ctx, b); e != nil {
				return r, e
			}
		}
		again, err := rt.Inventory(ctx)
		if err != nil || again.RecoveryRequired || len(again.Entries) != 0 {
			return r, errors.New("terminal runtime inventory changed")
		}
		if e = verifyTerminal(); e != nil {
			return r, e
		}
		if e = syncDirectory(root); e != nil {
			return r, e
		}
		if e = releaseMappings(mapping, rec.Mapping, dir); e != nil {
			return r, e
		}
		if e = lease.release(session); e != nil {
			return r, e
		}
		r.CleanupVerified = true
		r.RecoveryReviewRequired = false
		return r, nil
	}
	if finalization {
		if r.Running != r.Expected {
			return r, errors.New("all original live bindings required")
		}
		if e = checkFinalization(); e != nil {
			return r, e
		}
		r.StartFinalizationChecksPassed = true
		if !applyFinalize {
			return r, nil
		}
		if e = ctx.Err(); e != nil {
			return r, e
		}
		if e = appendPersistentFinalEvent(root, log, logIdentity, rec); e != nil {
			return r, e
		}
		if e = check(); e != nil {
			return r, e
		}
		if e = checkMapping(mapping, rec.Mapping, dir); e != nil {
			return r, e
		}
		if e = checkFinalization(); e != nil {
			return r, e
		}
		if e = checkReceipt(); e != nil {
			return r, e
		}
		observed, e := rt.Inventory(ctx)
		if e != nil || !reflect.DeepEqual(before, observed) {
			return r, errors.New("inventory changed after finalization; preserve session")
		}
		for _, b := range rec.Bindings {
			status, err := rt.CheckBinding(ctx, b)
			if err != nil || status != "running" {
				return r, errors.New("binding changed after finalization; preserve session")
			}
		}
		r.StartFinalized = true
		r.RecoveryReviewRequired = false
		return r, nil
	}
	r.StopResumeChecksPassed = resume
	r.RecoveryReviewRequired = resume
	if !stop {
		return r, nil
	}
	// Durable intent before first signal, never automatically resume on failure.
	r.RecoveryReviewRequired = true
	var f *os.File
	if !resume {
		f, e = root.OpenFile("stop-intent", os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if e != nil {
			return r, e
		}
		_, e = f.WriteString(pin + "\n")
		if e == nil {
			e = f.Sync()
		}
		if e = errors.Join(e, f.Close()); e != nil {
			return r, e
		}
		if e = syncDirectory(root); e != nil {
			return r, e
		}
		intent, info, err := readPrivateRecord(root, "stop-intent", 128)
		if err != nil || string(intent) != pin+"\n" {
			return r, errors.New("new stop intent not verified")
		}
		intentIdentity = info
	}

	checkStopEvidence := func() error {
		if e := check(); e != nil {
			return e
		}
		if e := checkMapping(mapping, rec.Mapping, dir); e != nil {
			return e
		}
		if e := checkReceipt(); e != nil {
			return e
		}
		if e := checkIntent(); e != nil {
			return e
		}
		return checkLog()
	}
	for _, wave := range rec.StopWaves {
		if e = checkStopEvidence(); e != nil {
			return r, e
		}
		remaining := []string{}
		for _, id := range wave {
			if present[rec.Bindings[id].ID] {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) == 0 {
			continue
		}
		if e = parallelInstances(ctx, remaining, func(c context.Context, id string) error { return rt.StopBound(c, rec.Bindings[id]) }); e != nil {
			return r, errors.New("bound stop wave incomplete; preserve session")
		}
		if e = checkStopEvidence(); e != nil {
			return r, e
		}
		if e = parallelInstances(ctx, remaining, func(c context.Context, id string) error { return rt.DeleteBound(c, rec.Bindings[id]) }); e != nil {
			return r, errors.New("bound deletion wave incomplete; preserve session")
		}
	}
	inv, e := rt.Inventory(ctx)
	if e != nil || inv.RecoveryRequired || len(inv.Entries) != 0 {
		return r, errors.New("local cleanup inventory uncertain")
	}
	for _, b := range rec.Bindings {
		if e = rt.CheckAbsentBinding(ctx, b); e != nil {
			return r, e
		}
	}
	if e = check(); e != nil {
		return r, e
	}
	if e = checkStopEvidence(); e != nil {
		return r, e
	}
	f, e = root.OpenFile("cleanup-verified", os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return r, e
	}
	_, e = f.WriteString(pin + "\n")
	if e == nil {
		e = f.Sync()
	}
	if e = errors.Join(e, f.Close()); e != nil {
		return r, e
	}
	if e = syncDirectory(root); e != nil {
		return r, e
	}
	if e = releaseMappings(mapping, rec.Mapping, dir); e != nil {
		return r, e
	}
	if e = lease.release(session); e != nil {
		return r, e
	}
	r.Running = 0
	r.Stopped = 0
	r.CleanupVerified = true
	r.RecoveryReviewRequired = false
	return r, nil
}
