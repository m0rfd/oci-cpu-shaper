# §9 Command-Line Interface

The `shaper` binary delivers a thin orchestration layer that connects configuration, logging, and subsystem wiring. It now prioritises predictable ergonomics while shipping the fully implemented adaptive controller that powers both `dry-run` and `enforce` workflows, so operators interact with the same code paths that production deployments execute.

## 9.1 Invocation

```bash
shaper --config /etc/oci-cpu-shaper/config.yaml --log-level info --mode enforce
```

When only build metadata is required, the CLI also exposes a fast-exit flag and
subcommand:

```bash
shaper --version
# {Version:1.2.3 GitCommit:abc123 BuildDate:2024-06-01}

shaper version
```

Both forms print the struct returned by `internal/buildinfo.Current()` without
loading configuration or initialising the logger, keeping diagnostics scripts
and packaging checks lightweight (§5.2).

Three foundational flags align with §§3.1 and 5.2 of the implementation plan:

| Flag | Description | Default |
| ---- | ----------- | ------- |
| `--config` | Path to the primary YAML configuration file. Relative paths resolve from the current working directory. | `/etc/oci-cpu-shaper/config.yaml` |
| `--log-level` | Structured logging level understood by the Zap logger (`debug`, `info`, `warn`, `error`, `dpanic`, `panic`, `fatal`). | `info` |
| `--mode` | Controller operating mode. `dry-run` and `enforce` now spin up the adaptive controller with real OCI metrics, estimator sampling, and worker pools; `noop` keeps the historical bypass for smoke tests. | `enforce` |
| `--shutdown-after` | Optional duration that cancels the run context after the requested window, letting CI smoke tests and diagnostics shut down predictably without external supervisors. | `0s` (disabled) |

Flags remain intentionally minimal so orchestration tools can template them alongside file-based configuration and environment overrides. When `--shutdown-after` is non-zero the CLI installs a context deadline and treats the resulting `context deadline exceeded`/`context canceled` errors as clean shutdowns so smoke tests can rely on exit status `0`.

`--mode` defaults to `enforce` so production-ready deployments do not need to pass the flag. Operators can opt into a metrics-only posture with `--mode dry-run` (or `SHAPER_MODE=dry-run` in Compose/Quadlet env files) and can bypass controller wiring entirely for diagnostics with `--mode noop`.

The CLI also installs `SIGINT`/`SIGTERM` handlers that wrap the run loop in a
`context.WithCancel`. Delivering either signal now cancels the controller,
worker pool, and HTTP server contexts just like the time-bounded shutdown,
letting supervisors stop the process without forcing an unclean exit or leaking
goroutines.

## 9.2 Configuration Layout

Bootstrap deployments rely on a compact YAML manifest that mirrors §§3.1 and 5.2 thresholds:

`pkg/runtimeconfig` now owns these structs and helpers so every binary can reuse the
same layering flow. `cmd/shaper` calls `runtimeconfig.Load` to stack the immutable defaults,
optional YAML file, and environment overlays before handing the values to
`runtimeconfig.Config.ToAdaptConfig()`, letting other entrypoints plug into the same
validated config without duplicating conversions or field assignments.

```yaml
controller:
  targetStart: 0.22
  targetMin: 0.20
  targetMax: 0.32
  stepUp: 0.01
  stepDown: 0.005
  fallbackTarget: 0.22
  goalLow: 0.21
  goalHigh: 0.27
  interval: 1h
  relaxedInterval: 4h
  relaxedThreshold: 0.26
  relaxedConfirmations: 2
  suppressThreshold: 0.80
  suppressResume: 0.68
  suppressRunnableThreshold: 1.20
  suppressRunnableResume: 0.96
estimator:
  interval: 1s
pool:
  workers: 2
  quantum: 1ms
  pauseThreshold: 0.80
  resumeThreshold: 0.68
http:
  bind: ":9108"
oci:
  offline: false
  compartmentId: "ocid1.compartment.oc1..example"
  region: "us-phoenix-1"
  instanceId: "ocid1.instance.oc1..example"
```

- The repository publishes these defaults as ready-to-use manifests at
  `configs/mode-a.yaml` and `configs/mode-b.yaml`. Both files copy the controller,
  estimator, pool thresholds, HTTP, and `oci.offline` defaults above (including
  the tighter 0.20–0.32 band, four-hour relaxed interval after two consecutive above-threshold samples, one-second estimator
  cadence, and two-worker pool) while omitting tenancy-specific OCIDs so the
  samples remain usable in source control. Operators should extend the manifest
  with their own `compartmentId`, `region`, and optional `instanceId` values
  before entering enforce mode.
