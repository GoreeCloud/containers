package stack

import "testing"

func TestRejectAmbiguousJSONMemberAliases(t *testing.T) {
	for _, input := range []string{`{"kind":1,"KIND":2}`, `{"nested":{"kind":1,"Kind":2}}`, `{"status":1,"ſtatus":2}`, `{"key":1,"\u006bey":2}`} {
		if err := rejectDuplicate([]byte(input)); err == nil {
			t.Fatalf("accepted ambiguous object: %s", input)
		}
	}
	if err := rejectDuplicate([]byte(`{"kind":1,"nested":{"kind":2},"other":3}`)); err != nil {
		t.Fatalf("rejected distinct object members: %v", err)
	}
}
