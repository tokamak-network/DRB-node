package leaderNode_helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

type SigRS struct {
	R [32]byte
	S [32]byte
}
var secretsOnChain = make(map[string]bool)

// Tracks EOAs that have been sent requests per round
var revealRequestStatus = make(map[string][]string)
var roundSecret = make(map[string]map[string]bool)

// StartSecretValueRequests initializes the secret value request process for a given round
func StartSecretValueRequests(h host.Host, fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string) {
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
	for order, node := range roundRevealData.OrderedNodes {
		eoa := node
		nodeInfo, exists := nodes[eoa]
		if !exists {
			log.Printf("Node info for EOA %s not found in registered nodes.", eoa)
			continue
		}

		sendSecretValueRequestToNode(h, fallbackEthClient, roundNum, eoa, nodeInfo, order)
		break
	}
}

func sendSecretValueRequestToNode(h host.Host, fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string, regularEoa string, nodeInfo NodeInfo, order int) {
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

	leaderEoa := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	log.Printf("EOA Address: %s", leaderEoa)

	// Sign the round number
	signature := utils.SignData(leaderEoa, privateKey)

	// Create the secret value request
	req := utils.SecretValueRequest{
		LeaderEoaAddress:  leaderEoa, // Leader's EOA
		RegularEoaAddress: regularEoa,
		Round:             roundNum,  // Round number
		Signature:         signature, // Signed round number
		Order:             order,
	}

	fmt.Println("Sending secret value request to EOA:", regularEoa)

	// Send the request
	err = sendToRegularNode(h, nodeInfo, "/sendSecretValue", req)
	if err != nil {
		log.Printf("Failed to send secret value request to EOA %s for round %s: %v", regularEoa, roundNum, err)
	} else {
		log.Printf("Secret value request sent to EOA %s for round %s", regularEoa, roundNum)

		// Start a timer to track if the response is received within 15 seconds
		go func() {
			timer := time.NewTimer(20 * time.Second)
			defer timer.Stop()

			// Wait for the timer to expire
			<-timer.C

			// If the timer expires and the secret value is not received, call handleMissingSecretValue
			if !roundSecret[roundNum][regularEoa] {
				log.Printf("Secret value not received for EOA %s in round %s within 15 seconds. Handling missing secret value.", regularEoa, roundNum)
				secretsOnChain[roundNum] = true
				requestToSubmitS(fallbackEthClient, roundNum)
			}
		}()

		// Mark this EOA as requested
		revealRequestStatus[roundNum] = append(revealRequestStatus[roundNum], regularEoa)
	}
}

func requestToSubmitS(fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string) {
	SecretRequestSentForWhichRound = CurrentRound
	allCos, secretsReceivedOffchainInRevealOrder, packedVs, cvNotOnChainCvAndSigRS, packedRevealOrders := prepareArgumentsForRequestToSubmitS(roundNum)

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}

	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode leader private key: %v", err)
		return
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"requestToSubmitS",
		big.NewInt(0),
		allCos,
		secretsReceivedOffchainInRevealOrder,
		packedVs,
		cvNotOnChainCvAndSigRS,
		packedRevealOrders,
	)
	if err != nil {
		log.Printf("Failed to submit commit request root for round %s: %v", roundNum, err)
		return
	}

	log.Printf("Successfully submitted cos request for round %s", roundNum)
}

func prepareArgumentsForRequestToSubmitS(roundNum string) ([][32]byte, [][32]byte, *big.Int, []SigRS, *big.Int) {
	_, cos, _, vs, rs, ss := LoadNodeData(roundNum)
	var notOnChainIndices []*big.Int
	i := big.NewInt(0)
	j := 0
	length := big.NewInt(int64(len(eth.ActivatedOperators)))
	fmt.Println("length", length)
	for i.Cmp(length) < 0 {
		if j < len(Indices) && i.Cmp(Indices[j]) == 0 {
			i = new(big.Int).Add(i, big.NewInt(1))
			j++
		} else {
			notOnChainIndices = append(notOnChainIndices, new(big.Int).Set(i))
			i = new(big.Int).Add(i, big.NewInt(1))
		}
	}
	fmt.Println(notOnChainIndices, "notOnChainIndices")
	var sigRSsForAllCvsNotOnChain []SigRS
	var vsForNotOnChain []uint8
	var allCos [][32]byte
	for i := range cos {
		var cos32 [32]byte
		copy(cos32[:], cos[i])
		allCos = append(allCos, cos32)
	}

	for _, i := range notOnChainIndices {
		index := int(i.Int64())
		vsForNotOnChain = append(vsForNotOnChain, uint8(vs[index]))
		var r32, s32 [32]byte
		copy(r32[:], rs[index].Bytes())
		copy(s32[:], ss[index].Bytes())
		cvAndSigRS := SigRS{
			R: r32,
			S: s32,
		}
		sigRSsForAllCvsNotOnChain = append(sigRSsForAllCvsNotOnChain, cvAndSigRS)
	}

	revealOrders, err := loadRevealOrders("reveal_orders.json")
	if err != nil {
		log.Printf("Failed to load reveal orders: %v", err)
	}

	roundRevealData, exists := revealOrders[roundNum]
	if !exists {
		log.Printf("No reveal order found for round %s.", roundNum)
	}

	order := roundRevealData.RevealOrder
	packedRevealOrders := packRevealOrder(order)
	packedVsForAllCvsNotOnChain := packVsValues(vsForNotOnChain)

	return allCos, RoundSecrets[roundNum], packedVsForAllCvsNotOnChain, sigRSsForAllCvsNotOnChain, packedRevealOrders
}

func PackIndices(indices []*big.Int) *big.Int {
	packed := big.NewInt(0)
	for i, index := range indices {
		packed.Or(packed, new(big.Int).Lsh(index, uint(8*i)))
	}
	return packed
}

// handleSecretValueResponse processes a response and sends the next request if applicable
func HandleSecretValueResponse(h host.Host, fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string, eoa string) {
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
	for order, node := range orderedNodes {
		nodeEOA := node
		if !contains(revealRequestStatus[roundNum], nodeEOA) {
			nodeInfo, exists := nodes[nodeEOA]
			if !exists {
				log.Printf("Node info for EOA %s not found in registered nodes.", nodeEOA)
				continue
			}

			// Send secret value request to the next node
			sendSecretValueRequestToNode(h, fallbackEthClient, roundNum, nodeEOA, nodeInfo, order)
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
