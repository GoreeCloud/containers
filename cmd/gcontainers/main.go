// gcontainers is the experimental GoreeCloud Containers command-line frontend.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/GoreeCloud/containers/internal/bundle"
	"github.com/GoreeCloud/containers/internal/engine"
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
	default:
		return errors.New("unknown command (try help)")
	}
}
func usage(w io.Writer) error {
	_, err := fmt.Fprintln(w, `GoreeCloud Containers 0.1.0-dev — Linux rootless OCI preview
Usage:
  gcontainers doctor
  gcontainers validate BUNDLE_DIRECTORY
  gcontainers oci create --id NAME --bundle DIRECTORY --experimental
  gcontainers oci start|stop|delete --id NAME --experimental
  gcontainers oci state --id NAME
  gcontainers oci pending --id NAME    # read-only unresolved intent inspection
Options for OCI commands: --runtime=crun|runc
No images, networking, Compose or production deployment is implemented yet.`)
	return err
}
func doctor(w io.Writer) error {
	uid := os.Geteuid()
	crunPath, crunErr := exec.LookPath("crun")
	runcPath, runcErr := exec.LookPath("runc")
	crunEligible := false
	runcEligible := false
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	if crunErr == nil && engine.CheckInstalledRuntime(ctx, crunPath) == nil {
		crunEligible = true
	}
	if runcErr == nil && engine.CheckInstalledRuntime(ctx, runcPath) == nil {
		runcEligible = true
	}
	_, cgErr := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	info := map[string]any{
		"product":                    "GoreeCloud Containers",
		"version":                    version,
		"os":                         runtime.GOOS,
		"arch":                       runtime.GOARCH,
		"rootless_user":              uid != 0,
		"cgroup_v2_present":          cgErr == nil,
		"crun_present":               crunErr == nil,
		"runc_present":               runcErr == nil,
		"crun_security_baseline_met": crunEligible,
		"runc_security_baseline_met": runcEligible,
		"xdg_runtime_dir_set":        os.Getenv("XDG_RUNTIME_DIR") != "",
		"production_ready":           false,
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
	case "create", "start", "state", "stop", "delete", "pending":
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
	if action != "state" && action != "pending" && !*experimental {
		return errors.New("OCI mutations require --experimental")
	}
	if action == "create" && *dir == "" {
		return errors.New("create requires --bundle")
	}
	if *backend != "crun" && *backend != "runc" {
		return errors.New("only crun and runc are allowed")
	}
	if action == "pending" {
		if *dir != "" || *experimental {
			return errors.New("pending inspection is read-only and accepts only --id")
		}
		base := os.Getenv("XDG_RUNTIME_DIR")
		if !filepath.IsAbs(base) || filepath.Clean(base) != base {
			return errors.New("XDG_RUNTIME_DIR must be a clean absolute path")
		}
		resolved, err := filepath.EvalSymlinks(base)
		if err != nil || resolved != base {
			return errors.New("symlinked runtime directory unsupported")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		pending, err := engine.ReadPendingMutation(ctx, filepath.Join(base, "goreecloud-containers"), *id)
		if err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(pending, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(encoded))
		return err
	}
	path, err := exec.LookPath(*backend)
	if err != nil {
		return fmt.Errorf("required OCI runtime %s not installed", *backend)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := engine.TrustedRuntimeBinary(path); err != nil {
		return err
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
