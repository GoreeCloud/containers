package bundle

import "encoding/json"

// HasExplicitGuestExecveAllowance is an intentionally *necessary*, not
// sufficient, seccomp bootstrap check. With a deny-default profile and no
// explicit guest execve allowance, the container init cannot exec its process.
// An allowance does NOT prove the runtime, remaining syscalls, cgroup, LSM,
// kernel, architecture or guests will actually start.
func HasExplicitGuestExecveAllowance(spec *Spec) bool {
	if spec == nil || spec.Linux == nil || spec.Linux.Seccomp == nil {
		return false
	}
	for _, raw := range spec.Linux.Seccomp.Syscalls {
		var rule struct {
			Names  []string `json:"names"`
			Action string   `json:"action"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil || rule.Action != "SCMP_ACT_ALLOW" {
			continue
		}
		for _, name := range rule.Names {
			if name == "execve" {
				return true
			}
		}
	}
	return false
}
