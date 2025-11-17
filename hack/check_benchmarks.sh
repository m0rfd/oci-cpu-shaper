#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOCACHE_DIR="${GOCACHE_DIR:-${REPO_ROOT}/.cache/go}"
mkdir -p "${GOCACHE_DIR}"

ARTIFACT_DIR="${REPO_ROOT}/artifacts/benchmarks"
mkdir -p "${ARTIFACT_DIR}"
OUTPUT_FILE="${ARTIFACT_DIR}/pool-bench.txt"

BENCH_PATTERN="${BENCH_PATTERN:-BenchmarkPoolDutyCycle}"
BENCH_PKG="${BENCH_PKG:-./pkg/shape}"

export GOCACHE="${GOCACHE_DIR}"

go test -run '^$' -bench "${BENCH_PATTERN}" "${BENCH_PKG}" | tee "${OUTPUT_FILE}"

python3 - "${OUTPUT_FILE}" "${BENCH_PATTERN}" <<'PY'
import sys

path = sys.argv[1]
pattern = sys.argv[2]

with open(path, encoding='utf-8') as handle:
    lines = handle.read().splitlines()

errors = []
processed = 0

for line in lines:
    if not line.startswith(pattern):
        continue

    parts = line.split()
    if len(parts) < 5:
        continue

    name_with_proc = parts[0]
    bench_name = name_with_proc.rsplit('-', 1)[0]

    try:
        ns_index = parts.index('ns/op')
    except ValueError:
        continue

    metrics = {}
    idx = ns_index + 1
    while idx + 1 < len(parts):
        value_token = parts[idx]
        key = parts[idx + 1]
        try:
            value = float(value_token)
        except ValueError:
            break
        metrics[key] = value
        idx += 2

    required = ['cpu_pct', 'avg_drift_ns', 'tick_stddev', 'quantum_ns', 'target_pct']
    if not all(k in metrics for k in required):
        errors.append(f"{bench_name}: missing metrics {required}")
        continue

    target_pct = metrics['target_pct']
    cpu_pct = metrics['cpu_pct']
    quantum_ns = metrics['quantum_ns']
    drift_ns = metrics['avg_drift_ns']
    tick_stddev = metrics['tick_stddev']

    cpu_delta = abs(cpu_pct - target_pct)
    if cpu_delta > 5.0:
        errors.append(
            f"{bench_name}: cpu_pct drift {cpu_pct:.2f} vs target {target_pct:.2f} exceeds 5%%"
        )

    drift_limit = quantum_ns * 0.15
    if drift_ns > drift_limit:
        errors.append(
            f"{bench_name}: avg_drift_ns {drift_ns:.2f} exceeds 15% of quantum ({drift_limit:.2f})"
        )

    if tick_stddev > 0.01:
        errors.append(
            f"{bench_name}: tick_stddev {tick_stddev:.4f} exceeds 0.01"
        )

    processed += 1

if processed == 0:
    sys.stderr.write('no benchmark lines matched\n')
    sys.exit(1)

if errors:
    for error in errors:
        sys.stderr.write(error + '\n')
    sys.exit(1)
PY

echo "Benchmark thresholds satisfied; output recorded at ${OUTPUT_FILE}"
