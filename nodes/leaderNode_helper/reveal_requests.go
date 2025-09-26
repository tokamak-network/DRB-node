package leaderNode_helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// Tracks EOAs that have been sent requests per round - protected with mutex
var revealRequestStatus = make(map[string][]string)
var revealRequestStatusMu sync.RWMutex

// Variables for monitoring failToSubmitS condition - using atomic for thread safety
var failToSubmitSMonitoringActive int32 // 0 = false, 1 = true
var failToSubmitSMonitoringTimer *time.Timer
var lastSubmitSTimestamp *big.Int
var timestampMu sync.RWMutex // Protect big.Int pointers

// StartSecretValueRequests initializes the secret value request process for a given round
func StartSecretValueRequests(h host.Host, fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	if GetHalted() {
		log.Println("System is halted. Skipping StartSecretValueRequests.")
		return
	}
	// Load reveal order for the round
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	roundRevealData, err := database.GetRevealOrder(round, trialNum)
	if err != nil {
		log.Printf("Failed to load reveal order: %v", err)
		return
	}

	// Load registered nodes
	nodes, err := database.GetNodeInfos()
	if err != nil {
		log.Printf("Failed to load registered nodes: %v", err)
		return
	}

	// Initialize reveal request status for the round if not already done
	if _, exists := GetRevealRequestStatus(uniqueKey); !exists {
		SetRevealRequestStatus(uniqueKey, []string{})
	}

	// Send the request to the first node in the reveal order
	eoaArray := roundRevealData.OrderedNodes
	eoa := eoaArray[0]
	for _, node := range nodes {
		if node.EOAAddress == eoa {
			sendSecretValueRequestToNode(h, fallbackEthClient, round, trialNum, uniqueKey, eoa, node, 0)
		} else {
			continue
		}
	}
}

func sendSecretValueRequestToNode(h host.Host, fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, uniqueKey string, regularEoa string, nodeInfo *utils.NodeInfo, order int) {
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
		Round:             round,     // Round number
		TrialNum:          trialNum,  // Trial number
		Signature:         signature, // Signed round number
		Order:             order,
	}

	fmt.Println("Sending secret value request to EOA:", regularEoa)

	// Send the request
	err = sendToRegularNode(h, *nodeInfo, "/sendSecretValue", req)
	if err != nil {
		log.Printf("Failed to send secret value request to EOA %s for round %s with trail %s: %v", regularEoa, round, trialNum, err)
	} else {
		log.Printf("✅ Secret value request sent to EOA %s for round %s with trail %s", regularEoa, round, trialNum)

		// Start a timer to track if the response is received within 15 seconds
		go func() {
			timer := time.NewTimer(20 * time.Second)
			defer timer.Stop()

			// Wait for the timer to expire
			<-timer.C

			// If the timer expires and the secret value is not received, call handleMissingSecretValue
			hasSecret, exists := GetRoundSecretValue(uniqueKey, regularEoa)
			if !exists || !hasSecret {
				log.Printf("Secret value not received for EOA %s in round %s with trail %s within 15 seconds. Handling missing secret value.", regularEoa, round, trialNum)
				SetSecretsOnChain(uniqueKey, true)
				requestToSubmitS(fallbackEthClient, round, trialNum)
			}
		}()

		// Mark this EOA as requested
		currentStatus, _ := GetRevealRequestStatus(uniqueKey)
		currentStatus = append(currentStatus, regularEoa)
		SetRevealRequestStatus(uniqueKey, currentStatus)
	}
}

func requestToSubmitS(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	SetSecretRequestSentForWhichRound(GetCurrentRound())
	allCos, secretsReceivedOffchainInRevealOrder, packedVs, cvNotOnChainCvAndSigRS, packedRevealOrders := prepareArgumentsForRequestToSubmitS(round, trialNum)

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
		log.Printf("Failed to submit secret request for round %s: %v", round, err)
		return
	}

	log.Printf("Successfully submitted secret request for round %s", round)

	// Start monitoring for failToSubmitS condition
	requestTimestamp := big.NewInt(time.Now().Unix())
	StartFailToSubmitSMonitoring(fallbackEthClient, round, trialNum, requestTimestamp)
}

