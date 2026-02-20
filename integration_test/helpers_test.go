package integration_test

import (
	"context"
	"fmt"
	"math/big"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/tokamak-network/DRB-node/integration_test/setup"
)

// logNow prints immediately to stdout AND records in test log
// Use this instead of t.Log for real-time output during long-running tests
func logNow(t *testing.T, args ...interface{}) {
	t.Helper()
	fmt.Println(args...)
	t.Log(args...)
}

// logNowf prints immediately to stdout AND records in test log (formatted)
func logNowf(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	fmt.Printf(format+"\n", args...)
	t.Logf(format, args...)
}

// Contract states
const (
	StateIDLE       = 0
	StateInProgress = 1
	StateCompleted  = 2
	StateHalted     = 3
)

// Test configuration constants
const (
	// Timeouts
	DefaultPollInterval    = 10 * time.Second
	ShortPollInterval      = 5 * time.Second
	ContainerRestartWait   = 30 * time.Second
	RoundFulfillmentWait   = 15 * time.Second
	PostCrashRecoveryWait  = 45 * time.Second

	// Retry limits
	MaxFulfillmentRetries = 15
	MaxStateCheckRetries  = 20
	MaxRecoveryRetries    = 10

	// Performance thresholds
	MinSuccessRate        = 0.9  // 90% success required
	MaxRequestLatency     = 10 * time.Second
	ConcurrentReaders     = 50   // Meaningful concurrency test
	ResourceExhaustionOps = 200  // Actual stress test
)

// Test container names
var TestContainers = []string{
	"test-leadernode",
	"test-regularnode1",
	"test-regularnode2",
	"test-regularnode3",
}

// ContractState represents the protocol state
type ContractState int

func (s ContractState) String() string {
	switch s {
	case StateIDLE:
		return "IDLE"
	case StateInProgress:
		return "IN_PROGRESS"
	case StateCompleted:
		return "COMPLETED"
	case StateHalted:
		return "HALTED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", s)
	}
}

// IsOperational returns true if the state allows new rounds
func (s ContractState) IsOperational() bool {
	return s == StateIDLE || s == StateCompleted
}

// IsTerminal returns true if the state is a final state for a round
func (s ContractState) IsTerminal() bool {
	return s == StateCompleted || s == StateHalted
}

// getContractState returns the current s_isInProcess state from the contract
func getContractState(ctx context.Context, geth *setup.GethTestEnv) (*big.Int, error) {
	if geth == nil || geth.ContractABI == nil {
		return nil, fmt.Errorf("geth environment not initialized")
	}

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

// getState is a helper that returns ContractState type
func getState(ctx context.Context, geth *setup.GethTestEnv) (ContractState, error) {
	state, err := getContractState(ctx, geth)
	if err != nil {
		return -1, err
	}
	return ContractState(state.Int64()), nil
}


// isContainerRunning checks if a Docker container is running
func isContainerRunning(containerName string) (bool, error) {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to inspect container %s: %w", containerName, err)
	}
	return strings.TrimSpace(string(output)) == "true", nil
}

// killContainer kills a Docker container and verifies it stopped
func killContainer(t *testing.T, containerName string) error {
	t.Helper()

	cmd := exec.Command("docker", "kill", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill %s: %w (output: %s)", containerName, err, string(output))
	}

	// Verify container stopped
	time.Sleep(1 * time.Second)
	running, err := isContainerRunning(containerName)
	if err != nil {
		return fmt.Errorf("failed to verify %s stopped: %w", containerName, err)
	}
	if running {
		return fmt.Errorf("container %s still running after kill", containerName)
	}

	t.Logf("Killed container: %s", containerName)
	return nil
}

// startContainer starts a Docker container and verifies it started
func startContainer(t *testing.T, containerName string) error {
	t.Helper()

	cmd := exec.Command("docker", "start", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start %s: %w (output: %s)", containerName, err, string(output))
	}

	// Verify container started
	time.Sleep(2 * time.Second)
	running, err := isContainerRunning(containerName)
	if err != nil {
		return fmt.Errorf("failed to verify %s started: %w", containerName, err)
	}
	if !running {
		return fmt.Errorf("container %s not running after start", containerName)
	}

	t.Logf("Started container: %s", containerName)
	return nil
}

