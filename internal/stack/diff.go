package stack

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
)

// ChangeKind distinguishes the intent of a proposed change from the ability to
// apply it. This package NEVER executes a plan.
type ChangeKind string

const (
	ChangeAdd     ChangeKind = "would-add"
	ChangeReplace ChangeKind = "would-replace-manual-review"
	ChangeRemove  ChangeKind = "would-remove-manual-review"
)

type ChangeStep struct {
	Service        string     `json:"service"`
	Action         ChangeKind `json:"action"`
	ImageDigest    string     `json:"imageDigest"`
	ReviewRequired bool       `json:"reviewRequired"`
}
type ChangePlan struct {
	StackName            string       `json:"stackName"`
	FromSpecSHA256       string       `json:"fromSpecSHA256"`
	ToSpecSHA256         string       `json:"toSpecSHA256"`
	DryRun               bool         `json:"dryRun"`
	ManualReviewRequired bool         `json:"manualReviewRequired"`
	Changes              []ChangeStep `json:"changes"`
	RollbackHint         string       `json:"rollbackHint"`
}

// Diff computes a deterministic, side-effect-free transition proposal. New
// services are ordered by target dependencies. Removed services are ordered in
// reverse old dependency order. Replacements and removals are never approved
// by this function.
func Diff(before, after Spec) (ChangePlan, error) {
	if err := Validate(before); err != nil {
		return ChangePlan{}, err
	}
	if err := Validate(after); err != nil {
		return ChangePlan{}, err
	}
	if before.Metadata.Name != after.Metadata.Name {
		return ChangePlan{}, errors.New("stack rename is not an implicit migration")
	}
	from, err := canonicalFingerprint(before)
	if err != nil {
		return ChangePlan{}, err
	}
	to, err := canonicalFingerprint(after)
	if err != nil {
		return ChangePlan{}, err
	}
	current := map[string]Service{}
	target := map[string]Service{}
	for _, service := range before.Services {
		current[service.Name] = service
	}
	for _, service := range after.Services {
		target[service.Name] = service
	}
	bOrder, err := order(current)
	if err != nil {
		return ChangePlan{}, err
	}
	aOrder, err := order(target)
	if err != nil {
		return ChangePlan{}, err
	}
	result := ChangePlan{StackName: before.Metadata.Name, FromSpecSHA256: from, ToSpecSHA256: to, DryRun: true,
		Changes: make([]ChangeStep, 0), RollbackHint: "No workload changed. A future executor requires checkpointed data, per-operation authorization and a verified rollback rehearsal."}
	for _, name := range aOrder {
		future := target[name]
		previous, exists := current[name]
		if !exists {
			result.Changes = append(result.Changes, ChangeStep{Service: name, Action: ChangeAdd, ImageDigest: future.Image, ReviewRequired: true})
			result.ManualReviewRequired = true
			continue
		}
		if equivalentService(previous, future) {
			continue
		}
		result.Changes = append(result.Changes, ChangeStep{Service: name, Action: ChangeReplace, ImageDigest: future.Image, ReviewRequired: true})
		result.ManualReviewRequired = true
	}
	// Decommission dependents first, never remove an old dependency while
	// a dependent still references it.
	for i := len(bOrder) - 1; i >= 0; i-- {
		name := bOrder[i]
		if _, exists := target[name]; exists {
			continue
		}
		result.Changes = append(result.Changes, ChangeStep{Service: name, Action: ChangeRemove, ImageDigest: current[name].Image, ReviewRequired: true})
		result.ManualReviewRequired = true
	}
	return result, nil
}
func equivalentService(left, right Service) bool {
	left.DependsOn = append([]string(nil), left.DependsOn...)
	right.DependsOn = append([]string(nil), right.DependsOn...)
	left.SecretRefs = append([]string(nil), left.SecretRefs...)
	right.SecretRefs = append([]string(nil), right.SecretRefs...)
	sort.Strings(left.DependsOn)
	sort.Strings(right.DependsOn)
	sort.Strings(left.SecretRefs)
	sort.Strings(right.SecretRefs)
	// JSON comparison also checks newly introduced fields after future schema
	// updates. Both declarations have already passed strict schema validation.
	l, err1 := json.Marshal(left)
	r, err2 := json.Marshal(right)
	return err1 == nil && err2 == nil && reflect.DeepEqual(l, r)
}
