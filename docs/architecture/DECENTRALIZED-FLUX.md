# Decentralized Flux routing

Status: implemented foundation, not a finished distributed control plane.

## Current versus target

| Concern | Existing local path | Added on this branch | Still missing for production multi-instance operation |
|---|---|---|---|
| Configuration | `setup` builds one `router.DeploymentRouter` from local config | `router.LiveDeploymentRouter` atomically replaces local snapshots; `router/controlplane.Replica` applies versioned shared manifests | Durable publisher and globally ordered revision allocation |
| Request execution | Each process calls providers itself | Each replica retains its own adapters, credentials, circuit breakers and request handling | Fleet-level load/health signals if cross-instance balancing is required |
| Updates | Rebuild provider on host reload | Pull signed manifests from bounded peer set; poll with `Replica.Run`; reject stale/conflicting/invalid revisions | Service discovery, persistence across restart, rollout/rollback orchestration |
| Outages | Local provider remains usable until process exits | A running replica keeps serving its last valid manifest when peers fail | Persist last-good manifest locally for cold-start survival |
| Credentials | Local config/store | Manifests contain no secrets; `runtime.NewReplicaFromState` resolves explicit local credentials | Secret rotation without replacing the host's replica reference |

Flux remains **one Go module in its own repository**. Feature-owned folders are
useful boundaries, but calling this a monorepo would be inaccurate: Rho is a
separate repository in the parent workspace.

## Data and control flow

```text
publisher with globally increasing revision (host responsibility)
             │
             ▼
      signed manifest endpoint(s)
             │ HTTPS + pinned Ed25519 key for remote peers
       ┌─────┴─────┐
       ▼           ▼
   replica A     replica B       independent data planes
   local keys    local keys      no request-time peer dependency
   router A      router B        atomic whole-snapshot replacement
       │           │
       └─── provider APIs ───┘
```

The manifest contains the model catalog, routing policy, deployment IDs, and
model mappings. It never contains credentials or provider clients. The local
resolver must be able to construct every named deployment before a revision is
published. A failed update leaves the previous route active. Calls already in
flight finish against the snapshot they loaded.

`PeerSource` queries at most 32 configured endpoints concurrently, with a
five-second request timeout and 16 MiB response cap. It chooses the highest
valid revision and rejects disagreement at that revision. Remote URLs require
HTTPS and a pinned Ed25519 signing key. Plain HTTP without signatures is
accepted only for loopback development; it is not a production deployment
mode. The host must secure the serving endpoint and signing private key.

## Host integration

Create a replica from explicit local deployment configuration via
`runtime.NewReplicaFromState(cfg)`. Call `replica.Refresh(ctx, source)` before
accepting traffic, then run `replica.Run(ctx, source, interval, reportError)`
under the host's lifecycle. The replica implements `core.Provider`; check
`replica.Revision() > 0` for readiness. `Run` keeps the last-good route active
after refresh errors, so the host should log/report those errors. A host can
instead call `Apply` with a manifest obtained from its own store or event
stream; polling is only one source strategy.

For single-instance dynamic configuration, use `router.LiveDeploymentRouter`.
It does not require a peer source or signing infrastructure. Both paths use
the same deployment router and provider interfaces.

## Honest limits

- There is **no consensus protocol**. Signatures prove who published a
  manifest; they do not establish a single writer. A deployment needs an
  external authority to allocate strictly increasing revisions and prevent
  divergent publications.
- Snapshots live in memory. On cold start without a reachable source, the
  replica cannot serve. Last-good persistence and crash recovery remain work.
- Peer URLs are static configuration. This is not automatic service discovery,
  sharding, or globally synchronized provider health.
- A successful higher-revision `Apply` builds a new router and resets its
  circuit-breaker state. The runtime constructor also rebuilds adapters;
  custom resolvers may reuse them. Frequent publication should be avoided
  until resource reuse and health-state transfer are designed and measured.
- The lower-level `FluxClient` supports instance-local custom provider
  registration, but some catalog and credential defaults remain process-wide.
  Claiming the entire module is free of globals would still be false.
- No production load or fault-injection benchmark has established a scaling
  ceiling. "Fully scalable" and "perfect" are not meaningful guarantees.

Next production steps: durable signed-manifest publisher with a single-writer
revision invariant; persisted last-good snapshot; a deterministic integration
test across real processes and TLS; operational metrics for revision lag and
failed refreshes; measured load/soak tests. Avoid adding a coordination
database to the request path.
