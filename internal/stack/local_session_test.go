package stack

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalLeasePersistsAndRefusesReplacement(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless lease")
	}
	parent := t.TempDir()
	if e := os.Chmod(parent, 0700); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	lease, e := acquireLocalLease(ctx, parent)
	if e != nil {
		t.Fatal(e)
	}
	if e = lease.reserve("stack-exercise-123"); e != nil {
		lease.close()
		t.Fatal(e)
	}
	short, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	if l, e := acquireLocalLease(short, parent); !errors.Is(e, context.DeadlineExceeded) {
		if l != nil {
			l.close()
		}
		lease.close()
		t.Fatal("concurrent lock accepted", e)
	}
	lease.close()
	next, e := acquireLocalLease(ctx, parent)
	if e != nil {
		t.Fatal(e)
	}
	defer next.close()
	if e = next.requireVacant(); e == nil {
		t.Fatal("durable reservation lost")
	}
	if e = next.release("stack-exercise-124"); e == nil {
		t.Fatal("wrong session removed lease")
	}
	if e = next.checkActive("stack-exercise-123"); e != nil {
		t.Fatal(e)
	}
	if e = next.release("stack-exercise-123"); e != nil {
		t.Fatal(e)
	}
	if e = next.requireVacant(); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(filepath.Join(parent, "local-session.lock"), filepath.Join(parent, "old.lock")); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(parent, "local-session.lock"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	if e = next.requireVacant(); e == nil {
		t.Fatal("replaced lock accepted")
	}
}
func TestLocalReceiptPinsAndPaths(t *testing.T) {
	d := "/private/stack-exercise-123"
	id := "gc-" + strings.Repeat("a", 40)
	pin := "sha256:" + strings.Repeat("b", 64)
	r := localReceipt{Version: 1, Mapping: mappingReference{Store: "/private/leases", Name: "lease-" + strings.Repeat("c", 32) + ".json", SHA256: pin}, Session: filepath.Base(d), PlanSHA256: pin, AdmissionSHA256: pin, StopWaves: [][]string{{id}}, Bindings: map[string]engine.InstanceBinding{id: {ID: "gc-ex-" + strings.Repeat("c", 40), Bundle: filepath.Join(d, "bundles", "prepared-1"), MetadataDevice: 1, MetadataInode: 2, ProcessPID: 123, BootSHA256: pin, ProcessSHA256: pin}}}
	raw, _ := json.Marshal(r)
	if _, e := parseLocalReceipt(raw, digestBytes(raw), r.Session, d); e != nil {
		t.Fatal(e)
	}
	if _, e := parseLocalReceipt(raw, pin, r.Session, d); e == nil {
		t.Fatal("wrong receipt pin accepted")
	}
	b := r.Bindings[id]
	b.Bundle = "/other/bundles/prepared-1"
	r.Bindings[id] = b
	raw, _ = json.Marshal(r)
	if _, e := parseLocalReceipt(raw, digestBytes(raw), r.Session, d); e == nil {
		t.Fatal("external bundle accepted")
	}
	b.Bundle = filepath.Join(d, "bundles", "prepared-1")
	r.Bindings[id] = b
	r.StopWaves = [][]string{{id, id}}
	raw, _ = json.Marshal(r)
	if _, e := parseLocalReceipt(raw, digestBytes(raw), r.Session, d); e == nil {
		t.Fatal("duplicate stop accepted")
	}
}

func TestPersistentEventsRequireExactStartup(t *testing.T) {
	_, p := exerciseSpec(t)
	rec := localReceipt{StopWaves: p.StopWaves}
	events := append(expectedExerciseEvents(p), exerciseEvent{Phase: "persistent-running-verified"})
	render := func(es []exerciseEvent) []byte {
		var lines []string
		for _, ev := range es {
			b, _ := json.Marshal(ev)
			lines = append(lines, string(b))
		}
		return []byte(strings.Join(lines, "\n") + "\n")
	}
	valid := render(events)
	if err := validatePersistentEvents(valid, rec); err != nil {
		t.Fatal(err)
	}
	wrongWave := append([]exerciseEvent(nil), events...)
	wrongWave[2].Wave++
	wrongID := append([]exerciseEvent(nil), events...)
	wrongID[2].IDs = []string{"wrong-instance"}
	cases := map[string][]byte{
		"final only":      render(events[len(events)-1:]),
		"missing intent":  render(append(append([]exerciseEvent(nil), events[:2]...), events[3:]...)),
		"wrong wave":      render(wrongWave),
		"wrong instance":  render(wrongID),
		"incomplete line": valid[:len(valid)-1],
		"duplicate field": []byte(strings.Replace(string(valid), `"phase":"session-created"`, `"phase":"session-created","phase":"session-created"`, 1)),
		"extra event":     render(append(append([]exerciseEvent(nil), events...), events[len(events)-1])),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validatePersistentEvents(data, rec); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}

func TestPrivateRecordRejectsSameContentRewrite(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "receipt")
	if err := os.WriteFile(f, []byte("fixed"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	raw, before, err := readPrivateRecord(root, "receipt", 64)
	if err != nil {
		t.Fatal(err)
	}
	_, same, err := readPrivateRecord(root, "receipt", 64)
	if err != nil || !samePrivateRecord(before, same) {
		t.Fatal("unchanged record rejected", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err = os.WriteFile(f, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(f, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	_, after, err := readPrivateRecord(root, "receipt", 64)
	if err != nil || samePrivateRecord(before, after) {
		t.Fatal("same-content rewrite accepted", err)
	}
	if err = os.Rename(f, f+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(f, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, replacement, err := readPrivateRecord(root, "receipt", 64)
	if err != nil || samePrivateRecord(after, replacement) {
		t.Fatal("replacement accepted", err)
	}
}
