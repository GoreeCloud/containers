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
