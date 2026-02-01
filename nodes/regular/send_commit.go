package regular_node

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sort"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

type CommitData struct {
	Round           string            `json:"round"`
	SecretValue     [32]byte          `json:"secret_value"`
	Cos             [32]byte          `json:"cos"`
	Cvs             [32]byte          `json:"cvs"`
	SendToLeader    bool              `json:"send_to_leader"`
	SendCosToLeader bool              `json:"send_cos_to_leader"`
	Sign            map[string]string `json:"sign"`
}

// StartTime getter/setter functions
func (n *RegularNode) SetStartTime(timestamp *big.Int) {
	n.startTimeMu.Lock()
	defer n.startTimeMu.Unlock()
	n.startTime = timestamp
}

func (n *RegularNode) GetStartTime() *big.Int {
	n.startTimeMu.RLock()
	defer n.startTimeMu.RUnlock()
	return n.startTime
}

type RoundData struct {
	MerkleRoot   bool
	RandomNumber bool
}

func (n *RegularNode) MonitorCommitRequest(ctx context.Context) {
	n.receiveCommitRequest(ctx)
}

func (n *RegularNode) receiveCommitRequest(ctx context.Context) {
	contractAddress := appconfig.Get().ContractAddress
	contractAddr := common.HexToAddress(contractAddress)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddr},
	}

	for { // Outer loop for reconnection
		logs := make(chan types.Log)
		sub, err := n.fallbackEthClient.SubscribeFilterLogs(ctx, query, logs)
		if err != nil {
			log.Printf("Failed to subscribe to logs: %v. Retrying in 2 seconds...", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Connection succeeded - catch up on any missed events during retry period
		log.Printf("Websocket connection established. Catching up on missed events...")
		if catchUpErr := n.catchUpMissedEvents(ctx, contractAddr, parsedABI); catchUpErr != nil {
			log.Printf("Error catching up on missed events: %v", catchUpErr)
		}
		reconnect := false
		for {
			select {
			case <-ctx.Done():
				log.Println("MonitorCommitRequest function received shutdown signal, closing subscription...")
				sub.Unsubscribe()
				return
			case err := <-sub.Err():
				log.Printf("Error in event subscription: %v", err)

				if err != nil {
					log.Printf("Websocket closed abnormally. Attempting to reconnect in 1 seconds...")
					if catchUpErr := n.catchUpMissedEvents(ctx, contractAddr, parsedABI); catchUpErr != nil {
						log.Printf("Error catching up on missed events: %v", catchUpErr)
					}
				}
				time.Sleep(1 * time.Second)
				reconnect = true

			case vLog := <-logs:
				n.processEventLog(ctx, vLog, parsedABI)
			}
			if reconnect {
				log.Printf("Reconnection triggered, breaking out of event loop to restart subscription...")
				// Clean up old subscription before reconnecting
				if sub != nil {
					log.Printf("Unsubscribing from old subscription to prevent connection leak...")
					sub.Unsubscribe()
				}
				break // break inner for loop to reconnect
			}
		}
	}
}

func (n *RegularNode) processCvSubmitted(round *big.Int, trialNum *big.Int, index *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCVS.")
		return fmt.Errorf("system is halted. Skipping processCVS.")
	}

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)

	// Use the new atomic setter function
	indexStr := index.String()
	n.SetSubmittedCvIndicesValue(uniqueKey, indexStr, true)

	log.Printf("CV submitted for index %s in round %s with trail %s", indexStr, round.String(), trialNum.String())
	return nil
}

func (n *RegularNode) checkAllCVsSubmittedOnChain(round string, trialNum string) bool {
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	// Check if the map doesn't exist
	submittedCvIndicesMap, exists := n.GetSubmittedCvIndicesMap(uniqueKey)
	if !exists {
		return false
	}

	// Check if all requested indices have submitted their CV values
	indices := n.GetCvRequestIndices()
	allSubmitted := true
	for _, requestedIndex := range indices {
		submitted, exists := submittedCvIndicesMap[requestedIndex.String()]
		if !exists || !submitted {
			allSubmitted = false
			break
		}
	}

	return allSubmitted
}

