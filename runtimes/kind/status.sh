#!/usr/bin/env bash
set -euo pipefail

# Report whether the lab's kind cluster is ready: it exists, the lab has its
# kubeconfig, and the API server answers. Exit 0 when ready; otherwise exit 1
# with one line on stderr saying why. Changes nothing.

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops-kind}}"

# shellcheck source=../_lib/kubeconfig.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/kubeconfig.sh"

not_ready() {
  echo "$*" >&2
  exit 1
}

command -v kind >/dev/null 2>&1 || not_ready "kind is not installed"
if ! clusters="$(kind get clusters 2>&1)"; then
  not_ready "kind cannot list clusters (is Docker running?): $(printf '%s\n' "$clusters" | head -n 1)"
fi
printf '%s\n' "$clusters" | grep -qx "$CLUSTER_NAME" ||
  not_ready "kind has no cluster named '$CLUSTER_NAME'; run 'labctl init'"

kubeconfig="$(lab_kubeconfig_file "$CLUSTER_NAME")"
[ -s "$kubeconfig" ] || not_ready "the lab has no kubeconfig ($kubeconfig); run 'labctl init'"

KUBECONFIG="$kubeconfig" kubectl --request-timeout=5s get --raw=/healthz >/dev/null 2>&1 ||
  not_ready "the API server of '$CLUSTER_NAME' is not answering; 'labctl init' brings the lab back"
