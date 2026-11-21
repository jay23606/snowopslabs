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

# write_outputs <cluster> <kubeconfig> <context> - record KUBECONFIG and
# KUBE_CONTEXT (left out when empty, as for incluster), the ingress ports and
# the domain suffix. Values that are not known are left out.
write_outputs() {
  local cluster="$1" kubeconfig="$2" context="$3" file tmp http https suffix
  file="$(lab_outputs_file "$cluster")"
  http="$(record_value "$cluster" HTTP_PORT)"
  https="$(record_value "$cluster" HTTPS_PORT)"
  suffix="$(record_value "$cluster" DOMAIN_SUFFIX)"
  [ -n "$suffix" ] || suffix="${DOMAIN_SUFFIX:-}"
  mkdir -p "$(dirname "$file")"
  tmp="${file}.tmp.$$"
  {
    [ -z "$kubeconfig" ] || printf 'KUBECONFIG=%s\n' "$kubeconfig"
    [ -z "$context" ] || printf 'KUBE_CONTEXT=%s\n' "$context"
    [ -z "$http" ] || printf 'HTTP_PORT=%s\n' "$http"
    [ -z "$https" ] || printf 'HTTPS_PORT=%s\n' "$https"
    [ -z "$suffix" ] || printf 'DOMAIN_SUFFIX=%s\n' "$suffix"
  } >"$tmp"
  mv "$tmp" "$file"
}

# refresh_outputs <cluster> - rewrite the ports and domain suffix in an existing
# outputs file after the cluster record changed, keeping its kubeconfig and
# context. A lab without an outputs file is left alone: its up.sh writes one.
refresh_outputs() {
  local file kubeconfig context
  file="$(lab_outputs_file "$1")"
  [ -f "$file" ] || return 0
  kubeconfig="$(sed -n 's/^KUBECONFIG=//p' "$file" | tail -n 1)"
  context="$(sed -n 's/^KUBE_CONTEXT=//p' "$file" | tail -n 1)"
  write_outputs "$1" "$kubeconfig" "$context"
}

# forget_outputs <cluster> - remove the outputs with the cluster they describe.
forget_outputs() {
  rm -f "$(lab_outputs_file "$1")"
}
