package constants

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestChains_AllChainsExist verifies that all expected chains are present in the Chains map
func TestChains_AllChainsExist(t *testing.T) {
	expectedChainIDs := []uint64{
		1,        // Ethereum Mainnet
		11155111, // Ethereum Sepolia
		10,       // Optimism Mainnet
		11155420, // Optimism Sepolia
	}

	for _, chainID := range expectedChainIDs {
		_, exists := Chains[chainID]
		assert.True(t, exists, "Chain ID %d should exist in Chains map", chainID)
	}
}

// TestChains_EthereumMainnet verifies Ethereum Mainnet configuration
func TestChains_EthereumMainnet(t *testing.T) {
	chainID := uint64(1)
	networkInfo, exists := Chains[chainID]

	assert.True(t, exists, "Ethereum Mainnet should exist")
	assert.Equal(t, "Ethereum Mainnet", networkInfo.Name)
	assert.Equal(t, 12*time.Second, networkInfo.BlockTime)
	assert.Equal(t, uint64(150), networkInfo.EstimatedGasFactorPercent)
}

// TestChains_EthereumSepolia verifies Ethereum Sepolia configuration
func TestChains_EthereumSepolia(t *testing.T) {
	chainID := uint64(11155111)
	networkInfo, exists := Chains[chainID]

	assert.True(t, exists, "Ethereum Sepolia should exist")
	assert.Equal(t, "Ethereum Sepolia", networkInfo.Name)
	assert.Equal(t, 12*time.Second, networkInfo.BlockTime)
	assert.Equal(t, uint64(150), networkInfo.EstimatedGasFactorPercent)
}

// TestChains_OptimismMainnet verifies Optimism Mainnet configuration
func TestChains_OptimismMainnet(t *testing.T) {
	chainID := uint64(10)
	networkInfo, exists := Chains[chainID]

	assert.True(t, exists, "Optimism Mainnet should exist")
	assert.Equal(t, "Optimism Mainnet", networkInfo.Name)
	assert.Equal(t, 2*time.Second, networkInfo.BlockTime)
	assert.Equal(t, uint64(200), networkInfo.EstimatedGasFactorPercent)
}

// TestChains_OptimismSepolia verifies Optimism Sepolia configuration
func TestChains_OptimismSepolia(t *testing.T) {
	chainID := uint64(11155420)
	networkInfo, exists := Chains[chainID]

	assert.True(t, exists, "Optimism Sepolia should exist")
	assert.Equal(t, "Optimism Sepolia", networkInfo.Name)
	assert.Equal(t, 2*time.Second, networkInfo.BlockTime)
	assert.Equal(t, uint64(200), networkInfo.EstimatedGasFactorPercent)
}

// TestChains_NonExistentChain verifies behavior for non-existent chain IDs
func TestChains_NonExistentChain(t *testing.T) {
	nonExistentChainIDs := []uint64{
		999,   // Random non-existent chain
		0,     // Zero chain ID
		12345, // Another non-existent chain
	}

	for _, chainID := range nonExistentChainIDs {
		_, exists := Chains[chainID]
		assert.False(t, exists, "Chain ID %d should not exist in Chains map", chainID)
	}
}

// TestChains_MapNotEmpty verifies that the Chains map is not empty
func TestChains_MapNotEmpty(t *testing.T) {
	assert.NotEmpty(t, Chains, "Chains map should not be empty")
	assert.Greater(t, len(Chains), 0, "Chains map should contain at least one entry")
}

// TestChains_MapSize verifies the expected number of chains
func TestChains_MapSize(t *testing.T) {
	expectedSize := 6
	assert.Equal(t, expectedSize, len(Chains), "Chains map should contain exactly %d entries", expectedSize)
}

// TestChains_BlockTimePositive verifies that all block times are positive
func TestChains_BlockTimePositive(t *testing.T) {
	for chainID, networkInfo := range Chains {
		assert.Greater(t, networkInfo.BlockTime, time.Duration(0),
			"Block time for chain %d (%s) should be positive", chainID, networkInfo.Name)
	}
}

// TestChains_EstimatedGasFactorPositive verifies that all gas factors are positive
func TestChains_EstimatedGasFactorPositive(t *testing.T) {
	for chainID, networkInfo := range Chains {
		assert.Greater(t, networkInfo.EstimatedGasFactorPercent, uint64(0),
			"Estimated gas factor for chain %d (%s) should be positive", chainID, networkInfo.Name)
	}
}

