package stack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func minimalService(name string) map[string]any {
	return map[string]any{
		"name":      name,
		"image":     "sha256:" + strings.Repeat("a", 64),
		"replicas":  1,
		"resources": map[string]any{"cpuMilli": 250, "memoryBytes": 33554432},
		"security":  map[string]any{"runAsNonRoot": true, "readOnlyRootFilesystem": true, "network": "isolated"},
	}
}
func fixture(services ...map[string]any) []byte {
	stack := map[string]any{"apiVersion": APIVersion, "kind": Kind, "metadata": map[string]any{"name": "notes"}, "services": services}
	b, _ := json.Marshal(stack)
	return b
}
func TestNativePlanSortedAndNoExecution(t *testing.T) {
	api := minimalService("api")
	api["dependsOn"] = []string{"db"}
	db := minimalService("db")
	s, err := Parse(fixture(api, db))
	if err != nil {
		t.Fatal(err)
	}
	p, err := DryRun(s)
	if err != nil {
		t.Fatal(err)
	}
	if !p.DryRun || len(p.Steps) != 2 || p.Steps[0].Service != "db" || p.Steps[1].Service != "api" ||
		p.TotalCPUMilli != 500 || p.TotalMemoryBytes != 67108864 {
		t.Fatalf("incorrect safe plan: %+v", p)
	}
}
func TestRejectUnsafeOrInvalidNativeStack(t *testing.T) {
	tests := map[string][]byte{}
	tests["unknown field"] = []byte(strings.Replace(string(fixture(minimalService("a"))), "\"replicas\":1", "\"privileged\":true,\"replicas\":1", 1))
	tests["duplicate key"] = []byte(strings.Replace(string(fixture(minimalService("a"))), "\"kind\":\"Stack\"", "\"kind\":\"Stack\",\"kind\":\"Stack\"", 1))
	bad := minimalService("a")
	bad["image"] = "busybox:latest"
	tests["mutable image"] = fixture(bad)
	bad = minimalService("a")
	bad["security"] = map[string]any{"runAsNonRoot": false, "readOnlyRootFilesystem": true, "network": "isolated"}
	tests["root privilege"] = fixture(bad)
	bad = minimalService("a")
	bad["ports"] = []int{8080}
	tests["undeclared port exposure"] = fixture(bad)
	bad = minimalService("a")
	bad["secretRefs"] = []string{"APP_PASSWORD=password"}
	tests["inline secret"] = fixture(bad)
	bad = minimalService("a")
	bad["resources"] = map[string]any{"cpuMilli": 0, "memoryBytes": 1}
	tests["resource limits"] = fixture(bad)
	a := minimalService("a")
	b := minimalService("b")
	a["dependsOn"] = []string{"b"}
	b["dependsOn"] = []string{"a"}
	tests["dependency cycle"] = fixture(a, b)
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(body); err == nil {
				t.Fatal("unsafe stack accepted")
			}
		})
	}
}
func TestDocumentBoundsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "stack.json")
	if err := os.WriteFile(file, fixture(minimalService("api")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Repeat(" ", int(MaxBytes)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err == nil {
		t.Fatal("oversized stack accepted")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", file); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err == nil {
		t.Fatal("symlink stack accepted")
	}
}

func FuzzNativeStackParser(f *testing.F) {
	f.Add(fixture(minimalService("web")))
	f.Add([]byte("{\"apiVersion\":0,\"kind\":\"Stack\"}"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8192 {
			t.Skip()
		}
		_, _ = Parse(b)
	})
}

func TestDeterministicParallelStagesAndFingerprint(t *testing.T) {
	db := minimalService("db")
	cache := minimalService("cache")
	web := minimalService("web")
	worker := minimalService("worker")
	web["dependsOn"] = []string{"db", "cache"}
	worker["dependsOn"] = []string{"cache"}
	raw1 := fixture(web, db, cache, worker)
	raw2 := fixture(worker, cache, db, web)
	s1, err := Parse(raw1)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Parse(raw2)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := DryRun(s1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := DryRun(s2)
	if err != nil {
		t.Fatal(err)
	}
	if p1.SpecSHA256 != p2.SpecSHA256 || !strings.HasPrefix(p1.SpecSHA256, "sha256:") {
		t.Fatalf("fingerprint should ignore order: %s vs %s", p1.SpecSHA256, p2.SpecSHA256)
	}
	if len(p1.Stages) != 2 || p1.Stages[0].Number != 0 ||
		len(p1.Stages[0].IndependentServices) != 2 || p1.Stages[1].Number != 1 ||
		len(p1.Stages[1].IndependentServices) != 2 {
		t.Fatalf("unexpected parallel stage plan: %+v", p1.Stages)
	}
	if p1.Steps[0].Service != "cache" || p1.Steps[1].Service != "db" ||
		p1.Steps[2].Service != "web" || p1.Steps[3].Service != "worker" {
		t.Fatalf("unexpected ordered plan steps: %+v", p1.Steps)
	}
	for _, step := range p1.Steps {
		if step.Service == "web" && step.Stage != 1 || step.Service == "db" && step.Stage != 0 {
			t.Fatalf("unsafe dependency stages: %+v", p1.Steps)
		}
	}
	if p1.DryRun != true {
		t.Fatal("dry run flag must be true")
	}
}
func TestChangedStackChangesFingerprint(t *testing.T) {
	a := minimalService("web")
	one, err := Parse(fixture(a))
	if err != nil {
		t.Fatal(err)
	}
	plan1, err := DryRun(one)
	if err != nil {
		t.Fatal(err)
	}
	a["replicas"] = 2
	two, err := Parse(fixture(a))
	if err != nil {
		t.Fatal(err)
	}
	plan2, err := DryRun(two)
	if err != nil {
		t.Fatal(err)
	}
	if plan1.SpecSHA256 == plan2.SpecSHA256 {
		t.Fatal("changed workload spec not reflected in plan fingerprint")
	}
}
