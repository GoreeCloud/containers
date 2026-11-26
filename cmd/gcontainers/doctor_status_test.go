package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorAdmissionStatusNeverAuthorizesProduction(t *testing.T) {
	var out bytes.Buffer
	if err := doctor(&out); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["production_ready"] != false {
		t.Fatal("read-only doctor claimed production readiness")
	}
	allowed := map[string]bool{
		"not_installed":                     true,
		"untrusted_executable":              true,
		"runtime_version_or_probe_rejected": true,
		"preview_preflight_passed":          true,
	}
	for _, runtimeName := range []string{"crun", "runc"} {
		value, ok := result[runtimeName+"_admission_status"].(string)
		if !ok || !allowed[value] {
			t.Fatalf("unrecognized admission state for %s: %v", runtimeName, result)
		}
		if value == "preview_preflight_passed" &&
			result[runtimeName+"_security_baseline_met"] != true {
			t.Fatal("admission status disagrees with approved preview baseline")
		}
	}
}

func TestDoctorAdmissionStatusMissingAndUntrusted(t *testing.T) {
	ctx := context.Background()
	if ok, status := runtimeAdmissionStatus(ctx, "", errors.New("not found")); ok || status != "not_installed" {
		t.Fatalf("missing runtime was not handled: %t %s", ok, status)
	}
	if os.Geteuid() == 0 {
		t.Skip("untrusted user-owned binary case requires nonroot runner")
	}
	runtimePath := filepath.Join(t.TempDir(), "crun")
	if err := os.WriteFile(runtimePath, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if ok, status := runtimeAdmissionStatus(ctx, runtimePath, nil); ok || status != "untrusted_executable" {
		t.Fatalf("untrusted user executable was admitted: %t %s", ok, status)
	}
}
