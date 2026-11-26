package main

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/GoreeCloud/containers/internal/bundle"
)

// bundlePreflight performs an inert, read-only two-stage OCI bundle check.
// It neither invokes runc/crun nor claims kernel isolation or deployability.
// Reasons are fixed identifiers so local paths and sensitive process metadata
// from untrusted OCI specifications never enter diagnostic output.
func bundlePreflight(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: gcontainers preflight BUNDLE_DIRECTORY")
	}
	report := struct {
		ReadOnly             bool   `json:"readOnly"`
		PolicyValid          bool   `json:"policyValid"`
		GuestLayoutValid     bool   `json:"guestLayoutValid"`
		SeccompExecveListed  bool   `json:"seccompExecveListed"`
		RuntimeInvoked       bool   `json:"runtimeInvoked"`
		IsolationVerified    bool   `json:"isolationVerified"`
		DeploymentAuthorized bool   `json:"deploymentAuthorized"`
		Reason               string `json:"reason,omitempty"`
	}{ReadOnly: true}
	spec, err := bundle.Load(args[0])
	if err != nil {
		report.Reason = "bundle_policy_rejected"
		if writeErr := json.NewEncoder(out).Encode(report); writeErr != nil {
			return writeErr
		}
		return errors.New("OCI bundle policy preflight rejected input")
	}
	report.PolicyValid = true
	if err := bundle.ValidateRuntimeLayout(args[0], spec); err != nil {
		report.Reason = "guest_layout_rejected"
		if writeErr := json.NewEncoder(out).Encode(report); writeErr != nil {
			return writeErr
		}
		return errors.New("OCI guest layout preflight rejected input")
	}
	report.GuestLayoutValid = true
	// A default-deny seccomp filter lacking execve can never launch the
	// configured guest process. Refuse an apparently successful preflight.
	// An explicit execve rule is necessary but does not prove runnable ABI,
	// all required syscalls, security policy enforcement or containment.
	report.SeccompExecveListed = bundle.HasExplicitGuestExecveAllowance(spec)
	if !report.SeccompExecveListed {
		report.Reason = "seccomp_execve_unlisted"
		if writeErr := json.NewEncoder(out).Encode(report); writeErr != nil {
			return writeErr
		}
		return errors.New("OCI guest bootstrap seccomp preflight rejected input")
	}
	report.Reason = "runtime_and_isolation_unverified"
	return json.NewEncoder(out).Encode(report)
}
