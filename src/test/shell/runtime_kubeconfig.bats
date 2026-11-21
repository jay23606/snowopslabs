#!/usr/bin/env bats
# SPDX-License-Identifier: Apache-2.0
#
# The lab's own kubeconfig. A runtime writes it into the lab's state directory,
# and every kubectl call its scripts make uses it, so the user's current
# kubectl context (another cluster, a colleague's staging) never decides which
# cluster labctl changes.

load 'helpers/stub'

setup() {
  stub_setup
  stub_command k3d kind kubectl docker
  ROOT="$(project_root)"
  export REACHABLE_WAIT=0
  export SNOWOPS_HOME="$STUB_DIR/home"
  unset LAB_STATE_DIR SNOWOPS_LAB_KUBECONFIG SNOWOPS_USER_KUBECONFIG KUBECONFIG
  LAB_FILE="$SNOWOPS_HOME/state/testcluster/kubeconfig"
  stub_when k3d "kubeconfig get" 0 "    server: https://0.0.0.0:6443"
  stub_when kind "get kubeconfig" 0 "    server: https://127.0.0.1:6443"
}

teardown() {
  stub_teardown
}

# mode_of <file> prints the file's permissions as ls shows them.
mode_of() {
  ls -ld "$1" | cut -c1-10
}

# kubectl_reaches_only <file> makes kubectl work only for the cluster in <file>,
# the way a user who has selected another context sees it: any call that reads
# a different kubeconfig fails.
kubectl_reaches_only() {
  stub_script kubectl <<STUB
if [ "\${KUBECONFIG:-}" != "$1" ]; then
  echo "kubectl was not pointed at the lab: KUBECONFIG=\${KUBECONFIG:-<unset>}" >&2
  exit 1
fi
STUB
}

@test "k3d up writes the lab's kubeconfig when it creates the cluster" {
  stub_when k3d "cluster list" 1 # not found
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  assert_called k3d "kubeconfig get testcluster"
  [ -s "$LAB_FILE" ]
  [ "$(mode_of "$LAB_FILE")" = "-rw-------" ]
}

@test "k3d up writes the lab's kubeconfig for a cluster that already exists" {
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  refute_called k3d "cluster create"
  [ -s "$LAB_FILE" ]
  [ "$(mode_of "$LAB_FILE")" = "-rw-------" ]
}

@test "k3d up points the lab's kubeconfig at 127.0.0.1, not 0.0.0.0" {
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  grep -q 'https://127.0.0.1:6443' "$LAB_FILE"
  ! grep -q '0\.0\.0\.0' "$LAB_FILE"
}

@test "k3d up writes the kubeconfig where labctl says to" {
  export SNOWOPS_LAB_KUBECONFIG="$STUB_DIR/elsewhere/kubeconfig"
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  [ -s "$SNOWOPS_LAB_KUBECONFIG" ]
  [ ! -e "$LAB_FILE" ]
}

@test "k3d up fails and says so when k3d cannot print the kubeconfig" {
  : >"$STUB_DIR/spec/k3d/rules" # the first matching rule wins, so drop setup's
  stub_when k3d "kubeconfig get" 1
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -ne 0 ]
  [ ! -e "$LAB_FILE" ]
  [[ "$output" == *"could not write the lab's kubeconfig"* ]] || false
}

@test "k3d up merges the context into the user's kubeconfig, not the lab's" {
  export SNOWOPS_USER_KUBECONFIG="$STUB_DIR/user-kubeconfig"
  export KUBECONFIG="$LAB_FILE"
  stub_script k3d <<'STUB'
case "$*" in
  "kubeconfig merge"*) echo "merge into ${KUBECONFIG:-<default>}" >>"$STUB_DIR/merges" ;;
  "kubeconfig get"*) echo "    server: https://127.0.0.1:6443" ;;
esac
exit 0
STUB
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  [ "$(cat "$STUB_DIR/merges")" = "merge into $STUB_DIR/user-kubeconfig" ]
}

@test "k3d up reaches the cluster through the lab's kubeconfig when the user has another context selected" {
  # Started by hand with the user's own KUBECONFIG, as a colleague's would be.
  export KUBECONFIG="$STUB_DIR/colleague-kubeconfig"
  kubectl_reaches_only "$LAB_FILE"
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
  [[ "$output" == *"is healthy; skipping creation"* ]] || false
}

@test "k3d up run by labctl reaches the cluster through the lab's kubeconfig" {
  export KUBECONFIG="$LAB_FILE"
  export SNOWOPS_LAB_KUBECONFIG="$LAB_FILE"
  export SNOWOPS_USER_KUBECONFIG="$STUB_DIR/colleague-kubeconfig"
  kubectl_reaches_only "$LAB_FILE"
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -eq 0 ]
}

@test "k3d up cannot reach the cluster through a kubeconfig that is not the lab's" {
  # The control for the two tests above: a kubectl that rejects the lab's file
  # makes up.sh fail, so those tests pass because of the file and not by luck.
  kubectl_reaches_only "$STUB_DIR/some-other-file"
  run bash "$ROOT/runtimes/k3d/up.sh" testcluster
  [ "$status" -ne 0 ]
}

@test "k3d down removes the lab's kubeconfig and deletes through the user's" {
  mkdir -p "$(dirname "$LAB_FILE")"
  echo "kubeconfig" >"$LAB_FILE"
  export SNOWOPS_USER_KUBECONFIG="$STUB_DIR/user-kubeconfig"
  export KUBECONFIG="$LAB_FILE"
  stub_script k3d <<'STUB'
case "$*" in
  "cluster list"*) exit 0 ;;
  "cluster delete"*) echo "delete from ${KUBECONFIG:-<default>}" >>"$STUB_DIR/deletes" ;;
esac
exit 0
STUB
  run bash "$ROOT/runtimes/k3d/down.sh" testcluster
  [ ! -e "$LAB_FILE" ]
  [ "$(cat "$STUB_DIR/deletes")" = "delete from $STUB_DIR/user-kubeconfig" ]
}

@test "kind up writes the lab's kubeconfig when it creates the cluster" {
  stub_when k3d "cluster list" 1 # no k3d cluster holds the name
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -eq 0 ]
  assert_called kind "create cluster"
  assert_called kind "get kubeconfig --name testcluster"
  [ -s "$LAB_FILE" ]
  [ "$(mode_of "$LAB_FILE")" = "-rw-------" ]
}

@test "kind up writes the lab's kubeconfig for a cluster that already exists" {
  stub_stdout kind "testcluster" # `get clusters` lists it
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -eq 0 ]
  refute_called kind "create cluster"
  [ -s "$LAB_FILE" ]
}

@test "kind up reaches the cluster through the lab's kubeconfig when the user has another context selected" {
  stub_stdout kind "testcluster"
  export KUBECONFIG="$STUB_DIR/colleague-kubeconfig"
  kubectl_reaches_only "$LAB_FILE"
  run bash "$ROOT/runtimes/kind/up.sh" testcluster
  [ "$status" -eq 0 ]
}

@test "kind down removes the lab's kubeconfig" {
  mkdir -p "$(dirname "$LAB_FILE")"
  echo "kubeconfig" >"$LAB_FILE"
  stub_stdout kind "testcluster"
  run bash "$ROOT/runtimes/kind/down.sh" testcluster
  [ "$status" -eq 0 ]
  [ ! -e "$LAB_FILE" ]
  assert_called kind "delete cluster --name testcluster"
}
