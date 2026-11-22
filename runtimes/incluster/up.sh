#!/usr/bin/env bash
set -euo pipefail

# In-cluster runtime "up" - no-op. The labctl server already runs inside the
# target cluster (team mode); there is no cluster to create. kubectl
# and helm use the in-cluster ServiceAccount config automatically.

# shellcheck source=../_lib/docker.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/docker.sh"
# shellcheck source=../_lib/kubeconfig.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/kubeconfig.sh"
# shellcheck source=../_lib/outputs.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/outputs.sh"

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops}}"

echo "In-cluster runtime: using the hosting cluster via the ServiceAccount token."

if ! kubectl cluster-info >/dev/null 2>&1; then
  echo "WARNING: kubectl cannot reach the API server. In a pod this should" >&2
  echo "         work via the mounted ServiceAccount; check RBAC and the" >&2
  echo "         automountServiceAccountToken setting." >&2
  exit 1
fi

kubectl version -o yaml 2>/dev/null | grep -i "gitVersion" | head -2 || true
# There is no kubeconfig or context to record: the ServiceAccount is the access.
# A pod's state directory can be read-only; the outputs are a convenience there.
DOMAIN_SUFFIX="${DOMAIN_SUFFIX:-cluster.local}" write_outputs "$CLUSTER_NAME" incluster "" "" ||
  echo "WARNING: could not write $(lab_outputs_file "$CLUSTER_NAME"); continuing without outputs." >&2
echo "In-cluster runtime ready."