- `controller.*` mirrors the slow-loop thresholds from §3.1, including the one-hour cadence and relaxed four-hour interval once OCI P95 stays at or above 0.26 for two consecutive polls (tuned via `controller.relaxedConfirmations`/`SHAPER_RELAXED_CONFIRMATIONS`). The fast-loop suppression settings (`suppressThreshold`, `suppressResume`) now reflect the 0.80/0.68 hysteresis that decides when estimator-driven contention drops the worker pool to zero and when work resumes after the host cools, while `suppressRunnableThreshold`/`suppressRunnableResume` clamp the loop immediately when runnable tasks exceed ~1.2 per CPU and only resume once the run queue cools. Set `controller.suppressThreshold` to `0` (or any non-positive value) to disable utilisation-based suppression entirely; the resume threshold is ignored in that case. The same runnable guard now flows into the worker pool so both the controller target and the workers pause in lock-step when the per-CPU run queue crosses the configured limit.
- The loader now enforces the documented ratios and cadences: `targetMin` must remain below `targetMax`, every slow-loop target and goal must fall within that band, and the `interval`, `relaxedInterval`, `stepUp`, `stepDown`, `pool.quantum`, and `pool.workers` values must be positive. Invalid manifests abort startup with an exit status of `2` so operators can fix the config before the controller touches system state (§§3.1, 5.2).
- Configuration processing now flows through four dedicated stages, all implemented in `pkg/runtimeconfig`: an immutable defaults builder, a YAML merge helper, environment overrides, and validators. Each stage is unit-tested individually so overrides and safety rails stay predictable, and env vars always win over file-sourced values without mutating the stored defaults (§5.2).
- Validation now enforces that every slow-loop target or goal remains below both suppression thresholds, so manifests that would immediately re-trigger the fast loop are rejected with an exit status of `2` and a descriptive error message (§§3.1, 5.2).
- `estimator.interval` controls the fast `/proc/stat` sampler cadence (§5.2) while the worker `pool` exposes quantum sizing that stays within the 1–5 ms duty-cycle budget. `pool.pauseThreshold`/`pool.resumeThreshold` mirror the controller suppression hysteresis (0.80/0.68) so the worker pool pauses entirely when host utilisation crosses the configured limit and only resumes once the load cools, and `pool.runnableGuard` pauses workers immediately on run-queue spikes even when utilisation is below the pause threshold. The manifests now explicitly pin `pool.workers` to `2` to keep deterministic load across instance shapes.
- `http.bind` retains the Prometheus listener address and now backs the `/metrics` exporter described in §9.5, while `oci.compartmentId` supplies the tenancy scope required by the Monitoring client and `oci.region` pins the Monitoring endpoint region when IMDS access is unavailable (for example, CI smoke tests).
- `oci.instanceId` is optional and lets operators bypass IMDS lookups when metadata access is blocked (for example, CI smoke tests or staging environments without instance principals). When `oci.offline` is set the CLI injects a static metrics client and fallback instance ID so dry-run/enforce can exercise the adaptive controller without IMDS or Monitoring access (§§5.2, 11).

When `oci.compartmentId` or `oci.region` are omitted in online deployments the CLI now consults IMDS to resolve both values before constructing the Monitoring client, ensuring metrics queries and structured logs include the canonical tenancy metadata without additional configuration.

Configuration parsing layers file contents with environment overrides so operators can tune production deployments without editing manifests directly.

## 9.3 Environment Overrides

The CLI honours the following environment variables, matching the naming in §5.2:

| Variable | Description | Default |
| -------- | ----------- | ------- |
| `SHAPER_TARGET_START` | Initial duty-cycle target when OCI data is unavailable. | `0.22` |
| `SHAPER_TARGET_MIN` / `SHAPER_TARGET_MAX` | Bounds applied to adaptive adjustments. | `0.20` / `0.32` |
| `SHAPER_STEP_UP` / `SHAPER_STEP_DOWN` | Target deltas when OCI P95 is below or above the goal band. | `0.01` / `0.005` |
| `SHAPER_FALLBACK_TARGET` | Fixed target while OCI metrics are unavailable. | `0.22` |
| `SHAPER_SLOW_INTERVAL` / `SHAPER_SLOW_INTERVAL_RELAXED` | Baseline and relaxed controller cadences. | `1h` / `4h` |
| `SHAPER_RELAXED_THRESHOLD` | P95 ratio that switches the controller to the relaxed cadence. | `0.26` |
| `SHAPER_RELAXED_CONFIRMATIONS` | Consecutive above-threshold samples required before switching to the relaxed cadence. | `2` |
| `SHAPER_FAST_INTERVAL` | Host CPU sampling cadence for the estimator. | `1s` |
| `SHAPER_SUPPRESS_THRESHOLD` / `SHAPER_SUPPRESS_RESUME` | Utilisation-based fast-loop suppression thresholds that gate the zero-target mode. Assign `SHAPER_SUPPRESS_THRESHOLD=0` (or any non-positive value) to disable utilisation suppression; the resume override is ignored when that path is off. | `0.80` / `0.68` |
| `SHAPER_SUPPRESS_RUNNABLE_THRESHOLD` / `SHAPER_SUPPRESS_RUNNABLE_RESUME` | Runnable-per-CPU band that pauses the controller immediately when the run queue spikes. Assign either to `0` to disable runnable-based suppression independently of utilisation thresholds. | `1.20` / `0.96` |
| `SHAPER_WORKER_COUNT` | Number of duty-cycle workers (`>=1`). | `2` |
| `SHAPER_POOL_PAUSE_THRESHOLD` / `SHAPER_POOL_RESUME_THRESHOLD` | Host CPU hysteresis that pauses/resumes the worker pool when the estimator detects contention. | `0.80` / `0.68` |
| `SHAPER_POOL_RUNNABLE_GUARD` | Runnable-per-CPU guard that pauses all workers immediately when the run queue exceeds the configured threshold. | `1.20` |
| `HTTP_ADDR` | Prometheus listener bind address. | `:9108` |
| `OCI_COMPARTMENT_ID` | Tenancy scope for OCI Monitoring API calls. | *(required for enforce/dry-run unless offline mode is enabled)* |
| `OCI_REGION` | Overrides the Monitoring region, avoiding live IMDS lookups when running in smoke-test environments. | *(empty)* |
| `OCI_INSTANCE_ID` | Overrides the instance OCID used for Monitoring queries and IMDS metadata logs, skipping live metadata calls. | *(empty)* |
| `OCI_OFFLINE` | Enables the static metrics client and metadata fallback described above so smoke tests can bootstrap without IMDS or Monitoring access. | `false` |

Unset or malformed overrides fall back to the defaults shown above. The
controller subtracts `SHAPER_STEP_DOWN` internally, so the configuration value
remains a positive delta even though it reduces the target when OCI P95 exceeds
the goal band.

Setting `HTTP_ADDR` to an empty string (for example, exporting `HTTP_ADDR=`)
disables the `/metrics` listener even when the YAML manifest enables it. This
helps CI smoke tests and containerised diagnostics avoid exposing the endpoint
while still recording metrics internally.

### Layering overrides

Environment variables sit on top of the YAML file, so operators can mount
`configs/mode-a.yaml` or `configs/mode-b.yaml` verbatim and then tune specific
thresholds without editing the manifest. For example:

```bash
SHAPER_TARGET_START=0.28 SHAPER_SUPPRESS_THRESHOLD=0.90 \
  SHAPER_SUPPRESS_RESUME=0.75 \
  shaper --config /etc/oci-cpu-shaper/config.yaml --mode enforce
```

Compose deployments use the `SHAPER_ENV_FILE` hook described in §6 to inject the
same overrides. Each line follows the shell `KEY=value` syntax, so adding
`SHAPER_TARGET_MAX=0.45` in `deploy/compose/mode-a.env.example` produces the
same runtime effect as exporting the variable directly.

## 9.4 Diagnostics

At startup the binary emits a structured log line containing build metadata derived from `internal/buildinfo`, the resolved OCI compartment/region pair, and the selected mode. The log now also includes `controllerState`, allowing operators to see whether the fast-loop suppression is active when the process initialises. When the shutdown timer is enabled the log also captures the requested duration so operators can confirm the controller will terminate automatically. This gives operators immediate confirmation of the version, Git commit, configuration path, tenancy metadata, suppression status, and lifecycle expectations before any controllers mutate system state.

The entrypoint now stages startup through dedicated helpers: `app_bootstrap.go` parses CLI arguments, loads the manifest, calls `logger_factory.go` to build the logger, and applies shutdown timers before logging the resolved settings, while `app_controller.go` leans on `metadata_resolver.go` to resolve IMDS metadata, configures metrics, and builds the controller/runtime wiring. `app_run.go` simply threads the two stages together so signal handling and shutdown remain thin and testable.