func prepareArgumentsForRequestToSubmitS(round string, trialNum string) ([][32]byte, [][32]byte, *big.Int, []SigRS, *big.Int) {
	_, cos, _, vs, rs, ss := LoadNodeData(round, trialNum)
	var notOnChainIndices []*big.Int
	i := big.NewInt(0)
	j := 0
	length := big.NewInt(eth.GetActivatedOperatorsLength())

	indices := GetIndices()
	for i.Cmp(length) < 0 {
		if j < len(indices) && i.Cmp(indices[j]) == 0 {
			i = new(big.Int).Add(i, big.NewInt(1))
			j++
		} else {
			notOnChainIndices = append(notOnChainIndices, new(big.Int).Set(i))
			i = new(big.Int).Add(i, big.NewInt(1))
		}
	}
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
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	revealOrders, err := database.GetRevealOrder(round, trialNum)
	if err != nil {
		log.Printf("Failed to load reveal order for round %s with trail %s: %v", round, trialNum, err)
	}

	order := revealOrders.RevealOrder
	packedRevealOrders := packRevealOrder(order)
	packedVsForAllCvsNotOnChain := packVsValues(vsForNotOnChain)

	roundSecrets, _ := GetRoundSecretsValue(uniqueKey)
	return allCos, roundSecrets, packedVsForAllCvsNotOnChain, sigRSsForAllCvsNotOnChain, packedRevealOrders
}

func PackIndices(indices []*big.Int) *big.Int {
	packed := big.NewInt(0)
	for i, index := range indices {
		packed.Or(packed, new(big.Int).Lsh(index, uint(8*i)))
	}
	return packed
}

// handleSecretValueResponse processes a response and sends the next request if applicable
func HandleSecretValueResponse(h host.Host, fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, eoa string) {
	if GetHalted() {
		log.Println("System is halted. Skipping HandleSecretValueResponse.")
		return
	}
	log.Printf("Secret value received for round %s with trail %s from EOA %s", round, trialNum, eoa)
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	// Load reveal order for the round
	roundRevealData, err := database.GetRevealOrder(round, trialNum)
	if err != nil {
		log.Printf("Failed to load reveal order: %v", err)
		return
	}

	// Load registered nodes
	nodes, err := database.GetNodeInfos()
	if err != nil {
		log.Printf("Failed to load registered nodes: %v", err)
		return
	}

	// Check which node is next in the reveal order
	currentStatus, _ := GetRevealRequestStatus(uniqueKey)
	for order, eoa := range roundRevealData.OrderedNodes {
		if !contains(currentStatus, eoa) {
			log.Printf("🎯 Next node in reveal order: %s (order %d) for round %s with trail %s", eoa, order, round, trialNum)
			for _, node := range nodes {
				if node.EOAAddress == eoa {
					sendSecretValueRequestToNode(h, fallbackEthClient, round, trialNum, uniqueKey, eoa, node, order)
					return
				}
			}
			log.Printf("⚠️ Node info not found for EOA %s", eoa)
		}
	}

	log.Printf("All nodes processed for round %s with trail %s.", round, trialNum)
}

