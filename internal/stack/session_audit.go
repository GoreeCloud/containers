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
)

type SessionAudit struct {
	ReadOnly                bool   `json:"readOnly"`
	PlanSHA256              string `json:"planSHA256"`
	AdmissionSHA256         string `json:"admissionSHA256"`
	Expected                int    `json:"expected"`
	ObservedRunning         int    `json:"observedRunning"`
	ObservedCreated         int    `json:"observedCreated"`
	ObservedStopped         int    `json:"observedStopped"`
	Missing                 int    `json:"missing"`
	Uncertain               int    `json:"uncertain"`
	Unmatched               int    `json:"unmatched"`
	RecordedPhase           string `json:"recordedPhase"`
	RecordedEvents          int    `json:"recordedEvents"`
	CleanupEvidenceObserved bool   `json:"cleanupEvidenceObserved"`
	RecoveryReviewRequired  bool   `json:"recoveryReviewRequired"`
	AtomicSnapshot          bool   `json:"atomicSnapshot"`
	IdentityVerified        bool   `json:"identityVerified"`
	RecoveryAuthorized      bool   `json:"recoveryAuthorized"`
}
type initialSessionReceipt struct {
	Review     ExerciseReview    `json:"review"`
	RuntimeIDs map[string]string `json:"runtimeIDs"`
	Bundles    map[string]string `json:"bundles"`
}

var exerciseRuntimeID = regexp.MustCompile("^gc-ex-[a-f0-9]{40}$")

