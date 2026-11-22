#!/usr/bin/env bash
# Shared helpers for the lab's own kubeconfig. Sourced by the runtime scripts.
#
# labctl passes SNOWOPS_LAB_KUBECONFIG (the lab's kubeconfig in its state
# directory) and LAB_STATE_DIR. When a script is run by hand, both are derived
# from SNOWOPS_HOME and the cluster name the way internal/config.StateDir does.

# lab_state_dir <cluster> - the directory that holds the lab's state.
lab_state_dir() {
  printf '%s' "${LAB_STATE_DIR:-${SNOWOPS_HOME:-$HOME/.snowops}/state/$1}"
}

# lab_kubeconfig_file <cluster> - where the lab's own kubeconfig lives.
lab_kubeconfig_file() {
  printf '%s' "${SNOWOPS_LAB_KUBECONFIG:-$(lab_state_dir "$1")/kubeconfig}"
}

# write_lab_kubeconfig <cluster> <command>... - run the command, which prints a
# kubeconfig, and keep its output as the lab's kubeconfig with mode 0600. A
# server address of 0.0.0.0, which k3d writes, becomes 127.0.0.1: 0.0.0.0 is a
# bind-all address and connecting to it stalls on macOS.
write_lab_kubeconfig() {
  local cluster="$1" file tmp
  shift
  file="$(lab_kubeconfig_file "$cluster")"
  mkdir -p "$(dirname "$file")"
  tmp="${file}.tmp.$$"
  if ! (umask 077 && "$@" | sed 's/0\.0\.0\.0/127.0.0.1/g' >"$tmp") || [ ! -s "$tmp" ]; then
    rm -f "$tmp"
    echo "ERROR: could not write the lab's kubeconfig ($file)." >&2
    return 1
  fi
  chmod 600 "$tmp"
  mv "$tmp" "$file"
}

# use_lab_kubeconfig <cluster> - point every kubectl and helm call in this
# script at the lab's kubeconfig.
use_lab_kubeconfig() {
  KUBECONFIG="$(lab_kubeconfig_file "$1")"
  export KUBECONFIG
}

# with_user_kubeconfig <command>... - run the command against the user's own
# kubeconfig, for merging the lab's context into it so plain kubectl keeps
# working. labctl records that kubeconfig in SNOWOPS_USER_KUBECONFIG; an empty
# one means kubectl's default.
with_user_kubeconfig() {
  (
    if [ "${SNOWOPS_USER_KUBECONFIG+set}" = set ]; then
      if [ -n "$SNOWOPS_USER_KUBECONFIG" ]; then
        export KUBECONFIG="$SNOWOPS_USER_KUBECONFIG"
      else
        unset KUBECONFIG
      fi
    fi
    "$@"
  )
}

# forget_lab_kubeconfig <cluster> - remove the lab's kubeconfig, which goes
# with the cluster it reaches.
forget_lab_kubeconfig() {
  rm -f "$(lab_kubeconfig_file "$1")"
}
