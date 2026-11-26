package localapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"github.com/GoreeCloud/containers/internal/jsonkey"
	"io"
)

type healthResponse struct {
	APIVersion                  string `json:"apiVersion"`
	ReadOnly                    bool   `json:"readOnly"`
	IdentityIntegrationVerified bool   `json:"identityIntegrationVerified"`
	DeploymentAuthorized        bool   `json:"deploymentAuthorized"`
}

type runtimeSummary struct {
	ReadOnly                    bool `json:"readOnly"`
	Total                       int  `json:"total"`
	Running                     int  `json:"running"`
	Created                     int  `json:"created"`
	Stopped                     int  `json:"stopped"`
	Uncertain                   int  `json:"uncertain"`
	RecoveryRequired            bool `json:"recoveryRequired"`
	AtomicSnapshot              bool `json:"atomicSnapshot"`
	IdentityIntegrationVerified bool `json:"identityIntegrationVerified"`
	DeploymentAuthorized        bool `json:"deploymentAuthorized"`
}

func validateResponse(data []byte, endpoint string, status int) (json.RawMessage, error) {
	// Reject duplicate, case-folded and unknown keys, missing fields and
	// non-scalar values before typed decoding. Never echo raw peer JSON.
	d := json.NewDecoder(bytes.NewReader(data))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return nil, errors.New("invalid API response object")
	}
	seen := map[string]bool{}
	exact := map[string]bool{}
	for d.More() {
		k, e := d.Token()
		s, ok := k.(string)
		if e != nil || !ok || seen[jsonkey.Fold(s)] {
			return nil, errors.New("ambiguous API response")
		}
		seen[jsonkey.Fold(s)] = true
		exact[s] = true
		v, e := d.Token()
		if e != nil {
			return nil, e
		}
		switch v.(type) {
		case string, bool, float64:
		default:
			return nil, errors.New("invalid API field type")
		}
	}
	if t, e := d.Token(); e != nil || t != json.Delim('}') {
		return nil, errors.New("invalid API response ending")
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errors.New("trailing API data")
	}
	decode := func(v any, keys ...string) error {
		if len(keys) != len(exact) {
			return errors.New("incomplete API response")
		}
		for _, k := range keys {
			if !exact[k] {
				return errors.New("unexpected API response field")
			}
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		return dec.Decode(v)
	}
	if endpoint == "health" {
		var r healthResponse
		if e = decode(&r, "apiVersion", "readOnly", "identityIntegrationVerified", "deploymentAuthorized"); e != nil {
			return nil, e
		}
		if status != 200 || r.APIVersion != "v1" || !r.ReadOnly || r.IdentityIntegrationVerified || r.DeploymentAuthorized {
			return nil, errors.New("unexpected local API health policy")
		}
		return json.Marshal(r)
	}
	if endpoint == "runtime-summary" {
		var r runtimeSummary
		if e = decode(&r, "readOnly", "total", "running", "created", "stopped", "uncertain", "recoveryRequired", "atomicSnapshot", "identityIntegrationVerified", "deploymentAuthorized"); e != nil {
			return nil, e
		}
		if !r.ReadOnly || r.AtomicSnapshot || r.IdentityIntegrationVerified || r.DeploymentAuthorized || r.Total < 0 || r.Total > 128 || r.RecoveryRequired != (r.Uncertain > 0) || r.RecoveryRequired != (status == 409) {
			return nil, errors.New("invalid runtime summary policy")
		}
		total := 0
		for _, n := range []int{r.Running, r.Created, r.Stopped, r.Uncertain} {
			if n < 0 || n > r.Total {
				return nil, errors.New("invalid runtime summary count")
			}
			total += n
		}
		if total != r.Total {
			return nil, errors.New("inconsistent runtime counters")
		}
		return json.Marshal(r)
	}
	if endpoint != "journal" {
		return nil, errors.New("unsupported response endpoint")
	}
	var r engine.JournalCensus
	if e = decode(&r, "readOnly", "pendingCount", "pendingCreate", "pendingStart", "pendingStop", "pendingDelete", "recoveryRequired", "runtimeStateVerified"); e != nil {
		return nil, e
	}
	if !r.ReadOnly || r.RuntimeStateVerified || r.PendingCount < 0 || r.PendingCount > 1024 || r.RecoveryRequired != (r.PendingCount > 0) || r.RecoveryRequired != (status == 409) {
		return nil, errors.New("invalid journal census policy")
	}
	n := 0
	for _, c := range []int{r.PendingCreate, r.PendingStart, r.PendingStop, r.PendingDelete} {
		if c < 0 || c > r.PendingCount {
			return nil, errors.New("invalid journal counter")
		}
		n += c
	}
	if n != r.PendingCount {
		return nil, errors.New("inconsistent journal counters")
	}
	return json.Marshal(r)
}
