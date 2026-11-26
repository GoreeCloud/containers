package stack

import (
	"testing"
)

func TestPrelaunchPhaseStrict(t *testing.T) {
	for _, v := range []string{"{\"phase\":\"session-created\"}\n", "{\"phase\":\"session-created\"}\n{\"phase\":\"all-bundles-prepared\"}\n"} {
		if !prelaunchPhase([]byte(v)) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"", "{\"phase\":\"session-created\"}", "{\"phase\":\"session-created\"}\n{\"phase\":\"create-intent\",\"wave\":1}\n", "{\"phase\":\"all-bundles-prepared\"}\n"} {
		if prelaunchPhase([]byte(v)) {
			t.Fatal("unsafe phase accepted")
		}
	}
}