func (n *RegularNode) processSubmittedSecretRequest(ctx context.Context, round, trialNum, index *big.Int) error {
	if n.GetHalted() {
		return fmt.Errorf("system is halted. Skipping processSubmittedSecretRequest.")
	}

	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	data, err := n.revealOrderRepository.GetRevealOrder(ctx, round.String(), trialNum.String())
	if err != nil {
		return fmt.Errorf("Failed to get reveal order for round %s with trail %s: %v", round.String(), trialNum.String(), err)
	}
	orderedNodes := data.OrderedNodes
	revealOrder := data.RevealOrder
	if index.Int64() >= int64(len(orderedNodes)) {
		return fmt.Errorf("index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(orderedNodes))
	}
	var indexInRevealOrder int
	intValue := int(index.Int64())
	for i, order := range revealOrder {
		if order == intValue {
			indexInRevealOrder = i
		}
	}
	eoaAddress := n.GetRegularNodeEOA()
	if indexInRevealOrder+1 < len(revealOrder) {
		if indexInRevealOrder+1 < len(orderedNodes) {
			regularEoaAddress := orderedNodes[indexInRevealOrder+1]
			if eoaAddress == regularEoaAddress {
				fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, TrialNum: %v, EOA: %v\n", round.String(), trialNum.String(), regularEoaAddress)
				if err := n.submitS(ctx, round.String(), trialNum.String()); err != nil {
					return fmt.Errorf("failed to submit S: %v", err)
				}
			}
		}
	}
	return nil
}

func (n *RegularNode) processSecretRequest(ctx context.Context, round, trialNum, index *big.Int) error {
	if n.GetHalted() {
		fmt.Println("system is halted. Skipping processSubmittedSecretRequest.")
		return nil
	}
	if appconfig.Get().DisableSecretSubmission {
		return fmt.Errorf("secret submission is disabled via configuration. Skipping processSecretRequest.")
	}
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)

	// Try to get reveal order, if it doesn't exist, try to create it
	revealOrder, err := n.revealOrderRepository.GetRevealOrder(ctx, round.String(), trialNum.String())
	if err != nil {
		log.Printf("Failed to get reveal order for round %s with trail %s: %v", round.String(), trialNum.String(), err)
		log.Printf("Attempting to determine reveal order for regular node...")

		// get activated operators
		activatedOps := eth.Service.GetActivatedOperatorsCached()
		// Try to determine reveal order for regular node
		success, err := n.revealOrderService.DetermineRegularRevealOrder(ctx, round.String(), trialNum.String(), activatedOps)
		if err != nil || !success {
			return fmt.Errorf("failed to determine reveal order: %v", err)
		}

		// Try to get reveal order again after creation
		revealOrder, err = n.revealOrderRepository.GetRevealOrder(ctx, round.String(), trialNum.String())
		if err != nil {
			return fmt.Errorf("still failed to get reveal order after creation: %v", err)
		}
		log.Printf("Successfully created and retrieved reveal order")
	}

	// Check bounds safely
	if revealOrder == nil {
		return fmt.Errorf("reveal order is nil for round %s with trail %s", round.String(), trialNum.String())
	}

	if revealOrder.OrderedNodes == nil {
		return fmt.Errorf("reveal order ordered nodes is nil for round %s with trail %s", round.String(), trialNum.String())
	}

	if index.Int64() >= int64(len(revealOrder.OrderedNodes)) {
		return fmt.Errorf("index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(revealOrder.OrderedNodes))
	}

	regularEoaAddress := revealOrder.OrderedNodes[index.Int64()]
	if n.GetRegularNodeEOA() == regularEoaAddress {
		fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\n", round, regularEoaAddress)
		err := n.submitS(ctx, round.String(), trialNum.String())
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *RegularNode) submitS(ctx context.Context, round string, trialNum string) error {
	roundData, err := n.regularCommitRepository.GetCommitByRound(ctx, round, trialNum)
	if err != nil {
		return fmt.Errorf("failed to get regular commit for round %s with : %v", round, err)
	}

	secretValueBytes := roundData.SecretValue

	fmt.Printf("Extracted secret_value as bytes32: %x\n", secretValueBytes)

	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("Failed to create EOA client: %v", err)
	}

	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"submitS",
		big.NewInt(0),
		secretValueBytes,
	)
	if err != nil {
		return fmt.Errorf("failed to submit secret_value: %v", err)
	}

	log.Printf("Successfully submitted secret_value: %x", secretValueBytes)
	return nil
}

func (n *RegularNode) processMerkleRoot(Round *big.Int, TrialNum *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v\n", Round, TrialNum)
	uniqueKey := utils.GetUniqueKey(Round.String(), TrialNum.String())
	roundData, exists := n.GetRoundData(uniqueKey)
	if !exists {
		roundData = RoundData{}
	}
	roundData.MerkleRoot = true
	n.SetRoundData(uniqueKey, roundData)
	return nil
}

