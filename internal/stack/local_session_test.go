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
	r := localReceipt{Version: 1, Session: filepath.Base(d), PlanSHA256: pin, AdmissionSHA256: pin, StopWaves: [][]string{{id}}, Bindings: map[string]engine.InstanceBinding{id: {ID: "gc-ex-" + strings.Repeat("c", 40), Bundle: filepath.Join(d, "bundles", "prepared-1"), MetadataDevice: 1, MetadataInode: 2, ProcessSHA256: pin}}}
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
