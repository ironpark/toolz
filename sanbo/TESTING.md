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

The suite is intentionally red while relay ownership, forwarding, Capacity,
Writer backpressure, and the load client remain unimplemented. Compile all
tests without running them with:

```sh
go test -run '^$' ./...
```

Run the currently implemented configuration, query, and operations baseline:

```sh
go test -run '^(TestLoadConfig|TestEnvironmentVariableInventory|TestConfig|TestParseConnection|TestOperationsHealthIsAlwaysLive|TestOperationsReadyRefusesNewWorkDuringDrain|TestOperationsMetricsExposeStablePrometheusSurface|TestOperationsReadyWaitsForMinimumClusterSize|TestOperationsUnknownPathReturnsNotFound)$' ./...
```

Internal scheduler, process-death, memory-pressure, and multi-node cases use a
same-package scenario controller. Each scenario still asserts public outcomes
such as WebSocket close status/reason, forwarding order, ownership cardinality,
capacity gauges, and cleanup. Implement scenarios incrementally together with
their production subsystem; do not skip or weaken them.

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