func (n *RegularNode) processRandomRequestNumber(ctx context.Context, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) error {

	fmt.Printf("Round %v, TrialNum %v, state %v\n", round, trialNum, state)

	// fetch the last round and trial, and cleanup the data (current round and trial has not been updated yet)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	n.EnqueueUniqueKeyForCleanup(uniqueKey)

	roundStr := round.String()
	// Reset monitoring state for new round
	n.ResetMonitoringState(round.String(), trialNum.String())

	// Store the current round and trialNum from Status event
	n.SetCurrentTrialNum(trialNum.String())

	eth.Service.UpdateActivatedOperators(ctx, n.fallbackEthClient)
	n.SetCurrentRound(round.String())

	if state.Cmp(big.NewInt(1)) == 0 {
		// Set Halted to 0 to resume the round
		n.SetHalted(false)
		// Delete old round data except current round from database
		err := n.batchRepository.DeleteOldRoundDataForRegularNode(ctx, round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}

		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		n.SetExecution(true)
		log.Printf("Execution started for round %s", roundStr)

		// Start leader monitoring for the new round using block timestamp
		n.StartLeaderMonitoring(ctx, blockTimestamp, round.String(), trialNum.String())
		n.SetRoundData(uniqueKey, RoundData{
			MerkleRoot:   false,
			RandomNumber: false,
		})

		go n.AllCosReceivedUnlocked(ctx, round.String(), trialNum.String())
	}

	if state.Cmp(big.NewInt(2)) == 0 {
		// Delete old round data except current round from database
		err := n.batchRepository.DeleteOldRoundDataForRegularNode(ctx, round.String())
		if err != nil {
			log.Printf("Failed to delete old round data %v for regular node\n", round)
		}
		// Update in-memory round data
		roundData, exists := n.GetRoundData(uniqueKey)
		if !exists {
			roundData = RoundData{}
		}
		roundData.RandomNumber = true
		n.SetRoundData(uniqueKey, roundData)
		n.SetExecution(false)
		log.Printf("Execution stopped for round %s", roundStr)
	}

	if state.Cmp(big.NewInt(3)) == 0 {
		// Delete round and trial data from database
		err := n.batchRepository.DeleteRoundTrialDataForRegularNode(ctx, n.GetCurrentRound(), trialNum.String())
		if err != nil {
			log.Printf("Failed to delete old round data %v for regular node\n", round)
		}
		// resume the round
		n.SetHalted(true)
		// resuming(fallbackEthClient)
		n.SetExecution(false)

		n.SetRoundData(uniqueKey, RoundData{
			MerkleRoot:   false,
			RandomNumber: false,
		})
	}
	return nil
}

func (n *RegularNode) CleanupRoundDataByUniqueKey(uniqueKey string) {
	n.DeleteRoundsData(uniqueKey)
	n.DeleteSubmittedCvIndices(uniqueKey)
	n.DeleteStrictOrder(uniqueKey)
}

func (n *RegularNode) AllCosReceivedUnlocked(ctx context.Context, round string, trialNum string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("AllCosReceivedUnlocked received shutdown signal, stopping...")
			return
		case <-ticker.C:
			// Stop if the round/trial has moved on
			if n.GetCurrentRound() != round || n.GetCurrentTrialNum() != trialNum {
				log.Printf("Round/trial changed (now %s/%s), stopping AllCosReceivedUnlocked for round %s/%s",
					n.GetCurrentRound(), n.GetCurrentTrialNum(), round, trialNum)
				return
			}
			activatedOps := eth.Service.GetActivatedOperatorsCached()
			if n.GetHalted() {
				log.Println("System is halted. Skipping AllCosReceivedUnlocked.")
				return
			}
			if n.allCosReceivedUnlockedRegular(round, trialNum, activatedOps) {
				flag, _ := n.revealOrderService.DetermineRegularRevealOrder(ctx, round, trialNum, activatedOps)
				if flag {
					return
				}
				// Continue checking
			}
		}
	}
}

func (n *RegularNode) allCosReceivedUnlockedRegular(round string, trialNum string, activatedOps []common.Address) bool {
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	for _, op := range activatedOps {
		v, ok := n.GetCosReceived(uniqueKey, op.Hex())
		if !ok || !v {
			return false
		}
	}
	return true
}

func (n *RegularNode) processCommitRequest(ctx context.Context, round *big.Int, trialNum *big.Int, packedIndices *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCommitRequest.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, packedIndices %v\n", round, trialNum, packedIndices)
	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to create EOA client: %v", err)
	}
	eoaAddress := crypto.PubkeyToAddress(clientUtils.PrivateKey.PublicKey).Hex()

	indices := n.unpackIndices(packedIndices)
	// Copy indices by value (deep copy)
	n.SetCvRequestIndices(indices)

	// Convert eth.ActivatedOperators to []string for compatibility
	activatedOps := eth.Service.GetActivatedOperatorsCached()
	activatedOpsStr := make([]string, len(activatedOps))
	for i, addr := range activatedOps {
		activatedOpsStr[i] = addr.Hex()
	}
	flag, err := n.findEOAAddress(indices, activatedOpsStr, eoaAddress)

	if err != nil {
		return fmt.Errorf("failed to find EOA address: %v", err)
	}
	if !flag {
		fmt.Println("Cv Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing RequestedToSubmitCv event for Round: %v\n", round.String())

	commitData, err := n.regularCommitRepository.GetCommitByRound(ctx, round.String(), trialNum.String())
	if err != nil {
		return fmt.Errorf("failed to get commit by round: %v", err)
	}

	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"submitCv",
		big.NewInt(0),
		commitData.Cvs,
	)
	if err != nil {
		return fmt.Errorf("failed to execute submitCv transaction: %v", err)
	}

	return nil
}

