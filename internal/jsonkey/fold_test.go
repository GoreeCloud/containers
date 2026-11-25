package jsonkey

import "testing"

func TestFold(t *testing.T) {
	for _, pair := range [][2]string{{"name", "NAME"}, {"kind", "Kind"}, {"status", "ſtatus"}, {"x", "x"}} {
		if Fold(pair[0]) != Fold(pair[1]) {
			t.Fatalf("equivalent keys did not fold together: %q", pair)
		}
	}
	if Fold("name") == Fold("names") {
		t.Fatal("different names collapsed")
	}
}
