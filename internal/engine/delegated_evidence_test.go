package engine

import (
	"strings"
	"testing"
)

func TestDelegatedStatusEvidence(t *testing.T) {
	valid := "Uid:\t100000 100000 100000 100000\nGid:\t200000 200000 200000 200000\nNSpid:\t123 1\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapBnd:\t0000000000000000\nCapAmb:\t0000000000000000\nNoNewPrivs:\t1\nSeccomp:\t2\n"
	r, e := observeDelegatedStatus(valid, 100000, 200000)
	if e != nil || !r.SubordinateIdentityObserved || !r.ZeroCapabilitiesObserved || !r.SeccompFilterObserved || r.BundleIdentityVerified || r.HealthVerified || r.DeploymentAuthorized {
		t.Fatal(r, e)
	}
	for _, bad := range []string{strings.Replace(valid, "100000", "100001", 1), strings.Replace(valid, "123 1", "123", 1), strings.Replace(valid, "0000000000000000", "0000000000000001", 1), strings.Replace(valid, "Seccomp:\t2", "Seccomp:\t0", 1), valid + "Uid: 100000 100000 100000 100000\n"} {
		if _, e = observeDelegatedStatus(bad, 100000, 200000); e == nil {
			t.Fatal("invalid evidence accepted")
		}
	}
}
