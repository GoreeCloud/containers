package engine

import (
	"context"
	"errors"
	"github.com/GoreeCloud/containers/internal/bundle"
	"os"
	"strconv"
	"strings"
)

type DelegatedEvidence struct {
	ReadOnly                    bool `json:"readOnly"`
	RunningObserved             bool `json:"runningObserved"`
	SubordinateIdentityObserved bool `json:"subordinateIdentityObserved"`
	PIDNamespaceObserved        bool `json:"pidNamespaceObserved"`
	ZeroCapabilitiesObserved    bool `json:"zeroCapabilitiesObserved"`
	NoNewPrivilegesObserved     bool `json:"noNewPrivilegesObserved"`
	SeccompFilterObserved       bool `json:"seccompFilterObserved"`
	ResourceLimitsObserved      bool `json:"resourceLimitsObserved"`
	BundleIdentityVerified      bool `json:"bundleIdentityVerified"`
	HealthVerified              bool `json:"healthVerified"`
	DeploymentAuthorized        bool `json:"deploymentAuthorized"`
}

func observeDelegatedStatus(raw string, uid, gid uint32) (DelegatedEvidence, error) {
	var empty DelegatedEvidence
	fields := map[string][]string{}
	wanted := map[string]bool{"Uid": true, "Gid": true, "NSpid": true, "CapInh": true, "CapPrm": true, "CapEff": true, "CapBnd": true, "CapAmb": true, "NoNewPrivs": true, "Seccomp": true}
	for _, line := range strings.Split(raw, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || !wanted[k] {
			continue
		}
		if _, ok := fields[k]; ok {
			return empty, errors.New("duplicate process status field")
		}
		fields[k] = strings.Fields(v)
	}
	exact := func(k, w string, n int) bool {
		v := fields[k]
		if len(v) != n {
			return false
		}
		for _, s := range v {
			if s != w {
				return false
			}
		}
		return true
	}
	if !exact("Uid", strconv.FormatUint(uint64(uid), 10), 4) || !exact("Gid", strconv.FormatUint(uint64(gid), 10), 4) {
		return empty, errors.New("delegated host identity mismatch")
	}
	n := fields["NSpid"]
	if len(n) < 2 || n[len(n)-1] != "1" {
		return empty, errors.New("guest PID namespace evidence unavailable")
	}
	for _, k := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
		v := fields[k]
		if len(v) != 1 {
			return empty, errors.New("capability evidence unavailable")
		}
		num, e := strconv.ParseUint(v[0], 16, 64)
		if e != nil || num != 0 {
			return empty, errors.New("nonzero guest capabilities")
		}
	}
	if !exact("NoNewPrivs", "1", 1) || !exact("Seccomp", "2", 1) {
		return empty, errors.New("guest security evidence mismatch")
	}
	return DelegatedEvidence{ReadOnly: true, RunningObserved: true, SubordinateIdentityObserved: true, PIDNamespaceObserved: true, ZeroCapabilitiesObserved: true, NoNewPrivilegesObserved: true, SeccompFilterObserved: true}, nil
}

// ObserveDelegated compares a separately reviewed bundle's declared mapping
// and limits against the running process. It does NOT bind the bundle to
// the process, authenticate image bytes or attest health.
func (e *Engine) ObserveDelegated(ctx context.Context, id, dir string) (DelegatedEvidence, error) {
	var report DelegatedEvidence
	if ctx == nil {
		return report, errors.New("context required")
	}
	spec, err := bundle.Load(dir)
	if err != nil {
		return report, err
	}
	if err = bundle.RequireDelegatedMapping(spec); err != nil {
		return report, err
	}
	var uid, gid uint32
	for _, m := range spec.Linux.UIDMappings {
		if m.ContainerID == 1000 {
			uid = m.HostID
		}
	}
	for _, m := range spec.Linux.GIDMappings {
		if m.ContainerID == 1000 {
			gid = m.HostID
		}
	}
	err = e.mutate(ctx, id, func() error {
		before, err := e.readState(ctx, id)
		if err != nil || before.Status != "running" || before.PID <= 0 {
			return errors.New("running delegated guest unavailable")
		}
		proc, err := os.OpenRoot("/proc/" + strconv.FormatInt(before.PID, 10))
		if err != nil {
			return err
		}
		defer proc.Close()
		raw, err := boundedKernelRead(proc, "status")
		if err != nil {
			return err
		}
		observed, err := observeDelegatedStatus(raw, uid, gid)
		if err != nil {
			return err
		}
		if spec.Linux.Resources != nil {
			if err = verifyResourceLimits(ctx, before, spec.Linux.Resources); err != nil {
				return err
			}
			observed.ResourceLimitsObserved = true
		}
		again, err := e.readState(ctx, id)
		if err != nil || again != before {
			return errors.New("runtime state changed during delegated observation")
		}
		after, err := boundedKernelRead(proc, "status")
		if err != nil {
			return err
		}
		confirmed, err := observeDelegatedStatus(after, uid, gid)
		if err != nil {
			return err
		}
		confirmed.ResourceLimitsObserved = observed.ResourceLimitsObserved
		if confirmed != observed {
			return errors.New("delegated evidence changed")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		report = observed
		return nil
	})
	if err != nil {
		return DelegatedEvidence{}, err
	}
	return report, nil
}