func (n *RegularNode) processCosRequest(ctx context.Context, Round *big.Int, TrialNum *big.Int, packedIndices *big.Int, indicesLength *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCosRequest.")
		return nil
	}
	if appconfig.Get().DisableCosSubmission {
		log.Println("Cos submission is disabled via configuration. Skipping processCosRequest.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v\n", Round, TrialNum)
	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to create EOA client: %v", err)
	}
	eoaAddress := crypto.PubkeyToAddress(clientUtils.PrivateKey.PublicKey).Hex()

	// Convert eth.ActivatedOperators to []string for compatibility
	activatedOps := eth.Service.GetActivatedOperatorsCached()
	activatedOpsStr := make([]string, len(activatedOps))
	for i, addr := range activatedOps {
		activatedOpsStr[i] = addr.Hex()
	}
	indices := n.unpackIndicesWithLength(packedIndices, indicesLength)
	flag, err := n.findEOAAddress(indices, activatedOpsStr, eoaAddress)

	if err != nil {
		return fmt.Errorf("failed to find EOA address: %v", err)
	}
	if !flag {
		fmt.Println("Cos Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing RequestedToSubmitCo event for Round: %v\n", Round.String())

	commitData, err := n.regularCommitRepository.GetCommitByRound(ctx, Round.String(), TrialNum.String())
	if err != nil {
		return fmt.Errorf("failed to get commit by round: %v", err)
	}

	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"submitCo",
		big.NewInt(0),
		commitData.Cos,
	)
	if err != nil {
		return fmt.Errorf("failed to execute submitCo transaction: %v", err)
	}
	return nil
}

func (n *RegularNode) unpackIndices(packedIndices *big.Int) []*big.Int {
	order := []*big.Int{}
	mask := big.NewInt(0xFF)
	i := 0
	for {
		shift := uint(8 * i)
		shifted := new(big.Int).Rsh(packedIndices, shift)
		value := new(big.Int).And(shifted, mask)

		if i != 0 && value.Cmp(big.NewInt(0)) == 0 {
			break
		}
		order = append(order, value)
		i++
	}
	return order
}

func (n *RegularNode) unpackIndicesWithLength(unpackIndices *big.Int, indicesLength *big.Int) []*big.Int {
	order := []*big.Int{}
	mask := big.NewInt(0xFF)
	i := 0
	for i < int(indicesLength.Int64()) {
		shift := uint(8 * i)
		shifted := new(big.Int).Rsh(unpackIndices, shift)
		value := new(big.Int).And(shifted, mask)
		order = append(order, value)
		i++
	}
	return order
}

func (n *RegularNode) findEOAAddress(indices []*big.Int, activatedOps []string, eoaAddress string) (bool, error) {
	if len(indices) > len(activatedOps) {
		return false, fmt.Errorf("indices length is greater than activated operators")
	}
	for _, index := range indices {
		idx := index.Int64()
		if idx < 0 || idx >= int64(len(activatedOps)) {
			return false, fmt.Errorf("index %d out of bounds for activated operators length %d", idx, len(activatedOps))
		}
		if eoaAddress == activatedOps[idx] {
			return true, nil
		}
	}
	return false, nil
}

// StartLeaderMonitoring starts monitoring the leader for the current round
func (n *RegularNode) StartLeaderMonitoring(ctx context.Context, startTime *big.Int, round string, trialNum string) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping StartLeaderMonitoring.")
		return
	}

	// Use atomic compare-and-swap to prevent double start
	if !atomic.CompareAndSwapInt32(&n.leaderMonitoringActive, 0, 1) {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	if startTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		atomic.StoreInt32(&n.leaderMonitoringActive, 0)
		return
	}

	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	// Stop existing timer if present
	if n.monitoringTimer != nil {
		n.monitoringTimer.Stop()
		n.monitoringTimer = nil
	}

	// Get timing parameters from config
	periods := appconfig.GetContractPeriods()

	// Calculate deadline: startTime + offChainSubmissionPeriod + requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(startTime, periods.OffChainSubmissionPeriod)
	deadline.Add(deadline, periods.RequestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	if duration <= 0 {
		log.Printf("Deadline has already passed for round %s with trail %s, calling failToRequestSubmitCVOrSubmitMerkleRoot immediately", round, trialNum)
		atomic.StoreInt32(&n.leaderMonitoringActive, 0)
		if err := n.callFailToRequestSubmitCVOrSubmitMerkleRoot(ctx, round, trialNum); err != nil {
			log.Printf("Failed to call failToRequestSubmitCVOrSubmitMerkleRoot: %v", err)
			return
		}
		return
	}

	log.Printf("Starting leader monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	n.monitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSubmitCVOrSubmitMerkleRoot", round)
		if err := n.callFailToRequestSubmitCVOrSubmitMerkleRoot(ctx, round, trialNum); err != nil {
			log.Printf("Failed to call failToRequestSubmitCVOrSubmitMerkleRoot: %v", err)
			return
		}
		atomic.StoreInt32(&n.leaderMonitoringActive, 0)
	})
}

// StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring stops the current leader monitoring
func (n *RegularNode) StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(round string, trialNum string) error {
	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	if n.monitoringTimer != nil {
		n.monitoringTimer.Stop()
		n.monitoringTimer = nil
	}

	atomic.StoreInt32(&n.leaderMonitoringActive, 0)
	log.Printf("Stopped leader monitoring for round %s", round)
	return nil
}

