#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops}}"

# shellcheck source=../_lib/docker.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/docker.sh"
# shellcheck source=../_lib/kubeconfig.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/kubeconfig.sh"
# shellcheck source=../_lib/outputs.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/outputs.sh"
echo -e "${YELLOW}Shutting down k3d cluster '${CLUSTER_NAME}'...${NC}"

# cluster_containers lists the cluster's containers by the label k3d gives
# them. After an unclean stop (a sleeping laptop, WSL shutting down) k3d can
# fail to read a cluster whose containers still exist; asking k3d alone would
# call it "not found" and leave them to collide with the next `labctl init`.
cluster_containers() {
  docker ps -aq --filter "label=k3d.cluster=${CLUSTER_NAME}" 2>/dev/null || true
}

if ! k3d cluster list "$CLUSTER_NAME" &>/dev/null && [ -z "$(cluster_containers)" ]; then
  echo -e "${YELLOW}Cluster '$CLUSTER_NAME' not found.${NC}"
  # Leftover state this runtime wrote for the cluster goes; another runtime's stays.
  if owns_lab_state "$CLUSTER_NAME" k3d; then
    forget_lab_state "$CLUSTER_NAME" k3d
  fi
  exit 0
fi

with_user_kubeconfig k3d cluster delete "$CLUSTER_NAME" ||
  echo -e "${YELLOW}k3d could not delete '$CLUSTER_NAME' cleanly; removing its containers directly.${NC}"

leftover="$(cluster_containers)"
if [ -n "$leftover" ]; then
  # shellcheck disable=SC2086 # one container ID per word
  docker rm -f $leftover >/dev/null || true
  docker network rm "k3d-${CLUSTER_NAME}" >/dev/null 2>&1 || true
fi

echo -e "${YELLOW}Validating cluster shutdown...${NC}"

if ! k3d cluster list "$CLUSTER_NAME" &>/dev/null && [ -z "$(cluster_containers)" ]; then
  echo -e "${GREEN}✓ Cluster '$CLUSTER_NAME' has been successfully shut down.${NC}"
  # The deleted cluster's recorded ports, kubeconfig and outputs go with it.
  forget_lab_state "$CLUSTER_NAME" k3d
  exit 0
else
  echo -e "${RED}✗ Validation failed: cluster '$CLUSTER_NAME' still exists.${NC}"
  echo "  Remove it by hand: k3d cluster delete $CLUSTER_NAME" >&2
  exit 1
fi
