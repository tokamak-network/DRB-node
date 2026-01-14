# Integration Test Coverage

## Overview

Comprehensive integration tests for the DRB (Distributed Random Beacon) node system. Tests run against a Docker environment with 1 leader node + 3 regular nodes.

## Test Files

| File | Tests | Purpose |
|------|-------|---------|
| `helpers_test.go` | - | Shared utilities, constants, and test helpers |
| `integration_node_failure_test.go` | 4 | Node crash and recovery scenarios |
| `integration_stress_test.go` | 3 | Multi-round stress testing |
| `integration_performance_test.go` | 4 + 2 benchmarks | Performance and load testing |
| `docker_nodes_quick_test.go` | 17 | Core protocol flow tests |

## Test Categories

### Node Failure Tests (`integration_node_failure_test.go`)

| Test | Description | Validates |
|------|-------------|-----------|
| `TestIntegrationNodeCrashDuringRound` | Kill regular node mid-round | System reaches terminal state (COMPLETED/HALTED) |
| `TestIntegrationLeaderNodeCrash` | Kill leader node, verify recovery | Leader restart enables new rounds |
| `TestIntegrationMultipleNodeFailures` | Kill 2 of 3 regular nodes | System handles quorum loss correctly |
| `TestIntegrationRecoveryFromHaltedState` | Verify post-HALTED recovery | New rounds can be requested after recovery |

### Stress Tests (`integration_stress_test.go`)

| Test | Description | Validates |
|------|-------------|-----------|
| `TestIntegrationMultipleConsecutiveRounds` | 3 rounds in sequence | Unique random numbers, 2/3 completion rate |
| `TestIntegrationSystemStability` | Monitor state transitions | Proper state machine flow |
| `TestIntegrationRoundCleanup` | Back-to-back rounds | Resource cleanup, unique round IDs |

### Performance Tests (`integration_performance_test.go`)

| Test | Description | Validates |
|------|-------------|-----------|
| `TestIntegrationRequestLatency` | Measure request submission time | < 10s latency threshold |
| `TestIntegrationConcurrentStateReads` | 50 concurrent state reads | 90%+ success rate |
| `TestIntegrationRapidStateQueries` | 200 rapid sequential queries | 90%+ success rate, system stability |
| `TestIntegrationSystemStabilityUnderLoad` | 2-minute health monitoring | 100% healthy checks |
| `BenchmarkStateRead` | Go benchmark for state reads | Throughput measurement |
| `BenchmarkConcurrentStateRead` | Parallel benchmark | Concurrent throughput |

## Configuration Constants

Defined in `helpers_test.go`:

```go
// Timeouts
DefaultPollInterval    = 10s
ShortPollInterval      = 5s
ContainerRestartWait   = 30s
PostCrashRecoveryWait  = 45s

// Retry limits
MaxFulfillmentRetries  = 15
MaxStateCheckRetries   = 20
MaxRecoveryRetries     = 10

// Performance thresholds
MinSuccessRate         = 0.9 (90%)
MaxRequestLatency      = 10s
ConcurrentReaders      = 50
ResourceExhaustionOps  = 200
```

## Contract States

```
0 = IDLE       - Ready for new round
1 = IN_PROGRESS - Round in progress
2 = COMPLETED  - Round completed successfully
3 = HALTED     - Error state, requires recovery
```

## Running Tests

```bash
# Prerequisites: Start geth in dev mode
geth --dev --http --http.api eth,net,web3 \
     --http.corsdomain "*" --ws --ws.api eth,net,web3 \
     --ws.origins "*" --ws.port 8546 --http.port 8545 &

# Run all integration tests (30-45 min)
go test -v -timeout 60m ./integration_test/... -run "TestIntegration"

# Run specific test category
go test -v -timeout 30m ./integration_test/... -run "TestIntegrationNode"
go test -v -timeout 20m ./integration_test/... -run "TestIntegrationPerformance"

# Run benchmarks
go test -bench=. -benchtime=10s ./integration_test/...
```

## Test Results

All tests pass with the following characteristics:
- Total runtime: ~30-45 minutes
- Node failure tests: Verify crash recovery
- Stress tests: Validate multi-round operation
- Performance tests: Confirm latency and throughput requirements