// ensureContainersRunning ensures all test containers are running
func ensureContainersRunning(t *testing.T) error {
	t.Helper()

	for _, container := range TestContainers {
		running, err := isContainerRunning(container)
		if err != nil || !running {
			if err := startContainer(t, container); err != nil {
				return fmt.Errorf("failed to ensure %s running: %w", container, err)
			}
		}
	}
	time.Sleep(ContainerRestartWait)
	return nil
}

// verifyAllContainersRunning checks that all containers are running and returns error if not
func verifyAllContainersRunning(t *testing.T) error {
	t.Helper()

	for _, container := range TestContainers {
		running, err := isContainerRunning(container)
		if err != nil {
			return fmt.Errorf("failed to check %s: %w", container, err)
		}
		if !running {
			return fmt.Errorf("container %s is not running", container)
		}
	}
	return nil
}

// registerContainerCleanup registers cleanup to restart containers after test
func registerContainerCleanup(t *testing.T, containers ...string) {
	t.Helper()

	t.Cleanup(func() {
		for _, container := range containers {
			running, _ := isContainerRunning(container)
			if !running {
				_ = startContainer(t, container)
			}
		}
		time.Sleep(5 * time.Second)
	})
}


// waitForState polls until the contract reaches one of the target states
func waitForState(ctx context.Context, t *testing.T, geth *setup.GethTestEnv, targetStates []ContractState, maxRetries int) (ContractState, bool) {
	t.Helper()

	targetMap := make(map[ContractState]bool)
	for _, s := range targetStates {
		targetMap[s] = true
	}

	for i := 0; i < maxRetries; i++ {
		state, err := getState(ctx, geth)
		if err != nil {
			t.Logf("Attempt %d: failed to get state: %v", i+1, err)
			time.Sleep(DefaultPollInterval)
			continue
		}

		t.Logf("Attempt %d: state = %s", i+1, state)

		if targetMap[state] {
			return state, true
		}

		time.Sleep(DefaultPollInterval)
	}

	return -1, false
}

// waitForRoundFulfillment waits for a round to be fulfilled
func waitForRoundFulfillment(ctx context.Context, t *testing.T, geth *setup.GethTestEnv, round *big.Int, maxRetries int) (bool, *big.Int) {
	t.Helper()

	for i := 0; i < maxRetries; i++ {
		fulfilled, randomNumber, err := checkRandomNumberFulfilled(ctx, geth, round)
		if err != nil {
			t.Logf("Attempt %d: check failed: %v", i+1, err)
		} else if fulfilled {
			t.Logf("Round fulfilled after %d attempts", i+1)
			return true, randomNumber
		} else {
			t.Logf("Attempt %d: not yet fulfilled", i+1)
		}

		if i < maxRetries-1 {
			time.Sleep(DefaultPollInterval)
		}
	}

	return false, nil
}

// StateTransitionRecorder records state transitions during a test
type StateTransitionRecorder struct {
	transitions []ContractState
	lastState   ContractState
	initialized bool
}

// NewStateTransitionRecorder creates a new recorder
func NewStateTransitionRecorder() *StateTransitionRecorder {
	return &StateTransitionRecorder{
		transitions: make([]ContractState, 0),
		lastState:   -1,
		initialized: false,
	}
}

// Record records a state if it's different from the last recorded state
func (r *StateTransitionRecorder) Record(state ContractState) bool {
	if !r.initialized || state != r.lastState {
		r.transitions = append(r.transitions, state)
		r.lastState = state
		r.initialized = true
		return true
	}
	return false
}

// Transitions returns all recorded transitions
func (r *StateTransitionRecorder) Transitions() []ContractState {
	return r.transitions
}

// HasTransition checks if a specific transition occurred
func (r *StateTransitionRecorder) HasTransition(from, to ContractState) bool {
	for i := 0; i < len(r.transitions)-1; i++ {
		if r.transitions[i] == from && r.transitions[i+1] == to {
			return true
		}
	}
	return false
}

// String returns a string representation of transitions
func (r *StateTransitionRecorder) String() string {
	strs := make([]string, len(r.transitions))
	for i, s := range r.transitions {
		strs[i] = s.String()
	}
	return strings.Join(strs, " → ")
}
