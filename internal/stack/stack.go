// Package stack validates a first-party GoreeCloud stack declaration.
// It produces a read-only dependency plan and never starts or installs workloads.
package stack

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/jsonkey"
	"io"
	"os"
	"path/filepath"
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
	Stage       int    `json:"stage"`
	Operation   string `json:"operation"`
}
type Stage struct {
	Number              int      `json:"number"`
	IndependentServices []string `json:"independentServices"`
}
type Plan struct {
	StackName        string  `json:"stackName"`
	SpecSHA256       string  `json:"specSHA256"`
	DryRun           bool    `json:"dryRun"`
	Steps            []Step  `json:"steps"`
	Stages           []Stage `json:"stages"`
	TotalCPUMilli    int64   `json:"totalCpuMilli"`
	TotalMemoryBytes int64   `json:"totalMemoryBytes"`
}

// Load reads one immutable-in-memory snapshot of a local stack declaration.
// It rejects symlinked source files and parent directories and checks inode
// identity to reduce path replacement attacks. It never applies workloads.
func Load(source string) (Spec, error) {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return Spec{}, err
	}
	parent := filepath.Dir(absolute)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return Spec{}, errors.New("symlinked stack parent directory prohibited")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return Spec{}, err
	}
	defer root.Close()
	name := filepath.Base(absolute)
	info, err := root.Lstat(name)
	if err != nil {
		return Spec{}, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > MaxBytes {
		return Spec{}, errors.New("stack file must be non-symlink regular file no larger than 1 MiB")
	}
	f, err := root.Open(name)
	if err != nil {
		return Spec{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return Spec{}, errors.New("stack file changed during opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return Spec{}, errors.New("unable to read stack declaration")
	}
	if int64(len(b)) != info.Size() || int64(len(b)) > MaxBytes {
		return Spec{}, errors.New("stack declaration grew, shrank or exceeded quota during read")
	}
	if err := verifyStackInputIdentity(root, f, info, name); err != nil {
		return Spec{}, err
	}
	return Parse(b)
}

// verifyStackInputIdentity rechecks the source under its anchored directory
// after read so a same-length replace/symlink swap cannot be silently
// accepted as the document that was opened before parsing. An in-place
// same-user edit after this check still requires authenticated live admission.
func verifyStackInputIdentity(root *os.Root, file *os.File, initial os.FileInfo, name string) error {
	if root == nil || file == nil || initial == nil {
		return errors.New("stack source handle missing")
	}
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := root.Lstat(name)
	if err != nil || !os.SameFile(initial, opened) || !os.SameFile(opened, current) ||
		!opened.Mode().IsRegular() || !current.Mode().IsRegular() ||
		opened.Size() != initial.Size() || current.Size() != initial.Size() {
		return errors.New("stack source pathname or size changed during read")
	}
	return nil
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

// DryRun produces a deterministic, fingerprinted proposal with explicit
// parallel stages. It never contacts a daemon or modifies any workload.
func DryRun(s Spec) (Plan, error) {
	if err := Validate(s); err != nil {
		return Plan{}, err
	}
	byName := map[string]Service{}
	for _, svc := range s.Services {
		byName[svc.Name] = svc
	}
	ordered, _ := order(byName)
	stageByName := map[string]int{}
	stages := map[int][]string{}
	highest := 0
	for _, name := range ordered {
		stage := 0
		for _, dep := range byName[name].DependsOn {
			if candidate := stageByName[dep] + 1; candidate > stage {
				stage = candidate
			}
		}
		stageByName[name] = stage
		stages[stage] = append(stages[stage], name)
		if stage > highest {
			highest = stage
		}
	}
	fingerprint, err := canonicalFingerprint(s)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{StackName: s.Metadata.Name, SpecSHA256: fingerprint, DryRun: true,
		Steps: make([]Step, 0, len(ordered)), Stages: make([]Stage, 0, highest+1)}
	for i := 0; i <= highest; i++ {
		names := stages[i]
		sort.Strings(names)
		plan.Stages = append(plan.Stages, Stage{Number: i, IndependentServices: append([]string(nil), names...)})
		for _, name := range names {
			svc := byName[name]
			plan.Steps = append(plan.Steps, Step{
				Service: name, ImageDigest: svc.Image, Replicas: svc.Replicas, Stage: i, Operation: "would-deploy"})
			plan.TotalCPUMilli += svc.Resources.CPUMilli * int64(svc.Replicas)
			plan.TotalMemoryBytes += svc.Resources.MemoryBytes * int64(svc.Replicas)
		}
	}
	return plan, nil
}

// canonicalFingerprint excludes JSON formatting and original service/reference
// ordering. It is an audit identifier, NOT a signature, policy approval, or
// authorization to apply a plan.
func canonicalFingerprint(s Spec) (string, error) {
	canonical := s
	canonical.Services = append([]Service(nil), s.Services...)
	for i := range canonical.Services {
		svc := &canonical.Services[i]
		svc.DependsOn = append([]string(nil), svc.DependsOn...)
		svc.SecretRefs = append([]string(nil), svc.SecretRefs...)
		sort.Strings(svc.DependsOn)
		sort.Strings(svc.SecretRefs)
	}
	sort.Slice(canonical.Services, func(i, j int) bool {
		return canonical.Services[i].Name < canonical.Services[j].Name
	})
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
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
			if !ok || keys[jsonkey.Fold(name)] {
				return errors.New("duplicate or invalid JSON key")
			}
			keys[jsonkey.Fold(name)] = true
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
