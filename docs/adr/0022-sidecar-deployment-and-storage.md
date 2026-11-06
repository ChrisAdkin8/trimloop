# ADR 0022: Sidecar deployment, storage and the local single-writer rule

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0016](0016-m1-sidecar-operation.md)

## Context

The third cold review found:

1. **Storage across replicas.** The sidecar runs inside the LiteLLM
   Deployment. A single ReadWriteOnce volume cannot be mounted by replicas on
   different nodes, and DuckDB allows one read-write process per file. Each
   replica also has its own findings page, so one page shows partial
   findings.
2. **Readiness.** In Kubernetes a pod is ready only when every container is.
   A failing sidecar readiness probe, or a crash loop, would take LiteLLM out
   of its Service — the opposite of failing open.
3. **Compose restarts.** A container using `network_mode: "service:litellm"`
   loses its network when LiteLLM restarts.
4. **Findings port.** The findings API listened on `0.0.0.0` with an empty
   token by default, over plain HTTP.
5. **One file, many writers.** `analyse`, the local web UI and a supervised
   proxy would all write `~/.trimloop/trimloop.duckdb`; DuckDB allows one
   writer. The pre-migration backup copied the file without checkpointing.

## Decision

1. **Per-pod storage.** Each sidecar has its own store file on a per-pod
   volume: an `emptyDir` by default (findings survive container restarts, not
   pod replacement), or a per-pod persistent volume in `litellm-bundle` mode,
   which deploys LiteLLM as a StatefulSet. No volume is shared between
   replicas.
2. **M1 output is per replica**, and says so. Each sidecar's findings page
   and API cover that replica only. `trimloop report --from` accepts several
   endpoints (or a Kubernetes label selector) and merges their findings into
   one report. The webhook sink sends from each replica, labelled with the
   replica. A single combined view is the team collector's job (M2).
3. **Probes.** The sidecar has a **liveness** probe only, never a readiness
   probe, and runs as a Kubernetes native sidecar (an init container with
   `restartPolicy: Always`) where the cluster supports it, so its state never
   affects LiteLLM's readiness. The M1 fail-open test runs on a real cluster:
   kill, crash-loop and starve the sidecar while traffic flows.
4. **Compose.** The example sets `restart: unless-stopped` and
   `depends_on: { litellm: { condition: service_started, restart: true } }`
   on the sidecar, so it restarts with LiteLLM and rejoins its network.
5. **Findings port security.** The findings listener refuses to start
   without a token. TLS is terminated by an ingress or provided with
   `findings.tls` (certificate and key from a secret); the Helm chart does not
   expose the port outside the cluster unless asked.
6. **Single writer locally.** One process owns the local store:
   `trimloop analyse` when run from the command line, or the long-running
   `trimloop` daemon (local UI, proxy) when it is running, in which case
   `analyse` hands its work to the daemon. Other processes open the store
   read-only. Before a migration backup, the owner runs `CHECKPOINT`, then
   copies the file.

## Consequences

- M1 findings for a multi-replica gateway are complete only when the report
  merges every replica, or when a collector is configured (M2).
- `emptyDir` storage loses findings when a pod is replaced; teams that need
  durable M1 findings use `litellm-bundle` mode or a collector.
- The local daemon becomes the writer whenever it runs.
