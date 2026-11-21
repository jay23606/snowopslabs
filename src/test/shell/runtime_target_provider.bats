#!/usr/bin/env bats
# SPDX-License-Identifier: Apache-2.0
#
# The target-provider contract (runtimes/README.md): every runtime has a
# status.sh that exits 0 when its cluster is ready and otherwise exits 1 with
# one line saying why, and its up.sh records what it produced in outputs.env in
# the lab's state directory.

load 'helpers/stub'

setup() {
  stub_setup
  stub_command k3d kind kubectl docker
  ROOT="$(project_root)"
  export REACHABLE_WAIT=0
  export SNOWOPS_HOME="$STUB_DIR/home"
  unset LAB_STATE_DIR SNOWOPS_LAB_KUBECONFIG SNOWOPS_USER_KUBECONFIG KUBECONFIG KUBERNETES_SERVICE_HOST
  STATE_DIR="$SNOWOPS_HOME/state/testcluster"
  LAB_FILE="$STATE_DIR/kubeconfig"
  OUTPUTS="$STATE_DIR/outputs.env"
  stub_when k3d "kubeconfig get" 0 "    server: https://0.0.0.0:6443"
  stub_when kind "get kubeconfig" 0 "    server: https://127.0.0.1:6443"
}

teardown() {
  stub_teardown
}

# lab_has_kubeconfig gives the lab the kubeconfig its up.sh would have written.
lab_has_kubeconfig() {
  mkdir -p "$STATE_DIR"
  echo "apiVersion: v1" >"$LAB_FILE"
}

# k3d_lists_cluster makes `k3d cluster list` show the lab's cluster.
k3d_lists_cluster() {
  stub_when k3d "cluster list" 0 "testcluster 1/1 0/0 true"
}

# one_line_reason fails unless the last run printed exactly one line.
one_line_reason() {
  [ "${#lines[@]}" -eq 1 ]
}

# docker_maps_ports makes the load balancer or node container answer
# `docker port` with the host ports 8080 and 8443.
docker_maps_ports() {
  stub_script docker <<'STUB'
case "$*" in
  "port "*" 80/tcp") echo "0.0.0.0:8080" ;;
  "port "*" 443/tcp") echo "0.0.0.0:8443" ;;
esac
exit 0
STUB
}

# --- k3d status -------------------------------------------------------------

@test "k3d status exits 0 when the cluster exists and answers through the lab's kubeconfig" {
  k3d_lists_cluster
  lab_has_kubeconfig
  run bash "$ROOT/runtimes/k3d/status.sh" testcluster
  [ "$status" -eq 0 ]
  [ -z "$output" ]
  assert_called kubectl "get --raw=/healthz"
}

@test "k3d status asks the lab's kubeconfig, not the user's" {
  k3d_lists_cluster
  lab_has_kubeconfig
  export KUBECONFIG="$STUB_DIR/colleague-kubeconfig"
  stub_script kubectl <<STUB
[ "\${KUBECONFIG:-}" = "$LAB_FILE" ]
STUB
  run bash "$ROOT/runtimes/k3d/status.sh" testcluster
  [ "$status" -eq 0 ]
}

@test "k3d status fails with one line when k3d has no such cluster" {
  lab_has_kubeconfig
  : >"$STUB_DIR/spec/k3d/rules"
  stub_when k3d "cluster list" 0 "some-other-cluster 1/1 0/0 true"
  run bash "$ROOT/runtimes/k3d/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"no cluster named 'testcluster'"* ]] || false
}

@test "k3d status names Docker, not a missing cluster, when k3d cannot list" {
  lab_has_kubeconfig
  stub_script k3d <<'STUB'
echo "FATA[0000] Cannot connect to the Docker daemon at unix:///var/run/docker.sock" >&2
exit 1
STUB
  run bash "$ROOT/runtimes/k3d/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"Docker"* ]] || false
  [[ "$output" != *"no cluster named"* ]] || false
}

@test "k3d status fails with one line when the lab has no kubeconfig" {
  k3d_lists_cluster
  run bash "$ROOT/runtimes/k3d/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"no kubeconfig"* ]] || false
  [[ "$output" == *"labctl init"* ]] || false
}

@test "k3d status fails with one line when the API server does not answer" {
  k3d_lists_cluster
  lab_has_kubeconfig
  stub_when kubectl "get --raw" 1
  run bash "$ROOT/runtimes/k3d/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"not answering"* ]] || false
}

@test "k3d status changes nothing" {
  k3d_lists_cluster
  lab_has_kubeconfig
  run bash "$ROOT/runtimes/k3d/status.sh" testcluster
  refute_called k3d "create"
  refute_called k3d "delete"
  refute_called k3d "stop"
  refute_called kubectl "delete"
  refute_called kubectl "apply"
}

