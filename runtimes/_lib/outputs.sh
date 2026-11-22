#!/usr/bin/env bash
# Shared helpers for a provider's outputs file. Sourced by the runtime scripts
# after docker.sh and kubeconfig.sh.
#
# outputs.env sits in the lab's state directory and says what the provider
# produced, so later steps read it instead of guessing. The ingress ports and
# domain suffix are not stored twice: they come from the cluster record that
# docker.sh keeps (<SNOWOPS_HOME>/clusters/<cluster>.env).

# lab_outputs_file <cluster> - where the provider records its outputs.
lab_outputs_file() {
  printf '%s/outputs.env' "$(lab_state_dir "$1")"
}

# write_outputs <cluster> <profile> <kubeconfig> <context> - record the
# runtime profile that owns the lab, KUBECONFIG and KUBE_CONTEXT (left out when
# empty, as for incluster), the ingress ports and the domain suffix. Values that
# are not known are left out. Fails, leaving any earlier file alone, when the
# state directory cannot be written.
write_outputs() {
  local cluster="$1" profile="$2" kubeconfig="$3" context="$4" file tmp http https suffix
  file="$(lab_outputs_file "$cluster")"
  http="$(record_value "$cluster" HTTP_PORT)"
  https="$(record_value "$cluster" HTTPS_PORT)"
  suffix="$(record_value "$cluster" DOMAIN_SUFFIX)"
  [ -n "$suffix" ] || suffix="${DOMAIN_SUFFIX:-}"
  mkdir -p "$(dirname "$file")" || return 1
  tmp="${file}.tmp.$$"
  {
    printf 'PROFILE=%s\n' "$profile"
    [ -z "$kubeconfig" ] || printf 'KUBECONFIG=%s\n' "$kubeconfig"
    [ -z "$context" ] || printf 'KUBE_CONTEXT=%s\n' "$context"
    [ -z "$http" ] || printf 'HTTP_PORT=%s\n' "$http"
    [ -z "$https" ] || printf 'HTTPS_PORT=%s\n' "$https"
    [ -z "$suffix" ] || printf 'DOMAIN_SUFFIX=%s\n' "$suffix"
  } >"$tmp" && mv "$tmp" "$file" || {
    rm -f "$tmp"
    return 1
  }
}

# refresh_outputs <cluster> - rewrite the ports and domain suffix in an existing
# outputs file after the cluster record changed, keeping its kubeconfig and
# context. A lab without an outputs file is left alone: its up.sh writes one.
refresh_outputs() {
  local file profile kubeconfig context
  file="$(lab_outputs_file "$1")"
  [ -f "$file" ] || return 0
  profile="$(sed -n 's/^PROFILE=//p' "$file" | tail -n 1)"
  kubeconfig="$(sed -n 's/^KUBECONFIG=//p' "$file" | tail -n 1)"
  context="$(sed -n 's/^KUBE_CONTEXT=//p' "$file" | tail -n 1)"
  write_outputs "$1" "$profile" "$kubeconfig" "$context"
}

# outputs_profile <cluster> - the runtime profile that wrote the outputs, empty
# when there are none or they predate the PROFILE key.
outputs_profile() {
  local file
  file="$(lab_outputs_file "$1")"
  [ -f "$file" ] || return 0
  sed -n 's/^PROFILE=//p' "$file" | tail -n 1
}

# forget_outputs <cluster> - remove the outputs with the cluster they describe.
forget_outputs() {
  rm -f "$(lab_outputs_file "$1")"
}

# forget_lab_state <cluster> <profile> - drop the recorded ports, the lab's
# kubeconfig and the outputs once <profile> has removed its cluster. State that
# another runtime profile wrote belongs to that runtime's cluster and stays.
forget_lab_state() {
  local owner
  owner="$(outputs_profile "$1")"
  if [ -n "$owner" ] && [ "$owner" != "$2" ]; then
    return 0
  fi
  forget_cluster "$1"
  forget_lab_kubeconfig "$1"
  forget_outputs "$1"
}

# owns_lab_state <cluster> <profile> - succeed when the outputs name <profile>
# as the runtime that wrote them.
owns_lab_state() {
  [ "$(outputs_profile "$1")" = "$2" ]
}
