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
	"github.com/GoreeCloud/containers/internal/ociimage"

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
	case "image":
		return imageCommand(args[1:], out)

	case "stack":
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
  gcontainers image verify OCI_LAYOUT_DIR
  gcontainers image inspect-layer TAR_FILE tar|tar+gzip
  gcontainers image import --store ABS_DIR --file ABS_BLOB --experimental

gcontainers stack validate NATIVE_STACK_JSON
  gcontainers stack plan NATIVE_STACK_JSON
  gcontainers stack diff OLD_STACK_JSON NEW_STACK_JSON
  gcontainers validate BUNDLE_DIRECTORY
  gcontainers oci create --id NAME --bundle DIRECTORY --experimental
  gcontainers oci start|stop|delete --id NAME --experimental
  gcontainers oci state --id NAME
Options for OCI commands: --runtime=crun|runc
No image pull/unpack, native stack execution, networking, Compose or production deployment is implemented yet.`)
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

func imageCommand(args []string, out io.Writer) error {
	if len(args) == 3 && args[0] == "inspect-layer" {
		media := ""
		switch args[2] {
		case "tar":
			media = ociimage.MediaLayer
		case "tar+gzip":
			media = ociimage.MediaLayerGzip
		default:
			return errors.New("supported layer inspection media: tar or tar+gzip")
		}
		fi, err := os.Lstat(args[1])
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() || fi.Size() > ociimage.MaxLayerCompressedBytes {
			return errors.New("layer inspection requires a regular file within the compressed input limit")
		}
		file, err := os.Open(args[1])
		if err != nil {
			return err
		}
		defer file.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		report, err := ociimage.InspectLayer(ctx, file, media)
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(b))
		return err
	}
	if len(args) == 2 && args[0] == "verify" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		report, err := ociimage.VerifyLayout(ctx, args[1])
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(b))
		return err
	}
	if len(args) == 0 || args[0] != "import" {
		return errors.New("usage: image verify OCI_LAYOUT_DIR | image import --store ABS_DIR --file ABS_BLOB --experimental")
	}
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		return errors.New("image import requires an unprivileged Linux user")
	}
	flags := flag.NewFlagSet("image import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("store", "", "absolute private content store directory")
	source := flags.String("file", "", "local regular file to import as opaque bytes")
	experimental := flags.Bool("experimental", false, "explicit permission for preview store write")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !*experimental || !filepath.IsAbs(*root) || !filepath.IsAbs(*source) {
		return errors.New("image import requires --store ABS_DIR --file ABS_BLOB --experimental")
	}
	info, err := os.Lstat(*source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("image import requires a regular non-symlink source file")
	}
	input, err := os.Open(*source)
	if err != nil {
		return err
	}
	defer input.Close()
	store, err := ociimage.OpenStore(*root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	blob, err := store.Put(ctx, input)
	if err != nil {
		return err
	}
	output, err := json.MarshalIndent(blob, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(output))
	return err
}
