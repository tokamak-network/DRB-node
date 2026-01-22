package setup

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// getIntegrationTestDir returns the absolute path to the integration_test directory
func getIntegrationTestDir() (string, error) {
	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	dockerComposePath := filepath.Join(wd, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		return filepath.Abs(wd)
	}
	integrationTestDir := filepath.Join(wd, "..")
	dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		absPath, err := filepath.Abs(integrationTestDir)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}
		return absPath, nil
	}

	// Check if we're in project root
	integrationTestDir = filepath.Join(wd, "integration_test")
	dockerComposePath = filepath.Join(integrationTestDir, "docker-compose-test.yml")
	if _, err := os.Stat(dockerComposePath); err == nil {
		absPath, err := filepath.Abs(integrationTestDir)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}
		return absPath, nil
	}
	currentDir := wd
	for i := 0; i < 5; i++ {
		integrationTestDir := filepath.Join(currentDir, "integration_test")
		dockerComposePath := filepath.Join(integrationTestDir, "docker-compose-test.yml")
		if _, err := os.Stat(dockerComposePath); err == nil {
			absPath, err := filepath.Abs(integrationTestDir)
			if err != nil {
				return "", fmt.Errorf("failed to get absolute path: %w", err)
			}
			return absPath, nil
		}
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	return "", fmt.Errorf("could not find integration_test directory")
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
	return []string{"docker", "compose"}
}

