#!/usr/bin/env bash
set -euo pipefail

# In-cluster runtime "down" - no-op. The labctl server does not own the cluster
# it runs in, so it must never tear it down. Removing the server is done by
# uninstalling its Helm release:
#   helm uninstall labctl-server -n labctl

# shellcheck source=../_lib/kubeconfig.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/kubeconfig.sh"
# shellcheck source=../_lib/outputs.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/outputs.sh"

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops}}"

# The outputs this runtime recorded go; the hosting cluster stays.
if owns_lab_state "$CLUSTER_NAME" incluster; then
  forget_outputs "$CLUSTER_NAME"
fi

echo "In-cluster runtime: refusing to delete the hosting cluster (by design)."
echo "To remove the server: helm uninstall labctl-server -n <namespace>"
