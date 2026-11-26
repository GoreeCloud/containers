package localapi

import (
	"context"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type summaryRunner struct{}

func (summaryRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if args[2] != "state" || args[3] != "private-worker" {
		return nil, errors.New("unexpected read")
	}
	return []byte(`{"id":"private-worker","status":"running","pid":1234,"bundle":"/private/synthetic"}`), nil
}
func TestRuntimeSummaryMinimized(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot")
	}
	base := t.TempDir()
	os.Chmod(base, 0700)
	state, e := engine.PrepareStateRoot(base)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(state, "private-worker"), 0700); e != nil {
		t.Fatal(e)
	}
	r, e := engine.New("/usr/bin/runc", state, summaryRunner{})
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("GET", "/v1/runtime-summary", nil)
	w := httptest.NewRecorder()
	Handler(state).ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	handler(state, r).ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-worker") || strings.Contains(w.Body.String(), "1234") || strings.Contains(w.Body.String(), "/private") {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e = validateResponse(w.Body.Bytes(), "runtime-summary", 200); e != nil {
		t.Fatal(e)
	}
	if _, e = validateResponse([]byte(strings.Replace(w.Body.String(), `"total":1`, `"total":2`, 1)), "runtime-summary", 200); e == nil {
		t.Fatal("inconsistent count accepted")
	}
	if _, e = validateResponse([]byte(strings.Replace(w.Body.String(), `"atomicSnapshot":false`, `"atomicSnapshot":true`, 1)), "runtime-summary", 200); e == nil {
		t.Fatal("atomic claim accepted")
	}
	_, e = engine.RunJournaledMutation(context.Background(), state, "pending", "create", func() ([]byte, error) { return nil, errors.New("synthetic") })
	if e == nil {
		t.Fatal("expected pending")
	}
	w = httptest.NewRecorder()
	handler(state, r).ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e = validateResponse(w.Body.Bytes(), "runtime-summary", 409); e != nil {
		t.Fatal(e)
	}
}
