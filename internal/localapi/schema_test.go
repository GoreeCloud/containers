package localapi

import (
	"strings"
	"testing"
)

func TestResponseSchemaRejectsDisclosureAndMisleadingStatus(t *testing.T) {
	base := `{"apiVersion":"v1","readOnly":true,"identityIntegrationVerified":false,"deploymentAuthorized":false}`
	if _, e := validateResponse([]byte(base), "health", 200); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Replace(base, "false}", "true}", 1), strings.Replace(base, "}", `,"secret":"synthetic"}`, 1), strings.Replace(base, "}", `,"ReadOnly":false}`, 1), `{"apiVersion":"v1","readOnly":true}`, `null`} {
		if _, e := validateResponse([]byte(bad), "health", 200); e == nil {
			t.Fatal("invalid health response accepted")
		}
	}
	census := `{"readOnly":true,"pendingCount":1,"pendingCreate":1,"pendingStart":0,"pendingStop":0,"pendingDelete":0,"recoveryRequired":true,"runtimeStateVerified":false}`
	if _, e := validateResponse([]byte(census), "journal", 409); e != nil {
		t.Fatal(e)
	}
	if _, e := validateResponse([]byte(census), "journal", 200); e == nil {
		t.Fatal("conflict hidden")
	}
	if _, e := validateResponse([]byte(strings.Replace(census, `"pendingCreate":1`, `"pendingCreate":0`, 1)), "journal", 409); e == nil {
		t.Fatal("invalid census accepted")
	}
}