func getHostIP() string {
	// On macOS and Windows, Docker provides host.docker.internal out of the box
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return "host.docker.internal"
	}

	// On Linux, we need to detect the Docker gateway IP
	// Try to get the default gateway from ip route
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err == nil {
		// Parse output like "default via 172.17.0.1 dev docker0"
		fields := strings.Fields(string(output))
		for i, field := range fields {
			if field == "via" && i+1 < len(fields) {
				ip := fields[i+1]
				// Validate it's an IP address
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}

	// Fallback: try to get Docker bridge gateway
	cmd = exec.Command("docker", "network", "inspect", "bridge", "--format", "{{range .IPAM.Config}}{{.Gateway}}{{end}}")
	output, err = cmd.Output()
	if err == nil {
		ip := strings.TrimSpace(string(output))
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	// Last resort: common Docker bridge gateway IP
	return "172.17.0.1"
}

type TestEnvironment struct {
	Geth         *GethTestEnv
	EnvFile      string
	LeaderPeerID string
	ActivatedOps []common.Address
	CleanupFunc  func()
}

// Logger interface for test setup logging
type Logger interface {
	Log(args ...interface{})
	Logf(format string, args ...interface{})
}

func SetupTestEnvironment(ctx context.Context, logger interface{}) (*TestEnvironment, error) {
	// Convert logger to our Logger interface
	var log Logger
	switch l := logger.(type) {
	case *testing.T:
		log = &testingTLogger{t: l}
	case Logger:
		log = l
	default:
		return nil, fmt.Errorf("logger must be *testing.T or implement Logger interface")
	}

	return setupTestEnvironmentWithLogger(ctx, log)
}

// testingTLogger wraps *testing.T to implement Logger interface
type testingTLogger struct {
	t *testing.T
}

func (l *testingTLogger) Log(args ...interface{}) {
	l.t.Log(args...)
}

func (l *testingTLogger) Logf(format string, args ...interface{}) {
	l.t.Logf(format, args...)
}

func setupTestEnvironmentWithLogger(ctx context.Context, t Logger) (*TestEnvironment, error) {
	// Get the absolute path to integration_test directory
	integrationTestDir, err := getIntegrationTestDir()
	if err != nil {
		return nil, fmt.Errorf("failed to find integration_test directory: %w", err)
	}

	env := &TestEnvironment{}

	// Setup cleanup function
	env.CleanupFunc = func() {
		cleanupDocker(t, integrationTestDir)
		if env.EnvFile != "" {
			os.Remove(env.EnvFile)
		}
	}

	t.Log("\n STEP 0: Cleaning Up Databases")

	dockerComposeCmd := getDockerComposeCmd()
	stopCmd := exec.CommandContext(ctx, dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"down", "-v", "--remove-orphans")...)
	stopCmd.Dir = integrationTestDir
	stopOutput, _ := stopCmd.CombinedOutput()
	if len(stopOutput) > 0 {
		t.Logf("Stop output: %s", string(stopOutput))
	}
	time.Sleep(3 * time.Second)

	containersToCheck := []string{
		"test-leadernode",
		"test-regularnode1",
		"test-regularnode2",
		"test-regularnode3",
		"test-leaderpostgres",
		"test-regular1postgres",
		"test-regular2postgres",
		"test-regular3postgres",
	}

	allStopped := true
	for _, container := range containersToCheck {
		running, err := isContainerRunningQuick(container)
		if err == nil && running {
			t.Logf("Container %s is still running, forcing stop...", container)
			forceStopCmd := exec.CommandContext(ctx, "docker", "stop", "-t", "2", container)
			forceStopCmd.Run()
			forceRemoveCmd := exec.CommandContext(ctx, "docker", "rm", "-f", container)
			forceRemoveCmd.Run()
			allStopped = false
		}
	}
	t.Log("Removing Docker volumes...")
	volumeNames := []string{
		"drb-test_test-leaderpostgres-data",
		"drb-test_test-regular1postgres-data",
		"drb-test_test-regular2postgres-data",
		"drb-test_test-regular3postgres-data",
	}
	for _, volume := range volumeNames {
		removeVolumeCmd := exec.CommandContext(ctx, "docker", "volume", "rm", "-f", volume)
		removeVolumeCmd.Run() // Ignore errors if volume doesn't exist
	}

	if allStopped {
		t.Log("All containers stopped successfully")
	} else {
		t.Log("Containers forcefully stopped")
	}
	t.Log("Database cleanup complete")

	t.Log("\nSTEP 1: Starting Geth and deploying contracts...")

	geth, err := StartGethDevNode(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start Geth: %w", err)
	}
	env.Geth = geth

	err = geth.DeployContracts(ctx)
	if err != nil {
		geth.Cleanup()
		return nil, fmt.Errorf("failed to deploy contracts: %w", err)
	}

	t.Logf("Commit2RevealDRB: %s", geth.ContractAddress.Hex())
	t.Logf("ConsumerExampleV2: %s", geth.ConsumerAddress.Hex())

	t.Log("\nSTEP 2: Generating node peer IDs...")
	// Get project root directory (parent of integration_test)
	projectRoot := filepath.Join(integrationTestDir, "..")
	projectRootAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		geth.Cleanup()
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}

	// Generate leader peer ID
	leaderPeerID, err := generateLeaderPeerIDForTest(projectRootAbs)
	if err != nil {
		geth.Cleanup()
		return nil, fmt.Errorf("failed to generate leader peer ID: %w", err)
	}
	t.Logf("Generated LEADER_PEER_ID: %s", leaderPeerID)

	// Generate peer IDs for regular nodes 1, 2, 3
	regularPeerIDs := make([]string, 3)
	for i := 1; i <= 3; i++ {
		peerID, err := generateRegularPeerIDForTest(projectRootAbs, i)
		if err != nil {
			geth.Cleanup()
			return nil, fmt.Errorf("failed to generate peer ID for regular node %d: %w", i, err)
		}
		regularPeerIDs[i-1] = peerID
		t.Logf("Generated REGULAR%d_PEER_ID: %s", i, peerID)
	}

	t.Log("\nSTEP 3: Configuring Docker environment...")
	wslHostIP := getHostIP()
	ethRPCURL := fmt.Sprintf("ws://%s:8546", wslHostIP)
	t.Logf("Detected host IP: %s", wslHostIP)
	t.Logf("Using ETH_RPC_URLS: %s", ethRPCURL)

	envContent := fmt.Sprintf(`CONTRACT_ADDRESS=%s
CONSUMER_ADDRESS=%s
ETH_RPC_URLS=%s
CHAIN_ID=%s
LEADER_PRIVATE_KEY=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
LEADER_EOA=%s
LEADER_PORT=61280
LEADER_PEER_ID=%s
REGULAR1_PRIVATE_KEY=59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d
REGULAR1_PEER_ID=%s
REGULAR1_PORT=61281
REGULAR2_PRIVATE_KEY=5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a
REGULAR2_PEER_ID=%s
REGULAR2_PORT=61282
REGULAR3_PRIVATE_KEY=7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6
REGULAR3_PEER_ID=%s
REGULAR3_PORT=61283
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
MOCK_SEND_COMMIT_TO_LEADER=false
MOCK_SEND_COS_TO_LEADER=false
MOCK_SEND_SECRET_TO_LEADER=false
DISABLE_COS_SUBMISSION=false
DISABLE_SECRET_SUBMISSION=false
DISABLE_MERKLE_ROOT_SUBMISSION=false
MOCK_GENERATE_RANDOM_NUMBER=false
`,
		geth.ContractAddress.Hex(),
		geth.ConsumerAddress.Hex(),
		ethRPCURL,
		geth.ChainID.String(),
		geth.LeaderAccount.Address.Hex(),
		leaderPeerID,
		regularPeerIDs[0],
		regularPeerIDs[1],
		regularPeerIDs[2],
	)

	envFile := filepath.Join(integrationTestDir, ".env.docker-test")
	err = os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		geth.Cleanup()
		return nil, fmt.Errorf("failed to write env file: %w", err)
	}
	env.EnvFile = envFile

	// Cleanup first
	cleanupDocker(t, integrationTestDir)

	t.Log("\n STEP 4: Starting Docker services...")
	dockerComposeCmd = getDockerComposeCmd()
	upCmd := exec.CommandContext(ctx, dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--build")...)
	upCmd.Dir = integrationTestDir
	output, err := upCmd.CombinedOutput()
	if err != nil {
		t.Logf("Docker output:\n%s", string(output))
		geth.Cleanup()
		return nil, fmt.Errorf("failed to start Docker services: %w", err)
	}

	t.Log("Docker services started")

	t.Log("\n STEP 5: Waiting for nodes to activate...")
	time.Sleep(10 * time.Second)

	// Leader PeerID is already set from generation step
	env.LeaderPeerID = leaderPeerID

	// Restart regular nodes with LEADER_PEER_ID
	dockerComposeCmd = getDockerComposeCmd()
	restartCmd := exec.CommandContext(ctx, dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"--env-file", ".env.docker-test",
			"up", "-d", "--no-deps", "regularnode1", "regularnode2", "regularnode3")...)
	restartCmd.Dir = integrationTestDir
	restartCmd.Run()
	time.Sleep(5 * time.Second)

	// Wait for activation
	t.Log("Waiting for nodes to auto-activate...")
	time.Sleep(10 * time.Second)

	var ops []common.Address
	maxActivationRetries := 5
	for i := 0; i < maxActivationRetries; i++ {
		ops, err = getActivatedOps(ctx, geth)
		if err == nil && len(ops) >= 3 {
			t.Logf("Found %d activated operators", len(ops))
			break
		}
		if i < maxActivationRetries-1 {
			time.Sleep(15 * time.Second)
		}
	}
	if err != nil {
		geth.Cleanup()
		return nil, fmt.Errorf("failed to get activated operators: %w", err)
	}
	if len(ops) < 3 {
		geth.Cleanup()
		return nil, fmt.Errorf("should have at least 3 activated operators, got %d", len(ops))
	}
	env.ActivatedOps = ops

	t.Log("Test environment setup complete!")
	return env, nil
}

