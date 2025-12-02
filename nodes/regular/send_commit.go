package regular_node

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
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
			log.Printf("Failed to subscribe to logs: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		RequestedToSubmitCvS := parsedABI.Events["RequestedToSubmitCv"].ID
		CvsEventSig := parsedABI.Events["CvSubmitted"].ID
		StatusSig := parsedABI.Events["Status"].ID
		MerkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID
		RequestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID
		RequestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID
		SSubmittedSig := parsedABI.Events["SSubmitted"].ID

		reconnect := false
		for {
			select {
			case <-ctx.Done():
				log.Println("MonitorCommitRequest function received shutdown signal, closing subscription...")
				sub.Unsubscribe()
				return
			case err := <-sub.Err():
				log.Printf("Error in event subscription: %v", err)

				if err != nil && (strings.Contains(err.Error(), "websocket: close 1006") || strings.Contains(err.Error(), "unexpected EOF")) {
					log.Printf("Websocket closed abnormally. Attempting to reconnect in 1 seconds...")
				} else {
					log.Printf("Fatal error in event subscription: %v", err)
				}
				time.Sleep(1 * time.Second)
				reconnect = true

			case vLog := <-logs:
				{
					isReorg := vLog.Removed
					if isReorg {
						log.Printf("Reorg detected. Skipping event: %v", vLog.TxHash)
						continue
					}
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
							continue
						}

						fmt.Printf("CommitRequest Event: Round %v, TrialNum %v, indices %v\n", eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)

						// Mark CV as requested and stop leader monitoring
						n.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(eventData.Round.String(), eventData.TrialNum.String())

						// Get the block timestamp for the RequestedToSubmitCv event
						blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}
						// requestedToSubmitCvTime = big.NewInt(int64(blockTimestamp))

						// Start monitoring for merkle root submission
						n.StartMerkleRootMonitoring(ctx, eventData.Round.String(), eventData.TrialNum.String(), big.NewInt(int64(blockTimestamp)))

						n.processCommitRequest(ctx, eventData.Round, eventData.TrialNum, eventData.PackedIndicesAscendingFromLSB)

					case StatusSig:
						eventData := struct {
							CurRound    *big.Int
							CurTrialNum *big.Int
							CurState    *big.Int
						}{}

						err := parsedABI.UnpackIntoInterface(&eventData, "Status", vLog.Data)
						if err != nil {
							log.Printf("Failed to decode Status event log: %v", err)
							continue
						}

						// Get the actual block timestamp
						blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}

						n.processRandomRequestNumber(ctx, big.NewInt(int64(blockTimestamp)), eventData.CurRound, eventData.CurTrialNum, eventData.CurState)

					case MerkleRootSubmittedSig:
						eventData := struct {
							Round      *big.Int
							TrialNum   *big.Int
							MerkleRoot [32]byte
						}{}

						err := parsedABI.UnpackIntoInterface(&eventData, "MerkleRootSubmitted", vLog.Data)
						if err != nil {
							log.Printf("Failed to decode MerkleRootSubmitted event log: %v", err)
							continue
						}
						fmt.Printf("MerkleRootSubmitted Event:\n Round %v, TrialNum %v, MerkleRoot: %v\n Round: %v\n",
							eventData.Round, eventData.TrialNum, eventData.MerkleRoot, eventData.Round.String())

						// Mark Merkle root as submitted and stop leader monitoring
						n.SetMerkleRootSubmittedEventEmitted(true)
						blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
						n.SetSubmitSMonitoringReferenceTime(big.NewInt(int64(blockTimestamp)))

						n.StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(eventData.Round.String(), eventData.TrialNum.String())
						n.StopFailToSubmitMerkleRootAfterDisputeMonitoring(eventData.Round.String(), eventData.TrialNum.String())
						n.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(ctx, eventData.Round.String(), eventData.TrialNum.String())

						n.processMerkleRoot(eventData.Round, eventData.TrialNum)

						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
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
							continue
						}
						fmt.Printf("RequestedToSubmitCo Event: Round %v, TrialNum %v, indicesLength %v\n, indices %v\n", eventData.Round, eventData.TrialNum, eventData.IndicesLength, eventData.PackedIndices)

						// Get the block timestamp for RequestedToSubmitCo event
						blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}

						// Stop the current monitoring
						n.StopRequestToSubmitSOrGenerateRandomNumberMonitoring(eventData.Round.String(), eventData.TrialNum.String())

						// Update the time reference to RequestedToSubmitCo event time
						n.SetSubmitSMonitoringReferenceTime(big.NewInt(int64(blockTimestamp)))

						// Restart monitoring with the new time reference
						n.StartRequestToSubmitSOrGenerateRandomNumberMonitoring(ctx, eventData.Round.String(), eventData.TrialNum.String())

						log.Printf("Restarted monitoring for round %s trial %s with RequestedToSubmitCo event time", eventData.Round.String(), eventData.TrialNum.String())

						n.processCosRequest(ctx, eventData.Round, eventData.TrialNum, eventData.PackedIndices, eventData.IndicesLength)

					case RequestedToSubmitSFromIndexKSig:
						eventData := struct {
							Round    *big.Int
							TrialNum *big.Int
							IndexK   *big.Int
						}{}

						err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitSFromIndexK", vLog.Data)

						if err != nil {
							log.Printf("Failed to decode RequestedToSubmitSFromIndexK event log: %v", err)
							continue
						}
						fmt.Printf("RequestedToSubmitSFromIndexK Event:\n Round %v, TrialNum %v, indexK %v\n", eventData.Round, eventData.TrialNum, eventData.IndexK)

						// Stop request to submit S or generate random number monitoring when RequestedToSubmitSFromIndexK event is received
						n.StopRequestToSubmitSOrGenerateRandomNumberMonitoring(eventData.Round.String(), eventData.TrialNum.String())
						blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
						n.SetSubmitSMonitoringReferenceTime(big.NewInt(int64(blockTimestamp)))
						if err != nil {
							log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
							continue
						}
						n.processSecretRequest(ctx, eventData.Round, eventData.TrialNum, eventData.IndexK)

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
							continue
						}
						fmt.Printf("SSubmitted Event:\n Round %v, TrialNum %v, Secret %v\n, indexK %v\n ", eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
						n.processSubmittedSecretRequest(ctx, eventData.Round, eventData.TrialNum, eventData.Index)
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
							continue
						}
						fmt.Printf("CvSubmitted Event: Fetched successfully")
						n.processCvSubmitted(eventData.Round, eventData.TrialNum, eventData.Index)
					}
				}
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

