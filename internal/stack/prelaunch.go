package stack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"os"
	"path/filepath"
	"syscall"
)

type prelaunchIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

func prelaunchFileIdentity(i os.FileInfo) prelaunchIdentity {
	if st, ok := i.Sys().(*syscall.Stat_t); ok {
		return prelaunchIdentity{uint64(st.Dev), st.Ino}
	}
	return prelaunchIdentity{}
}

type prelaunchRecord struct {
	SessionIdentity prelaunchIdentity `json:"sessionIdentity"`
	RuntimeIdentity prelaunchIdentity `json:"runtimeIdentity"`
	MappingIdentity prelaunchIdentity `json:"mappingIdentity"`
	Version         int               `json:"version"`
	Session         string            `json:"session"`
	PlanSHA256      string            `json:"planSHA256"`
	AdmissionSHA256 string            `json:"admissionSHA256"`
	MappingStore    string            `json:"mappingStore"`
}
type PrelaunchReport struct {
	ReadOnly                  bool `json:"readOnly"`
	EmptyInventoryObserved    bool `json:"emptyInventoryObserved"`
	NoLaunchIntentObserved    bool `json:"noLaunchIntentObserved"`
	MappingReservationPresent bool `json:"mappingReservationPresent"`
	ReservationReleased       bool `json:"reservationReleased"`
	RecoveryReviewRequired    bool `json:"recoveryReviewRequired"`
	DeploymentAuthorized      bool `json:"deploymentAuthorized"`
}

func writePrelaunchRecord(root *os.Root, name string, raw []byte) error {
	f, e := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	if e = errors.Join(e, f.Close()); e != nil {
		return e
	}
	return syncDirectory(root)
}
func prelaunchPhase(raw []byte) bool {
	// Only exact complete prefixes of the pre-mutation log are acceptable.
	first, _ := json.Marshal(exerciseEvent{Phase: "session-created"})
	second, _ := json.Marshal(exerciseEvent{Phase: "all-bundles-prepared"})
	first = append(first, '\n')
	second = append(second, '\n')
	return bytes.Equal(raw, first) || bytes.Equal(raw, append(first, second...))
}

