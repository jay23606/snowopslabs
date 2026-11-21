#!/usr/bin/env bash
set -euo pipefail

# Delete a kind cluster. Idempotent.

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops}}"

# shellcheck source=../_lib/docker.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/docker.sh"
# shellcheck source=../_lib/kubeconfig.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/kubeconfig.sh"
# shellcheck source=../_lib/outputs.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/outputs.sh"

if ! command -v kind >/dev/null 2>&1; then
  echo "ERROR: 'kind' is required but not installed." >&2
  exit 1
fi

if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
  echo "Cluster '$CLUSTER_NAME' not found, nothing to do."
  # Leftover state this runtime wrote for the cluster goes; another runtime's stays.
  if owns_lab_state "$CLUSTER_NAME" kind; then
    forget_lab_state "$CLUSTER_NAME" kind
  fi
  exit 0
fi

echo "Deleting kind cluster '$CLUSTER_NAME'..."
with_user_kubeconfig kind delete cluster --name "$CLUSTER_NAME"
# The deleted cluster's recorded ports, kubeconfig and outputs go with it.
forget_lab_state "$CLUSTER_NAME" kind
echo "Cluster '$CLUSTER_NAME' deleted."
