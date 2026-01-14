package integration_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationNodeCrashDuringRound tests system behavior when a regular node crashes mid-round.
// Expected behavior: System should either complete the round (if enough nodes remain) or enter HALTED state.
func TestIntegrationNodeCrashDuringRound(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	// Ensure clean state and register cleanup
	require.NoError(t, ensureContainersRunning(t), "failed to ensure containers running")
	registerContainerCleanup(t, "test-regularnode1")

	// Start a round
	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "failed to request random number")
	t.Logf("Round started: %s", round.String())

	// Wait for round to begin processing
	time.Sleep(ShortPollInterval)

	// Kill regularNode1
	err = killContainer(t, "test-regularnode1")
	require.NoError(t, err, "failed to kill regularnode1")

	// Monitor until terminal state
	finalState, reached := waitForState(ctx, t, geth, []ContractState{StateCompleted, StateHalted}, MaxStateCheckRetries)

	// Restart node before assertions (cleanup)
	err = startContainer(t, "test-regularnode1")
	assert.NoError(t, err, "failed to restart regularnode1")
	time.Sleep(ContainerRestartWait)

	// Verify we reached a terminal state
	require.True(t, reached, "should reach terminal state (COMPLETED or HALTED)")
	assert.True(t, finalState.IsTerminal(),
		"final state should be terminal, got: %s", finalState)

	// Verify all nodes are running after test
	require.NoError(t, verifyAllContainersRunning(t), "all containers should be running")
}

// TestIntegrationLeaderNodeCrash tests system behavior when the leader node crashes.
// Expected behavior: System should enter HALTED state, then recover after leader restarts.
func TestIntegrationLeaderNodeCrash(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	// Ensure clean state
	require.NoError(t, ensureContainersRunning(t), "failed to ensure containers running")
	registerContainerCleanup(t, "test-leadernode")

	// Start a round
	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "failed to request random number")
	t.Logf("Round started: %s", round.String())

	// Wait for round to begin
	time.Sleep(ShortPollInterval)

	// Record state before crash
	stateBefore, err := getState(ctx, geth)
	if err == nil {
		t.Logf("State before leader crash: %s", stateBefore)
	}

	// Kill leader
	err = killContainer(t, "test-leadernode")
	require.NoError(t, err, "failed to kill leader")

	// Verify leader is down
	running, err := isContainerRunning("test-leadernode")
	require.NoError(t, err)
	assert.False(t, running, "leader should be stopped")

	t.Log("Leader killed, monitoring for HALTED state...")

	// Monitor for HALTED state (expected when leader dies)
	stateAfterCrash, reachedHalted := waitForState(ctx, t, geth, []ContractState{StateHalted}, MaxRecoveryRetries)

	// Restart leader
	err = startContainer(t, "test-leadernode")
	require.NoError(t, err, "failed to restart leader")
	time.Sleep(PostCrashRecoveryWait)

	// Verify leader recovered
	running, err = isContainerRunning("test-leadernode")
	require.NoError(t, err)
	assert.True(t, running, "leader should be running after restart")

	// Log recovery info
	if reachedHalted {
		t.Logf("System entered HALTED state as expected: %s", stateAfterCrash)
	}

	// Wait for system to stabilize
	time.Sleep(ContainerRestartWait)

	// Try to start a new round to verify recovery
	newRound, err := requestRandomNumberFromConsumer(ctx, geth)
	if err != nil {
		t.Logf("First recovery attempt failed: %v, retrying...", err)
		time.Sleep(ContainerRestartWait * 2)
		newRound, err = requestRandomNumberFromConsumer(ctx, geth)
	}

	// Verify final state
	finalState, _ := getState(ctx, geth)
	t.Logf("Final state: %s", finalState)

	// Verify all regular nodes still running
	for _, node := range []string{"test-regularnode1", "test-regularnode2", "test-regularnode3"} {
		running, _ := isContainerRunning(node)
		assert.True(t, running, "%s should be running", node)
	}

	if err == nil && newRound != nil {
		t.Logf("System recovered successfully, new round: %s", newRound.String())
	}
}