func (n *RegularNode) processCvSubmitted(round *big.Int, trialNum *big.Int, index *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCVS.")
		return
	}

	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)

	// Use the new atomic setter function
	indexStr := index.String()
	n.SetSubmittedCvIndicesValue(uniqueKey, indexStr, true)

	log.Printf("CV submitted for index %s in round %s with trail %s", indexStr, round.String(), trialNum.String())

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

func (n *RegularNode) processSubmittedSecretRequest(ctx context.Context, round, trialNum, index *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}

	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	data, err := n.revealOrderRepository.GetRevealOrder(ctx, round.String(), trialNum.String())
	if err != nil {
		log.Printf("Failed to get reveal order for round %s with trail %s: %v", round.String(), trialNum.String(), err)
	}
	orderedNodes := data.OrderedNodes
	revealOrder := data.RevealOrder
	if index.Int64() >= int64(len(orderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(orderedNodes))
		return
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
				n.submitS(ctx, round.String(), trialNum.String())
			}
		}
	}
}

func (n *RegularNode) processSecretRequest(ctx context.Context, round, trialNum, index *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	if appconfig.Get().DisableSecretSubmission {
		log.Println("Secret submission is disabled via configuration. Skipping processSecretRequest.")
		return
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
			log.Printf("Failed to determine reveal order: %v", err)
			return
		}

		// Try to get reveal order again after creation
		revealOrder, err = n.revealOrderRepository.GetRevealOrder(ctx, round.String(), trialNum.String())
		if err != nil {
			log.Printf("Still failed to get reveal order after creation: %v", err)
			return
		}
		log.Printf("Successfully created and retrieved reveal order")
	}

	// Check bounds safely
	if revealOrder == nil {
		log.Printf("Reveal order is nil for round %s with trail %s", round.String(), trialNum.String())
		return
	}

	if revealOrder.OrderedNodes == nil {
		log.Printf("Reveal order ordered nodes is nil for round %s with trail %s", round.String(), trialNum.String())
		return
	}

	if index.Int64() >= int64(len(revealOrder.OrderedNodes)) {
		log.Printf("Index %d is out of bounds for the ordered nodes length %d", index.Int64(), len(revealOrder.OrderedNodes))
		return
	}

	regularEoaAddress := revealOrder.OrderedNodes[index.Int64()]
	if n.GetRegularNodeEOA() == regularEoaAddress {
		fmt.Printf("Processing RequestedToSubmitSFromIndexK event for Round: %v, EOA: %v\n", round, regularEoaAddress)
		n.submitS(ctx, round.String(), trialNum.String())
	}
}

