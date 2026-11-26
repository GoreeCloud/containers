package stack

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// No runtime action is authorized by this check. The caller holds the
// cooperative session and mapping locks and independently checks live bindings.
func checkStartFinalizationEvidence(root *os.Root, dir string, rec localReceipt, sessionIdentity, stateIdentity, mappingIdentity os.FileInfo) error {
	for _, name := range []string{"stop-intent", "cleanup-verified", "prelaunch-cancelled"} {
		if _, err := root.Lstat(name); !os.IsNotExist(err) {
			return errors.New("prior stop or terminal evidence exists")
		}
	}
	raw, _, err := readPrivateRecord(root, "prelaunch.json", 4096)
	if err != nil {
		return err
	}
	var pre prelaunchRecord
	if err = strictAuditJSON(raw, &pre); err != nil {
		return err
	}
	if pre.Version != 1 || pre.Session != dir || pre.PlanSHA256 != rec.PlanSHA256 ||
		pre.AdmissionSHA256 != rec.AdmissionSHA256 || pre.MappingStore != rec.Mapping.Store ||
		pre.SessionIdentity != prelaunchFileIdentity(sessionIdentity) ||
		pre.RuntimeIdentity != prelaunchFileIdentity(stateIdentity) ||
		pre.MappingIdentity != prelaunchFileIdentity(mappingIdentity) {
		return errors.New("prelaunch record does not bind this session")
	}
	intent, _, err := readPrivateRecord(root, "launch-intent", 128)
	if err != nil || string(intent) != digestBytes(raw)+"\n" {
		return errors.New("exact launch intent required")
	}
	data, _, err := readPrivateRecord(root, "mapping-reservation.json", 4096)
	var ref mappingReference
	if err != nil || strictAuditJSON(data, &ref) != nil || ref != rec.Mapping {
		return errors.New("local mapping reference changed")
	}
	state, err := privateExerciseDir(filepath.Join(dir, "runtime", "goreecloud-containers"))
	if err != nil || !os.SameFile(stateIdentity, state) {
		return errors.New("runtime identity changed")
	}
	return nil
}

func appendPersistentFinalEvent(root *os.Root, prefix []byte, identity os.FileInfo, rec localReceipt) error {
	if err := validatePersistentPhase(prefix, rec, true); err != nil {
		return err
	}
	current, info, err := readPrivateRecord(root, "events.ndjson", 64<<10)
	if err != nil || !os.SameFile(identity, info) || !bytes.Equal(current, prefix) {
		return errors.New("startup log changed before finalization")
	}
	f, err := root.OpenFile("events.ndjson", os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	opened, err := f.Stat()
	if err != nil || !privateRecord(opened, 0600) || !os.SameFile(info, opened) || verifyStackInputIdentity(root, f, info, "events.ndjson") != nil {
		f.Close()
		return errors.New("startup log identity changed before append")
	}
	event, err := json.Marshal(exerciseEvent{Phase: "persistent-running-verified"})
	if err != nil {
		f.Close()
		return err
	}
	event = append(event, '\n')
	n, err := f.Write(event)
	if err == nil && n != len(event) {
		err = errors.New("incomplete final event")
	}
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	observed, after, err := readPrivateRecord(root, "events.ndjson", 64<<10)
	expected := append(append([]byte(nil), prefix...), event...)
	if err != nil || !os.SameFile(identity, after) || !bytes.Equal(expected, observed) || validatePersistentEvents(observed, rec) != nil {
		return errors.New("finalization evidence not verified; preserve session")
	}
	return syncDirectory(root)
}
