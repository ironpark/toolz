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

The suite is intentionally red while distributed ownership, Capacity, Writer
backpressure, and the load client remain unimplemented. Compile all tests
without running them with:

```sh
go test -run '^$' ./...
```

Run the currently implemented configuration, query, and operations baseline:

```sh
go test -run '^(TestLoadConfig|TestEnvironmentVariableInventory|TestConfig|TestParseConnection|TestOperationsHealthIsAlwaysLive|TestOperationsReadyRefusesNewWorkDuringDrain|TestOperationsMetricsExposeStablePrometheusSurface|TestOperationsReadyWaitsForMinimumClusterSize|TestOperationsUnknownPathReturnsNotFound)$' ./...
```

The 100 upstream ports are supplemented by 24 Go regression tests, for 124
ordinary tests in total. `production_contract_test.go` adds real socket/state
coverage for readiness at capacity, the complete metrics surface, session and
buffer cleanup, handshake role boundaries, data-attach expiry, ingress budget
enforcement, rejection accounting, and the control read limit.

Internal scheduler, process-death, memory-pressure, multi-node, and load cases
still enter through the same-package scenario boundary. That boundary now
returns an explicit error instead of fabricated success values. Those tests
remain red until each scenario is backed by real sockets, state transitions,
and fault injection. Their assertions continue to define the required public
close codes, forwarding order, ownership cardinality, capacity gauges, and
cleanup behavior; do not skip or weaken them.

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