Invalid flag values are rejected during argument parsing: unknown controller modes surface an error and cause the program to exit with status `2`, unsupported log levels report a structured error before the logger is constructed, and negative `--shutdown-after` durations are rejected. This keeps early runs predictable while new policy engines are still being prototyped.

Configuration validation shares this behaviour: when thresholds conflict with the suppression bounds, when targets/goals drift outside `targetMin`/`targetMax`, or when intervals/worker counts fall to zero the CLI prints the descriptive failure and exits with code `2`, preventing partially initialised controllers (§§3.1, 5.2).

Smoke tests introduced in §11 now cover the dependency-injected entrypoint as well as adaptive-controller wiring, ensuring that enforce/dry-run builds start the OCI client, estimator sampler, and worker pool while `noop` preserves the bypass path for validation scenarios. Offline mode keeps this wiring intact by substituting the static metrics client so container smoke tests can run without live tenancy credentials, and new unit coverage exercises the IMDS-backed region/compartment resolver plus its failure modes to keep the ≥96% statement coverage guarantee intact.

Local contributors can validate the CLI wiring the same way: run `make lint` (or `make lint-fix` to autofix) and `make test` before checking in changes and finish with `make coverage MIN_COVERAGE=96` to confirm the documentation’s QA promise remains true.

Rootful binaries built with `-tags rootful` now issue their
`sched_setscheduler(0, SCHED_IDLE, ...)` request as soon as the worker pool is
constructed, before goroutines start consuming CPU (§§6, 9). Hosts running the
Compose or Quadlet stacks must grant `CAP_SYS_NICE`/`SYS_NICE` so the
`worker failed to enter sched_idle` warning remains informational rather than a
permanent indicator that the downgrade could not be applied; `EPERM` rejections
are silently ignored when the capability is intentionally withheld.

## 9.5 Metrics Exporter

`cmd/shaper` now wires metrics in two layers: `metrics_handlers.go` builds the OpenMetrics exporter from `pkg/http/metrics`, registers `/metrics` and `/healthz` handlers, and propagates worker-pool/health state, while `metrics_server.go` owns the HTTP lifecycle. The listener still defaults to `:9108` via `http.bind` (overridable with `HTTP_ADDR`), matching the Compose port mapping in §6 and the container `EXPOSE 9108` declaration. Production Prometheus servers can scrape the endpoint directly when the rootful stack runs in host-network mode, while rootless deployments forward `${SHAPER_METRICS_BIND:-127.0.0.1:9108}:9108` from the host loopback to the container port. The server derives a per-run context and invokes the returned shutdown hook when the controller exits so `/metrics` follows the process lifecycle even when the ambient context stays open.

Binding failures still abort startup: when the requested `http.bind` address is already in use the CLI logs `failed to start metrics server`, exits with a runtime error, and leaves the controller uninitialised so systemd or Kubernetes can retry immediately. The lifecycle helper emits structured `metrics server listen failed`/`metrics server serve` entries when the bind or serve loops encounter errors, making it obvious which phase failed before the controller came up. Unit coverage in §11 continues to confirm the fast-fail path, graceful shutdown, and `/metrics` content-type expectations alongside focused handler tests.

### Emitted series

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `shaper_target_ratio` | gauge | Current duty-cycle target assigned to the worker pool (0.0–1.0). |
| `shaper_mode{mode="<name>"}` | gauge | Active controller mode (`noop`, `dry-run`, or `enforce`) reported as a labelled one-hot gauge. |
| `controller_state{state="<name>"}` | gauge | Base controller state driven by OCI metrics (`normal`, `fallback`, or `unknown`). |
| `shaper_state{state="<name>"}` | gauge | Effective state-machine output after suppression overlays (`normal`, `fallback`, `suppressed`, or `unknown`). |
| `oci_p95` | gauge | Latest OCI `CpuUtilization` P95 ratio used for adaptive decisions. |
| `oci_last_success_epoch` | counter | Unix epoch seconds when `QueryP95CPU` last succeeded (`0` while offline). |
| `duty_cycle_ms` | gauge | Worker quantum configured for each duty-cycle interval in milliseconds. |
| `worker_count` | gauge | Number of goroutines currently driving CPU load. |
| `host_cpu_percent` | gauge | Most recent host CPU utilisation sample from the fast estimator loop. |

