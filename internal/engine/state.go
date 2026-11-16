package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RuntimeState contains only the minimal lifecycle fields the GoreeCloud
// preview needs. Raw OCI annotations and bundle paths must not be echoed.
type RuntimeState struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	PID    int64  `json:"pid"`
}

// DecodeRuntimeState rejects ambiguous, unbounded, or unrecognized OCI states.
// The low-level runtime is treated as an external data source, not an authority
// allowed to change GoreeCloud policy or emit arbitrary JSON to the user.
func DecodeRuntimeState(raw []byte, expectedID string) (RuntimeState, error) {
	var empty RuntimeState
	if err := ValidateID(expectedID); err != nil {
		return empty, err
	}
	if len(raw) == 0 || len(raw) > maxRuntimeOutput {
		return empty, errors.New("invalid OCI runtime state response size")
	}
	if err := uniqueRuntimeJSON(raw); err != nil {
		return empty, err
	}
	var state struct {
		OCIVersion  string            `json:"ociVersion"`
		ID          string            `json:"id"`
		Status      string            `json:"status"`
		PID         int64             `json:"pid"`
		Bundle      string            `json:"bundle"`
		Annotations map[string]string `json:"annotations"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&state); err != nil {
		return empty, errors.New("malformed or unsupported OCI runtime state")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return empty, errors.New("trailing OCI runtime state data")
	}
	if state.ID != expectedID {
		return empty, errors.New("OCI runtime returned a different container ID")
	}
	switch state.Status {
	case "created", "running", "stopped":
	default:
		return empty, errors.New("unknown or transitional OCI runtime state")
	}
	if state.PID < 0 || (state.Status == "running" && state.PID == 0) {
		return empty, errors.New("invalid OCI runtime process identifier")
	}
	return RuntimeState{ID: state.ID, Status: state.Status, PID: state.PID}, nil
}
func uniqueRuntimeJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := visitRuntimeJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing OCI JSON token")
	}
	return nil
}
func visitRuntimeJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("OCI runtime state JSON nested too deeply")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	var close json.Delim
	switch delimiter {
	case '{':
		close = '}'
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return errors.New("duplicate OCI runtime JSON key")
			}
			seen[s] = true
			if err := visitRuntimeJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		close = ']'
		for d.More() {
			if err := visitRuntimeJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid OCI runtime JSON delimiter")
	}
	finish, err := d.Token()
	if err != nil || finish != close {
		return fmt.Errorf("unbalanced OCI runtime JSON delimiter")
	}
	return nil
}
func (e *Engine) readState(ctx context.Context, id string) (RuntimeState, error) {
	var empty RuntimeState
	if err := ValidateID(id); err != nil {
		return empty, err
	}
	raw, err := e.call(ctx, "state", id)
	if err != nil {
		return empty, err
	}
	return DecodeRuntimeState(raw, id)
}
