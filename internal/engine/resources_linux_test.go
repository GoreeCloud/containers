package engine

import "testing"

func TestResourceCgroupPath(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{"0::/user.slice/worker\n", true}, {"0::/worker\n", false}, {"0::/user.slice/other\n", false}, {"0::/user.slice/../worker\n", false}, {"0::/user.slice/worker\n1:cpu:/worker", false}, {"0::/\n", false},
	} {
		_, err := resourceCgroupPath(tc.raw, "worker")
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%t %v", tc.valid, err)
		}
	}
}
