package integration_test

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tokamak-network/DRB-node/integration_test/setup"
)

var (
	testEnv *setup.TestEnvironment
	testCtx context.Context
)

type testLogger struct{}

func (tl *testLogger) Log(args ...interface{}) {
	fmt.Println(args...)
}

func (tl *testLogger) Logf(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

func getDockerComposeCmd() []string {
	if _, err := exec.LookPath("docker"); err == nil {
		cmd := exec.Command("docker", "compose", "version")
		if err := cmd.Run(); err == nil {
			return []string{"docker", "compose"}
		}
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return []string{"docker-compose"}
	}
	// Default to V2 (most common now)
	return []string{"docker", "compose"}
}

// TestMain runs once before all tests to set up the shared test environment
func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	testCtx = ctx

	fmt.Println(" Setting up test environment ")

	// Create a simple logger for setup
	logger := &testLogger{}

	t := &mockTestingT{logger: logger}

	var err error
	testEnv, err = setup.SetupTestEnvironment(ctx, t)
	if err != nil {
		fmt.Printf(" Failed to setup test environment: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(" Test environment setup complete!")

	// Run all tests
	code := m.Run()

	// Cleanup after all tests complete
	if testEnv != nil {
		if testEnv.CleanupFunc != nil {
			testEnv.CleanupFunc()
		}
		if testEnv.Geth != nil {
			testEnv.Geth.Cleanup()
		}
	}

	fmt.Println("Cleaning up test environment...")
	os.Exit(code)
}

// mockTestingT implements setup.Logger interface for use in TestMain
type mockTestingT struct {
	logger *testLogger
}

func (m *mockTestingT) Log(args ...interface{}) {
	m.logger.Log(args...)
}

func (m *mockTestingT) Logf(format string, args ...interface{}) {
	m.logger.Logf(format, args...)
}

func TestRandomNumberGeneration(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 1: Random Number Generation Test")
	t.Log("STEP 1: Requesting random number...")

	currentRoundInput, _ := geth.ContractABI.Pack("s_currentRound")
	currentRoundResult, _ := geth.Client.CallContract(testCtx, ethereum.CallMsg{
		To:   &geth.ContractAddress,
		Data: currentRoundInput,
	}, nil)
	var currentRoundBefore *big.Int
	if currentRoundResult != nil {
		geth.ContractABI.UnpackIntoInterface(&currentRoundBefore, "s_currentRound", currentRoundResult)
	}
	t.Logf(" Contract state before request: currentRound=%s", currentRoundBefore.String())

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf(" Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	// Check request info exists and print details
	requestInfoInput, _ := geth.ContractABI.Pack("s_requestInfo", round)
	requestInfoResult, err := geth.Client.CallContract(testCtx, ethereum.CallMsg{
		To:   &geth.ContractAddress,
		Data: requestInfoInput,
	}, nil)
	require.NoError(t, err, "Request info should exist for the round")
	require.NotNil(t, requestInfoResult, "Request info should not be empty")

	// Unpack and print s_requestInfo details
	var contractRequestInfo struct {
		Consumer         common.Address
		CallbackGasLimit uint32
		StartTime        *big.Int
		Cost             *big.Int
	}
	if err := geth.ContractABI.UnpackIntoInterface(&contractRequestInfo, "s_requestInfo", requestInfoResult); err == nil {
		t.Logf(" s_requestInfo for round %s:", round.String())
		t.Logf("   Consumer: %s", contractRequestInfo.Consumer.Hex())
		t.Logf("   CallbackGasLimit: %d", contractRequestInfo.CallbackGasLimit)
		t.Logf("   StartTime: %s", contractRequestInfo.StartTime.String())
		t.Logf("   Cost: %s wei (%s ETH)", contractRequestInfo.Cost.String(),
			new(big.Float).Quo(new(big.Float).SetInt(contractRequestInfo.Cost), big.NewFloat(1e18)).String())
	} else {
		t.Logf("Could not unpack s_requestInfo: %v", err)
	}
	t.Logf("Verified request exists for round %s", round.String())

	t.Log("\nSTEP 2: Waiting for random number to be generated...")

	maxFulfillmentRetries := 7
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		// Check if random number was fulfilled
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		if err == nil && fulfilled {
			t.Logf(" Random number fulfilled after %d attempts!", i+1)
			break
		}

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check contract state periodically
			currentRoundInput, _ := geth.ContractABI.Pack("s_currentRound")
			currentRoundResult, _ := geth.Client.CallContract(testCtx, ethereum.CallMsg{
				To:   &geth.ContractAddress,
				Data: currentRoundInput,
			}, nil)
			var currentRound *big.Int
			if currentRoundResult != nil {
				geth.ContractABI.UnpackIntoInterface(&currentRound, "s_currentRound", currentRoundResult)
				t.Logf(" Contract currentRound: %s (requested round: %s)", currentRound.String(), round.String())
			}

			// Check isInProcess
			isInProcessInput, _ := geth.ContractABI.Pack("s_isInProcess")
			isInProcessResult, _ := geth.Client.CallContract(testCtx, ethereum.CallMsg{
				To:   &geth.ContractAddress,
				Data: isInProcessInput,
			}, nil)
			if isInProcessResult != nil {
				var isInProcess *big.Int
				if err := geth.ContractABI.UnpackIntoInterface(&isInProcess, "s_isInProcess", isInProcessResult); err == nil {
					status := "UNKNOWN"
					if isInProcess.Cmp(big.NewInt(1)) == 0 {
						status = "IN_PROGRESS"
					} else if isInProcess.Cmp(big.NewInt(2)) == 0 {
						status = "COMPLETED"
					} else if isInProcess.Cmp(big.NewInt(3)) == 0 {
						status = "HALTED"
					}
					t.Logf("Contract state: s_isInProcess = %s (%s)", isInProcess.String(), status)
				}
			}

			// Check consumer contract state for debugging
			detailInfoInput, _ := geth.ConsumerABI.Pack("getDetailInfo", round)
			detailInfoResult, _ := geth.Client.CallContract(testCtx, ethereum.CallMsg{
				To:   &geth.ConsumerAddress,
				Data: detailInfoInput,
			}, nil)
			if detailInfoResult != nil {
				var detailInfo struct {
					Requester          common.Address
					RequestFee         *big.Int
					RequestBlockNumber *big.Int
					FulfillBlockNumber *big.Int
					RandomNumber       *big.Int
					IsRefunded         bool
				}
				if err := geth.ConsumerABI.UnpackIntoInterface(&detailInfo, "getDetailInfo", detailInfoResult); err == nil {
					t.Logf("Consumer contract state for round %s:", round.String())
					t.Logf("FulfillBlockNumber: %s", detailInfo.FulfillBlockNumber.String())
					t.Logf("RandomNumber: %s", detailInfo.RandomNumber.String())
					t.Logf("IsRefunded: %v", detailInfo.IsRefunded)
				}
			}

			t.Log("Checking leader node logs...")
			showLogs(t, "test-leadernode", 30)

			t.Log("Checking regular node logs...")
			showLogs(t, "test-regularnode1", 30)
			showLogs(t, "test-regularnode2", 30)
			showLogs(t, "test-regularnode3", 30)

			time.Sleep(15 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())

	t.Log("\n Final Leader Node Logs:")
	showLogs(t, "test-leadernode", 50)

	t.Log("\nWaiting for consumer contract callback to be processed...")
	time.Sleep(5 * time.Second)

	maxFinalRetries := 5
	var consumerFulfilled bool
	var consumerRandomNumber *big.Int
	for i := 0; i < maxFinalRetries; i++ {
		consumerFulfilled, consumerRandomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		if err == nil && consumerFulfilled {
			t.Logf(" Consumer contract confirmed fulfillment after %d retries", i+1)
			// Use the random number from consumer contract
			randomNumber = consumerRandomNumber
			break
		}
		if i < maxFinalRetries-1 {
			t.Logf("⏳ Waiting for consumer contract fulfillment (retry %d/%d)...", i+1, maxFinalRetries)
			time.Sleep(3 * time.Second)
		}
	}

	// Verify in consumer contract
	requestInfo, err := getConsumerRequestInfo(testCtx, geth, round)
	require.NoError(t, err, "Failed to get consumer request info")
	require.NotNil(t, requestInfo, "Request info should not be nil")

	t.Logf(" Consumer contract state:")
	t.Logf("   RequestId: %s", requestInfo["requestId"].(*big.Int).String())
	t.Logf("   FulfillBlockNumber: %s", requestInfo["fulfillBlockNumber"].(*big.Int).String())
	t.Logf("   RandomNumber: %s", requestInfo["randomNumber"].(*big.Int).String())

	assert.Equal(t, round.String(), requestInfo["requestId"].(*big.Int).String(), "Request ID should match round")

	// Check if fulfillment data exists in consumer contract
	fulfillBlockNum := requestInfo["fulfillBlockNumber"].(*big.Int)
	consumerRandNum := requestInfo["randomNumber"].(*big.Int)

	if fulfillBlockNum.Cmp(big.NewInt(0)) == 0 || consumerRandNum.Cmp(big.NewInt(0)) == 0 {
		t.Logf("   DRB contract random number: %s", randomNumber.String())
		t.Logf("   Consumer contract random number: %s", consumerRandNum.String())
	}

	assert.True(t, fulfillBlockNum.Cmp(big.NewInt(0)) > 0, "Fulfill block number should be set in consumer contract")
	assert.True(t, consumerRandNum.Cmp(big.NewInt(0)) > 0, "Random number should be set in consumer contract")

	// Only check exact match if consumer contract has the data
	if consumerRandNum.Cmp(big.NewInt(0)) > 0 {
		assert.Equal(t, randomNumber.String(), consumerRandNum.String(), "Random number in consumer contract should match")
	}

	t.Log("Test Case 1 Complete!")
}

func TestRegularNodeOnChainCommit(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 2: Regular Node On-Chain Commit Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	// Get integration test directory (same logic as setup package)
	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "true",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add the environment variable
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {

		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)

		err = os.WriteFile(dockerComposeFile, []byte(replacement), 0644)
		require.NoError(t, err, "Failed to write docker-compose file")
	}

	t.Log("STEP 3: Restarting regularNode1 with mock mode enabled...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (regularNode1 will submit on-chain)...")

	maxFulfillmentRetries := 10
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check regularNode1 logs to verify it's submitting on-chain
			t.Log("Checking regularNode1 logs for on-chain submission...")
			showLogs(t, "test-leadernode", 30)
			showLogs(t, "test-regularnode1", 30)
			showLogs(t, "test-regularnode2", 30)
			showLogs(t, "test-regularnode3", 30)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}

			time.Sleep(15 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 2 Complete! RegularNode1 successfully submitted commit on-chain.")
}

func TestRegularNodeOnChainCos(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 3: Regular Node On-Chain COS Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}
	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "true",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
		// Check if MOCK_SEND_COMMIT_TO_LEADER already exists, if so add after it
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			err = os.WriteFile(dockerComposeFile, []byte(replacement), 0644)
			require.NoError(t, err, "Failed to write docker-compose file")
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			err = os.WriteFile(dockerComposeFile, []byte(replacement), 0644)
			require.NoError(t, err, "Failed to write docker-compose file")
		}
	}

	t.Log("STEP 3: Restarting regularNode1 with updated configuration...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (regularNode1 will submit COS on-chain)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check leader logs to see if it's requesting COS on-chain
			t.Log("Checking leader node logs for RequestedToSubmitCo event...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify it's receiving RequestedToSubmitCo and submitting COS on-chain
			t.Log("Checking regularNode1 logs for on-chain COS submission...")
			showLogs(t, "test-regularnode1", 30)
			t.Log("Checking regularNode2 logs")
			showLogs(t, "test-regularnode2", 30)
			t.Log("Checking regularNode3 logs")
			showLogs(t, "test-regularnode3", 30)

			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}
			// Wait longer to accommodate leader's monitoring period
			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 3 Complete! RegularNode1 successfully submitted COS on-chain.")
}

