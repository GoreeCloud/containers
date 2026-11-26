# GoreeCloud Containers desktop preview

Native Flutter Linux dashboard; read-only, local, explicit refresh. Uses the existing Go CLI's private Unix peer and strict summary validation; no remote network access or credentials. This is a developer preview, not Stable, and not an authorization grant.

## Build and run

Flutter 3.27.4, Dart 3.6.2, standard Linux Flutter toolchain and Go 1.27.4. From repository root:

```sh
go build -o /absolute/reviewed/path/gcontainers ./cmd/gcontainers
cd desktop/dashboard
flutter pub get --enforce-lockfile
flutter analyze
flutter test
flutter build linux --debug --no-pub
```

Start the existing foreground local API against an already prepared private runtime root with the selected trusted runc/crun. Use an owner-private socket and a separately reviewed CLI binary. Pass the absolute CLI path, exact sha256:HEX digest, and absolute private socket path as the three arguments to the built executable. Without all three arguments, it opens disconnected. Do not point it at an unreviewed client.

```sh
build/linux/x64/debug/bundle/goreecloud_containers ABS_CLI SHA256 ABS_SOCKET
```

No startup or unattended refresh, persistence, command shell, workload mutations, or deployment controls. An explicit foreground monitoring session observes up to ten summaries at two-second intervals after each completed request; stops on recovery, unavailable response, operator stop, leaving overview or loss of foreground. An already in-flight read may finish; no new read is scheduled after stopping. No automatic repair or historical persistence. Only aggregate counters are rendered. The dashboard clears stale values before refresh and suppresses raw subprocess errors. Local CLI binary pin checks reduce accidental mismatch; same-user file replacement between verification and launch remains possible. The Go client validates socket peer and response schema; same UID is not GoreeCloud Identity.

## Evidence

On the authorized Linux laptop, Flutter analyze and four widget/schema tests passed, debug executable built, and a separate synthetic empty private runtime fixture verified the actual Dart-to-Go-to-Unix API bridge, wrong CLI digest rejection, and 960 x 680 normal-window startup under isolated Xvfb. Run the repeatable repository-root script after building: `bash scripts/desktop-dashboard-smoke.sh` (requires the already verified local runc, Xvfb and xwininfo). No active-display, physical assistive technology, signed installer, production, or full MIPS acceptance is claimed. Native compositor controls remain intact; in-app rounded surfaces use gc-ui, outer window corner appearance is compositor-dependent.

## Source and licensing

The exact gc-ui Flutter adapter files are in `../vendor/gc_ui_expressive`, from cc3e1042dcb13a1c821661ba232c734f230237b6, with MIT license and provenance. Flutter-generated Linux runner from SDK 3.27.4 and Dart crypto 3.0.5 BSD notices are in `../licenses`; package versions and hashes are locked. Full downstream release dependency/security/license review remains required; preserve original repository license notices.