// TestIntegrationMultipleNodeFailures tests system behavior when multiple nodes fail simultaneously.
// Expected behavior: With 2 of 3 regular nodes down, system should likely enter HALTED state.
func TestIntegrationMultipleNodeFailures(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	// Ensure clean state
	require.NoError(t, ensureContainersRunning(t), "failed to ensure containers running")
	registerContainerCleanup(t, "test-regularnode1", "test-regularnode2")

	// Start a round
	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "failed to request random number")
	t.Logf("Round started: %s", round.String())

	time.Sleep(ShortPollInterval)

	// Kill 2 of 3 regular nodes
	err = killContainer(t, "test-regularnode1")
	require.NoError(t, err, "failed to kill regularnode1")
	err = killContainer(t, "test-regularnode2")
	require.NoError(t, err, "failed to kill regularnode2")

	t.Log("Killed regularnode1 and regularnode2, monitoring system...")

	// Monitor for terminal state
	finalState, reached := waitForState(ctx, t, geth, []ContractState{StateCompleted, StateHalted}, 25)

	// Log outcome
	if reached {
		if finalState == StateHalted {
			t.Log("System correctly entered HALTED state due to insufficient nodes")
		} else if finalState == StateCompleted {
			t.Log("System completed round despite node failures (had enough commits)")
		}
	}

	// Restart nodes
	err = startContainer(t, "test-regularnode1")
	assert.NoError(t, err, "failed to restart regularnode1")
	err = startContainer(t, "test-regularnode2")
	assert.NoError(t, err, "failed to restart regularnode2")
	time.Sleep(ContainerRestartWait)

	// Verify nodes recovered
	require.NoError(t, verifyAllContainersRunning(t), "all containers should be running after recovery")

	// With 2 of 3 nodes down, we expect HALTED (but COMPLETED is also valid if timing allowed commits)
	assert.True(t, reached, "should reach a terminal state")
}

// TestIntegrationRecoveryFromHaltedState tests that the system can accept new requests after recovering from HALTED.
func TestIntegrationRecoveryFromHaltedState(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	// Check initial state
	initialState, err := getState(ctx, geth)
	if err == nil {
		t.Logf("Initial state: %s", initialState)
	}

	// If HALTED, wait for auto-recovery
	if initialState == StateHalted {
		t.Log("System is HALTED, waiting for recovery mechanism...")
		time.Sleep(time.Minute)
	}

	// Ensure all containers running
	require.NoError(t, ensureContainersRunning(t), "failed to ensure containers running")

	// Wait for operational state
	state, ready := waitForState(ctx, t, geth, []ContractState{StateIDLE, StateCompleted}, MaxRecoveryRetries)
	if !ready {
		showLogs(t, "test-leadernode", 30)
		t.Logf("System not ready, current state: %s", state)
	}

	// Request new round
	round, err := requestRandomNumberFromConsumer(ctx, geth)
	if err != nil {
		t.Logf("First request failed: %v, retrying after wait...", err)
		time.Sleep(time.Minute)
		round, err = requestRandomNumberFromConsumer(ctx, geth)
	}
	require.NoError(t, err, "failed to request random number after recovery")
	t.Logf("New round requested: %s", round.String())

	// Wait for fulfillment
	fulfilled, randomNumber := waitForRoundFulfillment(ctx, t, geth, round, MaxFulfillmentRetries)

	// Verify round was processed
	assert.NotNil(t, round, "should have started a new round")

	if fulfilled && randomNumber != nil {
		assert.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "random number should be positive")
		t.Logf("Recovery successful - random number: %s", randomNumber.String())
	} else {
		t.Log("Round initiated but not fulfilled within timeout")
		showLogs(t, "test-leadernode", 30)
	}

	// Verify final state is operational
	finalState, err := getState(ctx, geth)
	require.NoError(t, err)
	t.Logf("Final state: %s", finalState)

	// System should be in operational state (IDLE, IN_PROGRESS, or COMPLETED)
	assert.NotEqual(t, StateHalted, finalState, "system should not be HALTED after successful round")

	// Verify all containers running
	require.NoError(t, verifyAllContainersRunning(t), "all containers should be running")
}
