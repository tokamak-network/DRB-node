package regular_node

import (
	"sync"

	"github.com/eapache/queue"
)

// CreateTestRegularNode creates a properly initialized RegularNode for testing
// from external packages that cannot access unexported fields directly.
func CreateTestRegularNode() *RegularNode {
	return &RegularNode{
		submittedCvIndices:            make(map[string]map[string]bool),
		cleanupQueue:                  queue.New(),
		strictOrderWhileSecretRequest: make(map[string][]string),
		roundsData:                    make(map[string]RoundData),
		cosRecevied:                   sync.Map{},
	}
}
