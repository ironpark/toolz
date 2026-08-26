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
