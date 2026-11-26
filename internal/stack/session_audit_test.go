package stack

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditExactEventPrefix(t *testing.T) {
	_, p := exerciseSpec(t)
	events := expectedExerciseEvents(p)
	render := func(events []exerciseEvent) []byte {
		var lines []string
		for _, v := range events {
			b, _ := json.Marshal(v)
			lines = append(lines, string(b))
		}
		return []byte(strings.Join(lines, "\n") + "\n")
	}
	for n := 1; n <= len(events); n++ {
		phase, count, e := auditEvents(render(events[:n]), p)
		if e != nil || count != n || phase != events[n-1].Phase {
			t.Fatal(phase, count, e)
		}
	}
	persistent := append(append([]exerciseEvent{}, events...), exerciseEvent{Phase: "persistent-running-verified"})
	if phase, _, e := auditEvents(render(persistent), p); e != nil || phase != "persistent-running-verified" {
		t.Fatal(phase, e)
	}
	wrong := append([]exerciseEvent{}, events...)
	wrong[2].Phase = "running-observed"
	if _, _, e := auditEvents(render(wrong), p); e == nil {
		t.Fatal("skipped phase accepted")
	}
	raw := render(events)
	if _, _, e := auditEvents(raw[:len(raw)-1], p); e == nil {
		t.Fatal("partial record accepted")
	}
}
