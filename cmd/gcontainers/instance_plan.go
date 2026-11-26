package main

import (
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/stack"
	"io"
)

func instancePlan(args []string, out io.Writer) error {
	if len(args) != 1 && len(args) != 3 {
		return errors.New("usage: stack instance-plan SPEC [--expected-plan-sha256 DIGEST]")
	}
	s, e := stack.Load(args[0])
	if e != nil {
		return e
	}
	var r stack.InstancePlan
	if len(args) == 3 {
		if args[1] != "--expected-plan-sha256" {
			return errors.New("unexpected instance plan flag")
		}
		r, e = stack.ReviewInstancePlan(s, args[2])
	} else {
		r, e = stack.PlanInstances(s)
	}
	if e != nil {
		return e
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
