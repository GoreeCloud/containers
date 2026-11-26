package stack

import (
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
)

type InstanceObservation struct {
	ID                     string `json:"id"`
	Service                string `json:"service"`
	Replica                int    `json:"replica"`
	State                  string `json:"state"`
	DependencyRunningCount int    `json:"dependencyRunningCount"`
	DependencyCount        int    `json:"dependencyCount"`
	Review                 string `json:"review"`
}
type RuntimeObservation struct {
	PlanSHA256               string                `json:"planSHA256"`
	Instances                []InstanceObservation `json:"instances"`
	Expected                 int                   `json:"expected"`
	Missing                  int                   `json:"missing"`
	Running                  int                   `json:"running"`
	Created                  int                   `json:"created"`
	Stopped                  int                   `json:"stopped"`
	Uncertain                int                   `json:"uncertain"`
	UnmatchedRuntimeCount    int                   `json:"unmatchedRuntimeCount"`
	RecoveryRequired         bool                  `json:"recoveryRequired"`
	ReadOnly                 bool                  `json:"readOnly"`
	AtomicSnapshot           bool                  `json:"atomicSnapshot"`
	InstanceIdentityVerified bool                  `json:"instanceIdentityVerified"`
	HealthVerified           bool                  `json:"healthVerified"`
	DeploymentAuthorized     bool                  `json:"deploymentAuthorized"`
}

// ObserveInstances correlates names only; no ownership, image, readiness or
// instance identity is inferred from a matching deterministic identifier.
func ObserveInstances(spec Spec, pin string, inventory engine.InventoryReport) (RuntimeObservation, error) {
	var empty RuntimeObservation
	plan, e := ReviewInstancePlan(spec, pin)
	if e != nil {
		return empty, e
	}
	if !inventory.ReadOnly || inventory.AtomicSnapshot || len(inventory.Entries) > 128 {
		return empty, errors.New("invalid observed inventory")
	}
	byID := map[string]engine.InventoryEntry{}
	counts := map[string]int{}
	for _, x := range inventory.Entries {
		if engine.ValidateID(x.ID) != nil {
			return empty, errors.New("invalid inventory ID")
		}
		if _, ok := byID[x.ID]; ok {
			return empty, errors.New("duplicate inventory ID")
		}
		if x.Status != "running" && x.Status != "stopped" && x.Status != "created" && x.Status != "uncertain" {
			return empty, errors.New("invalid observed state")
		}
		if x.PendingOperation != "" || !x.MetadataPresent || x.Status == "uncertain" {
			counts["uncertain"]++
		} else {
			counts[x.Status]++
		}
		byID[x.ID] = x
	}
	if counts["running"] != inventory.Running || counts["created"] != inventory.Created || counts["stopped"] != inventory.Stopped || counts["uncertain"] != inventory.Uncertain || inventory.RecoveryRequired != (inventory.Uncertain > 0) {
		return empty, errors.New("inconsistent observed inventory")
	}
	r := RuntimeObservation{PlanSHA256: pin, Expected: plan.InstanceCount, ReadOnly: true, RecoveryRequired: inventory.RecoveryRequired, Instances: []InstanceObservation{}}
	expected := map[string]bool{}
	state := func(id string) string {
		x, ok := byID[id]
		if !ok {
			return "missing"
		}
		if x.PendingOperation != "" || !x.MetadataPresent || x.Status == "uncertain" {
			return "uncertain"
		}
		return x.Status
	}
	for _, wave := range plan.StartWaves {
		for _, i := range wave.Instances {
			expected[i.ID] = true
			o := InstanceObservation{ID: i.ID, Service: i.Service, Replica: i.Replica, State: state(i.ID), DependencyCount: len(i.Requires)}
			for _, id := range i.Requires {
				if state(id) == "running" {
					o.DependencyRunningCount++
				}
			}
			switch o.State {
			case "missing":
				r.Missing++
				o.Review = "missing_metadata_review"
			case "running":
				r.Running++
				o.Review = "running_identity_and_health_unverified"
			case "created":
				r.Created++
				o.Review = "created_operator_review"
			case "stopped":
				r.Stopped++
				o.Review = "stopped_operator_review"
			case "uncertain":
				r.Uncertain++
				o.Review = "recovery_review_required"
			}
			if o.DependencyRunningCount != o.DependencyCount && o.State != "uncertain" {
				o.Review = "dependency_state_review_required"
			}
			r.Instances = append(r.Instances, o)
		}
	}
	for id := range byID {
		if !expected[id] {
			r.UnmatchedRuntimeCount++
		}
	}
	return r, nil
}
