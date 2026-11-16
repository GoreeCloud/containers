// Package stack validates a first-party GoreeCloud stack declaration.
// It produces a read-only dependency plan and never starts or installs workloads.
package stack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
)

const MaxBytes int64 = 1 << 20
const APIVersion = "goreecloud.containers/v1alpha1"
const Kind = "Stack"

var namePattern = regexp.MustCompile("^[a-z][a-z0-9-]{0,30}$")
var imagePattern = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

type Spec struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Services []Service `json:"services"`
}
type Service struct {
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	Replicas   int      `json:"replicas"`
	DependsOn  []string `json:"dependsOn"`
	SecretRefs []string `json:"secretRefs"`
	Resources  struct {
		CPUMilli    int64 `json:"cpuMilli"`
		MemoryBytes int64 `json:"memoryBytes"`
	} `json:"resources"`
	Security struct {
		RunAsNonRoot           bool   `json:"runAsNonRoot"`
		ReadOnlyRootFilesystem bool   `json:"readOnlyRootFilesystem"`
		Network                string `json:"network"`
	} `json:"security"`
}
type Step struct {
	Service     string `json:"service"`
	ImageDigest string `json:"imageDigest"`
	Replicas    int    `json:"replicas"`
	Operation   string `json:"operation"`
}
type Plan struct {
	StackName        string `json:"stackName"`
	DryRun           bool   `json:"dryRun"`
	Steps            []Step `json:"steps"`
	TotalCPUMilli    int64  `json:"totalCpuMilli"`
	TotalMemoryBytes int64  `json:"totalMemoryBytes"`
}

func Load(path string) (Spec, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return Spec{}, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > MaxBytes {
		return Spec{}, errors.New("stack file must be a regular file no larger than 1 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return Spec{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return Spec{}, errors.New("unable to read stack declaration")
	}
	return Parse(b)
}
func Parse(b []byte) (Spec, error) {
	var spec Spec
	if len(b) == 0 || int64(len(b)) > MaxBytes {
		return spec, errors.New("invalid stack document size")
	}
	if err := rejectDuplicate(b); err != nil {
		return spec, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&spec); err != nil {
		return Spec{}, errors.New("invalid or unsupported stack declaration")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Spec{}, errors.New("trailing stack JSON content")
	}
	if err := Validate(spec); err != nil {
		return Spec{}, err
	}
	return spec, nil
}
func Validate(s Spec) error {
	if s.APIVersion != APIVersion || s.Kind != Kind || !namePattern.MatchString(s.Metadata.Name) {
		return errors.New("invalid stack version, kind or name")
	}
	if len(s.Services) == 0 || len(s.Services) > 32 {
		return errors.New("stack requires 1 to 32 services")
	}
	serviceByName := map[string]Service{}
	for _, service := range s.Services {
		if !namePattern.MatchString(service.Name) || !imagePattern.MatchString(service.Image) || serviceByName[service.Name].Name != "" {
			return errors.New("duplicate or invalid service name or image digest")
		}
		if service.Replicas < 1 || service.Replicas > 8 ||
			service.Resources.CPUMilli < 100 || service.Resources.CPUMilli > 8000 ||
			service.Resources.MemoryBytes < (16<<20) || service.Resources.MemoryBytes > (8<<30) {
			return errors.New("service replicas or resource budget out of preview range")
		}
		if !service.Security.RunAsNonRoot || !service.Security.ReadOnlyRootFilesystem || service.Security.Network != "isolated" {
			return errors.New("service requires nonroot, readonly filesystem and isolated network")
		}
		if len(service.SecretRefs) > 32 || len(service.DependsOn) > 32 {
			return errors.New("too many service references")
		}
		seenSecrets := map[string]bool{}
		for _, secret := range service.SecretRefs {
			if !namePattern.MatchString(secret) || seenSecrets[secret] {
				return errors.New("invalid or duplicate secret reference")
			}
			seenSecrets[secret] = true
		}
		serviceByName[service.Name] = service
	}
	for _, service := range s.Services {
		seen := map[string]bool{}
		for _, dep := range service.DependsOn {
			if _, found := serviceByName[dep]; !found || dep == service.Name || seen[dep] {
				return errors.New("invalid or duplicate dependency reference")
			}
			seen[dep] = true
		}
	}
	_, err := order(serviceByName)
	return err
}
func order(serviceByName map[string]Service) ([]string, error) {
	visited := map[string]int{}
	names := make([]string, 0, len(serviceByName))
	for name := range serviceByName {
		names = append(names, name)
	}
	sort.Strings(names)
	ordered := []string{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] == 1 {
			return fmt.Errorf("service dependency cycle")
		}
		if visited[name] == 2 {
			return nil
		}
		visited[name] = 1
		deps := append([]string(nil), serviceByName[name].DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[name] = 2
		ordered = append(ordered, name)
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
func DryRun(s Spec) (Plan, error) {
	if err := Validate(s); err != nil {
		return Plan{}, err
	}
	serviceByName := map[string]Service{}
	for _, svc := range s.Services {
		serviceByName[svc.Name] = svc
	}
	ordered, _ := order(serviceByName)
	plan := Plan{StackName: s.Metadata.Name, DryRun: true, Steps: make([]Step, 0, len(ordered))}
	for _, name := range ordered {
		svc := serviceByName[name]
		plan.Steps = append(plan.Steps, Step{Service: name, ImageDigest: svc.Image, Replicas: svc.Replicas, Operation: "would-deploy"})
		plan.TotalCPUMilli += svc.Resources.CPUMilli * int64(svc.Replicas)
		plan.TotalMemoryBytes += svc.Resources.MemoryBytes * int64(svc.Replicas)
	}
	return plan, nil
}
func rejectDuplicate(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if err := walk(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func walk(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting exceeds preview maximum")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := k.(string)
			if !ok || keys[name] {
				return errors.New("duplicate or invalid JSON key")
			}
			keys[name] = true
			if err := walk(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := walk(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}