// TestChains_EstimatedGasFactorReasonable verifies that gas factors are within reasonable range
func TestChains_EstimatedGasFactorReasonable(t *testing.T) {
	minFactor := uint64(100)  // At least 100% (no reduction)
	maxFactor := uint64(1000) // At most 10x (1000%)

	for chainID, networkInfo := range Chains {
		assert.GreaterOrEqual(t, networkInfo.EstimatedGasFactorPercent, minFactor,
			"Estimated gas factor for chain %d (%s) should be at least %d%%",
			chainID, networkInfo.Name, minFactor)
		assert.LessOrEqual(t, networkInfo.EstimatedGasFactorPercent, maxFactor,
			"Estimated gas factor for chain %d (%s) should be at most %d%%",
			chainID, networkInfo.Name, maxFactor)
	}
}

// TestChains_NameNotEmpty verifies that all chain names are not empty
func TestChains_NameNotEmpty(t *testing.T) {
	for chainID, networkInfo := range Chains {
		assert.NotEmpty(t, networkInfo.Name,
			"Name for chain %d should not be empty", chainID)
	}
}

// TestChains_EthereumChainsConfiguration verifies Ethereum chains have same block time
func TestChains_EthereumChainsConfiguration(t *testing.T) {
	ethereumMainnet := Chains[1]
	ethereumSepolia := Chains[11155111]

	assert.Equal(t, ethereumMainnet.BlockTime, ethereumSepolia.BlockTime,
		"Ethereum Mainnet and Sepolia should have the same block time")
	assert.Equal(t, ethereumMainnet.EstimatedGasFactorPercent, ethereumSepolia.EstimatedGasFactorPercent,
		"Ethereum Mainnet and Sepolia should have the same gas factor")
}

// TestChains_OptimismChainsConfiguration verifies Optimism chains have same block time
func TestChains_OptimismChainsConfiguration(t *testing.T) {
	optimismMainnet := Chains[10]
	optimismSepolia := Chains[11155420]

	assert.Equal(t, optimismMainnet.BlockTime, optimismSepolia.BlockTime,
		"Optimism Mainnet and Sepolia should have the same block time")
	assert.Equal(t, optimismMainnet.EstimatedGasFactorPercent, optimismSepolia.EstimatedGasFactorPercent,
		"Optimism Mainnet and Sepolia should have the same gas factor")
}

// TestChains_OptimismFasterThanEthereum verifies that Optimism has faster block time than Ethereum
func TestChains_OptimismFasterThanEthereum(t *testing.T) {
	ethereumBlockTime := Chains[1].BlockTime
	optimismBlockTime := Chains[10].BlockTime

	assert.Less(t, optimismBlockTime, ethereumBlockTime,
		"Optimism block time should be faster (less) than Ethereum block time")
}

// TestChains_BlockTimeConsistency verifies block time consistency across chain types
func TestChains_BlockTimeConsistency(t *testing.T) {
	testCases := []struct {
		name       string
		chainID    uint64
		expectTime time.Duration
	}{
		{"Ethereum Mainnet", 1, 12 * time.Second},
		{"Ethereum Sepolia", 11155111, 12 * time.Second},
		{"Optimism Mainnet", 10, 2 * time.Second},
		{"Optimism Sepolia", 11155420, 2 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			networkInfo, exists := Chains[tc.chainID]
			assert.True(t, exists, "%s should exist", tc.name)
			assert.Equal(t, tc.expectTime, networkInfo.BlockTime,
				"%s should have block time of %v", tc.name, tc.expectTime)
		})
	}
}

// TestChains_GasFactorConsistency verifies gas factor consistency across chain types
func TestChains_GasFactorConsistency(t *testing.T) {
	testCases := []struct {
		name         string
		chainID      uint64
		expectFactor uint64
	}{
		{"Ethereum Mainnet", 1, 150},
		{"Ethereum Sepolia", 11155111, 150},
		{"Optimism Mainnet", 10, 200},
		{"Optimism Sepolia", 11155420, 200},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			networkInfo, exists := Chains[tc.chainID]
			assert.True(t, exists, "%s should exist", tc.name)
			assert.Equal(t, tc.expectFactor, networkInfo.EstimatedGasFactorPercent,
				"%s should have gas factor of %d%%", tc.name, tc.expectFactor)
		})
	}
}

// TestChains_MapNotNil verifies that the Chains map is initialized
func TestChains_MapNotNil(t *testing.T) {
	assert.NotNil(t, Chains, "Chains map should be initialized")
}

// TestChains_AccessPattern verifies safe access pattern for Chains map
func TestChains_AccessPattern(t *testing.T) {
	// Test safe access pattern with exists check
	chainID := uint64(1)
	networkInfo, exists := Chains[chainID]

	if exists {
		assert.NotEmpty(t, networkInfo.Name)
		assert.Greater(t, networkInfo.BlockTime, time.Duration(0))
		assert.Greater(t, networkInfo.EstimatedGasFactorPercent, uint64(0))
	}

	// Test access for non-existent chain
	nonExistentChainID := uint64(999999)
	_, exists = Chains[nonExistentChainID]
	assert.False(t, exists, "Non-existent chain should return false for exists")
}
