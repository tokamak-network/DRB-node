package leader_node

import (
	"github.com/eapache/queue"
	"github.com/tokamak-network/DRB-node/utils"
)

// CreateTestLeaderNode creates a properly initialized LeaderNode for testing
// This function consolidates test node creation to avoid duplication
func CreateTestLeaderNode() *LeaderNode {
	node := &LeaderNode{
		// Initialize atomic fields to zero (default state)
		execution:                            0,
		halted:                              0,
		submittingMerkleRoot:                0,
		requestedToSubmitCoMonitoringActive: 0,
		requestedToSubmitCvMonitoringActive: 0,
		requestToSubmitCvMonitoringActive:   0,
		requestToSubmitCoTimerMonitoringActive: 0,
		failToSubmitSMonitoringActive:       0,
		
		// Initialize all maps
		roundsData:          make(map[string]RoundData),
		revealRequestStatus: make(map[string][]string),
		activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
		cvOnChain:          make(map[string]bool),
		roundSecret:        make(map[string]map[string]bool),
		roundSecrets:       make(map[string][][32]byte),
		secretsOnChain:     make(map[string]bool),
		
		// Initialize cleanup queue
		cleanupQueue: queue.New(),
	}
	
	return node
}

// CreateTestLeaderNodeMinimal creates a minimal LeaderNode for basic tests
// Use this for tests that don't need all fields initialized
func CreateTestLeaderNodeMinimal() *LeaderNode {
	return &LeaderNode{
		roundsData:          make(map[string]RoundData),
		activeBroadcasts:    make(map[string]*utils.BroadcastTracker),
		cvOnChain:           make(map[string]bool),
		roundSecrets:        make(map[string][][32]byte),
		roundSecret:         make(map[string]map[string]bool),
		secretsOnChain:      make(map[string]bool),
		revealRequestStatus: make(map[string][]string),
		cleanupQueue:        queue.New(),
	}
}