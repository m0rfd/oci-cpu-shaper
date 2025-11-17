#!/usr/bin/env bash
set -euo pipefail

: "${SHAPER_IMAGE:=oci-cpu-shaper:rootful}"
: "${SHAPER_CONTAINER_NAME:=oci-cpu-shaper-root}"
: "${SHAPER_CONFIG_PATH:=/etc/oci-cpu-shaper/configs/mode-b.yaml}"
: "${SHAPER_CONFIG_HOST_PATH:=}"
: "${SHAPER_MODE:=dry-run}"
: "${SHAPER_LOG_LEVEL:=info}"
: "${SHAPER_CPU_SHARES:=1024}"
: "${SHAPER_CPU_PERIOD:=}"
: "${SHAPER_CPU_QUOTA:=}"
: "${SHAPER_ENV_FILE:=}"

run_args=(
  --rm
  --name "${SHAPER_CONTAINER_NAME}"
  --cpu-shares "${SHAPER_CPU_SHARES}"
)

if [[ -n "${SHAPER_CONFIG_HOST_PATH}" ]]; then
  run_args+=(--volume "${SHAPER_CONFIG_HOST_PATH}:${SHAPER_CONFIG_PATH}:ro")
fi

if [[ -n "${SHAPER_CPU_PERIOD}" ]]; then
  run_args+=(--cpu-period "${SHAPER_CPU_PERIOD}")
fi

if [[ -n "${SHAPER_CPU_QUOTA}" ]]; then
  run_args+=(--cpu-quota "${SHAPER_CPU_QUOTA}")
fi

if [[ -n "${SHAPER_ENV_FILE}" ]]; then
  run_args+=(--env-file "${SHAPER_ENV_FILE}")
fi

run_args+=(
  "${SHAPER_IMAGE}"
  --config "${SHAPER_CONFIG_PATH}"
  --mode "${SHAPER_MODE}"
  --log-level "${SHAPER_LOG_LEVEL}"
)

exec docker run "${run_args[@]}"
