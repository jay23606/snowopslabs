#!/usr/bin/env bash
set -euo pipefail

# Report whether the in-cluster runtime is ready: labctl runs in a pod, and the
# ServiceAccount reaches the API server. Exit 0 when ready; otherwise exit 1
# with one line on stderr saying why. Changes nothing.

not_ready() {
  echo "$*" >&2
  exit 1
}

[ -n "${KUBERNETES_SERVICE_HOST:-}" ] || not_ready "labctl is not running inside a cluster"

kubectl --request-timeout=5s get --raw=/healthz >/dev/null 2>&1 ||
  not_ready "the hosting cluster's API server is not answering; check the pod's RBAC and ServiceAccount"
