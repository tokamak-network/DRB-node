package leaderNode_helper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/tokamak-network/DRB-node/utils"
)

// Tracks EOAs that have been sent requests per round
var revealRequestStatus = make(map[string][]string)

// StartSecretValueRequests initializes the secret value request process for a given round
func StartSecretValueRequests(h host.Host, roundNum string) {
	// Load reveal order for the round
	revealData, err := loadRevealOrders("reveal_orders.json")
	if err != nil {
		log.Printf("Failed to load reveal orders: %v", err)
		return
	}

	roundRevealData, exists := revealData[roundNum]
	if !exists {
		log.Printf("No reveal order found for round %s.", roundNum)
		return
	}

	// Load registered nodes
	filePath := "registered_nodes.json"
	nodes, err := LoadRegisteredNodes(filePath)
	if err != nil {
		log.Printf("Failed to load registered nodes: %v", err)
		return
	}

	// Initialize reveal request status for the round if not already done
	if _, exists := revealRequestStatus[roundNum]; !exists {
		revealRequestStatus[roundNum] = []string{}
	}

	// Send the request to the first node in the reveal order
	for _, node := range roundRevealData.OrderedNodes {
		eoa := node
		nodeInfo, exists := nodes[eoa]
		if !exists {
			log.Printf("Node info for EOA %s not found in registered nodes.", eoa)
			continue
		}

		sendSecretValueRequestToNode(h, roundNum, eoa, nodeInfo)
		break
	}
}

func sendSecretValueRequestToNode(h host.Host, roundNum string, eoa string, nodeInfo NodeInfo) {
	// Load private key from environment variable
	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode leader private key: %v", err)
		return
	}

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	log.Printf("EOA Address: %s", eoaAddress)

	// Sign the round number
	signature := utils.SignData(eoaAddress, privateKey)

	// Create the secret value request
	req := utils.SecretValueRequest{
		EOAAddress: eoaAddress, // Leader's EOA
		Round:      roundNum,   // Round number
		Signature:  signature,  // Signed round number
	}

	// Send the request
	err = sendToRegularNode(h, nodeInfo, "/sendSecretValue", req)
	if err != nil {
		log.Printf("Failed to send secret value request to EOA %s for round %s: %v", eoa, roundNum, err)
	} else {
		log.Printf("Secret value request sent to EOA %s for round %s", eoa, roundNum)

		// Mark this EOA as requested
		revealRequestStatus[roundNum] = append(revealRequestStatus[roundNum], eoa)
	}
}

// handleSecretValueResponse processes a response and sends the next request if applicable
func HandleSecretValueResponse(h host.Host, roundNum string, eoa string) {
	log.Printf("Secret value received for round %s from EOA %s", roundNum, eoa)

	// Load reveal order for the round
	revealData, err := loadRevealOrders("reveal_orders.json")
	if err != nil {
		log.Printf("Failed to load reveal orders: %v", err)
		return
	}

	roundRevealData, exists := revealData[roundNum]
	if !exists {
		log.Printf("No reveal order found for round %s.", roundNum)
		return
	}

	orderedNodes := roundRevealData.OrderedNodes

	// Load registered nodes
	filePath := "registered_nodes.json"
	nodes, err := LoadRegisteredNodes(filePath)
	if err != nil {
		log.Printf("Failed to load registered nodes: %v", err)
		return
	}

	// Check which node is next in the reveal order
	for _, node := range orderedNodes {
		nodeEOA := node
		if !contains(revealRequestStatus[roundNum], nodeEOA) {
			nodeInfo, exists := nodes[nodeEOA]
			if !exists {
				log.Printf("Node info for EOA %s not found in registered nodes.", nodeEOA)
				continue
			}

			// Send secret value request to the next node
			sendSecretValueRequestToNode(h, roundNum, nodeEOA, nodeInfo)
			return
		}
	}

	log.Printf("All nodes processed for round %s.", roundNum)
}

func loadRevealOrders(filePath string) (RevealOrders, error) {
	var orders RevealOrders
	file, err := os.ReadFile(filePath)
	if err != nil {
		return orders, err
	}
	if err := json.Unmarshal(file, &orders); err != nil {
		return orders, err
	}
	return orders, nil
}

// sendToRegularNode sends a request to a specific regular node
func sendToRegularNode(h host.Host, nodeInfo NodeInfo, protocol string, data interface{}) error {
	stream, err := utils.CreateStream(h, utils.NodeInfo{
		IP:     nodeInfo.IP,
		Port:   nodeInfo.Port,
		PeerID: nodeInfo.PeerID,
	}, protocol)
	if err != nil {
		return err
	}
	defer stream.Close()

	// Send the encoded data
	encoder := json.NewEncoder(stream)
	err = encoder.Encode(data)
	if err != nil {
		return fmt.Errorf("failed to encode and send data over stream: %v", err)
	}
	return nil
}

// contains checks if an item exists in a slice
func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