`controller_state` reflects only the normal/fallback branch of the controller (driven by OCI telemetry), while `shaper_state` includes suppression overlays so dashboards can distinguish OCI outages from local contention.

### Example scrape output

```
# HELP shaper_target_ratio Target duty cycle ratio assigned to worker pool.
# TYPE shaper_target_ratio gauge
shaper_target_ratio 0.275000
# HELP shaper_mode Controller operating mode (value set to 1 for the active mode).
# TYPE shaper_mode gauge
shaper_mode{mode="dry-run"} 1
# HELP controller_state Base controller state derived from OCI metrics (1 for the active state).
# TYPE controller_state gauge
controller_state{state="fallback"} 1
# HELP shaper_state Controller state machine output (value set to 1 for the active state).
# TYPE shaper_state gauge
shaper_state{state="fallback"} 1
# HELP oci_p95 Last observed OCI CPU P95 ratio.
# TYPE oci_p95 gauge
oci_p95 0.180000
# HELP oci_last_success_epoch Unix epoch seconds of the last successful OCI metrics query.
# TYPE oci_last_success_epoch counter
oci_last_success_epoch 0
# HELP duty_cycle_ms Duty cycle quantum configured for workers (milliseconds).
# TYPE duty_cycle_ms gauge
duty_cycle_ms 1.000
# HELP worker_count Number of worker goroutines consuming CPU.
# TYPE worker_count gauge
worker_count 4
# HELP host_cpu_percent Last recorded host CPU utilisation percentage.
# TYPE host_cpu_percent gauge
host_cpu_percent 6.25
# HELP cgroup_cpu_weight Detected cgroup v2 cpu.weight value for the process.
# TYPE cgroup_cpu_weight gauge
cgroup_cpu_weight 128
# HELP cgroup_cpu_max_quota Detected cpu.max quota (microseconds). Zero when unlimited.
# TYPE cgroup_cpu_max_quota gauge
cgroup_cpu_max_quota 30000
# HELP cgroup_cpu_max_period Detected cpu.max period (microseconds).
# TYPE cgroup_cpu_max_period gauge
cgroup_cpu_max_period 100000
# HELP cgroup_cpu_max_unlimited Flag set to 1 when cpu.max reports "max".
# TYPE cgroup_cpu_max_unlimited gauge
cgroup_cpu_max_unlimited 0
# EOF
```

Offline mode continues to populate each series so smoke tests and container health checks can rely on the exporter without live tenancy credentials; only `oci_last_success_epoch` remains `0` until Monitoring calls succeed. Unit and CLI tests exercise the handler through `httptest.Server`, preserving the ≥96% coverage floor mandated in §11.

The cgroup gauges mirror the detected `/proc/self/cgroup` path and the files under `/sys/fs/cgroup/.../cpu.weight`/`cpu.max`. Unlimited ceilings keep `cgroup_cpu_max_quota` at `0` and flip `cgroup_cpu_max_unlimited` to `1`, making it easy to alarm on drift from the §4 recommendations without shelling into the host.

## 9.6 Health Checks

`cmd/shaper` now serves a lightweight JSON status document at `/healthz` on the
same listener as `/metrics`. The handler reports the controller mode (`"noop"`,
`"dry-run"`, or `"enforce"`), the state machine (`"normal"`, `"fallback"`, or
`"suppressed"`), the last OCI metrics plus estimator errors, and the detected
`cpu.weight`/`cpu.max` values (or any file read errors). Container orchestrators
can poll the endpoint to surface degraded Monitoring connectivity, estimator
stalls, or cgroup drift while the process continues to run.

The response mirrors this structure:

```json
{
  "mode": "dry-run",
  "state": "normal",
  "ociError": "",
  "estimatorError": "",
  "cgroup": {
    "path": "/user.slice/shaper.scope",
    "cpuWeight": {
      "value": 128
    },
    "cpuMax": {
      "quota": 30000,
      "period": 100000,
      "unlimited": false
    }
  }
}
```

When errors are present the strings are populated with the underlying error
messages; otherwise they remain empty. `cpuWeight.error`/`cpuMax.error`
surface filesystem failures so Kubernetes probes can alarm on missing
`/sys/fs/cgroup` files in addition to controller regressions. Unit coverage in
`pkg/http/status` verifies the handler’s JSON output while the end-to-end
harness starts the CLI, injects a Monitoring outage, and polls `/healthz` until
it reports the `fallback` state with the recorded error string. This keeps the
≥96% coverage target documented in §11 intact and proves that health probes
surface Monitoring failures and cgroup drift without crashing the process.
