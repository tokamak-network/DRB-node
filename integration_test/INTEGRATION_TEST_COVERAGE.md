# Integration Test Coverage

## Test Files

| File | Tests | Purpose |
|------|-------|---------|
| `helpers_test.go` | - | Shared utilities and constants |
| `integration_node_failure_test.go` | 4 | Node crash and recovery |
| `integration_stress_test.go` | 3 | Multi-round stress testing |
| `integration_performance_test.go` | 4 + 2 benchmarks | Performance and load testing |

## Node Failure Tests

| Test | Description |
|------|-------------|
| `TestIntegrationNodeCrashDuringRound` | Kills a regular node mid-round, verifies system completes |
| `TestIntegrationLeaderNodeCrash` | Stops/restarts leader, verifies recovery |
| `TestIntegrationMultipleNodeFailures` | Kills 2 of 3 regular nodes |
| `TestIntegrationRecoveryFromHaltedState` | Verifies recovery from HALTED state |

## Stress Tests

| Test | Description |
|------|-------------|
| `TestIntegrationMultipleConsecutiveRounds` | Runs 3 rounds back-to-back |
| `TestIntegrationSystemStability` | Monitors state transitions |
| `TestIntegrationRoundCleanup` | Verifies cleanup between rounds |

## Performance Tests

| Test | Description |
|------|-------------|
| `TestIntegrationRequestLatency` | Measures request submission time |
| `TestIntegrationConcurrentStateReads` | 50 concurrent contract reads |
| `TestIntegrationRapidStateQueries` | 200 rapid sequential queries |
| `TestIntegrationSystemStabilityUnderLoad` | 2-minute health monitoring |
| `BenchmarkStateRead` | Single read throughput |
| `BenchmarkConcurrentStateRead` | Parallel read throughput |

## Running Tests

```bash
# Start geth 1.14+ first
geth --dev --dev.period 1 --ws --ws.port 8546 --ws.origins "*"

# Run all
go test -v -timeout 60m ./integration_test/...

# Run by category
go test -v -timeout 30m ./integration_test/... -run "NodeCrash|LeaderNode|MultipleNode|Recovery"
go test -v -timeout 30m ./integration_test/... -run "Consecutive|Stability|Cleanup"
go test -v -timeout 30m ./integration_test/... -run "Latency|Concurrent|Rapid|UnderLoad"
```