func auditPrivateBytes(root *os.Root, name string, limit int64) ([]byte, error) {
	i, e := root.Lstat(name)
	if e != nil {
		return nil, e
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || !i.Mode().IsRegular() || i.Mode().Perm() != 0600 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || i.Size() <= 0 || i.Size() > limit {
		return nil, errors.New("invalid private audit record")
	}
	f, e := regularinput.Open(root, name, i)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(raw)) != i.Size() {
		return nil, errors.New("audit record changed")
	}
	if e = verifyStackInputIdentity(root, f, i, name); e != nil {
		return nil, e
	}
	return raw, nil
}
func strictAuditJSON(raw []byte, dst any) error {
	if e := rejectDuplicate(raw); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return errors.New("trailing audit record")
	}
	return nil
}
func validateInitialReceipt(raw []byte, p InstancePlan, review ExerciseReview, dir string) (initialSessionReceipt, error) {
	var r initialSessionReceipt
	if e := strictAuditJSON(raw, &r); e != nil {
		return r, e
	}
	if r.Review != review || len(r.RuntimeIDs) != p.InstanceCount || len(r.Bundles) != p.InstanceCount {
		return r, errors.New("session receipt does not match reviewed plan")
	}
	seen := map[string]bool{}
	dirs := map[string]bool{}
	for _, w := range p.StartWaves {
		for _, i := range w.Instances {
			id, b := r.RuntimeIDs[i.ID], r.Bundles[i.ID]
			if !exerciseRuntimeID.MatchString(id) || seen[id] || dirs[b] || filepath.Dir(b) != filepath.Join(dir, "bundles") || filepath.Clean(b) != b {
				return r, errors.New("unsafe receipt identity or bundle path")
			}
			seen[id] = true
			dirs[b] = true
		}
	}
	return r, nil
}
func expectedExerciseEvents(p InstancePlan) []exerciseEvent {
	events := []exerciseEvent{{Phase: "session-created"}, {Phase: "all-bundles-prepared"}}
	for _, w := range p.StartWaves {
		ids := []string{}
		for _, i := range w.Instances {
			ids = append(ids, i.ID)
		}
		for _, phase := range []string{"create-intent", "create-observed", "start-intent", "running-observed"} {
			events = append(events, exerciseEvent{phase, w.Number, ids})
		}
	}
	events = append(events, exerciseEvent{Phase: "all-running-observed"})
	return events
}
func auditEvents(raw []byte, p InstancePlan) (string, int, error) {
	// An incomplete line or unexpected phase cannot be interpreted as success.
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return "", 0, errors.New("incomplete session phase record")
	}
	lines := bytes.Split(raw[:len(raw)-1], []byte("\n"))
	expected := expectedExerciseEvents(p)
	persistentAt := len(expected)
	for n, w := range p.StopWaves {
		for _, phase := range []string{"stop-intent", "stop-observed", "delete-intent", "delete-observed"} {
			expected = append(expected, exerciseEvent{phase, n + 1, w})
		}
	}
	expected = append(expected, exerciseEvent{Phase: "cleanup-verified"})
	if len(lines) > len(expected) {
		return "", 0, errors.New("phase record exceeds plan")
	}
	var last exerciseEvent
	for i, line := range lines {
		var v exerciseEvent
		if e := strictAuditJSON(line, &v); e != nil {
			return "", 0, e
		}
		if i == persistentAt && i == len(lines)-1 && reflect.DeepEqual(v, exerciseEvent{Phase: "persistent-running-verified"}) {
			return v.Phase, len(lines), nil
		}
		if !reflect.DeepEqual(v, expected[i]) {
			return "", 0, errors.New("session phase sequence mismatch")
		}
		last = v
	}
	return last.Phase, len(lines), nil
}
func AuditSession(ctx context.Context, s Spec, admission, pin, dir, binary string) (SessionAudit, error) {
	var out SessionAudit
	if ctx == nil || os.Geteuid() == 0 || filepath.Base(binary) != "runc" {
		return out, errors.New("nonroot runc session audit required")
	}
	_, p, review, e := LoadExercise(s, admission, pin)
	if e != nil {
		return out, e
	}
	i, e := privateExerciseDir(dir)
	if e != nil {
		return out, e
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return out, e
	}
	defer root.Close()
	check := func() error {
		a, e := privateExerciseDir(dir)
		if e != nil || !os.SameFile(i, a) {
			return errors.New("session root changed")
		}
		b, e := root.Stat(".")
		if e != nil || !os.SameFile(i, b) {
			return errors.New("session anchor changed")
		}
		return nil
	}
	if e = check(); e != nil {
		return out, e
	}
	raw, e := auditPrivateBytes(root, "receipt.json", 64<<10)
	if e != nil {
		return out, errors.New("initial receipt unavailable; preserve session")
	}
	r, e := validateInitialReceipt(raw, p, review, dir)
	if e != nil {
		return out, e
	}
	log, e := auditPrivateBytes(root, "events.ndjson", 64<<10)
	if e != nil {
		return out, e
	}
	phase, n, e := auditEvents(log, p)
	if e != nil {
		return out, e
	}
	out = SessionAudit{ReadOnly: true, PlanSHA256: p.PlanSHA256, AdmissionSHA256: pin, Expected: p.InstanceCount, RecordedPhase: phase, RecordedEvents: n, RecoveryReviewRequired: true}
	rt, e := engine.New(binary, filepath.Join(dir, "runtime", "goreecloud-containers"), nil)
	if e != nil {
		return out, e
	}
	inv, e := rt.Inventory(ctx)
	if e != nil {
		return out, e
	}
	expected := map[string]bool{}
	for _, id := range r.RuntimeIDs {
		expected[id] = true
	}
	out.Missing = len(expected)
	for _, x := range inv.Entries {
		if !expected[x.ID] {
			out.Unmatched++
			continue
		}
		out.Missing--
		if x.PendingOperation != "" || x.Status == "uncertain" || !x.MetadataPresent {
			out.Uncertain++
			continue
		}
		switch x.Status {
		case "running":
			out.ObservedRunning++
		case "created":
			out.ObservedCreated++
		case "stopped":
			out.ObservedStopped++
		default:
			return out, errors.New("unsupported inventory state")
		}
	}
	raw2, e := auditPrivateBytes(root, "receipt.json", 64<<10)
	if e != nil || !bytes.Equal(raw, raw2) {
		return out, errors.New("receipt changed during audit")
	}
	log2, e := auditPrivateBytes(root, "events.ndjson", 64<<10)
	if e != nil || !bytes.Equal(log, log2) {
		return out, errors.New("phase log changed during audit")
	}
	after, e := rt.Inventory(ctx)
	if e != nil || !reflect.DeepEqual(inv, after) {
		return out, errors.New("session inventory changed")
	}
	if e = check(); e != nil {
		return out, e
	}
	if phase == "cleanup-verified" && out.Missing == out.Expected && out.Unmatched == 0 && !inv.RecoveryRequired {
		out.CleanupEvidenceObserved = true
		out.RecoveryReviewRequired = false
	}
	// Persistent receipts and stop markers are a separate trust boundary;
	// this generic audit never authorizes recovery or calls a session healthy.
	return out, ctx.Err()
}