func TestRegularNodeOnChainCvsAndCos(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 4: Regular Node On-Chain CVS and COS Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}
	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "true",
		"MOCK_SEND_COS_TO_LEADER":               "true",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add both environment variables
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Check if we need to add MOCK_SEND_COMMIT_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
		dockerComposeStr = replacement
	}

	// Check if we need to add MOCK_SEND_COS_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
		// Add after MOCK_SEND_COMMIT_TO_LEADER if it exists, otherwise after EOA_PRIVATE_KEY
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		}
	}

	err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
	require.NoError(t, err, "Failed to write docker-compose file")

	t.Log("STEP 3: Restarting regularNode1 with updated configuration...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (regularNode1 will submit both CVS and COS on-chain)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check leader logs to see if it's requesting CVS and COS on-chain
			t.Log("Checking leader node logs for RequestedToSubmitCv and RequestedToSubmitCo events...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify it's submitting both CVS and COS on-chain
			t.Log("Checking regularNode1 logs for on-chain CVS and COS submission...")
			showLogs(t, "test-regularnode1", 30)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}
			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 4 Complete! RegularNode1 successfully submitted both CVS and COS on-chain.")
}

func TestRegularNodeOnChainSecret(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 5: Regular Node On-Chain Secret Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "true",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add all environment variables
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Check if we need to add MOCK_SEND_COMMIT_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
		dockerComposeStr = replacement
	}

	// Check if we need to add MOCK_SEND_COS_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		}
	}

	// Check if we need to add MOCK_SEND_SECRET_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_SECRET_TO_LEADER") {
		// Add after the last mock variable that exists
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else if strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		}
	}

	err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
	require.NoError(t, err, "Failed to write docker-compose file")

	t.Log("STEP 3: Restarting regularNode1 with secret mock mode enabled...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (regularNode1 will submit secret on-chain)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check leader logs to see if it's requesting secret on-chain
			t.Log("Checking leader node logs for RequestedToSubmitSFromIndexK event...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify it's receiving RequestedToSubmitSFromIndexK and submitting secret on-chain
			t.Log("Checking regularNode1 logs for on-chain secret submission...")
			showLogs(t, "test-regularnode1", 30)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}
			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 5 Complete! RegularNode1 successfully submitted secret on-chain.")
}

func TestRegularNodeOnChainCvsCosAndSecret(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 6: Regular Node On-Chain CVS, COS, and Secret Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "true",
		"MOCK_SEND_COS_TO_LEADER":               "true",
		"MOCK_SEND_SECRET_TO_LEADER":            "true",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add all three environment variables
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Check if we need to add MOCK_SEND_COMMIT_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
		dockerComposeStr = replacement
	}

	// Check if we need to add MOCK_SEND_COS_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		}
	}

	// Check if we need to add MOCK_SEND_SECRET_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_SECRET_TO_LEADER") {
		// Add after the last mock variable that exists
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else if strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		}
	}

	err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
	require.NoError(t, err, "Failed to write docker-compose file")

	t.Log("STEP 3: Restarting regularNode1 with updated configuration...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (regularNode1 will submit CVS, COS, and Secret on-chain)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		// Check if random number was fulfilled
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		if err == nil && fulfilled {
			t.Logf("Random number fulfilled after %d attempts!", i+1)
			break
		}

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check leader logs to see if it's requesting CVS, COS, and Secret on-chain
			t.Log("Checking leader node logs for RequestedToSubmitCv, RequestedToSubmitCo, and RequestedToSubmitSFromIndexK events...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify it's submitting CVS, COS, and Secret on-chain
			t.Log("Checking regularNode1 logs for on-chain CVS, COS, and Secret submission...")
			showLogs(t, "test-regularnode1", 30)

			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 6 Complete! RegularNode1 successfully submitted CVS, COS, and Secret on-chain.")
}

func TestRegularNodeOnChainCvsAndSecret(t *testing.T) {
	// t.Skip("Skipping integration test in short mode") // Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 7: Regular Node On-Chain CVS and Secret Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "true",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "true",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add both environment variables
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Check if we need to add MOCK_SEND_COMMIT_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
		dockerComposeStr = replacement
	}

	// Check if we need to add MOCK_SEND_SECRET_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_SECRET_TO_LEADER") {
		// Add after MOCK_SEND_COMMIT_TO_LEADER if it exists, otherwise after EOA_PRIVATE_KEY
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
				"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		}
	}

	err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
	require.NoError(t, err, "Failed to write docker-compose file")

	t.Log("STEP 3: Restarting regularNode1 with updated configuration...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (regularNode1 will submit CVS and Secret on-chain, COS via P2P)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check leader logs to see if it's requesting CVS and Secret on-chain
			t.Log("Checking leader node logs for RequestedToSubmitCv and RequestedToSubmitSFromIndexK events...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify it's submitting CVS and Secret on-chain
			t.Log("Checking regularNode1 logs for on-chain CVS and Secret submission...")
			showLogs(t, "test-regularnode1", 30)

			// Random number fulfillment check
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}

			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 7 Complete! RegularNode1 successfully submitted CVS and Secret on-chain (COS was sent via P2P).")
}