// ResetMonitoringState resets all monitoring variables
func (n *RegularNode) ResetMonitoringState(round string, trialNum string) {
	n.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(round, trialNum)
	n.StopFailToSubmitMerkleRootAfterDisputeMonitoring(round, trialNum)
	n.StopRequestToSubmitSOrGenerateRandomNumberMonitoring(round, trialNum)
	n.SetMerkleRootSubmittedEventEmitted(false)
	n.SetSubmitSMonitoringReferenceTime(nil)

	// Reset monitoring state variables
	n.ClearCvRequestIndices()
	log.Printf("Reset monitoring state")
}

// callFailToRequestSubmitCVOrSubmitMerkleRoot calls the contract function to fail the leader
func (n *RegularNode) callFailToRequestSubmitCVOrSubmitMerkleRoot(ctx context.Context, round string, trialNum string) error {
	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to create EOA client: %v", err)
	}

	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"failToRequestSubmitCvOrSubmitMerkleRoot",
		big.NewInt(0),
	)
	if err != nil {
		return fmt.Errorf("failed to call failToRequestSubmitCVOrSubmitMerkleRoot: %v", err)
	}

	log.Printf("Successfully called failToRequestSubmitCVOrSubmitMerkleRoot for round %swith trail %s", round, trialNum)
	return nil
}

// StartMerkleRootMonitoring starts monitoring for merkle root submission after CV request
func (n *RegularNode) StartMerkleRootMonitoring(ctx context.Context, round string, trialNum string, requestedToSubmitCvTime *big.Int) error {
	if requestedToSubmitCvTime == nil {
		return fmt.Errorf("requestedToSubmitCvTime is nil, cannot start merkle root monitoring")
	}

	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	// Stop existing timer if present
	if n.merkleRootMonitoringTimer != nil {
		n.merkleRootMonitoringTimer.Stop()
		n.merkleRootMonitoringTimer = nil
	}

	// Get timing parameters from config
	periods := appconfig.GetContractPeriods()

	// Calculate deadline: requestedToSubmitCvTime + onChainSubmissionPeriod + requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(requestedToSubmitCvTime, periods.OnChainSubmissionPeriod)
	deadline.Add(deadline, periods.RequestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting merkle root monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	n.merkleRootMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, checking conditions before calling failToSubmitMerkleRootAfterDispute", round)

		// Check if all CVs have been submitted on-chain and merkle root hasn't been submitted
		if n.checkAllCVsSubmittedOnChain(round, trialNum) && !n.GetMerkleRootSubmittedEventEmitted() {
			log.Printf("All CVs submitted on-chain but merkle root not submitted, calling failToSubmitMerkleRootAfterDispute")
			if err := n.callFailToSubmitMerkleRootAfterDispute(ctx, round, trialNum); err != nil {
				log.Printf("Failed to call failToSubmitMerkleRootAfterDispute: %v", err)
				return
			}
		} else {
			log.Printf("Conditions not met for failToSubmitMerkleRootAfterDispute - CVs not all submitted or merkle root already submitted")
		}
	})
	return nil
}

// StopFailToSubmitMerkleRootAfterDisputeMonitoring stops the current merkle root monitoring
func (n *RegularNode) StopFailToSubmitMerkleRootAfterDisputeMonitoring(round string, trialNum string) {
	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	if n.merkleRootMonitoringTimer != nil {
		n.merkleRootMonitoringTimer.Stop()
		n.merkleRootMonitoringTimer = nil
	}

	log.Printf("Stopped merkle root monitoring for round %s", round)
}

// callFailToSubmitMerkleRootAfterDispute calls the contract function to fail the leader for not submitting merkle root
func (n *RegularNode) callFailToSubmitMerkleRootAfterDispute(ctx context.Context, round string, trialNum string) error {
	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to create EOA client: %v", err)
	}

	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"failToSubmitMerkleRootAfterDispute",
		big.NewInt(0),
	)
	if err != nil {
		return fmt.Errorf("failed to call failToSubmitMerkleRootAfterDispute: %v", err)
	}

	log.Printf("Successfully called failToSubmitMerkleRootAfterDispute for round %s with trail %s", round, trialNum)
	return nil
}