// RecoverPrelaunch releases only an explicitly pinned, versioned session
// whose cooperative launch gate was never reached. No runtime mutations,
// deletion of workload files, or clearing of pending engine markers.
func RecoverPrelaunch(ctx context.Context, dir, pin, binary string, apply bool) (PrelaunchReport, error) {
	r := PrelaunchReport{ReadOnly: !apply, RecoveryReviewRequired: true}
	if ctx == nil || os.Geteuid() == 0 || filepath.Base(binary) != "runc" || !localSessionName.MatchString(filepath.Base(dir)) || !imagePattern.MatchString(pin) {
		return r, errors.New("private prelaunch session and pin required")
	}
	parent := filepath.Dir(dir)
	session := filepath.Base(dir)
	l, e := acquireLocalLease(ctx, parent)
	if e != nil {
		return r, e
	}
	defer l.close()
	if e = l.checkActive(session); e != nil {
		return r, e
	}
	identity, e := privateExerciseDir(dir)
	if e != nil {
		return r, e
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return r, e
	}
	defer root.Close()
	check := func() error {
		if e := l.checkActive(session); e != nil {
			return e
		}
		a, e := privateExerciseDir(dir)
		if e != nil || !os.SameFile(identity, a) {
			return errors.New("session changed")
		}
		b, e := root.Stat(".")
		if e != nil || !os.SameFile(identity, b) {
			return errors.New("session anchor changed")
		}
		for _, n := range []string{"launch-intent", "local-receipt.json", "stop-intent", "cleanup-verified", "prelaunch-cancelled"} {
			if _, e := root.Lstat(n); !os.IsNotExist(e) {
				return errors.New("launch or prior recovery evidence exists")
			}
		}
		raw, _, e := readPrivateRecord(root, "events.ndjson", 64<<10)
		if e != nil || !prelaunchPhase(raw) {
			return errors.New("prelaunch log not verified")
		}
		record, _, e := readPrivateRecord(root, "prelaunch.json", 4096)
		if e != nil || digestBytes(record) != pin {
			return errors.New("prelaunch record pin mismatch")
		}
		return nil
	}
	if e = check(); e != nil {
		return r, e
	}
	raw, _, e := readPrivateRecord(root, "prelaunch.json", 4096)
	if e != nil {
		return r, e
	}
	var rec prelaunchRecord
	if e = strictAuditJSON(raw, &rec); e != nil {
		return r, e
	}
	if rec.Version != 1 || rec.SessionIdentity.Inode == 0 || rec.RuntimeIdentity.Inode == 0 || rec.MappingIdentity.Inode == 0 || rec.SessionIdentity != prelaunchFileIdentity(identity) || rec.Session != dir || !imagePattern.MatchString(rec.PlanSHA256) || !imagePattern.MatchString(rec.AdmissionSHA256) {
		return r, errors.New("invalid prelaunch record")
	}
	m, e := acquireMappingStore(ctx, rec.MappingStore, parent)
	if e != nil {
		return r, e
	}
	defer m.close()
	if prelaunchFileIdentity(m.identity) != rec.MappingIdentity {
		return r, errors.New("mapping store identity changed")
	}
	reservations, e := scanMappingRecords(m)
	if e != nil {
		return r, e
	}
	var ref mappingReference
	for name, v := range reservations {
		if v.Session != dir {
			continue
		}
		if ref.Name != "" {
			return r, errors.New("multiple mapping reservations")
		}
		data, _, e := readPrivateRecord(m.root, name, 4096)
		if e != nil {
			return r, e
		}
		ref = mappingReference{Store: rec.MappingStore, Name: name, SHA256: digestBytes(data)}
	}
	if ref.Name != "" {
		r.MappingReservationPresent = true
		// An incomplete reference is preserved for manual review.
		b, _, e := readPrivateRecord(root, "mapping-reservation.json", 4096)
		if e != nil {
			return r, e
		}
		var stored mappingReference
		if e = strictAuditJSON(b, &stored); e != nil || stored != ref {
			return r, errors.New("mapping reference not verified")
		}
		if e = checkMapping(m, ref, dir); e != nil {
			return r, e
		}
	} else if _, e = root.Lstat("mapping-reservation.json"); !os.IsNotExist(e) {
		return r, errors.New("unmatched mapping reference")
	}
	state := filepath.Join(dir, "runtime", "goreecloud-containers")
	stateIdentity, e := privateExerciseDir(state)
	if e != nil {
		return r, e
	}
	if prelaunchFileIdentity(stateIdentity) != rec.RuntimeIdentity {
		return r, errors.New("runtime directory identity changed")
	}
	rt, e := engine.New(binary, state, nil)
	if e != nil {
		return r, e
	}
	inspect := func() error {
		a, e := privateExerciseDir(state)
		if e != nil || !os.SameFile(stateIdentity, a) {
			return errors.New("runtime root changed")
		}
		inv, e := rt.Inventory(ctx)
		if e != nil || len(inv.Entries) != 0 || inv.RecoveryRequired {
			return errors.New("runtime inventory not empty")
		}
		return check()
	}
	if e = inspect(); e != nil {
		return r, e
	}
	if e = inspect(); e != nil {
		return r, e
	}
	r.EmptyInventoryObserved = true
	r.NoLaunchIntentObserved = true
	if !apply {
		return r, nil
	}
	// Durable terminal evidence BEFORE releasing either reservation. If a
	// release fails, do not replay or silently remove ambiguous evidence.
	if e = writePrelaunchRecord(root, "prelaunch-cancelled", []byte(pin+"\n")); e != nil {
		return r, e
	}
	if ref.Name != "" {
		if e = releaseMappings(m, ref, dir); e != nil {
			return r, e
		}
	}
	if e = l.release(session); e != nil {
		return r, e
	}
	r.ReservationReleased = true
	r.RecoveryReviewRequired = false
	return r, nil
}
