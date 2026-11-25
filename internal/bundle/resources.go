package bundle

import "errors"

// Resources admits a narrow OCI cgroup v2 profile. All three limits must
// be present, positive and finite. No host cgroup paths or device rules.
type Resources struct {
	CPU    *CPUResources    `json:"cpu"`
	Memory *MemoryResources `json:"memory"`
	Pids   *PIDResources    `json:"pids"`
}
type CPUResources struct {
	Quota  int64  `json:"quota"`
	Period uint64 `json:"period"`
}
type MemoryResources struct {
	Limit int64 `json:"limit"`
}
type PIDResources struct {
	Limit int64 `json:"limit"`
}

func validateResources(r *Resources) error {
	if r == nil {
		return nil
	}
	if r.CPU == nil || r.Memory == nil || r.Pids == nil {
		return errors.New("resource profile requires CPU, memory and PID limits together")
	}
	if r.CPU.Period != 100000 || r.CPU.Quota < 1000 || r.CPU.Quota > 6400000 {
		return errors.New("resource CPU quota must be 1000..6400000 with period 100000")
	}
	if r.Memory.Limit < 16<<20 || r.Memory.Limit > 64<<30 {
		return errors.New("resource memory limit must be 16 MiB..64 GiB")
	}
	if r.Pids.Limit < 16 || r.Pids.Limit > 4096 {
		return errors.New("resource PID limit must be 16..4096")
	}
	return nil
}
