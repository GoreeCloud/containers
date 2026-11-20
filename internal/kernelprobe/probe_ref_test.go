package kernelprobe

import "testing"

// Namespace IDs must actually be kernel namespace references, not arbitrary
// strings that differ from each other. Malformed parent evidence must not
// produce a positive rootless namespace isolation finding.
func TestCompareRejectsFabricatedNamespaceReferences(t *testing.T) {
	before, after := map[string]string{}, map[string]string{}
	for _, kind := range names {
		before[kind] = kind + ":[123000]"
		after[kind] = kind + ":[123001]"
	}
	if !Compare(before, after, 0) {
		t.Fatal("valid distinct namespace inode references rejected")
	}
	cases := []struct{ label, kind, ref string }{
		{"arbitrary", "net", "untrusted-before-net"},
		{"wrong_kind", "net", "user:[123000]"},
		{"suffix", "net", "net:[123000]/fake"},
		{"empty_id", "net", "net:[]"},
		{"negative_id", "net", "net:[-10]"},
		{"zero_id", "net", "net:[0]"},
		{"overflow", "net", "net:[18446744073709551616000]"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			original := before[tc.kind]
			before[tc.kind] = tc.ref
			if Compare(before, after, 0) {
				t.Fatal("malformed parent evidence reported isolated")
			}
			before[tc.kind] = original
		})
	}
}