// CheckAndStartMonitoring checks if monitoring should be started and starts it if needed
func (n *RegularNode) CheckAndStartMonitoring(ctx context.Context, round string, trialNum string) {
	// Check if monitoring is already active
	if n.GetLeaderMonitoringActive() {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	// Check if Merkle root has been submitted
	if n.GetMerkleRootSubmittedEventEmitted() {
		log.Printf("Merkle root submitted event already emitted for round %s", round)
		return
	}

	// Check if we have a valid start time
	startTime := n.GetStartTime()
	if startTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	n.StartLeaderMonitoring(ctx, startTime, round, trialNum)
}

// StartRequestToSubmitSOrGenerateRandomNumberMonitoring starts monitoring for failToRequestSOrGenerateRandomNumber condition
func (n *RegularNode) StartRequestToSubmitSOrGenerateRandomNumberMonitoring(ctx context.Context, round string, trialNum string) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping StartRequestToSubmitSOrGenerateRandomNumberMonitoring.")
		return
	}

	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	// Stop existing timer if present
	if n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer != nil {
		n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer.Stop()
		n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer = nil
	}

	// Get timing parameters from config
	periods := appconfig.GetContractPeriods()
	activatedOperatorsLength := new(big.Int).SetInt64(eth.Service.GetActivatedOperatorsLength())

	referenceTime := n.GetSubmitSMonitoringReferenceTime()
	if referenceTime == nil {
		log.Printf("SubmitSMonitoringReferenceTime is nil, cannot start monitoring for round %s with trail %s", round, trialNum)
		return
	}

	// Calculate deadline: s_merkleRootSubmittedTime + s_offChainSubmissionPeriod + (s_offChainSubmissionPeriodPerOperator * activatedOperatorsLength) + s_requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(referenceTime, periods.OffChainSubmissionPeriod)
	operatorDelay := new(big.Int).Mul(periods.OffChainSubmissionPeriodPerOperator, activatedOperatorsLength)
	deadline.Add(deadline, operatorDelay)
	deadline.Add(deadline, periods.RequestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	if duration <= 0 {
		log.Printf("Deadline has already passed for round %s with trail %s, calling failToRequestSOrGenerateRandomNumber immediately", round, trialNum)
		if err := n.callFailToRequestSOrGenerateRandomNumber(ctx, round, trialNum); err != nil {
			log.Printf("Failed to call failToRequestSOrGenerateRandomNumber: %v", err)
		}
		return
	}

	log.Printf("Starting request to submit S or generate random number monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)
	log.Printf("Parameters - merkleRootSubmittedTime: %v, offChainSubmissionPeriod: %v, offChainSubmissionPeriodPerOperator: %v, activatedOperatorsLength: %v, requestOrSubmitOrFailDecisionPeriod: %v",
		n.GetSubmitSMonitoringReferenceTime(), periods.OffChainSubmissionPeriod, periods.OffChainSubmissionPeriodPerOperator, activatedOperatorsLength, periods.RequestOrSubmitOrFailDecisionPeriod)

	// Set timer to call the function when deadline is reached
	n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSOrGenerateRandomNumber", round)
		if err := n.callFailToRequestSOrGenerateRandomNumber(ctx, round, trialNum); err != nil {
			log.Printf("Failed to call failToRequestSOrGenerateRandomNumber: %v", err)
			return
		}
	})
}

// StopRequestToSubmitSOrGenerateRandomNumberMonitoring stops the monitoring
func (n *RegularNode) StopRequestToSubmitSOrGenerateRandomNumberMonitoring(round string, trialNum string) {
	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	if n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer != nil {
		n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer.Stop()
		n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer = nil
	}

	log.Printf("Stopped request to submit S or generate random number monitoring for round %s", round)
}

// callFailToRequestSOrGenerateRandomNumber calls the contract function to fail
func (n *RegularNode) callFailToRequestSOrGenerateRandomNumber(ctx context.Context, round string, trialNum string) error {
	log.Printf("Calling failToRequestSOrGenerateRandomNumber for round %s with trial %s", round, trialNum)

	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to create EOA client: %v", err)
	}

	// Execute the transaction
	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"failToRequestSorGenerateRandomNumber",
		big.NewInt(0),
	)
	if err != nil {
		return fmt.Errorf("failed to execute failToRequestSOrGenerateRandomNumber transaction: %v", err)
	}

	log.Printf("Successfully called failToRequestSOrGenerateRandomNumber for round %s with trial %s", round, trialNum)
	return nil
}

func (n *RegularNode) setLastProcessedBlock(blockNumber uint64) {
	n.lastProcessedCoordsMu.Lock()
	defer n.lastProcessedCoordsMu.Unlock()
	if blockNumber > n.lastProcessedBlock {
		n.lastProcessedBlock = blockNumber
	}
}

func (n *RegularNode) getLastProcessedCoords() (uint64, uint, uint) {
	n.lastProcessedCoordsMu.RLock()
	defer n.lastProcessedCoordsMu.RUnlock()
	return n.lastProcessedBlock, n.lastProcessedTxIndex, n.lastProcessedLogIndex
}

func (n *RegularNode) updateLastProcessedCoords(blockNumber uint64, txIndex uint, logIndex uint) {
	n.lastProcessedCoordsMu.RLock()
	currentBlock := n.lastProcessedBlock
	currentTxIndex := n.lastProcessedTxIndex
	currentLogIndex := n.lastProcessedLogIndex
	n.lastProcessedCoordsMu.RUnlock()

	isNewBlock := blockNumber > currentBlock
	shouldUpdate := false

	if isNewBlock {
		shouldUpdate = true
	} else if blockNumber == currentBlock {
		if txIndex > currentTxIndex {
			shouldUpdate = true
		} else if txIndex == currentTxIndex {
			if logIndex > currentLogIndex {
				shouldUpdate = true
			}
		}
	}

	if shouldUpdate {
		n.lastProcessedCoordsMu.Lock()
		n.lastProcessedBlock = blockNumber
		n.lastProcessedTxIndex = txIndex
		n.lastProcessedLogIndex = logIndex
		n.lastProcessedCoordsMu.Unlock()
	}
}