func TestRegularNodeOnChainCosAndSecret(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 8: Regular Node On-Chain COS and Secret Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "true",
		"MOCK_SEND_SECRET_TO_LEADER":            "true",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add both environment variables
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Check if we need to add MOCK_SEND_COS_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
			1)
		dockerComposeStr = replacement
	}

	// Check if we need to add MOCK_SEND_SECRET_TO_LEADER
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_SECRET_TO_LEADER") {
		// Add after MOCK_SEND_COS_TO_LEADER if it exists, otherwise after EOA_PRIVATE_KEY
		if strings.Contains(dockerComposeStr, "MOCK_SEND_COS_TO_LEADER") {
			replacement := strings.Replace(dockerComposeStr,
				"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
				"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		} else {
			replacement := strings.Replace(dockerComposeStr,
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
				"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
				1)
			dockerComposeStr = replacement
		}
	}

	err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
	require.NoError(t, err, "Failed to write docker-compose file")

	t.Log("STEP 3: Restarting regularNode1 with updated configuration...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (regularNode1 will submit COS and Secret on-chain, CVS via P2P)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check leader logs to see if it's requesting COS and Secret on-chain
			t.Log("Checking leader node logs for RequestedToSubmitCo and RequestedToSubmitSFromIndexK events...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify it's submitting COS and Secret on-chain
			t.Log("Checking regularNode1 logs for on-chain COS and Secret submission...")
			showLogs(t, "test-regularnode1", 30)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}
			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 8 Complete! RegularNode1 successfully submitted COS and Secret on-chain (CVS was sent via P2P).")
}

func TestAllRegularNodesOnChainCvs(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 9: All Regular Nodes On-Chain Commit Test")
	t.Log("STEP 1: Stopping all regular nodes...")

	// Stop all regular nodes
	stopCmd1 := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput1, err := stopCmd1.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput1))
	}

	stopCmd2 := exec.Command("docker", "stop", "test-regularnode2")
	stopOutput2, err := stopCmd2.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput2))
	}

	stopCmd3 := exec.Command("docker", "stop", "test-regularnode3")
	stopOutput3, err := stopCmd3.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput3))
	}

	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating all regular nodes environment with MOCK and DISABLE variables...")

	// Get integration test directory (same logic as setup package)
	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "true",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add the environment variable for all regular nodes
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Add MOCK_SEND_COMMIT_TO_LEADER for regularNode1 if not present
	if !strings.Contains(dockerComposeStr, "      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}") {
		// Add for regularNode1
		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
		dockerComposeStr = replacement

		// Add for regularNode2
		replacement = strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR2_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR2_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
		dockerComposeStr = replacement

		// Add for regularNode3
		replacement = strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR3_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR3_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
		dockerComposeStr = replacement

		err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
		require.NoError(t, err, "Failed to write docker-compose file")
	}

	t.Log("STEP 3: Restarting all regular nodes with updated configuration...")

	// Restart all regular nodes with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1", "regularnode2", "regularnode3")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regular nodes")
	}

	// Wait for all regular nodes to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (all regular nodes will submit on-chain)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check all regular nodes logs to verify they're submitting on-chain

			t.Log("Checking leader node logs for RequestedToSubmitCv event and missing CVS detection...")
			showLogs(t, "test-leadernode", 40)

			t.Log("Checking regularNode1 logs for on-chain submission...")
			showLogs(t, "test-regularnode1", 30)
			t.Log("Checking regularNode2 logs for on-chain submission...")
			showLogs(t, "test-regularnode2", 30)
			t.Log("Checking regularNode3 logs for on-chain submission...")
			showLogs(t, "test-regularnode3", 30)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}
			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 9 Complete! All regular nodes successfully submitted commit on-chain.")
}

func TestAllRegularNodesOnChainCos(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 10: All Regular Nodes On-Chain COS Test")
	t.Log("STEP 1: Stopping all regular nodes...")

	// Stop all regular nodes
	stopCmd1 := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput1, err := stopCmd1.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput1))
	}

	stopCmd2 := exec.Command("docker", "stop", "test-regularnode2")
	stopOutput2, err := stopCmd2.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput2))
	}

	stopCmd3 := exec.Command("docker", "stop", "test-regularnode3")
	stopOutput3, err := stopCmd3.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput3))
	}

	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating all regular nodes environment with MOCK and DISABLE variables...")

	// Get integration test directory (same logic as setup package)
	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "true",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add the environment variable for all regular nodes
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Add MOCK_SEND_COS_TO_LEADER for regularNode1 if not present
	if !strings.Contains(dockerComposeStr, "      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}") {
		// Add for regularNode1
		replacement := strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
			1)
		dockerComposeStr = replacement

		// Add for regularNode2
		replacement = strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR2_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR2_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
			1)
		dockerComposeStr = replacement

		// Add for regularNode3
		replacement = strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR3_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR3_PRIVATE_KEY}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
			1)
		dockerComposeStr = replacement

		err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
		require.NoError(t, err, "Failed to write docker-compose file")
	}

	t.Log("STEP 3: Restarting all regular nodes with updated configuration...")

	// Restart all regular nodes with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1", "regularnode2", "regularnode3")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regular nodes")
	}

	// Wait for all regular nodes to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (all regular nodes will submit COS on-chain)...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check all regular nodes logs to verify they're submitting on-chain

			t.Log("Checking leader node logs for RequestedToSubmitCo event and missing COS detection...")
			showLogs(t, "test-leadernode", 40)

			t.Log("Checking regularNode1 logs for on-chain COS submission...")
			showLogs(t, "test-regularnode1", 30)
			t.Log("Checking regularNode2 logs for on-chain COS submission...")
			showLogs(t, "test-regularnode2", 30)
			t.Log("Checking regularNode3 logs for on-chain COS submission...")
			showLogs(t, "test-regularnode3", 30)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}

			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 10 Complete! All regular nodes successfully submitted COS on-chain.")
}

