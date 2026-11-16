package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadOnlyCommands(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"version"}, &b); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != version {
		t.Fatal(b.String())
	}
	b.Reset()
	if err := run([]string{"doctor"}, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"production_ready": false`) {
		t.Fatal(b.String())
	}
}
func TestInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"validate"}, {"validate", "not-a-bundle"}, {"unknown"},
		{"oci", "unknown"}, {"oci", "create", "--id", "../bad", "--experimental"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
