#!/usr/bin/env bats
# SPDX-License-Identifier: Apache-2.0
#
# scripts/release-acceptance.sh walks a live lab and reports each step. These
# tests stub labctl and kubectl to pin the harness contract: every step runs
# and is reported, a failure fails the run, an injected fault is always
# resolved, even when its detection check never fires, and a lab that `init`
# just built is walked without false failures.

load 'helpers/stub'

setup() {
  stub_setup
  stub_command labctl kubectl
  ROOT="$(project_root)"
  export RA_OUT="$STUB_DIR/out" LABCTL="$STUB_BIN/labctl" RA_WAIT=0
}

teardown() {
  stub_teardown
}

@test "--list prints every step" {
  run bash "$ROOT/scripts/release-acceptance.sh" --list
  [ "$status" -eq 0 ]
  [[ "$output" == *"init"* ]] || false
  [[ "$output" == *"incident"* ]] || false
}

@test "an unknown step is refused before anything runs" {
  run bash "$ROOT/scripts/release-acceptance.sh" --only init,nope
  [ "$status" -eq 2 ]
  [[ "$output" == *"unknown step: nope"* ]] || false
  refute_called labctl
}

@test "a failing step fails the run, and later steps still run and are reported" {
  stub_when labctl "doctor" 0 "✗ Docker is not running"
  stub_when labctl "validate" 0 "✓ all content valid"
  run bash "$ROOT/scripts/release-acceptance.sh" --only doctor,content
  [ "$status" -ne 0 ]
  grep -q "| doctor | FAIL |" "$RA_OUT/report.md"
  grep -q "| content | PASS |" "$RA_OUT/report.md"
}

@test "the incident is resolved even when its detection check never fires" {
  stub_when labctl "incident status" 0 "No incident is active."
  run bash "$ROOT/scripts/release-acceptance.sh" --only incident
  [ "$status" -ne 0 ]
  assert_called labctl "incident inject crashloop-bad-config"
  assert_called labctl "incident resolve"
  grep -q "never fired" "$RA_OUT/incident.log"
}

@test "an incident already active is left alone" {
  stub_when labctl "incident status" 0 "NOT RESOLVED - detection check fails"
  run bash "$ROOT/scripts/release-acceptance.sh" --only incident
  [ "$status" -eq 0 ]
  refute_called labctl "incident inject"
  grep -q "| incident | SKIP |" "$RA_OUT/report.md"
}

@test "an incident whose app is not deployed is skipped with the reason" {
  stub_when labctl "incident status" 0 "No incident is active."
  stub_when labctl "incident inject" 1 "required app(s) not deployed: go-api"
  run bash "$ROOT/scripts/release-acceptance.sh" --only incident
  [ "$status" -eq 0 ]
  refute_called labctl "incident resolve"
  grep -q "| incident | SKIP |" "$RA_OUT/report.md"
  grep -q "required app(s) not deployed: go-api" "$RA_OUT/incident.log"
  grep -q "rerun with --only incident" "$RA_OUT/incident.log"
}

@test "apps skips when no app is deployed" {
  stub_when labctl "status" 0 "Context:  k3d-snowops"
  run bash "$ROOT/scripts/release-acceptance.sh" --only apps
  [ "$status" -eq 0 ]
  grep -q "| apps | SKIP |" "$RA_OUT/report.md"
  grep -q "no app is deployed" "$RA_OUT/apps.log"
}

@test "namespaces that init creates are not counted as left behind" {
  stub_script kubectl <<EOF
printf 'default\nkube-system\n'
[ -f "$STUB_DIR/init-ran" ] && printf 'monitoring\ntraefik\n'
exit 0
EOF
  stub_script labctl <<EOF
case "\$*" in
  init) touch "$STUB_DIR/init-ran"; echo "=== Lab is up ===" ;;
esac
exit 0
EOF
  run bash "$ROOT/scripts/release-acceptance.sh" --only init,leaks
  [ "$status" -eq 0 ]
  grep -q "| init | PASS |" "$RA_OUT/report.md"
  grep -q "| leaks | PASS |" "$RA_OUT/report.md"
}

@test "a namespace a later step leaves behind still fails leaks" {
  stub_script kubectl <<EOF
printf 'default\nkube-system\n'
[ -f "$STUB_DIR/init-ran" ] && printf 'monitoring\n'
[ -f "$STUB_DIR/up-ran" ] && printf 'opencost\n'
exit 0
EOF
  stub_script labctl <<EOF
case "\$*" in
  init) touch "$STUB_DIR/init-ran"; echo "=== Lab is up ===" ;;
  "scenario up"*) touch "$STUB_DIR/up-ran" ;;
esac
exit 0
EOF
  RA_SCENARIO=cost-right-sizing run bash "$ROOT/scripts/release-acceptance.sh" --only init,scenarios,leaks
  [ "$status" -ne 0 ]
  grep -q "| leaks | FAIL |" "$RA_OUT/report.md"
  grep -qx "opencost" "$RA_OUT/leaks.log"
  run grep -qx "monitoring" "$RA_OUT/leaks.log"
  [ "$status" -ne 0 ]
}

@test "a cycled scenario gets its prerequisites, and what it installed is removed after" {
  stub_script labctl <<EOF
case "\$*" in
  "scenario list") printf 'NAME  DISPLAY NAME  STATUS\ncost-right-sizing  Cost  inactive\n' ;;
  "scenario up"*) touch "$STUB_DIR/up-ran" ;;
  "scenario verify"*) echo "pending" ;;
  "platform status")
    printf 'COMPONENT       STATE       LAST RUN\ningress/traefik installed   -\n'
    if [ -f "$STUB_DIR/up-ran" ]; then
      echo "cost/opencost   installed   run-1"
    else
      echo "cost/opencost   not installed   -"
    fi
    ;;
  status)
    echo "Context:  k3d-snowops"
    [ -f "$STUB_DIR/up-ran" ] && echo "  go-api               replicas=1 ready=1"
    ;;
esac
exit 0
EOF
  RA_SCENARIO=cost-right-sizing run bash "$ROOT/scripts/release-acceptance.sh" --only scenarios
  [ "$status" -eq 0 ]
  assert_called labctl "scenario up cost-right-sizing --deploy-prereqs"
  assert_called labctl "scenario down cost-right-sizing"
  assert_called labctl "app destroy go-api"
  assert_called labctl "platform down cost/opencost"
  refute_called labctl "platform down ingress/traefik"
}