// Helper functions used by setup and tests

func isContainerRunningQuick(name string) (bool, error) {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", name), "--format", "{{.Names}}")
	output, err := cmd.Output()
	return strings.Contains(string(output), name), err
}

func cleanupDocker(logger Logger, integrationTestDir string) {
	dockerComposeCmd := getDockerComposeCmd()
	cmd := exec.Command(dockerComposeCmd[0],
		append(dockerComposeCmd[1:],
			"-f", "docker-compose-test.yml",
			"-p", "drb-test",
			"down", "-v")...)
	cmd.Dir = integrationTestDir
	cmd.Run() // Ignore errors
}

func getActivatedOps(ctx context.Context, env *GethTestEnv) ([]common.Address, error) {
	input, _ := env.ContractABI.Pack("getActivatedOperators")
	result, _ := env.Client.CallContract(ctx, ethereum.CallMsg{To: &env.ContractAddress, Data: input}, nil)

	var ops []common.Address
	env.ContractABI.UnpackIntoInterface(&ops, "getActivatedOperators", result)
	return ops, nil
}

// generateLeaderPeerIDForTest generates a peer ID for the leader node in test environment
func generateLeaderPeerIDForTest(projectRoot string) (string, error) {
	fileName := filepath.Join(projectRoot, "test/static-key/leadernode.bin")
	dir := filepath.Dir(fileName)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	var privKey crypto.PrivKey
	var peerID peer.ID

	// Check if the key file exists
	if _, err := os.Stat(fileName); err == nil {
		// File exists, load it
		buff, err := os.ReadFile(fileName)
		if err != nil {
			return "", fmt.Errorf("failed to read existing key file '%s': %v", fileName, err)
		}

		privKey, err = crypto.UnmarshalPrivateKey(buff)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal private key from '%s': %v", fileName, err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			return "", fmt.Errorf("failed to generate PeerID from private key in '%s': %v", fileName, err)
		}

		return peerID.String(), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("error checking for file '%s': %v", fileName, err)
	}

	// File does not exist, generate a new private key
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
	if err != nil {
		return "", fmt.Errorf("failed to generate private key for Leader Node: %v", err)
	}

	buff, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal private key for Leader Node: %v", err)
	}

	err = os.WriteFile(fileName, buff, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write private key to file '%s': %v", fileName, err)
	}

	peerID, err = peer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate PeerID from private key for Leader Node: %v", err)
	}

	return peerID.String(), nil
}

