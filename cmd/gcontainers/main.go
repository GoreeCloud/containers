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
	case "__kernel-probe-child":
		if len(args) != 1 {
			return errors.New("kernel probe child takes no arguments")
		}
		return runKernelProbeChild(out)
	case "kernel-probe":
		if len(args) != 1 {
			return errors.New("kernel-probe takes no flags")
		}
		return runKernelProbe(out)
	case "local-query":
		return localQuery(args[1:], out)
	case "serve-local":
		return localAPI(args[1:])
	case "doctor":
		if len(args) != 1 {
			return errors.New("doctor takes no arguments")
		}
		return doctor(out)
	case "snapshot-delegated":
		return delegatedSnapshot(args[1:], out)
	case "snapshot":
		return bundleSnapshot(args[1:], out)
	case "delegated-observe":
		return delegatedObserve(args[1:], out)
	case "preflight-delegated":
		return delegatedPreflight(args[1:], out)
	case "preflight":
		return bundlePreflight(args[1:], out)
	case "validate":
		if len(args) != 2 {
			return errors.New("usage: gcontainers validate BUNDLE_DIRECTORY")
		}
		if err := bundle.Validate(args[1]); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "OCI bundle passed preview policy validation (not a security certification)")
		return err
	case "wait":
		return waitCommand(args[1:], out)
	case "watch":
		return watchCommand(args[1:], out)
	case "inventory":
		return inventoryCommand(args[1:], out)
	case "recovery-summary":
		return recoverySummary(args[1:], out)
	case "recovery-assess":
		return recoveryAssess(args[1:], out)
	case "recovery-observe":
		return recoveryObserve(args[1:], out)
	case "oci":
		return oci(args[1:], out)
	case "volume":
		return volumeCommand(args[1:], out)
	case "image":
		return imageCommand(args[1:], out)
	case "host":
		if len(args) == 2 && args[1] == "helpers" {
			return helperReadiness(args[2:], out)
		}
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
		if len(args) > 1 && (args[1] == "start-local" || args[1] == "status-local" || args[1] == "stop-local" || args[1] == "resume-stop") {
			return localSession(args[1], args[2:], out)
		}
		if len(args) > 1 && args[1] == "review-exercise" {
			return stackExercise(args[2:], out, true)
		}
		if len(args) > 1 && args[1] == "exercise" {
			return stackExercise(args[2:], out, false)
		}
		if len(args) >= 2 && args[1] == "observe" {
			return stackObserve(args[2:], out)
		}
		if len(args) >= 2 && args[1] == "instance-plan" {
			return instancePlan(args[2:], out)
		}
		if len(args) == 4 && args[1] == "restart-plan" {
			spec, err := stack.Load(args[2])
			if err != nil {
				return err
			}
			plan, err := stack.PlanRestart(spec, args[3])
			if err != nil {
				return err
			}
			encoded, err := json.MarshalIndent(plan, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, string(encoded))
			return err
		}
		if len(args) == 3 && args[1] == "shutdown-plan" {
			spec, err := stack.Load(args[2])
			if err != nil {
				return err
			}
			plan, err := stack.PlanShutdown(spec)
			if err != nil {
				return err
			}
			encoded, err := json.MarshalIndent(plan, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, string(encoded))
			return err
		}
		if len(args) >= 4 && args[1] == "preflight" {
			return stackPreflight(args[2:], out)
		}
		if len(args) >= 4 && args[1] == "revision-check" {
			return stackRevisionCheck(args[2:], out)
		}
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
  gcontainers snapshot-delegated ABS_BUNDLE PRIVATE_PARENT --experimental
  gcontainers snapshot ABS_BUNDLE PRIVATE_PARENT --experimental
  gcontainers image select-index EXISTING_STORE INDEX_SHA SIZE amd64|arm64
  gcontainers image discover-manifest HTTPS_ORIGIN REPOSITORY SHA256 SIZE --experimental
  gcontainers image fetch-blob EXISTING_STORE HTTPS_ORIGIN REPOSITORY SHA256 SIZE --experimental
  gcontainers image pull-stage EXISTING_STORE HTTPS_ORIGIN REPOSITORY SHA256 SIZE PRIVATE_PARENT [--bearer-file PRIVATE_TOKEN_FILE] [--whiteouts] --experimental
  gcontainers image prepare-bundle EXISTING_STORE MANIFEST_SHA SIZE CONFIG_FILE CONFIG_SHA ENTRYPOINT_SHA PRIVATE_PARENT [--whiteouts] [--delegated-guest] --experimental
  gcontainers image assemble-manifest EXISTING_STORE SHA256 SIZE PRIVATE_PARENT [--whiteouts] --experimental
  gcontainers image assemble PRIVATE_PARENT (ABS_FILE tar|tar+gzip SHA256 SIZE){1,8} --experimental
  gcontainers volume verify-backup BACKUP_DIR SHA256
  gcontainers volume backup PRIVATE_STORE ID PRIVATE_BACKUP_PARENT --experimental
  gcontainers volume restore PRIVATE_STORE NEW_ID BACKUP_DIR SHA256 --experimental
  gcontainers volume create PRIVATE_STORE ID --experimental
  gcontainers volume inspect PRIVATE_STORE ID
  gcontainers volume count PRIVATE_STORE
  gcontainers serve-local ABS_SOCKET [--runtime runc|crun] --experimental
  gcontainers local-query ABS_SOCKET health|journal|runtime-summary
  gcontainers wait runc|crun ID SECONDS
  gcontainers watch runc|crun SAMPLES INTERVAL_SECONDS
  gcontainers inventory runc|crun
  gcontainers recovery-summary runc|crun
  gcontainers recovery-assess runc|crun ID [--expected-observation-sha256 DIGEST]
  gcontainers recovery-observe runc|crun ID
  gcontainers kernel-probe  # inert rootless namespace check, NOT OCI acceptance
  gcontainers image audit --store ABS_EXISTING_STORE_DIR
  gcontainers image verify OCI_LAYOUT_DIR
  gcontainers image inspect-layer TAR_FILE tar|tar+gzip
  gcontainers image stage-layer ABS_FILE tar|tar+gzip sha256:HEX DESCRIPTOR_BYTES PRIVATE_PARENT --experimental
  gcontainers image verify-layer ABS_TAR_FILE tar|tar+gzip sha256:EXPECTED_64_HEX [DESCRIPTOR_BYTES]
  gcontainers image import --store ABS_DIR --file ABS_BLOB --experimental
  gcontainers host helpers
  gcontainers host inspect
  gcontainers stack start-local SPEC ADMISSION ADMISSION_SHA EXISTING_STORE PRIVATE_PARENT PRIVATE_LEASE_STORE --experimental
  gcontainers stack status-local SESSION RECEIPT_SHA
  gcontainers stack stop-local SESSION RECEIPT_SHA --experimental
  gcontainers stack resume-stop SESSION RECEIPT_SHA --experimental
  gcontainers stack review-exercise SPEC ADMISSION ADMISSION_SHA
  gcontainers stack exercise SPEC ADMISSION ADMISSION_SHA EXISTING_STORE PRIVATE_PARENT --experimental
  gcontainers stack observe SPEC runc|crun --expected-plan-sha256 DIGEST
  gcontainers stack instance-plan SPEC [--expected-plan-sha256 DIGEST]
  gcontainers stack validate NATIVE_STACK_JSON
  gcontainers stack plan NATIVE_STACK_JSON
  gcontainers stack shutdown-plan NATIVE_STACK_JSON  # advisory only
  gcontainers stack restart-plan NATIVE_STACK_JSON SERVICE  # advisory only
  gcontainers stack diff OLD_STACK_JSON NEW_STACK_JSON
  gcontainers stack impact OLD_STACK_JSON NEW_STACK_JSON
  gcontainers stack transition-capacity OLD_STACK_JSON NEW_STACK_JSON --cpu-milli N --memory-mib N --max-instances N
  gcontainers stack revision-check OLD_STACK_JSON NEW_STACK_JSON --expected-current-sha256 SHA256:HEX
  gcontainers stack preflight OLD_STACK_JSON NEW_STACK_JSON --expected-current-sha256 sha256:HEX --cpu-milli N --memory-mib N --max-instances N
  gcontainers stack capacity NATIVE_STACK_JSON --cpu-milli N --memory-mib N --max-instances N
  gcontainers validate BUNDLE_DIRECTORY
  gcontainers delegated-observe runc ID ABS_BUNDLE
  gcontainers preflight-delegated ABS_BUNDLE
  gcontainers preflight BUNDLE_DIRECTORY  # read-only policy + guest layout diagnostics; no runtime
  gcontainers oci create --id NAME --bundle DIRECTORY --experimental
  gcontainers oci create --runtime runc --id NAME --bundle ABS_BUNDLE --delegated-guest --experimental
  gcontainers oci start|stop|delete --id NAME --experimental
  gcontainers oci state --id NAME
  gcontainers oci pending-summary  # privacy-minimized inventory, fails on incidents
  gcontainers oci pending --id NAME    # read-only unresolved intent inspection
