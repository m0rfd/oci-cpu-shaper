#!/usr/bin/env bash
set -euo pipefail

: "${SHAPER_IMAGE:=oci-cpu-shaper:rootless}"
: "${SHAPER_CONTAINER_NAME:=oci-cpu-shaper}"
: "${SHAPER_CONFIG_PATH:=/etc/oci-cpu-shaper/configs/mode-a.yaml}"
: "${SHAPER_CONFIG_HOST_PATH:=}"
: "${SHAPER_MODE:=dry-run}"
: "${SHAPER_LOG_LEVEL:=info}"
: "${SHAPER_CPU_SHARES:=128}"
: "${SHAPER_ENV_FILE:=}"  # optional env-file consumed by docker run

run_args=(
  --rm
  --name "${SHAPER_CONTAINER_NAME}"
  --read-only
  --tmpfs /tmp
  --security-opt no-new-privileges:true
  --cpu-shares "${SHAPER_CPU_SHARES}"
)

if [[ -n "${SHAPER_CONFIG_HOST_PATH}" ]]; then
  run_args+=(--volume "${SHAPER_CONFIG_HOST_PATH}:${SHAPER_CONFIG_PATH}:ro")
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
