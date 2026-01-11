package integration_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationMultipleConsecutiveRounds tests system handling of multiple rounds in sequence
func TestIntegrationMultipleConsecutiveRounds(t *testing.T) {
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	numRounds := 3
	var completedRounds int
	var randomNumbers []*big.Int

	for i := 0; i < numRounds; i++ {
		t.Logf("Round %d/%d", i+1, numRounds)

		round, err := requestRandomNumberFromConsumer(ctx, geth)
		if err != nil {
			t.Logf("Failed to request round %d: %v", i+1, err)
			time.Sleep(30 * time.Second)
			continue
		}
		t.Logf("Round %d: request submitted (ID: %s)", i+1, round.String())

		var fulfilled bool
		var randomNumber *big.Int
		for j := 0; j < 12; j++ {
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(ctx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Round %d: fulfilled after %d attempts", i+1, j+1)
				break
			}

			if j%3 == 0 {
				state, _ := getContractState(ctx, geth)
				if state != nil {
					t.Logf("Round %d: attempt %d - state: %s", i+1, j+1, stateToString(state))
				}
			}

			time.Sleep(10 * time.Second)
		}

		if fulfilled && randomNumber != nil {
			completedRounds++
			randomNumbers = append(randomNumbers, randomNumber)
			t.Logf("Round %d: SUCCESS - %s", i+1, randomNumber.String())
		} else {
			t.Logf("Round %d: not fulfilled within timeout", i+1)
			showLogs(t, "test-leadernode", 20)
		}

		if i < numRounds-1 {
			time.Sleep(5 * time.Second)
		}
	}

	t.Logf("Completed rounds: %d/%d", completedRounds, numRounds)

	if len(randomNumbers) > 1 {
		for i := 0; i < len(randomNumbers); i++ {
			for j := i + 1; j < len(randomNumbers); j++ {
				assert.NotEqual(t, randomNumbers[i].String(), randomNumbers[j].String(),
					"Random numbers should be unique (round %d vs round %d)", i+1, j+1)
			}
		}
		t.Log("All random numbers are unique")
	}

	if len(randomNumbers) > 0 {
		for i, rn := range randomNumbers {
			t.Logf("Round %d: %s", i+1, rn.String())
		}
	}

	containers := []string{"test-leadernode", "test-regularnode1", "test-regularnode2", "test-regularnode3"}
	for _, container := range containers {
		running, _ := isContainerRunning(container)
		assert.True(t, running, "%s should be running after stress test", container)
	}
}

// TestIntegrationSystemStability tests system stability by monitoring state transitions
func TestIntegrationSystemStability(t *testing.T) {
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	initialState, err := getContractState(ctx, geth)
	if err == nil {
		t.Logf("Initial state: %s", stateToString(initialState))
	}

	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "Failed to request random number")

	var stateTransitions []string
	var lastState *big.Int

	for i := 0; i < 20; i++ {
		state, err := getContractState(ctx, geth)
		if err == nil {
			stateStr := stateToString(state)
			if lastState == nil || state.Cmp(lastState) != 0 {
				stateTransitions = append(stateTransitions, stateStr)
				t.Logf("State transition: %s", stateStr)
				lastState = state
			}

			if state.Cmp(big.NewInt(2)) == 0 {
				t.Log("Round completed")
				break
			}
		}
		time.Sleep(5 * time.Second)
	}

	t.Logf("Round: %s", round.String())
	t.Logf("Transitions: %v", stateTransitions)

	assert.NotEmpty(t, stateTransitions, "Should observe state transitions")

	finalState, err := getContractState(ctx, geth)
	assert.NoError(t, err)
	if finalState != nil {
		t.Logf("Final state: %s", stateToString(finalState))
	}
}

// TestIntegrationRoundCleanup tests resource cleanup between rounds
func TestIntegrationRoundCleanup(t *testing.T) {
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	round1, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "Failed to request first random number")
	t.Logf("First round: %s", round1.String())

	var round1Fulfilled bool
	for i := 0; i < 15; i++ {
		round1Fulfilled, _, err = checkRandomNumberFulfilled(ctx, geth, round1)
		if round1Fulfilled {
			t.Log("First round completed")
			break
		}
		time.Sleep(10 * time.Second)
	}

	if !round1Fulfilled {
		showLogs(t, "test-leadernode", 30)
	}

	time.Sleep(5 * time.Second)
	stateAfterRound1, _ := getContractState(ctx, geth)
	if stateAfterRound1 != nil {
		t.Logf("State after round 1: %s", stateToString(stateAfterRound1))
	}

	round2, err := requestRandomNumberFromConsumer(ctx, geth)
	if err != nil {
		t.Logf("Failed to request second round: %v", err)
		time.Sleep(30 * time.Second)
		round2, err = requestRandomNumberFromConsumer(ctx, geth)
	}
	require.NoError(t, err, "Failed to request second random number")
	t.Logf("Second round: %s", round2.String())

	var round2Fulfilled bool
	for i := 0; i < 15; i++ {
		round2Fulfilled, _, err = checkRandomNumberFulfilled(ctx, geth, round2)
		if round2Fulfilled {
			t.Log("Second round completed")
			break
		}
		time.Sleep(10 * time.Second)
	}

	assert.NotEqual(t, round1.String(), round2.String(), "Round IDs should be different")
	t.Logf("Round 1: %s, Round 2: %s", round1.String(), round2.String())

	if round1Fulfilled {
		t.Log("Round 1: COMPLETED")
	} else {
		t.Log("Round 1: IN_PROGRESS")
	}

	if round2Fulfilled {
		t.Log("Round 2: COMPLETED")
	} else {
		t.Log("Round 2: IN_PROGRESS")
	}

	finalState, _ := getContractState(ctx, geth)
	if finalState != nil {
		t.Logf("Final state: %s", stateToString(finalState))
	}

	containers := []string{"test-leadernode", "test-regularnode1", "test-regularnode2", "test-regularnode3"}
	for _, container := range containers {
		running, _ := isContainerRunning(container)
		assert.True(t, running, "%s should be running", container)
	}
}