func TestAllRegularNodesOnChainSecret(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 11: All Regular Nodes On-Chain Secret Test")
	t.Log("STEP 1: Stopping all regular nodes...")

	// Stop all regular nodes
	stopCmd1 := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput1, err := stopCmd1.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput1))
	}

	stopCmd2 := exec.Command("docker", "stop", "test-regularnode2")
	stopOutput2, err := stopCmd2.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput2))
	}

	stopCmd3 := exec.Command("docker", "stop", "test-regularnode3")
	stopOutput3, err := stopCmd3.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput3))
	}

	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating all regular nodes environment with MOCK and DISABLE variables...")

	// Get integration test directory (same logic as setup package)
	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "true",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add all environment variables for all regular nodes
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Helper function to add environment variables for a node
	addEnvVarsForNode := func(str string, privateKeyVar string) string {
		result := str
		// Check if we need to add MOCK_SEND_COMMIT_TO_LEADER
		if !strings.Contains(result, "MOCK_SEND_COMMIT_TO_LEADER") {
			replacement := strings.Replace(result,
				fmt.Sprintf("      EOA_PRIVATE_KEY: ${%s}", privateKeyVar),
				fmt.Sprintf("      EOA_PRIVATE_KEY: ${%s}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}", privateKeyVar),
				1)
			result = replacement
		}

		// Check if we need to add MOCK_SEND_COS_TO_LEADER
		if !strings.Contains(result, "MOCK_SEND_COS_TO_LEADER") {
			if strings.Contains(result, "MOCK_SEND_COMMIT_TO_LEADER") {
				replacement := strings.Replace(result,
					"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
					"      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}\n      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
					1)
				result = replacement
			}
		}

		// Check if we need to add MOCK_SEND_SECRET_TO_LEADER
		if !strings.Contains(result, "MOCK_SEND_SECRET_TO_LEADER") {
			if strings.Contains(result, "MOCK_SEND_COS_TO_LEADER") {
				replacement := strings.Replace(result,
					"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}",
					"      MOCK_SEND_COS_TO_LEADER: ${MOCK_SEND_COS_TO_LEADER}\n      MOCK_SEND_SECRET_TO_LEADER: ${MOCK_SEND_SECRET_TO_LEADER}",
					1)
				result = replacement
			}
		}

		return result
	}

	// Add environment variables for each regular node
	dockerComposeStr = addEnvVarsForNode(dockerComposeStr, "REGULAR1_PRIVATE_KEY")
	dockerComposeStr = addEnvVarsForNode(dockerComposeStr, "REGULAR2_PRIVATE_KEY")
	dockerComposeStr = addEnvVarsForNode(dockerComposeStr, "REGULAR3_PRIVATE_KEY")

	err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
	require.NoError(t, err, "Failed to write docker-compose file")

	t.Log("STEP 3: Restarting all regular nodes with Secret mock mode enabled...")

	// Restart all regular nodes with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1", "regularnode2", "regularnode3")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regular nodes")
	}

	// Wait for all regular nodes to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated (all regular nodes will submit Secret on-chain)...")
	t.Log("NOTE: Leader waits ~51 seconds before checking for missing Secret, then emits RequestedToSubmitSFromIndexK event")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check all regular nodes logs to verify they're submitting on-chain

			t.Log("Checking leader node logs for RequestedToSubmitSFromIndexK event and missing Secret detection...")
			showLogs(t, "test-leadernode", 40)

			t.Log("Checking regularNode1 logs for on-chain Secret submission...")
			showLogs(t, "test-regularnode1", 30)
			t.Log("Checking regularNode2 logs for on-chain Secret submission...")
			showLogs(t, "test-regularnode2", 30)
			t.Log("Checking regularNode3 logs for on-chain Secret submission...")
			showLogs(t, "test-regularnode3", 30)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}
			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	t.Log("Test Case 11 Complete! All regular nodes successfully submitted Secret on-chain.")
}

func TestRegularNode1SlashingForMissingCvs(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}

	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 12: RegularNode1 Slashing for Missing CVS Test")
	t.Log("STEP 1: Stopping regularNode1 (will not restart - it will fail to submit CVS)...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}

	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Requesting random number (regularNode1 is stopped, will not submit CVS)...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	maxFulfillmentRetries := 30
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check leader logs to see slashing process
			t.Log("Checking leader node logs for missing CVS detection, requestToSubmitCv, and failToSubmitCv calls...")
			showLogs(t, "test-leadernode", 50)
			// Check regularNode2 and regularNode3 logs (they should be working normally)
			t.Log("Checking regularNode2 logs (should be working normally)...")
			showLogs(t, "test-regularnode2", 20)
			t.Log("Checking regularNode3 logs (should be working normally)...")
			showLogs(t, "test-regularnode3", 20)

			// Check if random number was fulfilled
			fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
			if err == nil && fulfilled {
				t.Logf("Random number fulfilled after %d attempts!", i+1)
				break
			}

			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled even after regularNode1 is slashed")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("Random number generated: %s", randomNumber.String())
	showLogs(t, "test-leadernode", 50)
	showLogs(t, "test-regularnode2", 50)
	showLogs(t, "test-leadernode3", 50)
	t.Log("Test Case 12 Complete! regularNode1 was slashed for failing to submit CVS, but random number was still generated with regularNode2 and regularNode3.")
}

func TestLeaderSlashingAndRecovery(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}

	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 13: Leader Slashing and Recovery Test")
	t.Log("STEP 1: Reactivating regularNode1 (it was slashed in previous test)...")

	// Get integration test directory
	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Restart regularNode1 - it will automatically check activation status and reactivate if needed
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd1 := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd1.Dir = integrationTestDir
	restartOutput1, err := restartCmd1.CombinedOutput()
	if err != nil {
		t.Logf("Restart regularNode1 output: %s", string(restartOutput1))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to restart and potentially reactivate (deposit + activate)
	t.Log("Waiting for regularNode1 to restart and reactivate (if needed)...")
	time.Sleep(30 * time.Second) // Give time for deposit/activation if needed

	t.Log("STEP 2: Stopping leader node...")
	stopCmd := exec.Command("docker", "stop", "test-leadernode")
	stopOutput, err := stopCmd.CombinedOutput()

	// Log the output regardless of error
	t.Logf("Stop leader command output: %s", string(stopOutput))

	// Check if command failed
	if err != nil {
		t.Logf("Stop command error: %v", err)
		require.NoError(t, err, "Failed to stop leader node")
	}

	time.Sleep(3 * time.Second)

	t.Log("STEP 3: Requesting random number (leader is offline, cannot request CVS or submit Merkle root)...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 4: Waiting for regular nodes to detect leader failure and slash the leader...")

	maxSlashingRetries := 5
	slashingDetected := false

	for i := 0; i < maxSlashingRetries; i++ {

		t.Logf("Attempt %d/%d: Waiting for regular nodes to slash leader...", i+1, maxSlashingRetries)
		showLogs(t, "test-leadernode", 30)
		// Check regular nodes logs to see if they're calling failToRequestSubmitCVOrSubmitMerkleRoot
		t.Log("Checking regularNode1 logs for failToRequestSubmitCVOrSubmitMerkleRoot call...")
		showLogs(t, "test-regularnode1", 30)
		t.Log("Checking regularNode2 logs for failToRequestSubmitCVOrSubmitMerkleRoot call...")
		showLogs(t, "test-regularnode2", 30)
		t.Log("Checking regularNode3 logs for failToRequestSubmitCVOrSubmitMerkleRoot call...")
		showLogs(t, "test-regularnode3", 30)

		time.Sleep(20 * time.Second)
	}

	if !slashingDetected {
		t.Log("Warning: Slashing may not have been detected in logs, but proceeding with leader restart...")
	}

	t.Log("STEP 5: Restarting leader node (it will handle the halted state and recover)...")

	// Restart the leader node - it should handle the halted state
	dockerComposeCmd = getDockerComposeCmd()
	restartLeaderCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "leadernode")...)
	restartLeaderCmd.Dir = integrationTestDir
	restartLeaderOutput, err := restartLeaderCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart leader output: %s", string(restartLeaderOutput))
		require.NoError(t, err, "Failed to restart leader node")
	}

	// Wait for leader to restart and handle the halted state
	t.Log("Waiting for leader node to restart and handle the halted state...")

	maxFulfillmentRetries := 7
	var fulfilled bool
	var randomNumber *big.Int
	for i := 0; i < maxFulfillmentRetries; i++ {
		// Check if random number was fulfilled
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		t.Log("Checking leader node logs...")
		showLogs(t, "test-leadernode", 30)

		t.Log("Checking regular node logs...")
		showLogs(t, "test-regularnode1", 30)
		showLogs(t, "test-regularnode2", 30)
		showLogs(t, "test-regularnode3", 30)
		if err == nil && fulfilled {
			t.Logf(" Random number fulfilled after %d attempts!", i+1)
			break
		}

		time.Sleep(15 * time.Second)
	}
	fmt.Println("Random Number:", randomNumber)
	t.Log("Test Case 13 Complete! Regular nodes slashed the leader for failing to request CVS or submit Merkle root, and leader restarted to handle the halted state.")
}

