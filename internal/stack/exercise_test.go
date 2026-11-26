package stack

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"strings"
	"sync"
	"testing"
	"time"
)

func exerciseSpec(t *testing.T) (Spec, InstancePlan) {
	t.Helper()
	db := minimalService("db")
	db["replicas"] = 2
	api := minimalService("api")
	api["dependsOn"] = []string{"db"}
	s, e := Parse(fixture(db, api))
	if e != nil {
		t.Fatal(e)
	}
	p, e := PlanInstances(s)
	if e != nil {
		t.Fatal(e)
	}
	return s, p
}
func TestExerciseAdmissionPins(t *testing.T) {
	s, p := exerciseSpec(t)
	a := ExerciseAdmission{PlanSHA256: p.PlanSHA256}
	for _, w := range p.StartWaves {
		for _, i := range w.Instances {
			a.Instances = append(a.Instances, ExerciseInput{ID: i.ID, ManifestSize: 1024, ConfigFile: "/private/config.json", ConfigSHA256: "sha256:" + strings.Repeat("b", 64), EntrypointSHA256: "sha256:" + strings.Repeat("c", 64)})
		}
	}
	raw, _ := json.Marshal(a)
	if _, q, e := parseExercise(s, raw, digestBytes(raw)); e != nil || q.InstanceCount != 3 {
		t.Fatal(q, e)
	}
	if _, _, e := parseExercise(s, raw, "sha256:"+strings.Repeat("0", 64)); e == nil {
		t.Fatal("wrong pin")
	}
	a.Instances[1].ID = a.Instances[0].ID
	raw, _ = json.Marshal(a)
	if _, _, e := parseExercise(s, raw, digestBytes(raw)); e == nil {
		t.Fatal("duplicate instance")
	}
	ids, e := sessionRuntimeIDs(p)
	if e != nil {
		t.Fatal(e)
	}
	other, e := sessionRuntimeIDs(p)
	if e != nil {
		t.Fatal(e)
	}
	for id, r := range ids {
		if r == id || r == other[id] || engine.ValidateID(r) != nil {
			t.Fatal("runtime ID collision")
		}
	}
}

type fakeExercise struct {
	mu          sync.Mutex
	state       map[string]string
	fail        string
	events      []string
	active, max int
}

func (f *fakeExercise) CreateDelegated(ctx context.Context, id, dir string) error {
	f.mu.Lock()
	f.active++
	if f.active > f.max {
		f.max = f.active
	}
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(20 * time.Millisecond):
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active--
	f.events = append(f.events, "create:"+id)
	if id == f.fail {
		return errors.New("synthetic uncertainty")
	}
	f.state[id] = "created"
	return nil
}
func (f *fakeExercise) transition(id, from, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state[id] != from {
		return errors.New("bad transition")
	}
	f.events = append(f.events, to+":"+id)
	if to == "deleted" {
		delete(f.state, id)
	} else {
		f.state[id] = to
	}
	return nil
}
func (f *fakeExercise) Start(_ context.Context, id string) error {
	return f.transition(id, "created", "running")
}
func (f *fakeExercise) Stop(_ context.Context, id string) error {
	return f.transition(id, "running", "stopped")
}
func (f *fakeExercise) Delete(_ context.Context, id string) error {
	return f.transition(id, "stopped", "deleted")
}
func (f *fakeExercise) State(_ context.Context, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return json.Marshal(engine.RuntimeState{ID: id, Status: f.state[id], PID: 10})
}
func TestExerciseWavesAndFailure(t *testing.T) {
	_, p := exerciseSpec(t)
	dirs := map[string]string{}
	for _, w := range p.StartWaves {
		for _, i := range w.Instances {
			dirs[i.ID] = "/private"
		}
	}
	for _, fail := range []string{"", p.StartWaves[0].Instances[0].ID} {
		f := &fakeExercise{state: map[string]string{}, fail: fail}
		r := ExerciseReport{}
		events := []exerciseEvent{}
		e := executeWaves(context.Background(), f, p, dirs, func(x exerciseEvent) error { events = append(events, x); return nil }, &r)
		if f.max != 2 {
			t.Fatal("wave not concurrent")
		}
		if fail != "" {
			if e == nil || len(events) != 1 || r.CompletedWaves != 0 || !r.RecoveryReviewRequired || len(f.state) != 1 {
				t.Fatal(e, r, events)
			}
			continue
		}
		if e != nil || !r.AllRunningObserved || r.CompletedWaves != 2 || r.CompletedStopWaves != 2 || len(f.state) != 0 {
			t.Fatal(e, r)
		}
		if events[9].Phase != "stop-intent" || events[9].IDs[0] != p.StartWaves[1].Instances[0].ID {
			t.Fatal(events)
		}
	}
}