func (n *RegularNode) submitS(ctx context.Context, round string, trialNum string) {
	roundData, err := n.regularCommitRepository.GetCommitByRound(ctx, round, trialNum)
	if err != nil {
		log.Printf("Failed to get regular commit for round %s with : %v", round, err)
	}

	secretValueBytes := roundData.SecretValue

	fmt.Printf("Extracted secret_value as bytes32: %x\n", secretValueBytes)

	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Fatalf("Failed to create EOA client: %v", err)
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
		log.Printf("Failed to submit secret_value: %v", err)
		return
	}

	log.Printf("Successfully submitted secret_value: %x", secretValueBytes)
}

func (n *RegularNode) processMerkleRoot(Round *big.Int, TrialNum *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	fmt.Printf("Round %v, TrialNum %v\n", Round, TrialNum)
	uniqueKey := utils.GetUniqueKey(Round.String(), TrialNum.String())
	roundData, exists := n.GetRoundData(uniqueKey)
	if !exists {
		roundData = RoundData{}
	}
	roundData.MerkleRoot = true
	n.SetRoundData(uniqueKey, roundData)
}

func (n *RegularNode) processRandomRequestNumber(ctx context.Context, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {

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
		go n.AllCosReceivedUnlocked(ctx, round.String(), trialNum.String())
	}

	if state.Cmp(big.NewInt(2)) == 0 {
		// Delete old round data except current round from database
		err := n.batchRepository.DeleteOldRoundDataForRegularNode(ctx, round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
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
		n.batchRepository.DeleteRoundTrialDataForRegularNode(ctx, n.GetCurrentRound(), trialNum.String())
		// resume the round
		n.SetHalted(true)
		// resuming(fallbackEthClient)
		n.SetExecution(false)
	}

	// Update rounds data
	if n.roundsData == nil {
		n.roundsData = make(map[string]RoundData)
	}

	n.SetRoundData(uniqueKey, RoundData{
		MerkleRoot:   false,
		RandomNumber: false,
	})
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
		fmt.Println(err)
	}
	if !flag {
		fmt.Println("Cv Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing RequestedToSubmitCv event for Round: %v\n", round.String())

	commitData, err := n.regularCommitRepository.GetCommitByRound(ctx, round.String(), trialNum.String())
	if err != nil {
		return err
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
		fmt.Println("It contains error")
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
		fmt.Println(err)
	}
	if !flag {
		fmt.Println("Cos Request does not contain our EOA")
		return nil
	}

	fmt.Printf("Processing RequestedToSubmitCo event for Round: %v\n", Round.String())

	commitData, err := n.regularCommitRepository.GetCommitByRound(ctx, Round.String(), TrialNum.String())
	if err != nil {
		fmt.Println("Error loading commits:", err)
		return err
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
		fmt.Println("It contains error")
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
		if eoaAddress == activatedOps[index.Int64()] {
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
	if n.GetLeaderMonitoringActive() {
		log.Printf("Leader monitoring already active for round %s", round)
		return
	}

	if startTime == nil {
		log.Printf("StartTime is nil, cannot start monitoring")
		return
	}

	n.SetLeaderMonitoringActive(true)

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
		n.callFailToRequestSubmitCVOrSubmitMerkleRoot(ctx, round, trialNum)
		return
	}

	log.Printf("Starting leader monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	n.monitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSubmitCVOrSubmitMerkleRoot", round)
		n.callFailToRequestSubmitCVOrSubmitMerkleRoot(ctx, round, trialNum)
		n.SetLeaderMonitoringActive(false)
	})
}

// StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring stops the current leader monitoring
func (n *RegularNode) StopFailToRequestSubmitCVOrSubmitMerkleRootMonitoring(round string, trialNum string) {
	if !n.GetLeaderMonitoringActive() {
		return
	}

	if n.monitoringTimer != nil {
		n.monitoringTimer.Stop()
		n.monitoringTimer = nil
	}

	n.SetLeaderMonitoringActive(false)
	log.Printf("Stopped leader monitoring for round %s", round)
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
func (n *RegularNode) callFailToRequestSubmitCVOrSubmitMerkleRoot(ctx context.Context, round string, trialNum string) {
	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create EOA client: %v", err)
		return
	}

	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"failToRequestSubmitCvOrSubmitMerkleRoot",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to call failToRequestSubmitCVOrSubmitMerkleRoot: %v", err)
		return
	}

	log.Printf("Successfully called failToRequestSubmitCVOrSubmitMerkleRoot for round %swith trail %s", round, trialNum)
}