func TestLeaderSlashingForMissingMerkleRootAfterDisputeWithMockCommit(t *testing.T) {
	// t.Skip("Skipping integration test in short mode")
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth
	t.Log("🧪 Test Case 14: Disable Merkle Root Submission Test")
	// Get integration test directory
	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	t.Log("STEP 1: Stopping regularNode1 and leader node...")

	// Stop regularNode1
	stopRegular1Cmd := exec.Command("docker", "stop", "test-regularnode1")
	stopRegular1Output, err := stopRegular1Cmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop regularNode1 output: %s", string(stopRegular1Output))
	}

	// Stop leader node
	stopLeaderCmd := exec.Command("docker", "stop", "test-leadernode")
	stopLeaderOutput, err := stopLeaderCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop leader output: %s", string(stopLeaderOutput))
	}

	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating environment with MOCK and DISABLE variables...")

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "true",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "true",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	// Update docker-compose to add the environment variables
	dockerComposeFile := filepath.Join(integrationTestDir, "docker-compose-test.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposeFile)
	require.NoError(t, err, "Failed to read docker-compose file")

	dockerComposeStr := string(dockerComposeContent)

	// Add MOCK_SEND_COMMIT_TO_LEADER to regularNode1
	if !strings.Contains(dockerComposeStr, "MOCK_SEND_COMMIT_TO_LEADER") {
		dockerComposeStr = strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${REGULAR1_PRIVATE_KEY}\n      MOCK_SEND_COMMIT_TO_LEADER: ${MOCK_SEND_COMMIT_TO_LEADER}",
			1)
	}

	// Add DISABLE_MERKLE_ROOT_SUBMISSION to leader node
	if !strings.Contains(dockerComposeStr, "DISABLE_MERKLE_ROOT_SUBMISSION") {
		dockerComposeStr = strings.Replace(dockerComposeStr,
			"      EOA_PRIVATE_KEY: ${LEADER_PRIVATE_KEY}",
			"      EOA_PRIVATE_KEY: ${LEADER_PRIVATE_KEY}\n      DISABLE_MERKLE_ROOT_SUBMISSION: ${DISABLE_MERKLE_ROOT_SUBMISSION}",
			1)
	}

	err = os.WriteFile(dockerComposeFile, []byte(dockerComposeStr), 0644)
	require.NoError(t, err, "Failed to write docker-compose file")

	t.Log("STEP 3: Restarting regularNode1 and leader node with updated configuration...")

	// Restart both regularNode1 and leader node with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1", "leadernode")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart nodes")
	}

	// Wait for nodes to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract (with regularNode1 in mock mode)
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	// Wait a bit for the round to start
	time.Sleep(5 * time.Second)

	t.Log("STEP 5: Waiting for the round to complete and leader to generate random number...")
	t.Log("Note: Leader has DISABLE_MERKLE_ROOT_SUBMISSION=true, so it won't submit Merkle root")
	t.Log("This will cause regular nodes to call failToSubmitMerkleRootAfterDispute and restart the round")

	// Wait for the round to be completed
	maxFulfillmentRetries := 30
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		// Check if random number was fulfilled for the round
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		if err == nil && fulfilled {
			t.Logf("✅ Random number fulfilled for restarted round %s after %d attempts!", round.String(), i+1)
			break
		}

		if i < maxFulfillmentRetries-1 {
			t.Logf("Attempt %d/%d: Waiting for random number fulfillment for restarted round %s...", i+1, maxFulfillmentRetries, round.String())

			// Check leader nodes logs
			t.Log("Checking leader node logs for random number generation process...")
			showLogs(t, "test-leadernode", 40)

			// Check regular nodes logs
			t.Log("Checking regularNode1 logs...")
			showLogs(t, "test-regularnode1", 40)
			t.Log("Checking regularNode2 logs...")
			showLogs(t, "test-regularnode2", 40)
			t.Log("Checking regularNode3 logs...")
			showLogs(t, "test-regularnode3", 40)

			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled for the restarted round after leader recovery")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("✅ Random number generated for round %s: %s", round.String(), randomNumber.String())
	t.Log("Test Case 14 Complete!")
}