# --- kind status ------------------------------------------------------------

@test "kind status exits 0 when the cluster exists and answers" {
  lab_has_kubeconfig
  stub_stdout kind "testcluster"
  run bash "$ROOT/runtimes/kind/status.sh" testcluster
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "kind status fails with one line when kind has no such cluster" {
  lab_has_kubeconfig
  stub_stdout kind "some-other-cluster"
  run bash "$ROOT/runtimes/kind/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"no cluster named 'testcluster'"* ]] || false
}

@test "kind status names Docker, not a missing cluster, when kind cannot list" {
  lab_has_kubeconfig
  stub_script kind <<'STUB'
echo "ERROR: failed to list clusters: Cannot connect to the Docker daemon" >&2
exit 1
STUB
  run bash "$ROOT/runtimes/kind/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"Docker"* ]] || false
  [[ "$output" != *"no cluster named"* ]] || false
}

@test "kind status fails with one line when the lab has no kubeconfig" {
  stub_stdout kind "testcluster"
  run bash "$ROOT/runtimes/kind/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"no kubeconfig"* ]] || false
}

@test "kind status fails with one line when the API server does not answer" {
  lab_has_kubeconfig
  stub_stdout kind "testcluster"
  stub_when kubectl "get --raw" 1
  run bash "$ROOT/runtimes/kind/status.sh" testcluster
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"not answering"* ]] || false
}

# --- incluster status -------------------------------------------------------

@test "incluster status exits 0 in a pod whose ServiceAccount reaches the API" {
  export KUBERNETES_SERVICE_HOST=10.0.0.1
  run bash "$ROOT/runtimes/incluster/status.sh"
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "incluster status fails with one line outside a cluster" {
  run bash "$ROOT/runtimes/incluster/status.sh"
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"not running inside a cluster"* ]] || false
  refute_called kubectl
}

@test "incluster status fails with one line when the API server does not answer" {
  export KUBERNETES_SERVICE_HOST=10.0.0.1
  stub_when kubectl "get --raw" 1
  run bash "$ROOT/runtimes/incluster/status.sh"
  [ "$status" -eq 1 ]
  one_line_reason
  [[ "$output" == *"not answering"* ]] || false
}

# --- outputs.env ------------------------------------------------------------

@test "k3d up records its outputs for a new cluster" {
  stub_when k3d "cluster list" 1 # not found
  docker_maps_ports
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "KUBECONFIG=$LAB_FILE" "$OUTPUTS"
  grep -qx "KUBE_CONTEXT=k3d-testcluster" "$OUTPUTS"
  grep -qx "HTTP_PORT=8080" "$OUTPUTS"
  grep -qx "HTTPS_PORT=8443" "$OUTPUTS"
  grep -qx "DOMAIN_SUFFIX=testcluster.localhost" "$OUTPUTS"
}

@test "k3d up records its outputs for a cluster that already exists" {
  docker_maps_ports
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  refute_called k3d "cluster create"
  grep -qx "KUBE_CONTEXT=k3d-testcluster" "$OUTPUTS"
  grep -qx "HTTP_PORT=8080" "$OUTPUTS"
}

@test "the outputs take their ports and suffix from the cluster record" {
  mkdir -p "$SNOWOPS_HOME/clusters"
  printf 'HTTP_PORT=8081\nHTTPS_PORT=8444\nDOMAIN_SUFFIX=lab.example\n' >"$SNOWOPS_HOME/clusters/testcluster.env"
  stub_script docker <<'STUB'
case "$*" in
  "port "*" 80/tcp") echo "0.0.0.0:8081" ;;
  "port "*" 443/tcp") echo "0.0.0.0:8444" ;;
esac
exit 0
STUB
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "HTTP_PORT=8081" "$OUTPUTS"
  grep -qx "HTTPS_PORT=8444" "$OUTPUTS"
  grep -qx "DOMAIN_SUFFIX=lab.example" "$OUTPUTS"
}

@test "up.sh run again rewrites the same outputs" {
  docker_maps_ports
  bash "$ROOT/runtimes/k3d/up.sh" testcluster
  first="$(cat "$OUTPUTS")"
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  [ "$(cat "$OUTPUTS")" = "$first" ]
}

@test "kind up records its outputs" {
  stub_when k3d "cluster list" 1 # no k3d cluster holds the name
  docker_maps_ports
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "KUBECONFIG=$LAB_FILE" "$OUTPUTS"
  grep -qx "KUBE_CONTEXT=kind-testcluster" "$OUTPUTS"
  grep -qx "HTTP_PORT=8080" "$OUTPUTS"
}