func (n *RegularNode) processEventLog(ctx context.Context, vLog types.Log, parsedABI abi.ABI) {
	isReorg := vLog.Removed
	if isReorg {
		log.Printf("Reorg detected. Skipping event: %v", vLog.TxHash)
		return
	}

	DeactivatedSig := parsedABI.Events["DeActivated"].ID
	StatusSig := parsedABI.Events["Status"].ID
	isCriticalEvent := vLog.Topics[0] == DeactivatedSig || vLog.Topics[0] == StatusSig

	if !isCriticalEvent {
		regularNodeEOA := n.GetRegularNodeEOA()
		if regularNodeEOA == "" {
			log.Println("Regular node EOA not set yet. Allowing event processing.")
		} else {
			activatedOps := eth.Service.GetActivatedOperatorsCached()
			isRegularNodeActivated := false
			for _, op := range activatedOps {
				if op.Hex() == regularNodeEOA {
					isRegularNodeActivated = true
					break
				}
			}

			if !isRegularNodeActivated {
				log.Printf("Regular node %s is deactivated. Skipping event processing.", regularNodeEOA)
				return
			}
		}
	}

	RequestedToSubmitCvS := parsedABI.Events["RequestedToSubmitCv"].ID
	CvsEventSig := parsedABI.Events["CvSubmitted"].ID
	MerkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID
	RequestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID
	RequestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID
	SSubmittedSig := parsedABI.Events["SSubmitted"].ID

	switch vLog.Topics[0] {
	case RequestedToSubmitCvS:
		eventData := struct {
			Round                         *big.Int
			TrialNum                      *big.Int
			PackedIndicesAscendingFromLSB *big.Int
		}{}
		err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCv", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode event log: %v", err)
			return
		}

		fmt.Printf("CommitRequest Event: Round %v, TrialNum %v, indices %v\n", eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)

		err = n.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(eventData.Round.String(), eventData.TrialNum.String())
		if err != nil {
			log.Printf("Failed to stop fail to request submit CV or submit merkle root monitoring: %v", err)
			return
		}

		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}

		err = n.StartMerkleRootMonitoring(ctx, eventData.Round.String(), eventData.TrialNum.String(), big.NewInt(int64(blockTimestamp)))
		if err != nil {
			log.Printf("Failed to start merkle root monitoring: %v", err)
			return
		}

		err = n.processCommitRequest(ctx, eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)
		if err != nil {
			log.Printf("Failed to process commit request: %v", err)
		}

	case StatusSig:
		eventData := struct {
			CurRound    *big.Int
			CurTrialNum *big.Int
			CurState    *big.Int
		}{}

		err := parsedABI.UnpackIntoInterface(&eventData, "Status", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode Status event log: %v", err)
			return
		}

		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}

		err = n.processRandomRequestNumber(ctx, big.NewInt(int64(blockTimestamp)), eventData.CurRound, eventData.CurTrialNum, eventData.CurState)
		if err != nil {
			log.Printf("Failed to process random request number: %v", err)
			return
		}

	case MerkleRootSubmittedSig:
		eventData := struct {
			Round      *big.Int
			TrialNum   *big.Int
			MerkleRoot [32]byte
		}{}

		err := parsedABI.UnpackIntoInterface(&eventData, "MerkleRootSubmitted", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode MerkleRootSubmitted event log: %v", err)
			return
		}
		fmt.Printf("MerkleRootSubmitted Event:\n Round %v, TrialNum %v, MerkleRoot: %v\n Round: %v\n",
			eventData.Round, eventData.TrialNum, eventData.MerkleRoot, eventData.Round.String())

		n.SetMerkleRootSubmittedEventEmitted(true)
		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}
		n.SetSubmitSMonitoringReferenceTime(big.NewInt(int64(blockTimestamp)))

		n.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(eventData.Round.String(), eventData.TrialNum.String())
		n.StopFailToSubmitMerkleRootAfterDisputeMonitoring(eventData.Round.String(), eventData.TrialNum.String())
		n.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(ctx, eventData.Round.String(), eventData.TrialNum.String())

		if err := n.processMerkleRoot(eventData.Round, eventData.TrialNum); err != nil {
			log.Printf("Failed to process merkle root: %v", err)
			return
		}

	case RequestedToSubmitCoSig:
		eventData := struct {
			Round         *big.Int
			TrialNum      *big.Int
			IndicesLength *big.Int
			PackedIndices *big.Int
		}{}
		err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCo", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode RequestedToSubmitCo event log: %v", err)
			return
		}
		fmt.Printf("RequestedToSubmitCo Event: Round %v, TrialNum %v, indicesLength %v\n, indices %v\n", eventData.Round, eventData.TrialNum, eventData.IndicesLength, eventData.PackedIndices)

		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}

		n.StopRequestToSubmitSOrGenerateRandomNumberMonitoring(eventData.Round.String(), eventData.TrialNum.String())
		n.SetSubmitSMonitoringReferenceTime(big.NewInt(int64(blockTimestamp)))
		n.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(ctx, eventData.Round.String(), eventData.TrialNum.String())

		log.Printf("Restarted monitoring for round %s trial %s with RequestedToSubmitCo event time", eventData.Round.String(), eventData.TrialNum.String())

		err = n.processCosRequest(ctx, eventData.Round, eventData.TrialNum, eventData.PackedIndices, eventData.IndicesLength)
		if err != nil {
			log.Printf("Failed to process cos request: %v", err)
		}

	case RequestedToSubmitSFromIndexKSig:
		eventData := struct {
			Round    *big.Int
			TrialNum *big.Int
			IndexK   *big.Int
		}{}

		err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitSFromIndexK", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode RequestedToSubmitSFromIndexK event log: %v", err)
			return
		}
		fmt.Printf("RequestedToSubmitSFromIndexK Event:\n Round %v, TrialNum %v, indexK %v\n", eventData.Round, eventData.TrialNum, eventData.IndexK)

		n.StopRequestToSubmitSOrGenerateRandomNumberMonitoring(eventData.Round.String(), eventData.TrialNum.String())
		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}
		n.SetSubmitSMonitoringReferenceTime(big.NewInt(int64(blockTimestamp)))

		if err := n.processSecretRequest(ctx, eventData.Round, eventData.TrialNum, eventData.IndexK); err != nil {
			log.Printf("Failed to process secret request: %v", err)
			return
		}

	case SSubmittedSig:
		eventData := struct {
			Round    *big.Int
			TrialNum *big.Int
			S        [32]byte
			Index    *big.Int
		}{}

		err := parsedABI.UnpackIntoInterface(&eventData, "SSubmitted", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode SSubmitted event log: %v", err)
			return
		}

		fmt.Printf("SSubmitted Event:\n Round %v, TrialNum %v, Secret %v\n, indexK %v\n ", eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
		if err := n.processSubmittedSecretRequest(ctx, eventData.Round, eventData.TrialNum, eventData.Index); err != nil {
			log.Printf("Failed to process submitted secret request: %v", err)
			return
		}

	case CvsEventSig:
		eventData := struct {
			Round    *big.Int
			TrialNum *big.Int
			Cv       [32]byte
			Index    *big.Int
		}{}
		err := parsedABI.UnpackIntoInterface(&eventData, "CvSubmitted", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode CvSubmitted event log: %v", err)
			return
		}

		fmt.Printf("CvSubmitted Event: Fetched successfully")
		err = n.processCvSubmitted(eventData.Round, eventData.TrialNum, eventData.Index)
		if err != nil {
			log.Printf("Failed to process cv submitted: %v", err)
			return
		}
	}

	n.updateLastProcessedCoords(vLog.BlockNumber, vLog.TxIndex, vLog.Index)
}

