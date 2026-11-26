package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpIncludesImplementedPreviewCommands(t *testing.T) {
	var b bytes.Buffer
	if e := usage(&b); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"snapshot ABS_BUNDLE", "image fetch-blob", "image assemble", "volume create", "volume inspect", "volume count", "serve-local", "local-query", "recovery-observe"} {
		if !strings.Contains(b.String(), s) {
			t.Fatal("missing help:", s)
		}
	}
}
