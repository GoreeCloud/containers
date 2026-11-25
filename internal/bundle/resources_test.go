package bundle

import (
	"strings"
	"testing"
)

func TestRestrictedResourceProfile(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{`{"cpu":{"quota":50000,"period":100000},"memory":{"limit":67108864},"pids":{"limit":32}}`, true},
		{`{}`, false},
		{`{"cpu":{"quota":-1,"period":100000},"memory":{"limit":67108864},"pids":{"limit":32}}`, false},
		{`{"cpu":{"quota":50000,"period":100000},"memory":{"limit":-1},"pids":{"limit":32}}`, false},
		{`{"cpu":{"quota":50000,"period":100000},"memory":{"limit":67108864},"pids":{"limit":0}}`, false},
		{`{"cpu":{"quota":50000,"period":100000,"shares":100},"memory":{"limit":67108864},"pids":{"limit":32}}`, false},
		{`{"cpu":{"quota":50000,"period":100000},"memory":{"limit":67108864},"pids":{"limit":32},"devices":[]}`, false},
	} {
		raw := strings.Replace(safeSpec, `"linux":{`, `"linux":{"resources":`+tc.raw+`,`, 1)
		if err := Validate(setup(t, raw)); (err == nil) != tc.valid {
			t.Fatalf("valid=%t: %v", tc.valid, err)
		}
	}
}