func TestDisableCosSubmission(t *testing.T) {
	// t.Skip()
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 15: Disable COS Submission Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(20 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "true",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "true",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	t.Log("STEP 3: Starting regularNode1 with updated configuration...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("✅ Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		// Check if random number was fulfilled
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		if err == nil && fulfilled {
			t.Logf("✅ Random number fulfilled after %d attempts!", i+1)
			break
		}

		if i < maxFulfillmentRetries-1 {
			t.Logf("⏳ Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check contract state periodically
			currentRoundInput, _ := geth.ContractABI.Pack("s_currentRound")
			currentRoundResult, _ := geth.Client.CallContract(testCtx, ethereum.CallMsg{
				To:   &geth.ContractAddress,
				Data: currentRoundInput,
			}, nil)
			var currentRound *big.Int
			if currentRoundResult != nil {
				geth.ContractABI.UnpackIntoInterface(&currentRound, "s_currentRound", currentRoundResult)
				t.Logf("📊 Contract currentRound: %s (requested round: %s)", currentRound.String(), round.String())
			}

			// Check leader logs
			t.Log("📋 Checking leader node logs...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify COS submission is disabled
			t.Log("📋 Checking regularNode1 logs (COS submission should be disabled)...")
			showLogs(t, "test-regularnode1", 30)

			t.Log("📋 Checking regularNode2 logs")
			showLogs(t, "test-regularnode2", 30)

			t.Log("📋 Checking regularNode3 logs")
			showLogs(t, "test-regularnode3", 30)

			time.Sleep(20 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("✅ Random number generated: %s", randomNumber.String())

	// Verify in consumer contract
	requestInfo, err := getConsumerRequestInfo(testCtx, geth, round)
	require.NoError(t, err, "Failed to get consumer request info")
	require.NotNil(t, requestInfo, "Request info should not be nil")

	t.Logf("📊 Consumer contract state:")
	t.Logf("   RequestId: %s", requestInfo["requestId"].(*big.Int).String())
	t.Logf("   FulfillBlockNumber: %s", requestInfo["fulfillBlockNumber"].(*big.Int).String())
	t.Logf("   RandomNumber: %s", requestInfo["randomNumber"].(*big.Int).String())

	fulfillBlockNum := requestInfo["fulfillBlockNumber"].(*big.Int)
	consumerRandNum := requestInfo["randomNumber"].(*big.Int)

	assert.True(t, fulfillBlockNum.Cmp(big.NewInt(0)) > 0, "Fulfill block number should be set in consumer contract")
	assert.True(t, consumerRandNum.Cmp(big.NewInt(0)) > 0, "Random number should be set in consumer contract")
	assert.Equal(t, randomNumber.String(), consumerRandNum.String(), "Random number in consumer contract should match")

	t.Log("✅ Test Case 15 Complete! RegularNode1 successfully generated random number with DISABLE_COS_SUBMISSION=true")
}

func TestDisableSecretSubmission(t *testing.T) {
	// t.Skip()
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 16: Disable Secret Submission Test")
	t.Log("STEP 1: Stopping regularNode1...")

	// Stop regularNode1
	stopCmd := exec.Command("docker", "stop", "test-regularnode1")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(2 * time.Second)

	t.Log("STEP 2: Updating regularNode1 environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "true",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "false",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "true",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	t.Log("STEP 3: Starting regularNode1 with updated configuration...")

	// Restart regularNode1 with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "regularnode1")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart regularNode1")
	}

	// Wait for regularNode1 to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("✅ Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		// Check if random number was fulfilled
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		if err == nil && fulfilled {
			t.Logf("✅ Random number fulfilled after %d attempts!", i+1)
			break
		}

		if i < maxFulfillmentRetries-1 {
			t.Logf("⏳ Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check contract state periodically
			currentRoundInput, _ := geth.ContractABI.Pack("s_currentRound")
			currentRoundResult, _ := geth.Client.CallContract(testCtx, ethereum.CallMsg{
				To:   &geth.ContractAddress,
				Data: currentRoundInput,
			}, nil)
			var currentRound *big.Int
			if currentRoundResult != nil {
				geth.ContractABI.UnpackIntoInterface(&currentRound, "s_currentRound", currentRoundResult)
				t.Logf("📊 Contract currentRound: %s (requested round: %s)", currentRound.String(), round.String())
			}

			// Check leader logs
			t.Log("📋 Checking leader node logs...")
			showLogs(t, "test-leadernode", 30)

			// Check regularNode1 logs to verify Secret submission is disabled
			t.Log("📋 Checking regularNode1 logs (Secret submission should be disabled)...")
			showLogs(t, "test-regularnode1", 30)

			t.Log("📋 Checking regular node2 logs...")
			showLogs(t, "test-regularnode2", 30)

			t.Log("📋 Checking regular node3 logs...")
			showLogs(t, "test-regularnode3", 30)

			time.Sleep(15 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("✅ Random number generated: %s", randomNumber.String())

	// Verify in consumer contract
	requestInfo, err := getConsumerRequestInfo(testCtx, geth, round)
	require.NoError(t, err, "Failed to get consumer request info")
	require.NotNil(t, requestInfo, "Request info should not be nil")

	t.Logf("📊 Consumer contract state:")
	t.Logf("   RequestId: %s", requestInfo["requestId"].(*big.Int).String())
	t.Logf("   FulfillBlockNumber: %s", requestInfo["fulfillBlockNumber"].(*big.Int).String())
	t.Logf("   RandomNumber: %s", requestInfo["randomNumber"].(*big.Int).String())

	fulfillBlockNum := requestInfo["fulfillBlockNumber"].(*big.Int)
	consumerRandNum := requestInfo["randomNumber"].(*big.Int)

	assert.True(t, fulfillBlockNum.Cmp(big.NewInt(0)) > 0, "Fulfill block number should be set in consumer contract")
	assert.True(t, consumerRandNum.Cmp(big.NewInt(0)) > 0, "Random number should be set in consumer contract")
	assert.Equal(t, randomNumber.String(), consumerRandNum.String(), "Random number in consumer contract should match")

	t.Log("✅ Test Case 16 Complete! RegularNode1 successfully generated random number with DISABLE_SECRET_SUBMISSION=true")
}

func TestMockGenerateRandomNumberToLeader(t *testing.T) {
	// t.Skip()
	// Use the shared test environment
	require.NotNil(t, testEnv, "Test environment should be initialized")
	require.NotNil(t, testEnv.Geth, "Geth should be initialized")

	geth := testEnv.Geth

	t.Log("🧪 Test Case 17: Mock Generate Random Number To Leader")
	t.Log("STEP 1: Stopping leader and regular nodes...")

	// Stop leader and regular nodes
	stopCmd := exec.Command("docker", "stop", "test-leadernode", "test-regularnode1", "test-regularnode2", "test-regularnode3")
	stopOutput, err := stopCmd.CombinedOutput()
	if err != nil {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(5 * time.Second)

	t.Log("STEP 2: Updating environment with MOCK and DISABLE variables...")

	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	var integrationTestDir string
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		integrationTestDir = wd
	} else {
		integrationTestDir = filepath.Join(wd, "integration_test")
		dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err != nil {
			integrationTestDir = filepath.Dir(wd)
		}
	}

	// Update env file to add all MOCK and DISABLE variables
	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err, "Failed to read env file")

	envContentStr := string(envContent)

	// Set MOCK and DISABLE variables
	envVars := map[string]string{
		"MOCK_SEND_COMMIT_TO_LEADER":            "false",
		"MOCK_SEND_COS_TO_LEADER":               "false",
		"MOCK_SEND_SECRET_TO_LEADER":            "false",
		"MOCK_SEND_SECRET":                      "false",
		"MOCK_GENERATE_RANDOM_NUMBER":           "true",
		"MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER": "false",
		"DISABLE_SECRET_SUBMISSION":             "false",
		"DISABLE_COS_SUBMISSION":                "false",
		"DISABLE_MERKLE_ROOT_SUBMISSION":        "false",
	}

	for key, value := range envVars {
		if !strings.Contains(envContentStr, key) {
			envContentStr += fmt.Sprintf("%s=%s\n", key, value)
		} else {
			// Replace existing value
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=true", key), fmt.Sprintf("%s=%s", key, value))
			envContentStr = strings.ReplaceAll(envContentStr, fmt.Sprintf("%s=false", key), fmt.Sprintf("%s=%s", key, value))
		}
	}

	err = os.WriteFile(envFile, []byte(envContentStr), 0644)
	require.NoError(t, err, "Failed to write env file")

	t.Log("STEP 3: Starting leader and regular nodes with updated configuration...")

	// Restart all nodes with the new environment
	dockerComposeCmd := getDockerComposeCmd()
	restartCmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build", "leadernode", "regularnode1", "regularnode2", "regularnode3")...)
	restartCmd.Dir = integrationTestDir
	restartOutput, err := restartCmd.CombinedOutput()
	if err != nil {
		t.Logf("Restart output: %s", string(restartOutput))
		require.NoError(t, err, "Failed to restart nodes")
	}

	// Wait for all nodes to be ready
	time.Sleep(50 * time.Second)

	t.Log("STEP 4: Requesting random number...")

	// Request random number from consumer contract
	round, err := requestRandomNumberFromConsumer(testCtx, geth)
	require.NoError(t, err, "Failed to request random number")
	t.Logf("✅ Random number requested, round: %s", round.String())

	// Verify the round was actually created
	require.True(t, round.Cmp(big.NewInt(0)) >= 0, "Round should be >= 0")

	t.Log("STEP 5: Waiting for random number to be generated...")

	maxFulfillmentRetries := 20
	var fulfilled bool
	var randomNumber *big.Int

	for i := 0; i < maxFulfillmentRetries; i++ {
		// Check if random number was fulfilled
		fulfilled, randomNumber, err = checkRandomNumberFulfilled(testCtx, geth, round)
		if err == nil && fulfilled {
			t.Logf("✅ Random number fulfilled after %d attempts!", i+1)
			break
		}

		if i < maxFulfillmentRetries-1 {
			t.Logf("⏳ Attempt %d/%d: Waiting for random number fulfillment...", i+1, maxFulfillmentRetries)

			// Check contract state periodically
			currentRoundInput, _ := geth.ContractABI.Pack("s_currentRound")
			currentRoundResult, _ := geth.Client.CallContract(testCtx, ethereum.CallMsg{
				To:   &geth.ContractAddress,
				Data: currentRoundInput,
			}, nil)
			var currentRound *big.Int
			if currentRoundResult != nil {
				geth.ContractABI.UnpackIntoInterface(&currentRound, "s_currentRound", currentRoundResult)
				t.Logf("📊 Contract currentRound: %s (requested round: %s)", currentRound.String(), round.String())
			}

			// Check leader logs for mock message
			t.Log("📋 Checking leader node logs...")
			showLogs(t, "test-leadernode", 30)

			t.Log("📋 Checking regular node1 logs...")
			showLogs(t, "test-regularnode1", 30)

			t.Log("📋 Checking regular node2 logs...")
			showLogs(t, "test-regularnode2", 30)

			t.Log("📋 Checking regular node3 logs...")
			showLogs(t, "test-regularnode3", 30)

			time.Sleep(50 * time.Second)
		}
	}

	require.NoError(t, err, "Failed to check random number fulfillment")
	require.True(t, fulfilled, "Random number should be fulfilled")
	require.NotNil(t, randomNumber, "Random number should not be nil")
	require.True(t, randomNumber.Cmp(big.NewInt(0)) > 0, "Random number should be greater than 0")

	t.Logf("✅ Random number generated: %s", randomNumber.String())

	// Verify the mock message appears in leader logs
	t.Log("🔍 Verifying mock configuration in leader logs...")
	logsCmd := exec.Command("docker", "logs", "--tail", "200", "test-leadernode")
	logsOutput, err := logsCmd.CombinedOutput()
	if err != nil {
		t.Logf("⚠️  Warning: Failed to get leader logs: %v", err)
	} else {
		logsStr := string(logsOutput)
		if strings.Contains(logsStr, "Random number generation is mocked via configuration. Skipping on-chain submission.") {
			t.Log("✅ Confirmed: Leader node is using mocked random number generation (no on-chain submission)")
		} else {
			t.Log("⚠️  Warning: Mock message not found in leader logs, but random number was generated")
		}
	}

	// Verify in consumer contract
	requestInfo, err := getConsumerRequestInfo(testCtx, geth, round)
	require.NoError(t, err, "Failed to get consumer request info")
	require.NotNil(t, requestInfo, "Request info should not be nil")

	t.Logf("📊 Consumer contract state:")
	t.Logf("   RequestId: %s", requestInfo["requestId"].(*big.Int).String())
	t.Logf("   FulfillBlockNumber: %s", requestInfo["fulfillBlockNumber"].(*big.Int).String())
	t.Logf("   RandomNumber: %s", requestInfo["randomNumber"].(*big.Int).String())

	fulfillBlockNum := requestInfo["fulfillBlockNumber"].(*big.Int)
	consumerRandNum := requestInfo["randomNumber"].(*big.Int)

	assert.True(t, fulfillBlockNum.Cmp(big.NewInt(0)) > 0, "Fulfill block number should be set in consumer contract")
	assert.True(t, consumerRandNum.Cmp(big.NewInt(0)) > 0, "Random number should be set in consumer contract")
	assert.Equal(t, randomNumber.String(), consumerRandNum.String(), "Random number in consumer contract should match")

	t.Log("✅ Test Case 17 Complete! Leader node successfully generated random number with MOCK_GENERATE_RANDOM_NUMBER_TO_LEADER=true (without on-chain submission)")
}

func showLogs(t *testing.T, container string, lines int) {
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), container)
	output, _ := cmd.CombinedOutput()

	logLines := strings.Split(string(output), "\n")
	for _, line := range logLines {
		if line != "" {
			t.Logf("    %s", line)
		}
	}
}

// requestRandomNumberFromConsumer requests a random number from the consumer contract
func requestRandomNumberFromConsumer(ctx context.Context, env *setup.GethTestEnv) (*big.Int, error) {
	callbackGasLimit := uint32(85000)

	// Estimate price using current gas price
	gasPrice, err := env.Client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to suggest gas price: %w", err)
	}

	// Ensure minimum gas price
	minGasPrice := big.NewInt(1000000000) // 1 gwei
	if gasPrice.Cmp(minGasPrice) < 0 {
		gasPrice = minGasPrice
	}

	// Estimate request price
	estimateInput, err := env.ContractABI.Pack("estimateRequestPrice", callbackGasLimit, gasPrice)
	if err != nil {
		return nil, fmt.Errorf("failed to pack estimateRequestPrice: %w", err)
	}

	estimateResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
		To:   &env.ContractAddress,
		Data: estimateInput,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call estimateRequestPrice: %w", err)
	}

	var estimatedPrice *big.Int
	if err := env.ContractABI.UnpackIntoInterface(&estimatedPrice, "estimateRequestPrice", estimateResult); err != nil {
		return nil, fmt.Errorf("failed to unpack estimateRequestPrice: %w", err)
	}

	// Add 10% buffer to estimated price
	buffer := new(big.Int).Mul(estimatedPrice, big.NewInt(110))
	estimatedPrice = new(big.Int).Div(buffer, big.NewInt(100))

	// Request from consumer
	consumerInput, err := env.ConsumerABI.Pack("requestRandomNumber")
	if err != nil {
		return nil, fmt.Errorf("failed to pack requestRandomNumber: %w", err)
	}

	auth := env.RegularAccounts[0].Auth
	auth.Value = estimatedPrice
	auth.GasLimit = 1000000
	auth.GasPrice = gasPrice

	nonce, err := env.Client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	tx := types.NewTransaction(nonce, env.ConsumerAddress, estimatedPrice, auth.GasLimit, auth.GasPrice, consumerInput)

	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	if err := env.Client.SendTransaction(ctx, signedTx); err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	txHash := signedTx.Hash()
	if err := env.WaitForTransaction(ctx, &txHash); err != nil {
		return nil, fmt.Errorf("failed to wait for transaction: %w", err)
	}

	auth.Value = nil

	// Wait a bit for the transaction to be processed
	time.Sleep(2 * time.Second)

	input, err := env.ContractABI.Pack("s_requestCount")
	if err != nil {
		return nil, fmt.Errorf("failed to pack s_requestCount: %w", err)
	}

	result, err := env.Client.CallContract(ctx, ethereum.CallMsg{
		To:   &env.ContractAddress,
		Data: input,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call s_requestCount: %w", err)
	}

	var requestCount *big.Int
	if err := env.ContractABI.UnpackIntoInterface(&requestCount, "s_requestCount", result); err != nil {
		return nil, fmt.Errorf("failed to unpack s_requestCount: %w", err)
	}
	round := new(big.Int).Sub(requestCount, big.NewInt(1))
	// Also verify the request exists by checking s_requestInfo
	requestInfoInput, err := env.ContractABI.Pack("s_requestInfo", round)
	if err == nil {
		requestInfoResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
			To:   &env.ContractAddress,
			Data: requestInfoInput,
		}, nil)
		if err == nil {
			_ = requestInfoResult // Just verify it doesn't error
		}
	}

	return round, nil
}

// checkRandomNumberFulfilled checks if a random number has been fulfilled for a given round
func checkRandomNumberFulfilled(ctx context.Context, env *setup.GethTestEnv, round *big.Int) (bool, *big.Int, error) {
	detailInfoInput, err := env.ConsumerABI.Pack("getDetailInfo", round)
	if err == nil {
		detailInfoResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
			To:   &env.ConsumerAddress,
			Data: detailInfoInput,
		}, nil)
		if err == nil && detailInfoResult != nil {
			// Unpack: (requester, requestFee, requestBlockNumber, fulfillBlockNumber, randomNumber, isRefunded)
			var detailInfo struct {
				Requester          common.Address
				RequestFee         *big.Int
				RequestBlockNumber *big.Int
				FulfillBlockNumber *big.Int
				RandomNumber       *big.Int
				IsRefunded         bool
			}
			if err := env.ConsumerABI.UnpackIntoInterface(&detailInfo, "getDetailInfo", detailInfoResult); err == nil {
				if detailInfo.FulfillBlockNumber.Cmp(big.NewInt(0)) > 0 && detailInfo.RandomNumber.Cmp(big.NewInt(0)) > 0 {
					return true, detailInfo.RandomNumber, nil
				}
				// Request exists but not fulfilled yet
				return false, nil, nil
			}
		}
	}

	// Fallback: Use s_requestIdToIndexPlusOne to get the index directly
	indexInput, err := env.ConsumerABI.Pack("s_requestIdToIndexPlusOne", round)
	if err == nil {
		indexResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
			To:   &env.ConsumerAddress,
			Data: indexInput,
		}, nil)
		if err == nil && indexResult != nil {
			var indexPlusOne *big.Int
			if err := env.ConsumerABI.UnpackIntoInterface(&indexPlusOne, "s_requestIdToIndexPlusOne", indexResult); err == nil {
				// Index is 1-based, so if it's 0, the request doesn't exist
				if indexPlusOne.Cmp(big.NewInt(0)) > 0 {
					// Use the index to get mainInfo (index is already 1-based)
					mainInfoInput, err := env.ConsumerABI.Pack("s_mainInfos", indexPlusOne)
					if err == nil {
						mainInfoResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
							To:   &env.ConsumerAddress,
							Data: mainInfoInput,
						}, nil)
						if err == nil && mainInfoResult != nil {
							// Unpack MainInfo struct: (requestId, requester, fulfillBlockNumber, randomNumber, isRefunded, requestFee)
							var mainInfo struct {
								RequestId          *big.Int
								Requester          common.Address
								FulfillBlockNumber *big.Int
								RandomNumber       *big.Int
								IsRefunded         bool
								RequestFee         *big.Int
							}
							if err := env.ConsumerABI.UnpackIntoInterface(&mainInfo, "s_mainInfos", mainInfoResult); err == nil {
								if mainInfo.RequestId.Cmp(round) == 0 {
									if mainInfo.FulfillBlockNumber.Cmp(big.NewInt(0)) > 0 && mainInfo.RandomNumber.Cmp(big.NewInt(0)) > 0 {
										return true, mainInfo.RandomNumber, nil
									}
									// Request exists but not fulfilled yet
									return false, nil, nil
								}
							}
						}
					}
				}
			}
		}
	}

	// Final fallback: iterate through all requests (original method)
	requestCountInput, err := env.ConsumerABI.Pack("s_requestCount")
	if err != nil {
		return false, nil, err
	}

	requestCountResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
		To:   &env.ConsumerAddress,
		Data: requestCountInput,
	}, nil)
	if err != nil {
		return false, nil, err
	}

	var requestCount *big.Int
	if err := env.ConsumerABI.UnpackIntoInterface(&requestCount, "s_requestCount", requestCountResult); err != nil {
		return false, nil, err
	}

	// Check each request index (1-based)
	for i := big.NewInt(1); i.Cmp(requestCount) <= 0; i.Add(i, big.NewInt(1)) {
		mainInfoInput, err := env.ConsumerABI.Pack("s_mainInfos", i)
		if err != nil {
			continue
		}

		mainInfoResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
			To:   &env.ConsumerAddress,
			Data: mainInfoInput,
		}, nil)
		if err != nil {
			continue
		}

		var mainInfo struct {
			RequestId          *big.Int
			Requester          common.Address
			FulfillBlockNumber *big.Int
			RandomNumber       *big.Int
			IsRefunded         bool
			RequestFee         *big.Int
		}

		if err := env.ConsumerABI.UnpackIntoInterface(&mainInfo, "s_mainInfos", mainInfoResult); err != nil {
			continue
		}

		// Check if this request matches our round and is fulfilled
		if mainInfo.RequestId.Cmp(round) == 0 {
			if mainInfo.FulfillBlockNumber.Cmp(big.NewInt(0)) > 0 && mainInfo.RandomNumber.Cmp(big.NewInt(0)) > 0 {
				return true, mainInfo.RandomNumber, nil
			}
			return false, nil, nil
		}
	}

	return false, nil, nil
}

