package bundle

import (
	"testing"
)

func TestDelegatedAllocationAndExactMappings(t *testing.T) {
	for _, raw := range []string{"me:100000:65536\n", "1000:100000:65536\n"} {
		if e := allocated(raw, "me", 1000, 100001); e != nil {
			t.Fatal(e)
		}
	}
	for _, raw := range []string{"other:100000:65536\n", "me:100000:65536\nother:100001:1\n", "me:100000:65536\nme:100001:1\n", "me:0:1\n", "me:4294967295:2\n"} {
		if e := allocated(raw, "me", 1000, 100001); e == nil {
			t.Fatal("unsafe allocation accepted")
		}
	}
	m := []IDMap{{0, 1000, 1}, {1000, 100001, 1}}
	if n, e := exactDelegatedMaps(m, 1000); e != nil || n != 100001 {
		t.Fatal(n, e)
	}
	for _, bad := range [][]IDMap{{{0, 1000, 1}}, {{0, 1000, 1}, {1000, 100001, 2}}, {{0, 1000, 1}, {1000, 1000, 1}}, {{0, 1000, 1}, {1001, 100001, 1}}} {
		if _, e := exactDelegatedMaps(bad, 1000); e == nil {
			t.Fatal("unsafe map accepted")
		}
	}
}
