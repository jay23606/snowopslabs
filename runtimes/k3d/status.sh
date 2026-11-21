#!/usr/bin/env bash
set -euo pipefail

# Report whether the lab's k3d cluster is ready: it exists, the lab has its
# kubeconfig, and the API server answers. Exit 0 when ready; otherwise exit 1
# with one line on stderr saying why. Changes nothing.

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops}}"

# shellcheck source=../_lib/kubeconfig.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/kubeconfig.sh"

not_ready() {
  echo "$*" >&2
  exit 1
}

command -v k3d >/dev/null 2>&1 || not_ready "k3d is not installed"
k3d cluster list "$CLUSTER_NAME" >/dev/null 2>&1 || not_ready "k3d has no cluster named '$CLUSTER_NAME'; run 'labctl init'"

kubeconfig="$(lab_kubeconfig_file "$CLUSTER_NAME")"
[ -s "$kubeconfig" ] || not_ready "the lab has no kubeconfig ($kubeconfig); run 'labctl init'"

KUBECONFIG="$kubeconfig" kubectl --request-timeout=5s get --raw=/healthz >/dev/null 2>&1 ||
  not_ready "the API server of '$CLUSTER_NAME' is not answering; 'labctl init' brings the lab back"
