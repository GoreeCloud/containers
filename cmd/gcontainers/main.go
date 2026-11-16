// gcontainers is the experimental GoreeCloud Containers command-line frontend.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/GoreeCloud/containers/internal/bundle"
	"github.com/GoreeCloud/containers/internal/engine"
	"github.com/GoreeCloud/containers/internal/hostreadiness"
	"github.com/GoreeCloud/containers/internal/stack"
)

const version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gcontainers:", err)
		os.Exit(1)
	}
}
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return usage(out)
	}
	switch args[0] {
	case "version", "--version", "-v":
		_, err := fmt.Fprintln(out, version)
		return err
	case "help", "--help", "-h":
		return usage(out)
	case "doctor":
		if len(args) != 1 {
			return errors.New("doctor takes no arguments")
		}
		return doctor(out)
	case "validate":
		if len(args) != 2 {
			return errors.New("usage: gcontainers validate BUNDLE_DIRECTORY")
		}
		if err := bundle.Validate(args[1]); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "OCI bundle passed preview policy validation (not a security certification)")
		return err
	case "oci":
		return oci(args[1:], out)
	case "host":
		if len(args) != 2 || args[1] != "inspect" {
			return errors.New("usage: gcontainers host inspect")
		}
		if runtime.GOOS != "linux" {
			return errors.New("host readiness inspection supports Linux only")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		report, err := hostreadiness.Inspect(ctx, os.DirFS("/"), os.Geteuid() != 0)
		if err != nil {
			return err
		}
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(payload))
		return err
	case "stack":
		if len(args) >= 4 && args[1] == "transition-capacity" {
			return stackTransitionCapacity(args[2:], out)
		}
		if len(args) == 4 && args[1] == "impact" {
			old, err := stack.Load(args[2])
			if err != nil {
				return err
			}
			target, err := stack.Load(args[3])
			if err != nil {
				return err
			}
			report, err := stack.AnalyzeImpact(old, target)
			if err != nil {
				return err
			}
			encoded, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, string(encoded))
			return err
		}
		if len(args) >= 3 && args[1] == "capacity" {
			return stackCapacity(args[2:], out)
		}
		if len(args) == 4 && args[1] == "diff" {
			before, err := stack.Load(args[2])
			if err != nil {
				return err
			}
			after, err := stack.Load(args[3])
			if err != nil {
				return err
			}
			changes, err := stack.Diff(before, after)
			if err != nil {
				return err
			}
			serialized, err := json.MarshalIndent(changes, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, string(serialized))
			return err
		}
		if len(args) != 3 || (args[1] != "validate" && args[1] != "plan") {
			return errors.New("usage: gcontainers stack validate|plan NATIVE_STACK_JSON")
		}
		parsed, err := stack.Load(args[2])
		if err != nil {
			return err
		}
		if args[1] == "validate" {
			_, err = fmt.Fprintln(out, "native stack declaration valid (no workloads launched)")
			return err
		}
		plan, err := stack.DryRun(parsed)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(data))
		return err
	default:
		return errors.New("unknown command (try help)")
	}
}
func usage(w io.Writer) error {
	_, err := fmt.Fprintln(w, `GoreeCloud Containers 0.1.0-dev — Linux rootless OCI preview
Usage:
  gcontainers doctor
  gcontainers host inspect
  gcontainers stack validate NATIVE_STACK_JSON
  gcontainers stack plan NATIVE_STACK_JSON
  gcontainers stack diff OLD_STACK_JSON NEW_STACK_JSON
  gcontainers stack impact OLD_STACK_JSON NEW_STACK_JSON
  gcontainers stack transition-capacity OLD_STACK_JSON NEW_STACK_JSON --cpu-milli N --memory-mib N --max-instances N
  gcontainers stack capacity NATIVE_STACK_JSON --cpu-milli N --memory-mib N --max-instances N
  gcontainers validate BUNDLE_DIRECTORY
  gcontainers oci create --id NAME --bundle DIRECTORY --experimental
  gcontainers oci start|stop|delete --id NAME --experimental
  gcontainers oci state --id NAME
Options for OCI commands: --runtime=crun|runc
No images, networking, Compose or production deployment is implemented yet.`)
	return err
}
func doctor(w io.Writer) error {
	uid := os.Geteuid()
	_, crunErr := exec.LookPath("crun")
	_, runcErr := exec.LookPath("runc")
	_, cgErr := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	info := map[string]any{
		"product":             "GoreeCloud Containers",
		"version":             version,
		"os":                  runtime.GOOS,
		"arch":                runtime.GOARCH,
		"rootless_user":       uid != 0,
		"cgroup_v2_present":   cgErr == nil,
		"crun_present":        crunErr == nil,
		"runc_present":        runcErr == nil,
		"xdg_runtime_dir_set": os.Getenv("XDG_RUNTIME_DIR") != "",
		"production_ready":    false,
	}
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
func oci(args []string, w io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing OCI action")
	}
	action := args[0]
	switch action {
	case "create", "start", "state", "stop", "delete":
	default:
		return errors.New("unsupported OCI action")
	}
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		return errors.New("OCI operations require Linux and an unprivileged user")
	}
	fs := flag.NewFlagSet("oci "+action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("id", "", "container identifier")
	dir := fs.String("bundle", "", "OCI bundle directory")
	backend := fs.String("runtime", "crun", "OCI runtime: crun or runc")
	experimental := fs.Bool("experimental", false, "explicit consent to preview execution")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	if err := engine.ValidateID(*id); err != nil {
		return err
	}
	if action != "state" && !*experimental {
		return errors.New("OCI mutations require --experimental")
	}
	if action == "create" && *dir == "" {
		return errors.New("create requires --bundle")
	}
	if *backend != "crun" && *backend != "runc" {
		return errors.New("only crun and runc are allowed")
	}
	path, err := exec.LookPath(*backend)
	if err != nil {
		return fmt.Errorf("required OCI runtime %s not installed", *backend)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	stateRoot, err := engine.PrepareStateRoot(os.Getenv("XDG_RUNTIME_DIR"))
	if err != nil {
		return err
	}
	e, err := engine.New(path, stateRoot, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch action {
	case "create":
		err = e.Create(ctx, *id, *dir)
	case "start":
		err = e.Start(ctx, *id)
	case "stop":
		err = e.Stop(ctx, *id)
	case "delete":
		err = e.Delete(ctx, *id)
	case "state":
		var b []byte
		b, err = e.State(ctx, *id)
		if err == nil {
			_, err = w.Write(b)
			if err == nil {
				_, err = fmt.Fprintln(w)
			}
		}
	}
	if err != nil {
		return err
	}
	if action != "state" {
		_, err = fmt.Fprintf(w, "OCI %s: %s (preview)\n", action, *id)
	}
	return err
}

// stackCapacity is a resource-only estimate from caller-supplied budget;
// it never measures live host resources or launches a workload.
func stackCapacity(args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: stack capacity STACK_JSON --cpu-milli N --memory-mib N --max-instances N")
	}
	flags := flag.NewFlagSet("stack capacity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cpu := flags.Int64("cpu-milli", 0, "maximum declared millicores")
	memoryMiB := flags.Int64("memory-mib", 0, "maximum declared MiB")
	instances := flags.Int("max-instances", 0, "maximum declared instances")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *memoryMiB <= 0 || *memoryMiB > math.MaxInt64/(1<<20) {
		return errors.New("invalid capacity flags or memory quota")
	}
	parsed, err := stack.Load(args[0])
	if err != nil {
		return err
	}
	review, err := stack.AssessResources(parsed, stack.ResourceBudget{
		CPUMilli: *cpu, MemoryBytes: *memoryMiB * (1 << 20), MaxInstances: *instances})
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintln(out, string(encoded)); err != nil {
		return err
	}
	if !review.WithinLimits {
		return errors.New("native stack capacity budget exceeded; review blockers")
	}
	return nil
}

// stackTransitionCapacity applies ONLY explicit user-provided budgets to an
// old+new coexistence estimate; success never approves actual deployment.
func stackTransitionCapacity(args []string, out io.Writer) error {
	if len(args) < 3 {
		return errors.New("usage: stack transition-capacity OLD_JSON NEW_JSON --cpu-milli N --memory-mib N --max-instances N")
	}
	flags := flag.NewFlagSet("stack transition-capacity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cpu := flags.Int64("cpu-milli", 0, "declared CPU budget in millicores")
	memoryMiB := flags.Int64("memory-mib", 0, "declared memory budget in MiB")
	instances := flags.Int("max-instances", 0, "declared max count of instances")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *memoryMiB <= 0 || *memoryMiB > math.MaxInt64/(1<<20) {
		return errors.New("invalid transition capacity budget flags")
	}
	before, err := stack.Load(args[0])
	if err != nil {
		return err
	}
	after, err := stack.Load(args[1])
	if err != nil {
		return err
	}
	report, err := stack.AssessTransitionCapacity(before, after, stack.ResourceBudget{
		CPUMilli: *cpu, MemoryBytes: *memoryMiB * (1 << 20), MaxInstances: *instances,
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
	if !report.WithinBudget {
		return errors.New("native transition overlap exceeds supplied resource budget; manual review required")
	}
	return nil
}