// generateRegularPeerIDForTest generates a peer ID for a regular node in test environment
func generateRegularPeerIDForTest(projectRoot string, nodeNumber int) (string, error) {
	fileName := filepath.Join(projectRoot, fmt.Sprintf("test/static-key/regularnode%d.bin", nodeNumber))
	dir := filepath.Dir(fileName)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	var privKey crypto.PrivKey
	var peerID peer.ID

	// Check if the key file exists
	if _, err := os.Stat(fileName); err == nil {
		// File exists, load it
		buff, err := os.ReadFile(fileName)
		if err != nil {
			return "", fmt.Errorf("failed to read existing key file '%s': %v", fileName, err)
		}

		privKey, err = crypto.UnmarshalPrivateKey(buff)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal private key from '%s': %v", fileName, err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			return "", fmt.Errorf("failed to generate PeerID from private key in '%s': %v", fileName, err)
		}

		return peerID.String(), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("error checking for file '%s': %v", fileName, err)
	}

	// File does not exist, generate a new private key
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
	if err != nil {
		return "", fmt.Errorf("failed to generate private key for Regular Node %d: %v", nodeNumber, err)
	}

	buff, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal private key for Regular Node %d: %v", nodeNumber, err)
	}

	err = os.WriteFile(fileName, buff, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write private key to file '%s': %v", fileName, err)
	}

	peerID, err = peer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate PeerID from private key for Regular Node %d: %v", nodeNumber, err)
	}

	return peerID.String(), nil
}

// extractLeaderPeerID extracts the PeerID from the leader node container
func extractLeaderPeerID(containerName string) string {
	tempKeyFile := filepath.Join(os.TempDir(), fmt.Sprintf("leader_key_%s.bin", containerName))
	defer os.Remove(tempKeyFile) // Clean up temp file

	// Copy the key file from the container
	cpCmd := exec.Command("docker", "cp", fmt.Sprintf("%s:/app/static-key/leadernode.bin", containerName), tempKeyFile)
	if err := cpCmd.Run(); err == nil {
		keyData, err := os.ReadFile(tempKeyFile)
		if err == nil {
			privKey, err := crypto.UnmarshalPrivateKey(keyData)
			if err == nil {
				peerID, err := peer.IDFromPrivateKey(privKey)
				if err == nil {
					return peerID.String()
				}
			}
		}
	}
	return ""
}