func (n *RegularNode) catchUpMissedEvents(ctx context.Context, contractAddr common.Address, parsedABI abi.ABI) error {
	lastProcessedBlock, _, _ := n.getLastProcessedCoords()
	if lastProcessedBlock == 0 {
		return nil
	}

	currentHeader, err := n.fallbackEthClient.HeaderByNumber(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get current block header: %v", err)
	}

	currentBlock := currentHeader.Number.Uint64()
	if currentBlock <= lastProcessedBlock {
		return nil
	}

	log.Printf("Catching up on missed events from block %d to %d", lastProcessedBlock+1, currentBlock)

	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddr},
		FromBlock: big.NewInt(int64(lastProcessedBlock)),
		ToBlock:   big.NewInt(int64(currentBlock)),
	}

	missedLogs, err := n.fallbackEthClient.FilterLogs(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to fetch missed logs: %v", err)
	}

	if len(missedLogs) == 0 {
		log.Printf("No missed events found between blocks %d and %d", lastProcessedBlock+1, currentBlock)
		return nil
	}

	sort.Slice(missedLogs, func(i, j int) bool {
		if missedLogs[i].BlockNumber != missedLogs[j].BlockNumber {
			return missedLogs[i].BlockNumber < missedLogs[j].BlockNumber
		}
		// Then by TxIndex
		if missedLogs[i].TxIndex != missedLogs[j].TxIndex {
			return missedLogs[i].TxIndex < missedLogs[j].TxIndex
		}
		//Finally by Index
		return missedLogs[i].Index < missedLogs[j].Index
	})

	log.Printf("Processing %d missed events in blockchain order", len(missedLogs))

	// Process events sequentially in blockchain order
	for _, eventLog := range missedLogs {
		lastProcessedBlock, lastProcessedTxIndex, lastProcessedLogIndex := n.getLastProcessedCoords() // 101,0,0

		// Check if this event is already processed (duplicate detection)
		isDuplicate := false
		if eventLog.BlockNumber < lastProcessedBlock {
			isDuplicate = true
		} else if eventLog.BlockNumber == lastProcessedBlock {
			if eventLog.TxIndex < lastProcessedTxIndex {
				isDuplicate = true
			} else if eventLog.TxIndex == lastProcessedTxIndex {
				if eventLog.Index <= lastProcessedLogIndex {
					isDuplicate = true
				}
			}
		}

		if isDuplicate {
			log.Printf("Skipping duplicate event log: block %d, txIndex %d, logIndex %d",
				eventLog.BlockNumber, eventLog.TxIndex, eventLog.Index)
			continue
		}

		n.processEventLog(ctx, eventLog, parsedABI)
	}

	log.Printf("Successfully caught up on missed events")

	return nil
}