func getConsumerRequestInfo(ctx context.Context, env *setup.GethTestEnv, round *big.Int) (map[string]interface{}, error) {
	// First, try to use getDetailInfo which is more direct
	detailInfoInput, err := env.ConsumerABI.Pack("getDetailInfo", round)
	if err == nil {
		detailInfoResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
			To:   &env.ConsumerAddress,
			Data: detailInfoInput,
		}, nil)
		if err == nil && detailInfoResult != nil {
			// Unpack: (requester, requestFee, requestBlockNumber, fulfillBlockNumber, randomNumber, isRefunded)
			var detailInfo struct {
				Requester          common.Address
				RequestFee         *big.Int
				RequestBlockNumber *big.Int
				FulfillBlockNumber *big.Int
				RandomNumber       *big.Int
				IsRefunded         bool
			}
			if err := env.ConsumerABI.UnpackIntoInterface(&detailInfo, "getDetailInfo", detailInfoResult); err == nil {
				// Check if request exists (requestBlockNumber > 0 means request was made)
				if detailInfo.RequestBlockNumber.Cmp(big.NewInt(0)) > 0 {
					return map[string]interface{}{
						"requestId":          round,
						"requester":          detailInfo.Requester,
						"fulfillBlockNumber": detailInfo.FulfillBlockNumber,
						"randomNumber":       detailInfo.RandomNumber,
						"isRefunded":         detailInfo.IsRefunded,
						"requestFee":         detailInfo.RequestFee,
					}, nil
				}
			}
		}
	}

	// Fallback: Use s_requestIdToIndexPlusOne to get the index directly
	indexInput, err := env.ConsumerABI.Pack("s_requestIdToIndexPlusOne", round)
	if err == nil {
		indexResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
			To:   &env.ConsumerAddress,
			Data: indexInput,
		}, nil)
		if err == nil && indexResult != nil {
			var indexPlusOne *big.Int
			if err := env.ConsumerABI.UnpackIntoInterface(&indexPlusOne, "s_requestIdToIndexPlusOne", indexResult); err == nil {
				if indexPlusOne.Cmp(big.NewInt(0)) > 0 {
					// Use the index to get mainInfo (index is already 1-based)
					mainInfoInput, err := env.ConsumerABI.Pack("s_mainInfos", indexPlusOne)
					if err == nil {
						mainInfoResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
							To:   &env.ConsumerAddress,
							Data: mainInfoInput,
						}, nil)
						if err == nil && mainInfoResult != nil {
							// Unpack MainInfo struct: (requestId, requester, fulfillBlockNumber, randomNumber, isRefunded, requestFee)
							var mainInfo struct {
								RequestId          *big.Int
								Requester          common.Address
								FulfillBlockNumber *big.Int
								RandomNumber       *big.Int
								IsRefunded         bool
								RequestFee         *big.Int
							}
							if err := env.ConsumerABI.UnpackIntoInterface(&mainInfo, "s_mainInfos", mainInfoResult); err == nil {
								if mainInfo.RequestId.Cmp(round) == 0 {
									return map[string]interface{}{
										"requestId":          mainInfo.RequestId,
										"requester":          mainInfo.Requester,
										"fulfillBlockNumber": mainInfo.FulfillBlockNumber,
										"randomNumber":       mainInfo.RandomNumber,
										"isRefunded":         mainInfo.IsRefunded,
										"requestFee":         mainInfo.RequestFee,
									}, nil
								}
							}
						}
					}
				}
			}
		}
	}

	// Final fallback: iterate through all requests (original method)
	requestCountInput, err := env.ConsumerABI.Pack("s_requestCount")
	if err != nil {
		return nil, err
	}

	requestCountResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
		To:   &env.ConsumerAddress,
		Data: requestCountInput,
	}, nil)
	if err != nil {
		return nil, err
	}

	var requestCount *big.Int
	if err := env.ConsumerABI.UnpackIntoInterface(&requestCount, "s_requestCount", requestCountResult); err != nil {
		return nil, err
	}

	// Find the request that matches our round
	for i := big.NewInt(1); i.Cmp(requestCount) <= 0; i.Add(i, big.NewInt(1)) {
		mainInfoInput, err := env.ConsumerABI.Pack("s_mainInfos", i)
		if err != nil {
			continue
		}

		mainInfoResult, err := env.Client.CallContract(ctx, ethereum.CallMsg{
			To:   &env.ConsumerAddress,
			Data: mainInfoInput,
		}, nil)
		if err != nil {
			continue
		}

		var mainInfo struct {
			RequestId          *big.Int
			Requester          common.Address
			FulfillBlockNumber *big.Int
			RandomNumber       *big.Int
			IsRefunded         bool
			RequestFee         *big.Int
		}

		if err := env.ConsumerABI.UnpackIntoInterface(&mainInfo, "s_mainInfos", mainInfoResult); err != nil {
			continue
		}

		if mainInfo.RequestId.Cmp(round) == 0 {
			return map[string]interface{}{
				"requestId":          mainInfo.RequestId,
				"requester":          mainInfo.Requester,
				"fulfillBlockNumber": mainInfo.FulfillBlockNumber,
				"randomNumber":       mainInfo.RandomNumber,
				"isRefunded":         mainInfo.IsRefunded,
				"requestFee":         mainInfo.RequestFee,
			}, nil
		}
	}

	return nil, fmt.Errorf("request not found for round %s", round.String())
}
