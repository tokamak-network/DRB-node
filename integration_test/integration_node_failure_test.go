package integration_test

import (
	"context"
	"fmt"
	"math/big"
	"os/exec"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/integration_test/setup"
)

// TestIntegrationNodeCrashDuringRound tests system behavior when a regular node crashes mid-round
func TestIntegrationNodeCrashDuringRound(t *testing.T) {
	require.NotNil(t, testEnv, "test env should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Round started: %s", round.String())

	time.Sleep(5 * time.Second)

	// Kill regularNode1
	killCmd := exec.Command("docker", "kill", "test-regularnode1")
	killCmd.CombinedOutput()

	t.Log("Killed regularnode1, monitoring system...")

	var finalState *big.Int
	for i := 0; i < 20; i++ {
		state, err := getContractState(ctx, geth)
		if err == nil {
			t.Logf("Contract state: %s", stateToString(state))
			finalState = state

			if state.Cmp(big.NewInt(2)) == 0 || state.Cmp(big.NewInt(3)) == 0 {
				break
			}
		}

		if i%5 == 0 {
			showLogs(t, "test-leadernode", 20)
		}

		time.Sleep(10 * time.Second)
	}

	// Restart node
	exec.Command("docker", "start", "test-regularnode1").Run()
	time.Sleep(30 * time.Second)

	assert.NotNil(t, finalState, "Should have captured contract state")

	if finalState != nil {
		validStates := finalState.Cmp(big.NewInt(1)) == 0 ||
			finalState.Cmp(big.NewInt(2)) == 0 ||
			finalState.Cmp(big.NewInt(3)) == 0
		assert.True(t, validStates, "Protocol should be in valid state, got: %s", stateToString(finalState))
	}

	isRunning, err := isContainerRunning("test-regularnode1")
	assert.NoError(t, err)
	assert.True(t, isRunning, "regularNode1 should be running after restart")

	for _, node := range []string{"test-regularnode2", "test-regularnode3", "test-leadernode"} {
		running, _ := isContainerRunning(node)
		assert.True(t, running, "%s should still be running", node)
	}
}

