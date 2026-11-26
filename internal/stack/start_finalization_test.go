package stack

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalizationEvidenceRefusesConflictingRecords(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "runtime", "goreecloud-containers")
	mapping := t.TempDir()
	for _, d := range []string{dir, mapping} {
		if err := os.Chmod(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	sessionInfo, err := privateExerciseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	stateInfo, err := privateExerciseDir(state)
	if err != nil {
		t.Fatal(err)
	}
	mappingInfo, err := privateExerciseDir(mapping)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	pin := digestBytes([]byte("synthetic"))
	rec := localReceipt{PlanSHA256: pin, AdmissionSHA256: pin, Mapping: mappingReference{Store: mapping, Name: "lease-0123456789abcdef0123456789abcdef.json", SHA256: pin}}
	pre := prelaunchRecord{Version: 1, Session: dir, PlanSHA256: pin, AdmissionSHA256: pin, MappingStore: mapping, SessionIdentity: prelaunchFileIdentity(sessionInfo), RuntimeIdentity: prelaunchFileIdentity(stateInfo), MappingIdentity: prelaunchFileIdentity(mappingInfo)}
	raw, err := json.Marshal(pre)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := json.Marshal(rec.Mapping)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"prelaunch.json": raw, "launch-intent": []byte(digestBytes(raw) + "\n"), "mapping-reservation.json": ref} {
		if err = writePrelaunchRecord(root, name, data); err != nil {
			t.Fatal(err)
		}
	}
	check := func() error {
		return checkStartFinalizationEvidence(root, dir, rec, sessionInfo, stateInfo, mappingInfo)
	}
	if err = check(); err != nil {
		t.Fatal("valid records rejected", err)
	}
	for _, name := range []string{"stop-intent", "cleanup-verified", "prelaunch-cancelled"} {
		if err = writePrelaunchRecord(root, name, []byte("fixture")); err != nil {
			t.Fatal(err)
		}
		if err = check(); err == nil {
			t.Fatal("conflicting record accepted", name)
		}
		if err = root.Remove(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"launch-intent", "mapping-reservation.json", "prelaunch.json"} {
		original, err := root.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err = root.WriteFile(name, []byte("invalid"), 0600); err != nil {
			t.Fatal(err)
		}
		if err = check(); err == nil {
			t.Fatal("corrupt record accepted", name)
		}
		if err = root.WriteFile(name, original, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = check(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(state, state+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err = check(); err == nil {
		t.Fatal("replacement runtime directory accepted")
	}
}

func TestFinalizationAppendRejectsReplayAndReplacement(t *testing.T) {
	_, p := exerciseSpec(t)
	rec := localReceipt{StopWaves: p.StopWaves}
	var prefix bytes.Buffer
	enc := json.NewEncoder(&prefix)
	for _, ev := range expectedExerciseEvents(p) {
		if err := enc.Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
	for _, scenario := range []string{"valid", "replaced", "invalid-prefix"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			initial := prefix.Bytes()
			if scenario == "invalid-prefix" {
				initial = []byte("{\"phase\":\"all-running-observed\"}\n")
			}
			if err = writePrelaunchRecord(root, "events.ndjson", initial); err != nil {
				t.Fatal(err)
			}
			raw, info, err := readPrivateRecord(root, "events.ndjson", 64<<10)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "replaced" {
				if err = root.Rename("events.ndjson", "original"); err != nil {
					t.Fatal(err)
				}
				if err = writePrelaunchRecord(root, "events.ndjson", raw); err != nil {
					t.Fatal(err)
				}
			}
			err = appendPersistentFinalEvent(root, raw, info, rec)
			got, readErr := root.ReadFile("events.ndjson")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if scenario != "valid" {
				if err == nil || !bytes.Equal(got, raw) {
					t.Fatal("refusal did not preserve evidence", err)
				}
				return
			}
			if err != nil || validatePersistentEvents(got, rec) != nil {
				t.Fatal("finalization failed", err)
			}
			if err = appendPersistentFinalEvent(root, raw, info, rec); err == nil {
				t.Fatal("replay accepted")
			}
			final, err := root.ReadFile("events.ndjson")
			if err != nil || !bytes.Equal(got, final) {
				t.Fatal("replay changed evidence", err)
			}
		})
	}
}
