# Testing

The compatibility baseline is `getpaseo/paseo-relay` commit
`3fc41c96c8c63f3a7109e832899cc57d473c4531` in the ignored
`references/paseo-relay` checkout.

The upstream suite contains 109 tests. Sanbo ports all 100 provider-neutral
tests and intentionally excludes the 9 Fly adapter tests in:

- `fly_diagnostics_test.exs`
- `fly_replay_e2e_test.exs`
- `fly_staging_gate_test.exs`

Run the suite from this directory:

```sh
go test ./...
```

Compile all tests without running them with:

```sh
go test -run '^$' ./...
```

Run the full ordinary and race-detector suites:

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

Run only the cross-process cluster contracts with:

```sh
go test -count=1 -run '^TestMultiNode' ./...
```

The 100 upstream ports are supplemented by 38 Go regression tests, for 138
ordinary tests in total. `production_contract_test.go` adds real socket/state
coverage for readiness at capacity, the complete metrics surface, session and
waiting-delivery cleanup, handshake role boundaries, data-attach expiry, ingress budget
enforcement, rejection accounting, and the control read limit.

`memory_pressure_test.go` drives the watermark sampler itself: watermark
crossing, admission closure, peer and waiting-delivery shedding with the `1013` close
code, once-per-crossing shedding, hysteresis on release, and readmission after
relief.

`capacity_reconcile_test.go` covers the ingress ledger reconciler: orphaned
reservations reclaimed with a capacity-epoch bump, in-flight and waiting
reservations left untouched, ledgers balancing after ordinary routing, and the
reconciler goroutine running on its configured interval.

Internal scheduler, process-death, memory-pressure, multi-node, and load cases
enter through a same-package scenario boundary. Every scenario opens real
HTTP/WebSocket connections or mutates an actual production capacity/ownership
state machine through a deterministic fault hook. Results are derived from
socket close frames, forwarded payloads, ownership records, and production
counters/gauges; the boundary never selects expected values by scenario name.
The assertions define the required public close codes, forwarding order,
ownership cardinality, capacity gauges, and cleanup behavior and remain active.

## Multi-process cluster tests

`multinode_integration_test.go` launches the current Go test binary as two or
three independent relay processes. The nodes share only public cluster
configuration and a host-level locked lease registry keyed by cluster query and
release cookie. Heartbeat leases drive membership and stale-owner reclamation;
atomic owner records serialize claims before WebSocket upgrade. The suite checks:

- readiness transitions as peers join and leave;
- an opaque reroute response from a non-owner process;
- exactly one winner for simultaneous claims;
- remote reclaim after the owner process exits;
- convergence of conflicting owners with a `1012` close for the loser;
- ownership visibility during a three-node distinct-server surge;
- isolation for different release cookies and cluster queries.

The helper process test is inert during an ordinary in-process run and starts a
real listener only when launched with its private test environment marker. The
backend itself has no test-binary branch and is also selected by a normally
started relay whenever cluster query, release node, and release cookie are set.

## Fuzzing

`fuzz_test.go` fuzzes the pure, attacker-reachable parsers rather than the
socket loop, so each target is fast enough to be useful:

| Target | Property under test |
| --- | --- |
| `FuzzParseConnectionQuery` | Accepted `/ws` queries always yield a routable `Connection`, and re-parsing one is a fixed point. |
| `FuzzRelayWebSocketURLRoundTrip` | Identifiers survive URL encoding unchanged on the way to the parser. |
| `FuzzValidHandshake` | Arbitrary frames never panic the read loop, and well-formed non-handshake frames stay opaque. |
| `FuzzValidHandshakeKey` | A handshake is admitted exactly when its key is a canonical, non low-order X25519 point. |
| `FuzzLoadConfig` | No environment yields a `Config` that violates the bounds `LoadConfig` advertises. |

Seed corpora run as ordinary tests under `go test ./...`. To fuzz for real, one
target at a time:

```sh
go test -run '^FuzzValidHandshake$' -fuzz '^FuzzValidHandshake$' -fuzztime 60s .
```

`FuzzValidHandshakeKey` performs a real ECDH per execution, so it explores
orders of magnitude more slowly than the others; give it a longer `-fuzztime`.
Any failing input is written to `testdata/fuzz/<Target>/` — commit it, since it
then replays as a regression case in the normal suite.