// StartMerkleRootMonitoring starts monitoring for merkle root submission after CV request
func (n *RegularNode) StartMerkleRootMonitoring(ctx context.Context, round string, trialNum string, requestedToSubmitCvTime *big.Int) {

	if requestedToSubmitCvTime == nil {
		log.Printf("RequestedToSubmitCvTime is nil, cannot start merkle root monitoring")
		return
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
			n.callFailToSubmitMerkleRootAfterDispute(ctx, round, trialNum)
		} else {
			log.Printf("Conditions not met for failToSubmitMerkleRootAfterDispute - CVs not all submitted or merkle root already submitted")
		}
	})
}

// StopFailToSubmitMerkleRootAfterDisputeMonitoring stops the current merkle root monitoring
func (n *RegularNode) StopFailToSubmitMerkleRootAfterDisputeMonitoring(round string, trialNum string) {

	if n.merkleRootMonitoringTimer != nil {
		n.merkleRootMonitoringTimer.Stop()
		n.merkleRootMonitoringTimer = nil
	}

	log.Printf("Stopped merkle root monitoring for round %s", round)
}

// callFailToSubmitMerkleRootAfterDispute calls the contract function to fail the leader for not submitting merkle root
func (n *RegularNode) callFailToSubmitMerkleRootAfterDispute(ctx context.Context, round string, trialNum string) {
	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create EOA client: %v", err)
		return
	}

	_, _, err = eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"failToSubmitMerkleRootAfterDispute",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to call failToSubmitMerkleRootAfterDispute: %v", err)
		return
	}

	log.Printf("Successfully called failToSubmitMerkleRootAfterDispute for round %s with trail %s", round, trialNum)
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

	// Get timing parameters from config
	periods := appconfig.GetContractPeriods()
	activatedOperatorsLength := new(big.Int).SetInt64(eth.Service.GetActivatedOperatorsLength())

	// Calculate deadline: s_merkleRootSubmittedTime + s_offChainSubmissionPeriod + (s_offChainSubmissionPeriodPerOperator * activatedOperatorsLength) + s_requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(n.GetSubmitSMonitoringReferenceTime(), periods.OffChainSubmissionPeriod)
	operatorDelay := new(big.Int).Mul(periods.OffChainSubmissionPeriodPerOperator, activatedOperatorsLength)
	deadline.Add(deadline, operatorDelay)
	deadline.Add(deadline, periods.RequestOrSubmitOrFailDecisionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting request to submit S or generate random number monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)
	log.Printf("Parameters - merkleRootSubmittedTime: %v, offChainSubmissionPeriod: %v, offChainSubmissionPeriodPerOperator: %v, activatedOperatorsLength: %v, requestOrSubmitOrFailDecisionPeriod: %v",
		n.GetSubmitSMonitoringReferenceTime(), periods.OffChainSubmissionPeriod, periods.OffChainSubmissionPeriodPerOperator, activatedOperatorsLength, periods.RequestOrSubmitOrFailDecisionPeriod)

	// Set timer to call the function when deadline is reached
	n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToRequestSOrGenerateRandomNumber", round)
		n.callFailToRequestSOrGenerateRandomNumber(ctx, round, trialNum)
	})
}

// StopRequestToSubmitSOrGenerateRandomNumberMonitoring stops the monitoring
func (n *RegularNode) StopRequestToSubmitSOrGenerateRandomNumberMonitoring(round string, trialNum string) {

	if n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer != nil {
		n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer.Stop()
		n.requestToSubmitSOrGenerateRandomNumberMonitoringTimer = nil
	}

	log.Printf("Stopped request to submit S or generate random number monitoring for round %s", round)
}

// callFailToRequestSOrGenerateRandomNumber calls the contract function to fail
func (n *RegularNode) callFailToRequestSOrGenerateRandomNumber(ctx context.Context, round string, trialNum string) {
	log.Printf("Calling failToRequestSOrGenerateRandomNumber for round %s with trial %s", round, trialNum)

	clientUtils, err := utils.NewEOAClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create EOA client: %v", err)
		return
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
		log.Printf("Failed to execute failToRequestSOrGenerateRandomNumber transaction: %v", err)
		return
	}

	log.Printf("Successfully called failToRequestSOrGenerateRandomNumber for round %s with trial %s", round, trialNum)
}