// sendToRegularNode sends a request to a specific regular node
func sendToRegularNode(h host.Host, nodeInfo utils.NodeInfo, protocol string, data interface{}) error {
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

// StartFailToSubmitSMonitoring starts monitoring for failToSubmitS condition
// Should be called when requestToSubmitS transaction is confirmed
func StartFailToSubmitSMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, requestTimestamp *big.Int) {
	if GetHalted() {
		log.Println("System is halted. Skipping StartFailToSubmitSMonitoring.")
		return
	}

	if GetFailToSubmitSMonitoringActive() {
		log.Printf("FailToSubmitS monitoring already active for round %s", round)
		return
	}

	if GetLastSubmitSTimestamp() == nil {
		SetLastSubmitSTimestamp(requestTimestamp)
	}

	onChainSubmissionPeriodPerOperator := big.NewInt(40)
	startMonitoringWithPeriod(fallbackEthClient, round, trialNum, onChainSubmissionPeriodPerOperator)
}

// startMonitoringWithPeriod starts the actual monitoring with the given period
func startMonitoringWithPeriod(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, period *big.Int) {
	// Calculate deadline: s_previousSSubmitTimestamp + s_onChainSubmissionPeriodPerOperator
	deadline := new(big.Int).Add(GetLastSubmitSTimestamp(), period)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	SetFailToSubmitSMonitoringActive(true)

	log.Printf("Starting failToSubmitS monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)
	log.Printf("Parameters - lastSubmitSTimestamp: %v, onChainSubmissionPeriodPerOperator: %v",
		GetLastSubmitSTimestamp(), period)

	// Set timer to call the function when deadline is reached
	failToSubmitSMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToSubmitS", round)
		callFailToSubmitS(fallbackEthClient, round, trialNum)
		SetFailToSubmitSMonitoringActive(false)
	})
}

// StopFailToSubmitSMonitoring stops the monitoring
func StopFailToSubmitSMonitoring(round string, trialNum string) {
	if !GetFailToSubmitSMonitoringActive() {
		return
	}

	if failToSubmitSMonitoringTimer != nil {
		failToSubmitSMonitoringTimer.Stop()
		failToSubmitSMonitoringTimer = nil
	}

	SetFailToSubmitSMonitoringActive(false)
	log.Printf("Stopped failToSubmitS monitoring for round %s", round)
}

// UpdateLastSubmitSTimestamp updates the timestamp when a submitS event is received
// This should be called when SSubmitted event is received
func UpdateLastSubmitSTimestamp(newTimestamp *big.Int, round string, trialNum string) {
	SetLastSubmitSTimestamp(newTimestamp)
	log.Printf("Updated lastSubmitSTimestamp to %v for round %s", newTimestamp, round)

	// If monitoring is active, restart it with the new timestamp
	if GetFailToSubmitSMonitoringActive() {
		log.Printf("Restarting failToSubmitS monitoring with updated timestamp")
		StopFailToSubmitSMonitoring(round, trialNum)

		// Use hardcoded period for restart - this should ideally get the period from contract
		onChainSubmissionPeriodPerOperator := big.NewInt(30) // 30 seconds
		startMonitoringWithPeriod(nil, round, trialNum, onChainSubmissionPeriodPerOperator)
	}
}

// callFailToSubmitS calls the contract function to fail
func callFailToSubmitS(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	log.Printf("Calling failToSubmitS for round %s with trial %s", round, trialNum)

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
		log.Printf("Failed to decode private key: %v", err)
		return
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	// Execute the transaction
	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
		"failToSubmitS",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to execute failToSubmitS transaction: %v", err)
		return
	}

	log.Printf("Successfully called failToSubmitS for round %s with trial %s", round, trialNum)
}

// ResetLeaderMonitoringState resets all leader monitoring variables for a round
func ResetLeaderMonitoringState(round string, trialNum string) {
	// Stop secret submission monitoring (S monitoring)
	StopFailToSubmitSMonitoring(round, trialNum)

	// Reset secret submission monitoring timestamps
	SetLastSubmitSTimestamp(nil)

	// Reset reveal request status for the round
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	DeleteRevealRequestStatus(uniqueKey)

	// Call COS and CVS monitoring reset from acceptCommit.go
	ResetCosAndCvsMonitoringState(round, trialNum)

	log.Printf("Reset all leader monitoring state for round %s", round)
}
