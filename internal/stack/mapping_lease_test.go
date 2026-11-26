package stack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMappingLeaseAcrossParents(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless")
	}
	base := t.TempDir()
	os.Chmod(base, 0700)
	store := filepath.Join(base, "leases")
	if e := os.Mkdir(store, 0700); e != nil {
		t.Fatal(e)
	}
	one := filepath.Join(base, "one", "stack-exercise-123")
	two := filepath.Join(base, "two", "stack-exercise-456")
	ids := func(n uint32) map[uint32]bool { return map[uint32]bool{n: true} }
	l, e := acquireMappingStore(context.Background(), store, filepath.Dir(one))
	if e != nil {
		t.Fatal(e)
	}
	a, e := reserveMappings(l, one, ids(100001), ids(200001))
	if e != nil {
		l.close()
		t.Fatal(e)
	}
	l.close()
	l, e = acquireMappingStore(context.Background(), store, filepath.Dir(two))
	if e != nil {
		t.Fatal(e)
	}
	defer l.close()
	if _, e = reserveMappings(l, two, ids(100001), ids(200002)); !errors.Is(e, errMappingInUse) {
		t.Fatal("UID conflict", e)
	}
	if _, e = reserveMappings(l, two, ids(100002), ids(200001)); !errors.Is(e, errMappingInUse) {
		t.Fatal("GID conflict", e)
	}
	b, e := reserveMappings(l, two, ids(100002), ids(200002))
	if e != nil {
		t.Fatal(e)
	}
	if e = releaseMappings(l, a, two); e == nil {
		t.Fatal("wrong session released")
	}
	if e = releaseMappings(l, a, one); e != nil {
		t.Fatal(e)
	}
	if e = checkMapping(l, b, two); e != nil {
		t.Fatal(e)
	}
	if e = releaseMappings(l, b, two); e != nil {
		t.Fatal(e)
	}
}
