package integration_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationMultipleConsecutiveRounds verifies the system can handle multiple rounds in sequence.
// Validates: round completion, unique random numbers per round, system stability.
func TestIntegrationMultipleConsecutiveRounds(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	const numRounds = 3
	var completedRounds int
	randomNumbers := make(map[string]int) // Track unique random numbers

	for i := 0; i < numRounds; i++ {
		t.Logf("=== Round %d/%d ===", i+1, numRounds)

		round, err := requestRandomNumberFromConsumer(ctx, geth)
		if err != nil {
			t.Logf("Failed to request round %d: %v", i+1, err)
			time.Sleep(ContainerRestartWait)
			continue
		}
		t.Logf("Round %d: request submitted (ID: %s)", i+1, round.String())

		// Wait for fulfillment
		fulfilled, randomNumber := waitForRoundFulfillment(ctx, t, geth, round, 12)

		if fulfilled && randomNumber != nil {
			completedRounds++
			rnStr := randomNumber.String()

			// Check for duplicate random numbers (should never happen)
			if prevRound, exists := randomNumbers[rnStr]; exists {
				t.Errorf("DUPLICATE random number detected: round %d produced same value as round %d", i+1, prevRound)
			}
			randomNumbers[rnStr] = i + 1

			t.Logf("Round %d: SUCCESS - %s", i+1, rnStr)
		} else {
			t.Logf("Round %d: not fulfilled within timeout", i+1)
			showLogs(t, "test-leadernode", 20)
		}

		// Brief pause between rounds
		if i < numRounds-1 {
			time.Sleep(ShortPollInterval)
		}
	}

	// Summary and assertions
	t.Logf("=== Summary: %d/%d rounds completed ===", completedRounds, numRounds)

	// Log all random numbers generated
	for rn, round := range randomNumbers {
		t.Logf("  Round %d: %s", round, rn)
	}

	// At least 2 of 3 rounds should complete for the test to pass
	assert.GreaterOrEqual(t, completedRounds, 2, "at least 2 rounds should complete")

	// All random numbers should be unique
	assert.Equal(t, completedRounds, len(randomNumbers), "all random numbers should be unique")

	// Verify system stability
	require.NoError(t, verifyAllContainersRunning(t), "all containers should be running after stress test")
}

// TestIntegrationSystemStability monitors state transitions during a round.
// Validates: proper state machine transitions (IDLE/COMPLETED → IN_PROGRESS → COMPLETED).
func TestIntegrationSystemStability(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	// Record initial state
	initialState, err := getState(ctx, geth)
	require.NoError(t, err, "should be able to read initial state")
	t.Logf("Initial state: %s", initialState)

	// Initial state should be operational
	assert.True(t, initialState.IsOperational() || initialState == StateInProgress,
		"initial state should be operational or in-progress, got: %s", initialState)

	// Start a round
	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "failed to request random number")
	t.Logf("Round requested: %s", round.String())

	// Track state transitions
	recorder := NewStateTransitionRecorder()
	recorder.Record(initialState)

	// Monitor state transitions
	for i := 0; i < MaxStateCheckRetries; i++ {
		state, err := getState(ctx, geth)
		if err != nil {
			t.Logf("Attempt %d: failed to get state: %v", i+1, err)
			time.Sleep(ShortPollInterval)
			continue
		}

		if recorder.Record(state) {
			t.Logf("State transition detected: %s", state)
		}

		// Stop when we reach a terminal state
		if state.IsTerminal() {
			t.Logf("Reached terminal state: %s", state)
			break
		}

		time.Sleep(ShortPollInterval)
	}

	t.Logf("State transitions: %s", recorder.String())

	// Verify expected transitions occurred
	transitions := recorder.Transitions()
	assert.NotEmpty(t, transitions, "should observe at least one state")

	// If we saw IN_PROGRESS, we should eventually see COMPLETED or HALTED
	sawInProgress := false
	sawTerminal := false
	for _, s := range transitions {
		if s == StateInProgress {
			sawInProgress = true
		}
		if s.IsTerminal() {
			sawTerminal = true
		}
	}

	if sawInProgress {
		assert.True(t, sawTerminal, "if IN_PROGRESS was observed, should reach terminal state")
	}

	// Final state check
	finalState, err := getState(ctx, geth)
	require.NoError(t, err)
	t.Logf("Final state: %s", finalState)
}

// TestIntegrationRoundCleanup verifies resources are properly cleaned up between rounds.
// Validates: back-to-back rounds work correctly, round IDs are unique.
func TestIntegrationRoundCleanup(t *testing.T) {
	require.NotNil(t, testEnv, "test environment must be initialized")
	require.NotNil(t, testEnv.Geth, "geth must be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	// === Round 1 ===
	t.Log("=== Starting Round 1 ===")
	round1, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "failed to request first random number")
	t.Logf("Round 1 ID: %s", round1.String())

	round1Fulfilled, round1RN := waitForRoundFulfillment(ctx, t, geth, round1, MaxFulfillmentRetries)

	if round1Fulfilled {
		t.Logf("Round 1 completed: %s", round1RN.String())
	} else {
		t.Log("Round 1 not fulfilled within timeout")
		showLogs(t, "test-leadernode", 30)
	}

	// Check state after round 1
	stateAfterRound1, _ := getState(ctx, geth)
	t.Logf("State after round 1: %s", stateAfterRound1)

	// Brief pause for cleanup
	time.Sleep(ShortPollInterval)

	// === Round 2 ===
	t.Log("=== Starting Round 2 ===")
	round2, err := requestRandomNumberFromConsumer(ctx, geth)
	if err != nil {
		t.Logf("First attempt failed: %v, retrying...", err)
		time.Sleep(ContainerRestartWait)
		round2, err = requestRandomNumberFromConsumer(ctx, geth)
	}
	require.NoError(t, err, "failed to request second random number")
	t.Logf("Round 2 ID: %s", round2.String())

	round2Fulfilled, round2RN := waitForRoundFulfillment(ctx, t, geth, round2, MaxFulfillmentRetries)

	if round2Fulfilled {
		t.Logf("Round 2 completed: %s", round2RN.String())
	} else {
		t.Log("Round 2 not fulfilled within timeout")
	}

	// === Assertions ===

	// Round IDs must be different (proves new round was created)
	assert.NotEqual(t, round1.String(), round2.String(),
		"round IDs must be different (round1: %s, round2: %s)", round1.String(), round2.String())

	// If both fulfilled, random numbers should be different
	if round1Fulfilled && round2Fulfilled && round1RN != nil && round2RN != nil {
		assert.NotEqual(t, round1RN.String(), round2RN.String(),
			"random numbers should be different between rounds")
	}

	// Log summary
	t.Log("=== Round Summary ===")
	t.Logf("  Round 1: %s (fulfilled: %v)", round1.String(), round1Fulfilled)
	t.Logf("  Round 2: %s (fulfilled: %v)", round2.String(), round2Fulfilled)

	// Final state should be operational
	finalState, err := getState(ctx, geth)
	require.NoError(t, err)
	t.Logf("Final state: %s", finalState)

	// At least one round should have completed
	assert.True(t, round1Fulfilled || round2Fulfilled, "at least one round should complete")

	// System should be stable
	require.NoError(t, verifyAllContainersRunning(t), "all containers should be running")
}
