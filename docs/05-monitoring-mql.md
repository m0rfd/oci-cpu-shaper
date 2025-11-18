# §5 Monitoring & MQL

Monitoring data informs the shaper’s adaptive policy and must be retrievable from the tenant without manual credential handling. The controller issues Monitoring Query Language (MQL) requests through instance principals so the runtime binary can operate without embedding user API keys.[^oci-monitoring-auth] Alarm wiring that mirrors these queries is documented in [`07-alarms.md`](./07-alarms.md).

## 5.1 Instance principal metrics access

1. Place every compute instance that runs the shaper into a dynamic group (for example, `Any { instance.compartment.id = '<compartment_ocid>' }`).
2. Attach a policy granting read-only Monitoring access to that group, such as:

   ```text
   Allow dynamic-group cpu-shaper to read metrics in compartment <compartment_name>
   ```

3. Ensure the instances have outbound access to the regional `telemetry` endpoint documented by Oracle so the SummarizeMetricsData API succeeds.[^oci-monitoring-endpoint]

The Go SDK automatically exchanges the instance principal token set for a temporary keypair and signs each Monitoring request. No additional configuration files are necessary on the host.

## 5.2 Querying CpuUtilization P95

`pkg/oci.Client.QueryP95CPU` sends `SummarizeMetricsData` requests using the following MQL expression:

```text
CpuUtilization[1m]{resourceId = "<instance_ocid>"}.percentile(0.95)
```

The method wraps the OCI Go SDK client, paginates over `opc-next-page` tokens, and selects the most recent aggregated datapoint. `cmd/shaper` consumes this helper through the narrow `MetricsClient` interface (`QueryP95CPU(ctx, resourceID) (float64, error)`). The instance-principal adapter bridges that interface by passing `last7d = true` internally so each scrape considers the trailing seven days at one-minute granularity, matching the reclaim evaluation period. The helper automatically truncates the interval to the Monitoring service’s resolution ceiling so the API never rejects the call.[^oci-monitoring-mql] It returns `ErrNoMetricsData` when no datapoints are available, allowing the controller to fall back to on-host estimators. Unit tests exercise pagination, constructor wiring, empty-result handling, and the exact query string via the HTTP-backed mock in `monitoring_http_test.go`, while `pkg/oci/query_test.go` focuses on the query/pagination suites and `sdk_overrides_test.go` keeps the instance-principal overrides so the ≥95% coverage floor in §11 stays enforced. CLI wiring now swaps `pkg/oci.ClientFactory` instances via `oci.WithFactory` instead of mutating package-level globals, letting integration tests or offline smoke modes replace the Monitoring client seams without touching shared state.

Offline smoke tests rely on `pkg/oci.NewStaticMetricsClient`, which implements the same interface and serves a constant `QueryP95CPU` value without hitting the API. The packaged container enables this mode by default (`oci.offline: true`) so `oci_last_success_epoch` remains zero until tenancy credentials are available, while the adaptive controller continues to exercise its decision loop against the synthetic datapoint.

Prometheus scrapes expose the same datapoints locally: `curl -fsSL ${HTTP_ADDR:-http://127.0.0.1:9108}/metrics` surfaces the Prometheus text export described in §9.5, letting operators confirm that `oci_p95` and `oci_last_success_epoch` match the Monitoring console while validating that `shaper_target_ratio`, `worker_count`, and `host_cpu_percent` stay within the thresholds defined earlier in this section.

## 5.3 Troubleshooting

- **`ErrNoMetricsData`** – Verify that the instance publishes `CpuUtilization` metrics (enabled by the Compute Agent) and that the queried window contains traffic. Check the Monitoring console for gaps or disablement in the agent plugin.[^oci-compute-agent]
- **HTTP 401/403 responses** – Confirm the instance belongs to the dynamic group referenced by the policy and that the policy grants `read metrics` on the target compartment.
- **HTTP 429/5xx responses** – The helper wraps the raw error so controllers can trigger retries or fall back to cached data. Validate regional connectivity and consider enabling per-request retry logic before escalating. `/healthz` mirrors this state machine output (§9.6) so Kubernetes probes can immediately observe the fallback transition and bubble up the underlying Monitoring error string.

## 5.4 Grafana dashboard setup

Import `deploy/grafana/oci-cpu-shaper-dashboard.json` into Grafana to visualise the controller alongside the upstream OCI signal:

1. Navigate to **Dashboards → New → Import** and upload the JSON file (or paste its contents). When prompted, map the `Prometheus` data source to the instance that scrapes the shaper’s `/metrics` endpoint.
2. Select the shaper instance from the `Instance` drop-down. The dashboard filters all queries (for example, `oci_p95{instance="$instance"}`) to that target so multi-host deployments can reuse the same view.
3. Review the built-in panels:
   - **OCI CpuUtilization P95** – Tracks the tenancy-side percentile produced by `pkg/oci.Client.QueryP95CPU` to confirm Monitoring reads remain healthy (§5.2).
   - **Shaper target duty cycle** – Charts the controller’s current worker target ratio emitted as `shaper_target_ratio`, helping correlate slow-loop adjustments with observed load.
   - **Controller state timeline** – Uses the `shaper_state{state="<label>"}` series to highlight transitions between fallback, enforce, and suppressed modes.
   - **Host CPU versus shaper target** – Overlays the `host_cpu_percent` estimator output with the target ratio so operators can verify reclaim pressure stays within the Always Free guardrails (§3.1).
   - **Controller interval** – Visualises `controller_interval_seconds` to reveal when the controller relaxes into six-hour loops or drops back to the hourly cadence after OCI errors or hot P95 readings.
   - **Last OCI error** – Stat/panel fed by `controller_last_error_info{error="<string>"}` pinpoints the current Monitoring failure message, letting responders match it with `/healthz` output and OCI console logs.

Grafana’s refresh interval defaults to 30 seconds in the export; adjust it to match the site’s Prometheus scrape cadence if the charts appear sparse.

[^oci-monitoring-auth]: Oracle Cloud Infrastructure, "Ways to Access Oracle Cloud Infrastructure". <https://docs.oracle.com/en-us/iaas/Content/Identity/Concepts/whoisusingoci.htm#ways_access>
[^oci-monitoring-endpoint]: Oracle Cloud Infrastructure, "Monitoring Endpoints". <https://docs.oracle.com/en-us/iaas/Content/Monitoring/Concepts/monitoringoverview.htm#endpoints>
[^oci-monitoring-mql]: Oracle Cloud Infrastructure, "Monitoring Query Language (MQL) Reference". <https://docs.oracle.com/en-us/iaas/Content/Monitoring/Reference/mql.htm>
[^oci-compute-agent]: Oracle Cloud Infrastructure, "Enabling Compute Agent Plugins". <https://docs.oracle.com/en-us/iaas/Content/Compute/Tasks/manage-plugins.htm>