Options for OCI commands: --runtime=crun|runc
Restricted previews only. No complete image pull/unpack, attached volumes, native stack execution, networking, Compose or production deployment.`)
	return err
}

// runtimeAdmissionStatus is a read-only explanation of preview admission.
// It must never be used as proof of production safety or replace per-operation
// executable and version checks. Avoid exposing host filesystem paths.
func runtimeAdmissionStatus(ctx context.Context, binary string, lookupErr error) (bool, string) {
	if lookupErr != nil {
		return false, "not_installed"
	}
	if err := engine.TrustedRuntimeBinary(binary); err != nil {
		return false, "untrusted_executable"
	}
	if err := engine.CheckInstalledRuntime(ctx, binary); err != nil {
		return false, "runtime_version_or_probe_rejected"
	}
	return true, "preview_preflight_passed"
}

func doctor(w io.Writer) error {
	uid := os.Geteuid()
	crunPath, crunErr := exec.LookPath("crun")
	runcPath, runcErr := exec.LookPath("runc")
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	crunEligible, crunStatus := runtimeAdmissionStatus(ctx, crunPath, crunErr)
	runcEligible, runcStatus := runtimeAdmissionStatus(ctx, runcPath, runcErr)
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
		"crun_admission_status":      crunStatus,
		"runc_admission_status":      runcStatus,
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
	case "create", "start", "state", "stop", "delete", "pending", "pending-summary":
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
	delegated := fs.Bool("delegated-guest", false, "explicit nonroot guest preview")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	if *delegated && (action != "create" || !*experimental || *backend != "runc") {
		return errors.New("delegated guest requires explicit experimental runc create")
	}
	if action == "pending-summary" {
		if *id != "" || *dir != "" || *experimental || *backend != "crun" {
			return errors.New("OCI pending-summary is read-only, takes no flags and never invokes a runtime")
		}
		base := os.Getenv("XDG_RUNTIME_DIR")
		if !filepath.IsAbs(base) || filepath.Clean(base) != base {
			return errors.New("XDG_RUNTIME_DIR must be a clean absolute path")
		}
		resolved, err := filepath.EvalSymlinks(base)
		if err != nil || resolved != base {
			return errors.New("symlinked runtime directory unsupported")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := engine.InspectJournal(ctx, filepath.Join(base, "goreecloud-containers"))
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintln(w, string(data)); err != nil {
			return err
		}
		if report.RecoveryRequired {
			return engine.ErrUnresolvedMutation
		}
		return nil
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
		if *delegated {
			err = e.CreateDelegated(ctx, *id, *dir)
		} else {
			err = e.Create(ctx, *id, *dir)
		}
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
	if len(args) > 0 && args[0] == "select-index" {
		return selectImageIndex(args[1:], out)
	}
	if len(args) > 0 && args[0] == "prepare-bundle" {
		return prepareImageBundle(args[1:], out)
	}
	if len(args) > 0 && args[0] == "pull-stage" {
		return pullStage(args[1:], out)
	}
	if len(args) > 0 && args[0] == "assemble-manifest" {
		return storeManifestAssembly(args[1:], out)
	}
	if len(args) > 0 && args[0] == "discover-manifest" {
		return manifestDiscovery(args[1:], out)
	}
	if len(args) > 0 && args[0] == "assemble" {
		return imageAssemble(args[1:], out)
	}
	if len(args) > 0 && args[0] == "fetch-blob" {
		return registryBlob(args[1:], out)
	}
	if len(args) > 0 && args[0] == "stage-layer" {
		return imageStageLayer(args[1:], out)
	}
	if len(args) > 0 && args[0] == "verify-layer" {
		return imageVerifyLayer(args[1:], out)
	}
	if len(args) == 3 && args[0] == "audit" && args[1] == "--store" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		store, err := ociimage.OpenExistingStore(args[2])
		if err != nil {
			return err
		}
		report, err := store.Audit(ctx)
		if err != nil {
			return err
		}
		result, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintln(out, string(result)); err != nil {
			return err
		}
		if report.OperatorReviewRequired {
			return errors.New("unresolved or active OCI import staging requires operator review")
		}
		return nil
	}
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
		return errors.New("usage: image audit --store ABS_EXISTING_STORE_DIR | image verify OCI_LAYOUT_DIR | image import --store ABS_DIR --file ABS_BLOB --experimental")
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