// TestIntegrationLeaderNodeCrash tests system behavior when the leader node crashes
func TestIntegrationLeaderNodeCrash(t *testing.T) {
	require.NotNil(t, testEnv, "test env should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	containers := []string{"test-leadernode", "test-regularnode1", "test-regularnode2", "test-regularnode3"}
	for _, container := range containers {
		running, _ := isContainerRunning(container)
		if !running {
			exec.Command("docker", "start", container).Run()
		}
	}
	time.Sleep(10 * time.Second)

	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Round started: %s", round.String())

	time.Sleep(5 * time.Second)

	state, err := getContractState(ctx, geth)
	if err == nil {
		t.Logf("State before leader crash: %s", stateToString(state))
	}

	// Kill leader
	exec.Command("docker", "kill", "test-leadernode").Run()

	leaderRunning, _ := isContainerRunning("test-leadernode")
	assert.False(t, leaderRunning, "Leader should be stopped after kill")

	t.Log("Leader killed, checking regular nodes...")
	time.Sleep(10 * time.Second)

	for _, node := range []string{"test-regularnode1", "test-regularnode2", "test-regularnode3"} {
		showLogs(t, node, 10)
	}

	var stateAfterCrash *big.Int
	for i := 0; i < 10; i++ {
		state, err := getContractState(ctx, geth)
		if err == nil {
			t.Logf("Contract state: %s", stateToString(state))
			stateAfterCrash = state

			if state.Cmp(big.NewInt(3)) == 0 {
				t.Log("System entered HALTED state")
				break
			}
		}
		time.Sleep(10 * time.Second)
	}

	// Restart leader
	exec.Command("docker", "start", "test-leadernode").Run()
	t.Log("Restarted leader, waiting for recovery...")
	time.Sleep(45 * time.Second)

	leaderRunning, _ = isContainerRunning("test-leadernode")
	assert.True(t, leaderRunning, "Leader should be running after restart")

	showLogs(t, "test-leadernode", 20)

	time.Sleep(30 * time.Second)

	newRound, err := requestRandomNumberFromConsumer(ctx, geth)
	if err != nil {
		time.Sleep(60 * time.Second)
		newRound, err = requestRandomNumberFromConsumer(ctx, geth)
	}

	for _, node := range []string{"test-regularnode1", "test-regularnode2", "test-regularnode3"} {
		running, _ := isContainerRunning(node)
		assert.True(t, running, "%s should be running", node)
	}

	finalState, _ := getContractState(ctx, geth)
	if finalState != nil {
		t.Logf("Final state: %s", stateToString(finalState))
	}

	if stateAfterCrash != nil {
		t.Logf("State after crash: %s", stateToString(stateAfterCrash))
	}

	if err == nil && newRound != nil {
		t.Logf("System recovered, new round: %s", newRound.String())
	}
}

// TestIntegrationMultipleNodeFailures tests system behavior when multiple nodes fail simultaneously
func TestIntegrationMultipleNodeFailures(t *testing.T) {
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	round, err := requestRandomNumberFromConsumer(ctx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Round started: %s", round.String())

	time.Sleep(5 * time.Second)

	// Kill 2 nodes
	exec.Command("docker", "kill", "test-regularnode1").Run()
	exec.Command("docker", "kill", "test-regularnode2").Run()
	t.Log("Killed regularnode1 and regularnode2")

	var reachedHalted bool
	for i := 0; i < 25; i++ {
		state, err := getContractState(ctx, geth)
		if err == nil {
			t.Logf("Contract state: %s", stateToString(state))

			if state.Cmp(big.NewInt(3)) == 0 {
				reachedHalted = true
				t.Log("Protocol entered HALTED state")
				break
			}
			if state.Cmp(big.NewInt(2)) == 0 {
				t.Log("Protocol completed despite failures")
				break
			}
		}

		if i%5 == 0 {
			showLogs(t, "test-leadernode", 15)
		}

		time.Sleep(10 * time.Second)
	}

	// Restart nodes
	exec.Command("docker", "start", "test-regularnode1").Run()
	exec.Command("docker", "start", "test-regularnode2").Run()
	time.Sleep(30 * time.Second)

	t.Logf("Reached HALTED: %v", reachedHalted)

	node1Running, _ := isContainerRunning("test-regularnode1")
	node2Running, _ := isContainerRunning("test-regularnode2")
	assert.True(t, node1Running, "regularNode1 should be running")
	assert.True(t, node2Running, "regularNode2 should be running")
}

// TestIntegrationRecoveryFromHaltedState tests system recovery after entering HALTED state
func TestIntegrationRecoveryFromHaltedState(t *testing.T) {
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	ctx := testCtx

	initialState, err := getContractState(ctx, geth)
	if err == nil {
		t.Logf("Initial state: %s", stateToString(initialState))
	}

	if initialState != nil && initialState.Cmp(big.NewInt(3)) == 0 {
		t.Log("System is HALTED, waiting for recovery...")
		time.Sleep(60 * time.Second)
	}

	containers := []string{"test-leadernode", "test-regularnode1", "test-regularnode2", "test-regularnode3"}
	for _, container := range containers {
		running, err := isContainerRunning(container)
		if !running || err != nil {
			exec.Command("docker", "start", container).Run()
		}
	}
	time.Sleep(30 * time.Second)

	var readyForRound bool
	for i := 0; i < 10; i++ {
		state, err := getContractState(ctx, geth)
		if err == nil && (state.Cmp(big.NewInt(0)) == 0 || state.Cmp(big.NewInt(1)) == 0 || state.Cmp(big.NewInt(2)) == 0) {
			readyForRound = true
			t.Logf("System ready (state: %s)", stateToString(state))
			break
		}
		time.Sleep(10 * time.Second)
	}

	if !readyForRound {
		showLogs(t, "test-leadernode", 30)
	}

	round, err := requestRandomNumberFromConsumer(ctx, geth)
	if err != nil {
		t.Logf("Request failed, waiting: %v", err)
		time.Sleep(60 * time.Second)
		round, err = requestRandomNumberFromConsumer(ctx, geth)
	}
	require.NoError(t, err, "Failed to request random number after recovery")
	t.Logf("New round: %s", round.String())

	var fulfilled bool
	var randomNumber *big.Int
	for i := 0; i < 15; i++ {
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(ctx, geth, round)
		if err == nil && fulfilled {
			t.Logf("Random number generated after %d attempts", i+1)
			break
		}

		if i%3 == 0 {
			showLogs(t, "test-leadernode", 15)
		}

		time.Sleep(15 * time.Second)
	}

	assert.NotNil(t, round, "Should have started a new round")

	if fulfilled && randomNumber != nil {
		assert.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be > 0")
		t.Logf("Random number: %s", randomNumber.String())
	} else {
		t.Log("Round initiated but not fulfilled yet")
	}

	for _, container := range containers {
		running, _ := isContainerRunning(container)
		assert.True(t, running, "%s should be running", container)
	}

	finalState, _ := getContractState(ctx, geth)
	if finalState != nil {
		t.Logf("Final state: %s", stateToString(finalState))
		validState := finalState.Cmp(big.NewInt(0)) == 0 ||
			finalState.Cmp(big.NewInt(1)) == 0 ||
			finalState.Cmp(big.NewInt(2)) == 0
		assert.True(t, validState, "System should be in operational state")
	}
}

// getContractState returns the current s_isInProcess state from the contract
func getContractState(ctx context.Context, geth *setup.GethTestEnv) (*big.Int, error) {
	input, err := geth.ContractABI.Pack("s_isInProcess")
	if err != nil {
		return nil, fmt.Errorf("failed to pack s_isInProcess: %w", err)
	}

	result, err := geth.Client.CallContract(ctx, ethereum.CallMsg{
		To:   &geth.ContractAddress,
		Data: input,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call s_isInProcess: %w", err)
	}

	var state *big.Int
	if err := geth.ContractABI.UnpackIntoInterface(&state, "s_isInProcess", result); err != nil {
		return nil, fmt.Errorf("failed to unpack s_isInProcess: %w", err)
	}

	return state, nil
}

func stateToString(state *big.Int) string {
	if state == nil {
		return "NIL"
	}
	switch state.Int64() {
	case 0:
		return "IDLE"
	case 1:
		return "IN_PROGRESS"
	case 2:
		return "COMPLETED"
	case 3:
		return "HALTED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", state.Int64())
	}
}

func isContainerRunning(containerName string) (bool, error) {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return string(output) == "true\n", nil
}