@test "kind up records its outputs for a cluster that already exists" {
  stub_stdout kind "testcluster"
  docker_maps_ports
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "KUBE_CONTEXT=kind-testcluster" "$OUTPUTS"
}

@test "incluster up records the domain suffix and no kubeconfig or context" {
  run bash "$ROOT/runtimes/incluster/up.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "DOMAIN_SUFFIX=cluster.local" "$OUTPUTS"
  ! grep -q "KUBECONFIG\|KUBE_CONTEXT" "$OUTPUTS"
}

# cluster_gone_after_delete makes <tool> list the cluster until its delete
# command runs, the way a real one stops listing a deleted cluster.
cluster_gone_after_delete() {
  case "$1" in
    k3d) list='cluster list' del='cluster delete' out='testcluster 1/1 0/0 true' ;;
    kind) list='get clusters' del='delete cluster' out='testcluster' ;;
  esac
  stub_script "$1" <<STUB
case "\$*" in
  *"$del"*) touch "$STUB_DIR/deleted" ;;
  *"$list"*) if [ -e "$STUB_DIR/deleted" ]; then [ "\$1" != cluster ] || exit 1; else echo "$out"; fi ;;
esac
exit 0
STUB
}

# lab_state_of <profile> gives the lab the state <profile>'s up.sh would write.
lab_state_of() {
  lab_has_kubeconfig
  printf 'PROFILE=%s\nKUBECONFIG=%s\nKUBE_CONTEXT=%s-testcluster\n' "$1" "$LAB_FILE" "$1" >"$OUTPUTS"
  mkdir -p "$SNOWOPS_HOME/clusters"
  printf 'HTTP_PORT=8080\n' >"$SNOWOPS_HOME/clusters/testcluster.env"
}

@test "k3d down removes the state of the cluster it deletes" {
  lab_state_of k3d
  cluster_gone_after_delete k3d
  run bash "$ROOT/runtimes/k3d/down.sh" testcluster
  [ "$status" -eq 0 ]
  [ ! -e "$OUTPUTS" ]
  [ ! -e "$LAB_FILE" ]
  [ ! -e "$SNOWOPS_HOME/clusters/testcluster.env" ]
}

@test "kind down removes the state of the cluster it deletes" {
  lab_state_of kind
  cluster_gone_after_delete kind
  run bash "$ROOT/runtimes/kind/down.sh" testcluster
  [ "$status" -eq 0 ]
  [ ! -e "$OUTPUTS" ]
  [ ! -e "$LAB_FILE" ]
  [ ! -e "$SNOWOPS_HOME/clusters/testcluster.env" ]
}

@test "k3d down leaves a kind lab's state alone when it has no such cluster" {
  lab_state_of kind
  : >"$STUB_DIR/spec/k3d/rules"
  stub_when k3d "cluster list" 1
  run bash "$ROOT/runtimes/k3d/down.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "PROFILE=kind" "$OUTPUTS"
  [ -s "$LAB_FILE" ]
  [ -e "$SNOWOPS_HOME/clusters/testcluster.env" ]
}

@test "kind down leaves a k3d lab's state alone when it has no such cluster" {
  lab_state_of k3d
  run bash "$ROOT/runtimes/kind/down.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "PROFILE=k3d" "$OUTPUTS"
  [ -s "$LAB_FILE" ]
  [ -e "$SNOWOPS_HOME/clusters/testcluster.env" ]
}

@test "down clears leftover state its own runtime wrote for a cluster that is gone" {
  lab_state_of k3d
  : >"$STUB_DIR/spec/k3d/rules"
  stub_when k3d "cluster list" 1
  run bash "$ROOT/runtimes/k3d/down.sh" testcluster
  [ "$status" -eq 0 ]
  [ ! -e "$OUTPUTS" ]
  [ ! -e "$LAB_FILE" ]
}

@test "up records the runtime profile in the outputs" {
  docker_maps_ports
  bash "$ROOT/runtimes/k3d/up.sh" testcluster
  grep -qx "PROFILE=k3d" "$OUTPUTS"
  stub_stdout kind "testcluster"
  bash "$ROOT/runtimes/kind/up.sh" testcluster
  grep -qx "PROFILE=kind" "$OUTPUTS"
}

@test "moving the ingress keeps the profile in the outputs" {
  lab_state_of k3d
  printf 'HTTP_PORT=80\nHTTPS_PORT=443\nDOMAIN_SUFFIX=testcluster.localhost\n' >"$SNOWOPS_HOME/clusters/testcluster.env"
  run bash "$ROOT/runtimes/k3d/move-ingress.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "PROFILE=k3d" "$OUTPUTS"
}

# --- kind after a restart ---------------------------------------------------

