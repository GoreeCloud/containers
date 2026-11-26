package engine

import (
	"strings"
	"testing"
)

func TestProcessGenerationAndBinding(t *testing.T) {
	raw := "123 (probe ) name) S " + strings.Repeat("0 ", 18) + "9876 0 0"
	gen, e := procGeneration(raw, 123)
	if e != nil || gen != "9876" {
		t.Fatal(gen, e)
	}
	if _, e = procGeneration(raw, 124); e == nil {
		t.Fatal("wrong PID accepted")
	}
	a := InstanceBinding{"worker", "/private/bundle", 1, 2, "sha256:" + strings.Repeat("a", 64)}
	if !sameBinding(a, a, "running") {
		t.Fatal("matching generation rejected")
	}
	b := a
	b.ProcessSHA256 = "sha256:" + strings.Repeat("b", 64)
	if sameBinding(a, b, "running") {
		t.Fatal("reused generation accepted")
	}
	b = a
	b.ProcessSHA256 = ""
	if !sameBinding(a, b, "stopped") {
		t.Fatal("stopped binding rejected")
	}
	b.MetadataInode++
	if sameBinding(a, b, "stopped") {
		t.Fatal("replacement metadata accepted")
	}
}
