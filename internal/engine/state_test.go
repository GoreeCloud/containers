package engine

import (
	"context"
	"strings"
	"testing"
)

func TestDecodeRuntimeStates(t *testing.T) {
	good := []string{
		`{"id":"alpha","status":"created"}`,
		`{"ociVersion":"1.0.2","id":"alpha","status":"running","pid":391,"bundle":"/private/bundle","annotations":{"secret.note":"not-for-logs"}}`,
		`{"id":"alpha","status":"stopped","pid":0}`,
	}
	for _, data := range good {
		state, err := DecodeRuntimeState([]byte(data), "alpha")
		if err != nil {
			t.Fatalf("valid state rejected: %v", err)
		}
		if state.ID != "alpha" || state.Status == "" {
			t.Fatalf("invalid parsed state: %+v", state)
		}
	}
}
func TestRejectUntrustedOCIRuntimeState(t *testing.T) {
	cases := []string{
		"",
		"null",
		`{"id":"other","status":"created"}`,
		`{"id":"alpha","status":"running","pid":0}`,
		`{"id":"alpha","status":"running","pid":-1}`,
		`{"id":"alpha","status":"paused"}`,
		`{"id":"alpha","status":"creating"}`,
		`{"id":"alpha","status":"created","status":"running","pid":42}`,
		`{"id":"alpha","status":"created","annotations":{"a":"1","a":"2"}}`,
		`{"id":"alpha","status":"created","evil":true}`,
		`{"id":"alpha","status":"created"}{}`,
		strings.Repeat(" ", maxRuntimeOutput+1),
	}
	for _, data := range cases {
		if _, err := DecodeRuntimeState([]byte(data), "alpha"); err == nil {
			t.Errorf("unsafe state accepted: %q", data[:min(len(data), 140)])
		}
	}
}
func TestStateRedactsRuntimeMetadata(t *testing.T) {
	fake := &scriptedStateRunner{output: `{"id":"alpha","status":"running","pid":342,"bundle":"/private/secret","annotations":{"credential":"do-not-share"}}`}
	engine, err := New("/usr/bin/crun", "/tmp/synthetic-private", fake)
	if err != nil {
		t.Fatal(err)
	}
	output, err := engine.State(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "do-not-share") || strings.Contains(string(output), "/private/secret") {
		t.Fatal("sensitive metadata leaked")
	}
	if !strings.Contains(string(output), `"running"`) {
		t.Fatal("state omitted")
	}
}

type scriptedStateRunner struct{ output string }

func (f *scriptedStateRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	return []byte(f.output), nil
}
func FuzzRuntimeStateParser(f *testing.F) {
	f.Add([]byte(`{"id":"alpha","status":"created"}`))
	f.Add([]byte(`{"id":"alpha","status":"running","pid":29}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 8192 {
			t.Skip()
		}
		_, _ = DecodeRuntimeState(raw, "alpha")
	})
}
