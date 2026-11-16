package engine

import "testing"

func TestRuntimeSecurityBaselines(t *testing.T) {
	cases := []struct {
		backend  string
		output   string
		accepted bool
	}{
		{"crun", "crun version 1.14.1\n", false},
		{"crun", "crun version 1.29.1\n", false},
		{"crun", "crun version 1.30.0\n", false},
		{"crun", "crun version 1.30.1\ncommit: opaque\n", true},
		{"crun", "crun version 1.31.0\n", true},
		{"runc", "runc version 1.5.2\n", false},
		{"runc", "runc version 1.5.3\n", true},
		{"runc", "runc version v1.5.3\n", true},
		{"runc", "runc version 1.6.0-rc.2\n", false},
		{"runc", "runc version 1.6.0\n", true},
		{"runc", "crun version 1.30.1\n", false},
		{"other", "other version 99.99.99\n", false},
		{"crun", "crun version 01.30.1\n", false},
		{"crun", "unexpected output\n", false},
		{"crun", "crun version 1.30.1+unverified\n", false},
		{"crun", "crun version 1.30.1-rc1\n", false},
		{"crun", "crun version 1.30.1\ncrun version 0.0.0\n", true},
	}
	for _, tc := range cases {
		err := CheckVersionOutput(tc.backend, tc.output)
		if (err == nil) != tc.accepted {
			t.Errorf("backend=%s output=%q accepted=%t err=%v", tc.backend, tc.output, tc.accepted, err)
		}
	}
}
func TestInvalidVersionTriple(t *testing.T) {
	for _, input := range []string{"", "1", "1.2", "1.2.3.4", "-1.2.3", "1.a.3", "1.2.0-rc", "1.02.3", "1.2.18446744073709551616"} {
		if _, err := versionTriple(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
