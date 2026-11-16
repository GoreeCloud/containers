package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/GoreeCloud/containers/internal/stack"
)

func stackPreflight(args []string, out io.Writer) error {
	if len(args) < 3 {
		return errors.New("usage: stack preflight OLD_JSON NEW_JSON --expected-current-sha256 sha256:HEX --cpu-milli N --memory-mib N --max-instances N")
	}
	flags := flag.NewFlagSet("stack preflight", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expected-current-sha256", "", "expected canonical prior declaration fingerprint")
	cpu := flags.Int64("cpu-milli", 0, "user-supplied CPU millicores")
	mem := flags.Int64("memory-mib", 0, "user-supplied memory in MiB")
	instances := flags.Int("max-instances", 0, "user-supplied maximum instance count")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *mem <= 0 || *mem > math.MaxInt64/(1<<20) {
		return errors.New("invalid or extra native preflight budget flags")
	}
	before, err := stack.Load(args[0])
	if err != nil {
		return err
	}
	after, err := stack.Load(args[1])
	if err != nil {
		return err
	}
	report, err := stack.ReviewPreflight(before, after, *expected, stack.ResourceBudget{
		CPUMilli: *cpu, MemoryBytes: *mem * (1 << 20), MaxInstances: *instances,
	})
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintln(out, string(encoded)); err != nil {
		return err
	}
	if !report.SourceAndBudgetChecksPassed || report.SensitivePolicyReviewRequired {
		return errors.New("native preflight source, budget or MIPS sensitive-policy review unresolved")
	}
	return nil
}
