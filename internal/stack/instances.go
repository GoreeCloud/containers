package stack

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type Instance struct {
	ID          string   `json:"id"`
	Service     string   `json:"service"`
	Replica     int      `json:"replica"`
	ImageDigest string   `json:"imageDigest"`
	CPUMilli    int64    `json:"cpuMilli"`
	MemoryBytes int64    `json:"memoryBytes"`
	Requires    []string `json:"requires"`
}
type InstanceWave struct {
	Number    int        `json:"number"`
	Instances []Instance `json:"instances"`
}
type InstancePlan struct {
	SpecSHA256                string         `json:"specSHA256"`
	PlanSHA256                string         `json:"planSHA256"`
	InstanceCount             int            `json:"instanceCount"`
	StartWaves                []InstanceWave `json:"startWaves"`
	StopWaves                 [][]string     `json:"stopWaves"`
	SecretReferenceCount      int            `json:"secretReferenceCount"`
	DryRun                    bool           `json:"dryRun"`
	HealthVerified            bool           `json:"healthVerified"`
	DeploymentAuthorized      bool           `json:"deploymentAuthorized"`
	MIPSAuthorizationVerified bool           `json:"mipsAuthorizationVerified"`
}

func instanceID(stackName, service string, replica int) string {
	// Domain-separated ID includes all three components; fixed-length hash
	// avoids concatenation ambiguity and exceeds the preview's instance count.
	d := sha256.Sum256([]byte(fmt.Sprintf("goreecloud-instance-v1\x00%s\x00%s\x00%d", stackName, service, replica)))
	return fmt.Sprintf("gc-%x", d[:20])
}
func PlanInstances(s Spec) (InstancePlan, error) {
	var empty InstancePlan
	p, e := DryRun(s)
	if e != nil {
		return empty, e
	}
	byName := map[string]Service{}
	ids := map[string][]string{}
	seen := map[string]bool{}
	result := InstancePlan{SpecSHA256: p.SpecSHA256, DryRun: true, StartWaves: []InstanceWave{}, StopWaves: [][]string{}}
	for _, svc := range s.Services {
		byName[svc.Name] = svc
		result.SecretReferenceCount += len(svc.SecretRefs)
		for i := 1; i <= svc.Replicas; i++ {
			id := instanceID(s.Metadata.Name, svc.Name, i)
			if seen[id] {
				return empty, errors.New("instance ID collision")
			}
			seen[id] = true
			ids[svc.Name] = append(ids[svc.Name], id)
			result.InstanceCount++
		}
	}
	for _, stage := range p.Stages {
		wave := InstanceWave{Number: stage.Number, Instances: []Instance{}}
		for _, name := range stage.IndependentServices {
			svc := byName[name]
			deps := []string{}
			for _, dep := range svc.DependsOn {
				deps = append(deps, ids[dep]...)
			}
			sort.Strings(deps)
			for i, id := range ids[name] {
				wave.Instances = append(wave.Instances, Instance{ID: id, Service: name, Replica: i + 1, ImageDigest: svc.Image, CPUMilli: svc.Resources.CPUMilli, MemoryBytes: svc.Resources.MemoryBytes, Requires: append([]string{}, deps...)})
			}
		}
		result.StartWaves = append(result.StartWaves, wave)
	}
	for i := len(result.StartWaves) - 1; i >= 0; i-- {
		wave := []string{}
		for _, inst := range result.StartWaves[i].Instances {
			wave = append(wave, inst.ID)
		}
		result.StopWaves = append(result.StopWaves, wave)
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return empty, e
	}
	result.PlanSHA256 = fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	return result, nil
}

// ReviewInstancePlan recomputes the exact plan from the validated spec;
// a stale approval or edited proposal cannot be treated as the same plan.
func ReviewInstancePlan(s Spec, expected string) (InstancePlan, error) {
	p, e := PlanInstances(s)
	if e != nil {
		return InstancePlan{}, e
	}
	if expected != p.PlanSHA256 {
		return InstancePlan{}, errors.New("instance plan digest mismatch")
	}
	return p, nil
}
