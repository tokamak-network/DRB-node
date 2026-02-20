package integration_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationRequestLatency measures the time taken to submit random number requests.
// Validates: request submission completes within acceptable latency.
func TestIntegrationRequestLatency(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	const numSamples = 3
	var totalLatency time.Duration
	var successCount int

	for i := 0; i < numSamples; i++ {
		// Wait for system to be ready
		state, err := getState(ctx, geth)
		if err != nil || !state.IsOperational() {
			t.Logf("Sample %d: waiting for operational state (current: %v)", i+1, state)
			time.Sleep(ContainerRestartWait)
		}

		start := time.Now()
		round, err := requestRandomNumberFromConsumer(ctx, geth)
		latency := time.Since(start)

		if err != nil {
			t.Logf("Sample %d: request failed: %v", i+1, err)
			time.Sleep(ContainerRestartWait)
			continue
		}

		successCount++
		totalLatency += latency
		t.Logf("Sample %d: request latency = %v (round: %s)", i+1, latency, round.String())

		// Assert individual request is within threshold
		assert.Less(t, latency, MaxRequestLatency,
			"request latency should be under %v, got %v", MaxRequestLatency, latency)

		// Wait for round to complete before next sample
		waitForRoundFulfillment(ctx, t, geth, round, 10)
		time.Sleep(ShortPollInterval)
	}

	require.Greater(t, successCount, 0, "at least one request should succeed")

	avgLatency := totalLatency / time.Duration(successCount)
	t.Logf("=== Latency Summary ===")
	t.Logf("  Samples: %d/%d successful", successCount, numSamples)
	t.Logf("  Average latency: %v", avgLatency)
	t.Logf("  Threshold: %v", MaxRequestLatency)

	assert.Less(t, avgLatency, MaxRequestLatency, "average latency should be under threshold")
}

// TestIntegrationConcurrentStateReads verifies the system handles concurrent read operations.
// Validates: high success rate under concurrent load, no crashes.
func TestIntegrationConcurrentStateReads(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	t.Logf("Starting %d concurrent state reads...", ConcurrentReaders)
	start := time.Now()

	for i := 0; i < ConcurrentReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := getContractState(ctx, geth)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
			} else {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	total := successCount + errorCount
	successRate := float64(successCount) / float64(total)

	t.Logf("=== Concurrent Read Results ===")
	t.Logf("  Total requests: %d", total)
	t.Logf("  Successful: %d (%.1f%%)", successCount, successRate*100)
	t.Logf("  Failed: %d (%.1f%%)", errorCount, (1-successRate)*100)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.1f req/sec", float64(total)/duration.Seconds())

	// Require high success rate (90%+)
	assert.GreaterOrEqual(t, successRate, MinSuccessRate,
		"success rate should be at least %.0f%%, got %.1f%%", MinSuccessRate*100, successRate*100)

	// Verify system is still healthy
	require.NoError(t, verifyAllContainersRunning(t), "all containers should survive concurrent load")
}

// TestIntegrationRapidStateQueries tests system stability under rapid sequential queries.
// Validates: system handles rapid queries without degradation.
func TestIntegrationRapidStateQueries(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	var successCount int
	var lastState ContractState

	t.Logf("Executing %d rapid state queries...", ResourceExhaustionOps)
	start := time.Now()

	for i := 0; i < ResourceExhaustionOps; i++ {
		state, err := getState(ctx, geth)
		if err == nil {
			successCount++
			lastState = state
		}
	}

	duration := time.Since(start)
	successRate := float64(successCount) / float64(ResourceExhaustionOps)
	qps := float64(successCount) / duration.Seconds()

	t.Logf("=== Rapid Query Results ===")
	t.Logf("  Total queries: %d", ResourceExhaustionOps)
	t.Logf("  Successful: %d (%.1f%%)", successCount, successRate*100)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.1f queries/sec", qps)
	t.Logf("  Last state: %s", lastState)

	// High success rate required
	assert.GreaterOrEqual(t, successRate, MinSuccessRate,
		"success rate should be at least %.0f%%", MinSuccessRate*100)

	// Verify all nodes survived the load
	require.NoError(t, verifyAllContainersRunning(t), "all containers should survive rapid queries")
}

// TestIntegrationSystemStabilityUnderLoad monitors system health during sustained operations.
// Validates: containers remain healthy, state remains consistent.
func TestIntegrationSystemStabilityUnderLoad(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	const (
		monitorDuration = 2 * time.Minute
		checkInterval   = 15 * time.Second
	)

	t.Logf("Monitoring system stability for %v...", monitorDuration)

	startTime := time.Now()
	var checks int
	var healthyChecks int
	var stateErrors int

	for time.Since(startTime) < monitorDuration {
		checks++

		// Check container health
		allHealthy := true
		for _, container := range TestContainers {
			running, err := isContainerRunning(container)
			if err != nil || !running {
				t.Logf("Check %d: %s unhealthy", checks, container)
				allHealthy = false
			}
		}

		// Check state readability
		state, err := getState(ctx, geth)
		if err != nil {
			stateErrors++
			t.Logf("Check %d: state read failed: %v", checks, err)
		} else {
			t.Logf("Check %d: state = %s, containers healthy = %v", checks, state, allHealthy)
		}

		if allHealthy && err == nil {
			healthyChecks++
		}

		time.Sleep(checkInterval)
	}

	healthRate := float64(healthyChecks) / float64(checks)

	t.Logf("=== Stability Summary ===")
	t.Logf("  Total checks: %d", checks)
	t.Logf("  Healthy checks: %d (%.1f%%)", healthyChecks, healthRate*100)
	t.Logf("  State read errors: %d", stateErrors)
	t.Logf("  Duration: %v", time.Since(startTime))

	// All checks should be healthy
	assert.Equal(t, checks, healthyChecks, "all health checks should pass")
	assert.Zero(t, stateErrors, "should have no state read errors")
}

// BenchmarkStateRead provides a formal Go benchmark for state read operations.
func BenchmarkStateRead(b *testing.B) {
	if testEnv == nil || testEnv.Geth == nil {
		b.Skip("test environment not initialized")
	}

	geth := testEnv.Geth
	ctx := testCtx

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = getContractState(ctx, geth)
	}
}

// BenchmarkConcurrentStateRead benchmarks concurrent state reads.
func BenchmarkConcurrentStateRead(b *testing.B) {
	if testEnv == nil || testEnv.Geth == nil {
		b.Skip("test environment not initialized")
	}

	geth := testEnv.Geth
	ctx := testCtx

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = getContractState(ctx, geth)
		}
	})
}