# kind_prints_kubeconfig_once_started makes `kind get kubeconfig` fail until the
# node container has been started, as it does while Docker's VM is stopped.
kind_prints_kubeconfig_once_started() {
  stub_script kind <<STUB
case "\$*" in
  "get clusters") echo testcluster ;;
  "get kubeconfig"*)
    [ -e "$STUB_DIR/started" ] || exit 1
    echo "    server: https://127.0.0.1:6443"
    ;;
esac
exit 0
STUB
  stub_script docker <<STUB
case "\$*" in
  "ps -a"*) echo testcluster-control-plane ;;
  "start "*) touch "$STUB_DIR/started" ;;
  "port "*" 80/tcp") echo "0.0.0.0:8080" ;;
  "port "*" 443/tcp") echo "0.0.0.0:8443" ;;
esac
exit 0
STUB
}

@test "kind up after a restart writes the kubeconfig once its nodes run" {
  kind_prints_kubeconfig_once_started
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -eq 0 ]
  grep -q 'https://127.0.0.1:6443' "$LAB_FILE"
  grep -qx "KUBECONFIG=$LAB_FILE" "$OUTPUTS"
}

@test "kind up recreates a missing lab kubeconfig for a healthy cluster" {
  stub_stdout kind "testcluster"
  rm -f "$LAB_FILE"
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -eq 0 ]
  [ -s "$LAB_FILE" ]
}

@test "kind up never records outputs that name a kubeconfig it could not write" {
  stub_script kind <<'STUB'
case "$*" in
  "get clusters") echo testcluster ;;
  "get kubeconfig"*) exit 1 ;;
esac
exit 0
STUB
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -ne 0 ]
  [ ! -e "$OUTPUTS" ]
}

# --- incluster state --------------------------------------------------------

@test "incluster up warns once and carries on when the state directory is read-only" {
  : >"$STUB_DIR/not-a-directory"
  export SNOWOPS_HOME="$STUB_DIR/not-a-directory"
  run bash "$ROOT/runtimes/incluster/up.sh" testcluster
  [ "$status" -eq 0 ]
  [ "$(printf '%s\n' "$output" | grep -c 'could not write')" -eq 1 ]
  [[ "$output" == *"In-cluster runtime ready"* ]] || false
}

@test "incluster up records its profile" {
  run bash "$ROOT/runtimes/incluster/up.sh" testcluster
  grep -qx "PROFILE=incluster" "$OUTPUTS"
}

@test "incluster down removes the outputs it recorded and never deletes the cluster" {
  bash "$ROOT/runtimes/incluster/up.sh" testcluster
  [ -e "$OUTPUTS" ]
  run bash "$ROOT/runtimes/incluster/down.sh" testcluster
  [ "$status" -eq 0 ]
  [ ! -e "$OUTPUTS" ]
  refute_called kubectl "delete"
}

@test "incluster down leaves another runtime's outputs alone" {
  lab_state_of k3d
  run bash "$ROOT/runtimes/incluster/down.sh" testcluster
  [ "$status" -eq 0 ]
  grep -qx "PROFILE=k3d" "$OUTPUTS"
}

@test "moving the ingress updates the ports in the outputs" {
  mkdir -p "$STATE_DIR" "$SNOWOPS_HOME/clusters"
  printf 'KUBECONFIG=%s\nKUBE_CONTEXT=k3d-testcluster\nHTTP_PORT=80\nHTTPS_PORT=443\n' "$LAB_FILE" >"$OUTPUTS"
  printf 'HTTP_PORT=80\nHTTPS_PORT=443\nDOMAIN_SUFFIX=testcluster.localhost\n' >"$SNOWOPS_HOME/clusters/testcluster.env"
  run bash "$ROOT/runtimes/k3d/move-ingress.sh" testcluster
  [ "$status" -eq 0 ]
  assert_called k3d "cluster edit testcluster"
  moved="$(sed -n 's/^HTTP_PORT=//p' "$SNOWOPS_HOME/clusters/testcluster.env")"
  [ "$moved" != "80" ]
  grep -qx "HTTP_PORT=$moved" "$OUTPUTS"
  grep -qx "KUBE_CONTEXT=k3d-testcluster" "$OUTPUTS"
  grep -qx "KUBECONFIG=$LAB_FILE" "$OUTPUTS"
}

@test "moving the ingress leaves a lab without outputs alone" {
  mkdir -p "$SNOWOPS_HOME/clusters"
  printf 'HTTP_PORT=80\nHTTPS_PORT=443\nDOMAIN_SUFFIX=testcluster.localhost\n' >"$SNOWOPS_HOME/clusters/testcluster.env"
  run bash "$ROOT/runtimes/k3d/move-ingress.sh" testcluster
  [ "$status" -eq 0 ]
  [ ! -e "$OUTPUTS" ]
}
