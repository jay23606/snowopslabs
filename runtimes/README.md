# Runtimes

A runtime profile is a directory here that brings up one **target provider**.
A lab is a set of named targets, and the Kubernetes cluster is the default one,
named `k8s`. `k3d`, `kind` and `incluster` are the providers of `k8s`. Later
targets (a Git forge, a state store) follow the same contract, which is what
lets labctl treat them alike. See
[ADR-0017](../docs/adr/0017-tracks-and-targets.md) for why, and
[runtime profiles](../docs/runtime-profiles.md) for how to choose and write one.

## The provider contract

A provider directory `runtimes/<name>/` holds:

| File | Contract |
|---|---|
| `up.sh` | Bring the target up. Idempotent: if it already exists, make sure it is healthy and exit 0. Writes `outputs.env` last, once the target is ready. |
| `down.sh` | Remove the target, and its outputs once it is gone. Idempotent: exit 0 when it is already gone. State that another provider wrote (the `PROFILE` in `outputs.env` names it) is left alone, because the state directory is per cluster and shared by providers. |
| `status.sh` | Report whether the target is ready. Exit 0 when it is. Otherwise exit non-zero with one line on stderr that says why, naming Docker when the tool cannot list clusters instead of claiming the cluster is missing. Never changes anything and returns within a few seconds; labctl gives up after 20 seconds and reports that the check did not answer. |
| `runtime.env` | Defaults for the provider, as `KEY=value` lines. |

Each script takes the lab's cluster name as its first argument and reads the
rest of its settings from the environment. labctl passes:

| Variable | Meaning |
|---|---|
| `CLUSTER_NAME` | The lab's name. |
| `LAB_STATE_DIR` | `~/.snowops/state/<cluster>`, where the lab's state lives. |
| `SNOWOPS_LAB_KUBECONFIG` | The lab's own kubeconfig, inside `LAB_STATE_DIR`. Not set for `incluster`. |
| `SNOWOPS_USER_KUBECONFIG` | The kubeconfig the user started labctl with, kept for merging the lab's context into it. |

By hand, none of these are set, so the scripts derive the state directory from
`SNOWOPS_HOME` and the cluster name.

## The outputs file

`up.sh` writes `$LAB_STATE_DIR/outputs.env`. It is the only way later steps
learn what the provider produced, so nothing reads a guess instead.

| Key | Meaning | Required |
|---|---|---|
| `PROFILE` | The provider that wrote the file. `down.sh` removes only state its own provider wrote. | written by every provider |
| `KUBECONFIG` | Path of the kubeconfig that reaches the cluster. It holds one context. | every provider but `incluster` |
| `KUBE_CONTEXT` | That context's name. | every provider but `incluster` |
| `HTTP_PORT`, `HTTPS_PORT` | The host ports of the cluster's ingress. | no |
| `DOMAIN_SUFFIX` | The suffix of the cluster's ingress hostnames. | no |

The ports and suffix are not a second source of truth. They come from the
cluster record in `~/.snowops/clusters/<cluster>.env` that `_lib/docker.sh`
keeps, and `outputs.env` is rewritten whenever that record changes
(`move-ingress.sh`, `_lib/move-domain.sh`). `internal/target` reads and
validates the file: a missing required key, or a port that is not a number,
is an error that says to run `labctl init` again.

`incluster` writes the file as a convenience: when its state directory is not
writable, `up.sh` warns once and carries on, and `down.sh` removes the file it
wrote.

A run that uses the target gets these values in its environment, read from
`outputs.env` when it is valid (a lab with none yet gets the values labctl
computes from `.env`): `KUBECONFIG`
and `SNOWOPS_LAB_KUBECONFIG` (the file above), `HTTP_PORT`, `HTTPS_PORT` and
`DOMAIN_SUFFIX`. Content picks the target with `target:` and says it needs it
with `requires:` ([Targets](../docs/reference/scenario-schema.md#targets-requires-and-target)).

## Helpers

`_lib/` holds what providers share. They are sourced, never run.

| File | Holds |
|---|---|
| `docker.sh` | The Docker engine and the cluster record. |
| `kubeconfig.sh` | Writing the lab's kubeconfig, and running a command against the user's own. |
| `outputs.sh` | Writing, refreshing and removing `outputs.env`. |
| `move-domain.sh` | Moving a lab's hostnames to another domain suffix. |

## Tests

`src/test/shell/runtime_target_provider.bats` runs each provider's `status.sh`
ready and not ready, and checks that each `up.sh` writes `outputs.env`.
`runtime_kubeconfig.bats` covers the lab's kubeconfig, and
`runtime_lifecycle.bats` the idempotency of `up.sh` and `down.sh`.
