package config

import (
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// EnvConfig captures the environment variables required by the node services.
type EnvConfig struct {
	// General node configuration
	NodeType          string
	Port              string
	RegularNodeNumber string
	RegularPeerID     string // Regular node peer ID (REGULAR{N}_PEER_ID)

	// Leader connection details for regular nodes
	LeaderIP     string
	LeaderPort   string
	LeaderPeerID string

	// Ethereum interaction configuration
	ContractAddress  string
	ChainID          string
	EOAPrivateKey    string
	LeaderPrivateKey string
	LeaderEOA        string

	// Flags for test/mocked flows
	MockSendSecretToLeader      bool
	MockSendCosToLeader         bool
	MockSendCommitToLeader      bool
	MockGenerateRandomNumber    bool
	DisableMerkleRootSubmission bool
	DisableCosSubmission        bool
	DisableSecretSubmission     bool

	// External RPC endpoints and credentials
	RPCURLs    []string
	EthRPCURLs []string
	PrivateKey string

	// Database configuration
	Database DatabaseConfig

	// Contract period configuration
	OffChainSubmissionPeriod            *big.Int
	RequestOrSubmitOrFailDecisionPeriod *big.Int
	OnChainSubmissionPeriod             *big.Int
	OffChainSubmissionPeriodPerOperator *big.Int
	OnChainSubmissionPeriodPerOperator  *big.Int
}

// DatabaseConfig stores Postgres connection settings.
type DatabaseConfig struct {
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresName     string
	PostgresSSLMode  string
}

// ContractPeriods holds all time period configuration parameters for contract operations
type ContractPeriods struct {
	OffChainSubmissionPeriod            *big.Int // Time window for off-chain submissions
	RequestOrSubmitOrFailDecisionPeriod *big.Int // Decision period for request handling
	OnChainSubmissionPeriod             *big.Int // Time window for on-chain submissions
	OffChainSubmissionPeriodPerOperator *big.Int // Off-chain submission time per operator
	OnChainSubmissionPeriodPerOperator  *big.Int // On-chain submission time per operator
}

func GetContractPeriods() *ContractPeriods {
	cfg := Get()
	return &ContractPeriods{
		OffChainSubmissionPeriod:            cfg.OffChainSubmissionPeriod,
		RequestOrSubmitOrFailDecisionPeriod: cfg.RequestOrSubmitOrFailDecisionPeriod,
		OnChainSubmissionPeriod:             cfg.OnChainSubmissionPeriod,
		OffChainSubmissionPeriodPerOperator: cfg.OffChainSubmissionPeriodPerOperator,
		OnChainSubmissionPeriodPerOperator:  cfg.OnChainSubmissionPeriodPerOperator,
	}
}

var (
	envConfig     *EnvConfig
	loadOnce      sync.Once
	configMu      sync.RWMutex
	runningInTest bool
)

func init() {
	base := filepath.Base(os.Args[0])
	runningInTest = strings.HasSuffix(base, ".test")
}

// Get returns the process-wide EnvConfig, loading it from the environment the first time.
// When running under "go test" we always reload to respect per-test overrides.
func Get() *EnvConfig {
	if runningInTest {
		return loadEnv()
	}

	loadOnce.Do(func() {
		cfg := loadEnv()
		configMu.Lock()
		defer configMu.Unlock()
		envConfig = cfg
	})

	configMu.RLock()
	defer configMu.RUnlock()
	return envConfig
}

// Reload forces the config to be reloaded from the environment.
// This is primarily useful for integration tests.
func Reload() *EnvConfig {
	cfg := loadEnv()
	configMu.Lock()
	defer configMu.Unlock()
	envConfig = cfg
	return envConfig
}

func loadEnv() *EnvConfig {
	_ = godotenv.Load()
	return &EnvConfig{
		NodeType:                    os.Getenv("NODE_TYPE"),
		Port:                        os.Getenv("PORT"),
		RegularNodeNumber:           os.Getenv("REGULAR_NODE_NUMBER"),
		RegularPeerID:               os.Getenv("REGULAR_PEER_ID"),
		LeaderIP:                    os.Getenv("LEADER_IP"),
		LeaderPort:                  os.Getenv("LEADER_PORT"),
		LeaderPeerID:                os.Getenv("LEADER_PEER_ID"),
		ContractAddress:             os.Getenv("CONTRACT_ADDRESS"),
		ChainID:                     os.Getenv("CHAIN_ID"),
		EOAPrivateKey:               os.Getenv("EOA_PRIVATE_KEY"),
		LeaderPrivateKey:            os.Getenv("LEADER_PRIVATE_KEY"),
		LeaderEOA:                   os.Getenv("LEADER_EOA"),
		MockSendSecretToLeader:      parseBool(os.Getenv("MOCK_SEND_SECRET_TO_LEADER")),
		MockSendCosToLeader:         parseBool(os.Getenv("MOCK_SEND_COS_TO_LEADER")),
		MockSendCommitToLeader:      parseBool(os.Getenv("MOCK_SEND_COMMIT_TO_LEADER")),
		MockGenerateRandomNumber:    parseBool(os.Getenv("MOCK_GENERATE_RANDOM_NUMBER")),
		DisableMerkleRootSubmission: parseBool(os.Getenv("DISABLE_MERKLE_ROOT_SUBMISSION")),
		DisableCosSubmission:        parseBool(os.Getenv("DISABLE_COS_SUBMISSION")),
		DisableSecretSubmission:     parseBool(os.Getenv("DISABLE_SECRET_SUBMISSION")),
		RPCURLs:                     splitAndTrim(os.Getenv("RPC_URLS")),
		EthRPCURLs:                  splitAndTrim(os.Getenv("ETH_RPC_URLS")),
		PrivateKey:                  os.Getenv("PRIVATE_KEY"),
		Database: DatabaseConfig{
			PostgresHost:     getEnvOrDefault("POSTGRES_HOST", "localhost"),
			PostgresPort:     getEnvAsInt("POSTGRES_PORT", 5432),
			PostgresUser:     getEnvOrDefault("POSTGRES_USER", "postgres"),
			PostgresPassword: getEnvOrDefault("POSTGRES_PASSWORD", ""),
			PostgresName:     getEnvOrDefault("POSTGRES_NAME", "postgres"),
			PostgresSSLMode:  getEnvOrDefault("POSTGRES_SSLMODE", "disable"),
		},
		OffChainSubmissionPeriod:            getEnvAsBigIntOrDefault("OFF_CHAIN_SUBMISSION_PERIOD", 40),
		RequestOrSubmitOrFailDecisionPeriod: getEnvAsBigIntOrDefault("REQUEST_OR_SUBMIT_OR_FAIL_DECISION_PERIOD", 30),
		OnChainSubmissionPeriod:             getEnvAsBigIntOrDefault("ON_CHAIN_SUBMISSION_PERIOD", 60),
		OffChainSubmissionPeriodPerOperator: getEnvAsBigIntOrDefault("OFF_CHAIN_SUBMISSION_PERIOD_PER_OPERATOR", 20),
		OnChainSubmissionPeriodPerOperator:  getEnvAsBigIntOrDefault("ON_CHAIN_SUBMISSION_PERIOD_PER_OPERATOR", 30),
	}
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "y":
		return true
	default:
		return false
	}
}

func splitAndTrim(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvAsInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func getEnvAsBigIntOrDefault(key string, defaultValue int64) *big.Int {
	value := os.Getenv(key)
	if value == "" {
		return big.NewInt(defaultValue)
	}

	parsed := new(big.Int)
	parsed, ok := parsed.SetString(value, 10)
	if !ok {
		return big.NewInt(defaultValue)
	}
	return parsed
}

// GetChainIDAsBigInt returns the ChainID from environment config as *big.Int.
// Returns nil if CHAIN_ID is not set or invalid.
func GetChainIDAsBigInt() *big.Int {
	cfg := Get()
	if cfg.ChainID == "" {
		return nil
	}
	chainID := new(big.Int)
	chainID, ok := chainID.SetString(cfg.ChainID, 10)
	if !ok {
		return nil
	}
	return chainID
}
