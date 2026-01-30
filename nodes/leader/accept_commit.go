package leader_node

import (
	"context"
	"encoding/hex"
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
	"github.com/libp2p/go-libp2p/core/peer"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/constants"
	"github.com/tokamak-network/DRB-node/utils"
)

type RandomRequest struct {
	Round     *big.Int
	TrialNum  *big.Int
	StartTime *big.Int
	State     *big.Int
}
type RoundData struct {
	MerkleRoot   bool
	RandomNumber bool
}

type LeaderCommitData struct {
	Round                 string            `json:"round"`
	EOAAddress            string            `json:"eoa_address"`
	Cvs                   [32]byte          `json:"cvs"`
	CvsHex                string            `json:"cvs_hex,omitempty"`
	Cos                   [32]byte          `json:"cos"`
	CosHex                string            `json:"cos_hex"`
	SecretValue           [32]byte          `json:"secret_value"`
	SecretValueHex        string            `json:"secret_value_hex"`
	Sign                  map[string]string `json:"sign"`
	SubmitMerkleRootDone  bool              `json:"submit_merkle_root_done"`
	RandomNumberGenerated bool              `json:"random_number_generated"`
	CreatedAt             int64             `json:"created_at"`
}

func (n *LeaderNode) ReceiveCommit(ctx context.Context) {
	n.receiveCommit(ctx)
}

func (n *LeaderNode) receiveCommit(ctx context.Context) {
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

		log.Printf("Websocket connection established. Catching up on missed events...")
		if catchUpErr := n.catchUpMissedEvents(ctx, contractAddr, parsedABI); catchUpErr != nil {
			log.Printf("Error catching up on missed events: %v", catchUpErr)
		}

		reconnect := false
		for {
			select {
			case <-ctx.Done():
				log.Println("ReceiveCommit function received shutdown signal, closing subscription...")
				sub.Unsubscribe()
				return
			case err := <-sub.Err():
				log.Printf("Error in event subscription: %v", err)

				if err != nil {
					log.Printf("Websocket closed abnormally. Attempting to reconnect in 1 seconds...")
					// Catch up on missed events before reconnecting
					if catchUpErr := n.catchUpMissedEvents(ctx, contractAddr, parsedABI); catchUpErr != nil {
						log.Printf("Error catching up on missed events due to subscription error: %v", catchUpErr)
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

func (n *LeaderNode) processDeactivated(ctx context.Context, operator common.Address) {
	fmt.Printf("Deactivated Event:\n Operator %v\n", operator)
	eth.Service.UpdateActivatedOperators(ctx, n.fallbackEthClient)

	var peerIDStr string
	nodeInfos, err := n.nodeInfoRepository.GetNodeInfos(ctx)
	if err == nil {
		for _, nodeInfo := range nodeInfos {
			if nodeInfo.EOAAddress == operator.Hex() {
				peerIDStr = nodeInfo.PeerID
				break
			}
		}
	}

	if peerIDStr != "" {
		hostInstance := n.p2pClient.GetHostInstance()
		if hostInstance != nil {
			peerID, err := peer.Decode(peerIDStr)
			if err == nil {
				conns := hostInstance.Network().ConnsToPeer(peerID)
				if len(conns) > 0 {
					log.Printf("Closing libp2p connection for deactivated operator %s (PeerID: %s)", operator.Hex(), peerIDStr)
					if err := hostInstance.Network().ClosePeer(peerID); err != nil {
						log.Printf("Failed to close connection for peer %s: %v", peerIDStr, err)
					} else {
						log.Printf("Successfully closed connection for deactivated operator %s", operator.Hex())
					}
				} else {
					log.Printf("No active connection found for operator %s (PeerID: %s)", operator.Hex(), peerIDStr)
				}
			} else {
				log.Printf("Invalid PeerID format for operator %s: %s", operator.Hex(), peerIDStr)
			}
		}
	}

	if err := n.nodeInfoRepository.DeleteNodeInfoByEOA(ctx, operator.Hex()); err != nil {
		log.Printf("Failed to delete node info for operator %s: %v", operator.Hex(), err)
	} else {
		log.Printf("Deleted node info for deactivated operator %s", operator.Hex())
	}
}

func (n *LeaderNode) processSubmittedSecretRequest(ctx context.Context, round *big.Int, trialNum *big.Int, secret [32]byte, index *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	intValue := int(index.Int64())
	activatedOps := n.ethService.GetActivatedOperatorsCached()
	if intValue >= len(activatedOps) {
		log.Printf("Index %d out of bounds for activated operators length %d", intValue, len(activatedOps))
		return
	}
	regularNodeAddress := activatedOps[intValue]
	fmt.Println("GetSecretRequestSentForWhichRound", n.GetSecretRequestSentForWhichRound())
	leaderCommits, err := n.leaderCommitRepository.GetLeaderCommitByRoundAndEoaAddr(ctx, n.GetSecretRequestSentForWhichRound(), trialNum.String(), regularNodeAddress.Hex())
	if err != nil {
		log.Printf("Failed to get leadercommit data from database by round and eoaAddress %v", err)
		return
	}

	// Update leader commit data with secretValue
	leaderCommits.SecretValue = secret
	secretHex := hex.EncodeToString(secret[:])
	leaderCommits.SecretValueHex = secretHex

	err = n.leaderCommitRepository.UpdateLeaderCommit(ctx, leaderCommits)
	if err != nil {
		log.Printf("Failed to save updated leader commits: %v", err)
	}

	// Broadcast the secret value to all activated regular nodes
	n.ReliableBroadCastSSync(ctx, n.p2pClient.GetHostInstance(), n.GetSecretRequestSentForWhichRound(), trialNum.String(), regularNodeAddress.Hex(), secret, activatedOps)
}

func (n *LeaderNode) processRandomRequestNumber(ctx context.Context, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, state %v\n", round, trialNum, state)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	// internally calls the cleanup function
	n.EnqueueUniqueKeyForCleanup(uniqueKey)

	// update the current round and trial
	n.SetCurrentRound(round.String())
	n.SetCurrentTrial(trialNum.String())

	// Reset leader monitoring state for new round or trail
	n.ResetLeaderMonitoringState(ctx, round.String(), trialNum.String())
	n.SetReq(RandomRequest{
		Round:     round,
		TrialNum:  trialNum,
		StartTime: blockTimestamp,
		State:     state,
	})
	if state.Cmp(big.NewInt(1)) == 0 {
		// Set Halted to 0 to resume the round
		n.SetHalted(false)
		// Delete round and trial data from database
		err := n.batchRepository.DeleteOldRoundDataForLeaderNode(ctx, round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}

		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		// Update the activated operators
		n.ethService.UpdateActivatedOperators(ctx, n.fallbackEthClient)
		// Reset the indices for the new round
		n.ResetIndicesForNewRound()
		log.Printf("Reset Indices array for new round %s with trail %s", n.GetCurrentRound(), n.GetCurrentTrial())

		// Start monitoring for automatic requestToSubmitCv
		n.startRequestToSubmitCvMonitoring(ctx, round.String(), trialNum.String(), blockTimestamp)

		n.SetExecution(true)
	}
	if state.Cmp(big.NewInt(2)) == 0 {
		data, exists := n.GetRoundData(uniqueKey)
		if !exists {
			data = RoundData{}
		}
		data.RandomNumber = true
		n.SetRoundData(uniqueKey, data)
		n.SetExecution(false)

		// Delete round and trial data from database
		err := n.batchRepository.DeleteOldRoundDataForLeaderNode(ctx, round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}
	}

	if state.Cmp(big.NewInt(3)) == 0 {
		// Set Execution to false to stop the round
		n.SetExecution(false)
		// Set Halted to 1 to halt the round
		n.SetHalted(true)
		// Delete round and trial data from database
		n.batchRepository.DeleteRoundTrialDataForLeaderNode(ctx, n.GetCurrentRound(), n.GetCurrentTrial())

		// resume the round
		n.resuming(ctx)
	}
}

func (n *LeaderNode) resuming(ctx context.Context) {
	// Load contract client (address, ABI, and leader private key)
	clientUtils, err := utils.NewLeaderClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create leader client: %v", err)
		return
	}
	contractAddress := clientUtils.ContractAddress
	parsedABI := clientUtils.ContractABI
	privateKey := clientUtils.PrivateKey
	leaderEOA := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Check deposit amount
	depositResult, err := n.ethService.CallSmartContract(ctx, n.fallbackEthClient, parsedABI, "s_depositAmount", contractAddress, leaderEOA)
	if err != nil {
		log.Printf("Failed to call s_depositAmount: %v", err)
		return
	}
	depositAmount, ok := depositResult.(*big.Int)
	if !ok {
		log.Printf("Unexpected type for depositAmount: %T", depositResult)
		return
	}
	minDeposit := new(big.Int).SetUint64(1e16) // 0.01 ETH in wei
	if depositAmount.Cmp(minDeposit) < 0 {
		// Need to top up
		amountToDeposit := new(big.Int).Sub(minDeposit, depositAmount)
		clientUtils := &utils.Client{
			ContractAddress: contractAddress,
			PrivateKey:      privateKey,
			ContractABI:     parsedABI,
		}
		_, _, err := n.ethService.ExecuteTransaction(
			ctx,
			clientUtils,
			n.fallbackEthClient,
			"deposit",
			amountToDeposit,
		)
		if err != nil {
			log.Printf("Failed to deposit: %v", err)
			return
		}
		log.Printf("Deposited %s wei to reach 0.01 ETH minimum.", amountToDeposit.String())
	}

	// Now poll getActivatedOperatorsLength and call resume when >=2
	for {
		opsLenResult, err := n.ethService.CallSmartContract(ctx, n.fallbackEthClient, parsedABI, "getActivatedOperatorsLength", contractAddress)
		if err != nil {
			log.Printf("Failed to call getActivatedOperatorsLength: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		opsLen, ok := opsLenResult.(*big.Int)
		if !ok {
			log.Printf("Unexpected type for getActivatedOperatorsLength: %T", opsLenResult)
			time.Sleep(5 * time.Second)
			continue
		}
		if opsLen.Cmp(big.NewInt(2)) >= 0 {
			// Call resume
			clientUtils := &utils.Client{
				ContractAddress: contractAddress,
				PrivateKey:      privateKey,
				ContractABI:     parsedABI,
			}
			_, _, err := n.ethService.ExecuteTransaction(
				ctx,
				clientUtils,
				n.fallbackEthClient,
				"resume",
				big.NewInt(0),
			)
			if err != nil {
				log.Printf("Failed to call resume: %v", err)
				return
			}
			log.Printf("Called resume() as activated operators >= 2.")
			return
		}
		log.Printf("Activated operators (%v) < 2 . Waiting to call resume...", opsLen)
		time.Sleep(5 * time.Second)
	}
}

func (n *LeaderNode) processCOS(ctx context.Context, round *big.Int, trialNum *big.Int, cos [32]byte, activatedOperatorIndex *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCOS.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, activatedOperatorIndex %v\n", round, trialNum, activatedOperatorIndex)

	activatedOps := n.ethService.GetActivatedOperatorsCached()
	if activatedOperatorIndex.Int64() >= int64(len(activatedOps)) {
		log.Printf("Index %d out of bounds for activated operators length %d", activatedOperatorIndex.Int64(), len(activatedOps))
		return nil
	}
	eoa := activatedOps[activatedOperatorIndex.Int64()]
	cosHex := hex.EncodeToString(cos[:])
	roundStr := round.String()
	trialNumStr := trialNum.String()
	uniqueKey := utils.GetUniqueKey(roundStr, trialNumStr)

	leaderCommitData, err := n.leaderCommitRepository.GetLeaderCommitByRoundAndEoaAddr(ctx, roundStr, trialNumStr, eoa.Hex())
	if err != nil {
		signInfo := utils.SignInfo{
			R: "",
			S: "",
			V: "",
		}
		leaderCommit := utils.LeaderCommitData{
			UniqueKey:             uniqueKey,
			Round:                 roundStr,
			TrialNum:              trialNumStr,
			EOAAddress:            eoa.Hex(),
			Cvs:                   [32]byte{},
			CvsHex:                "",
			Cos:                   [32]byte{},
			CosHex:                "",
			SecretValue:           [32]byte{},
			SecretValueHex:        "",
			Sign:                  signInfo,
			SubmitMerkleRootDone:  false,
			RandomNumberGenerated: false,
			CreatedAt:             time.Now().Unix(),
		}
		err := n.leaderCommitRepository.AddLeaderCommit(ctx, &leaderCommit)
		if err != nil {
			return fmt.Errorf("failed to add leader commit for round %s, trial %s, EOA %s: %v",
				roundStr, trialNumStr, eoa.Hex(), err)
		}
	} else {
		leaderCommitData.Cos = cos
		leaderCommitData.CosHex = cosHex

		err := n.leaderCommitRepository.UpdateLeaderCommit(ctx, leaderCommitData)
		if err != nil {
			return fmt.Errorf("failed to update leader commit for round %s, trial %s, EOA %s: %v",
				roundStr, trialNumStr, eoa.Hex(), err)
		}
	}

	n.updateCOS(ctx, roundStr, trialNumStr, uniqueKey, eoa, cos)
	fmt.Printf("Successfully stored COS for Round %s with Trail %s, EOA %s\n", roundStr, trialNumStr, eoa.Hex())

	// Broadcast the COS value to all activated regular nodes
	n.ReliableBroadCastCOS(ctx, roundStr, trialNumStr, eoa, cos, activatedOps)

	// Check if all COS values are received and stop monitoring if so
	n.checkAndStopFailToSubmitCoMonitoring(roundStr, trialNumStr)

	return nil
}

func (n *LeaderNode) updateCOS(ctx context.Context, round string, trialNum string, uniqueKey string, eoa common.Address, cos [32]byte) {
	utils.EnsureCommittedNodesRoundExists(uniqueKey)

	commitData, exists := utils.GetCommittedNodeData(uniqueKey, eoa)
	if !exists {
		commitData = utils.LeaderCommitData{}
	}

	commitData.EOAAddress = eoa.Hex()
	commitData.Round = round
	commitData.Cos = cos
	cosHex := hex.EncodeToString(cos[:])
	commitData.CosHex = cosHex
	utils.SetCommittedNodeData(uniqueKey, eoa, commitData)

	activatedOps := n.ethService.GetActivatedOperatorsCached()

	if n.AllCosReceivedUnlocked(uniqueKey) {
		log.Printf("All COS received for round %s with trail %s.", round, trialNum)
		_, err := n.revealOrderService.DetermineRevealOrder(ctx, round, trialNum, activatedOps)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s with trail %s: %v", round, trialNum, err)
			return
		}
		n.StartSecretValueRequests(ctx, n.p2pClient.GetHostInstance(), round, trialNum)
	}
}

func (n *LeaderNode) processCVS(ctx context.Context, round *big.Int, trialNum *big.Int, cvs [32]byte, activatedOperatorIndex *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCVS.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, activatedOperatorIndex %v\n", round, trialNum, activatedOperatorIndex)
	roundStr := round.String()
	trialNumStr := trialNum.String()
	activatedOps := n.ethService.GetActivatedOperatorsCached()
	if activatedOperatorIndex.Int64() >= int64(len(activatedOps)) {
		log.Printf("Index %d out of bounds for activated operators length %d", activatedOperatorIndex.Int64(), len(activatedOps))
		return nil
	}
	eoa := activatedOps[activatedOperatorIndex.Int64()]
	cvsHex := hex.EncodeToString(cvs[:])

	uniqueKey := utils.GetUniqueKey(roundStr, trialNumStr)
	leaderCommitData, err := n.leaderCommitRepository.GetLeaderCommitByRoundAndEoaAddr(ctx, roundStr, trialNumStr, eoa.Hex())
	if err != nil {
		signInfo := utils.SignInfo{
			R: "",
			S: "",
			V: "",
		}
		leaderCommit := utils.LeaderCommitData{
			UniqueKey:             uniqueKey,
			Round:                 roundStr,
			TrialNum:              trialNumStr,
			EOAAddress:            eoa.Hex(),
			Cvs:                   cvs,
			CvsHex:                cvsHex,
			Cos:                   [32]byte{},
			CosHex:                "",
			SecretValue:           [32]byte{},
			SecretValueHex:        "",
			Sign:                  signInfo,
			SubmitMerkleRootDone:  false,
			RandomNumberGenerated: false,
			CreatedAt:             time.Now().Unix(),
		}
		err := n.leaderCommitRepository.AddLeaderCommit(ctx, &leaderCommit)
		if err != nil {
			return fmt.Errorf("failed to add leader commit for round %s, trial %s, EOA %s: %v",
				roundStr, trialNumStr, eoa.Hex(), err)
		}
	} else {
		leaderCommitData.Cvs = cvs
		leaderCommitData.CvsHex = cvsHex

		err := n.leaderCommitRepository.UpdateLeaderCommit(ctx, leaderCommitData)
		if err != nil {
			return fmt.Errorf("failed to update leader commit for round %s, trial %s, EOA %s: %v",
				roundStr, trialNumStr, eoa.Hex(), err)
		}
	}

	n.updateCVS(roundStr, uniqueKey, eoa, cvs)
	fmt.Printf("Successfully stored CVS for Round %s with Trail %s, EOA %s\n", roundStr, trialNumStr, eoa.Hex())

	// Check if all CVS values are received and stop monitoring if needed
	n.checkAndStopFailToSubmitCvMonitoring(roundStr, trialNumStr)

	// Broadcast the CVS value to all activated regular nodes
	n.ReliableBroadCastCVS(ctx, roundStr, trialNumStr, eoa, cvs, activatedOps)
	if n.AllCvsReceivedUnlocked(uniqueKey) {
		n.GenerateMerkleRoot(ctx, roundStr, trialNumStr)
	}
	return nil
}

func (n *LeaderNode) updateCVS(round string, uniqueKey string, eoa common.Address, cvs [32]byte) {
	utils.EnsureCommittedNodesRoundExists(uniqueKey)

	commitData, exists := utils.GetCommittedNodeData(uniqueKey, eoa)
	if !exists {
		commitData = utils.LeaderCommitData{}
	}

	commitData.EOAAddress = eoa.Hex()
	commitData.Round = round
	commitData.Cvs = cvs
	cvsHex := hex.EncodeToString(cvs[:])
	commitData.CvsHex = cvsHex
	utils.SetCommittedNodeData(uniqueKey, eoa, commitData)

}

func (n *LeaderNode) AllCosReceivedUnlocked(uniqueKey string) bool {
	ops := n.ethService.GetActivatedOperatorsCached()
	if len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists || len(roundCommits) == 0 {
		return false
	}

	for _, op := range ops {
		data, ok := roundCommits[op]
		if !ok || data.Cos == [32]byte{} {
			return false
		}
	}
	return true
}

func (n *LeaderNode) AllCvsReceivedUnlocked(uniqueKey string) bool {
	ops := n.ethService.GetActivatedOperatorsCached()
	if len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists || len(roundCommits) == 0 {
		return false
	}

	for _, op := range ops {
		data, ok := roundCommits[op]
		if !ok || data.Cvs == [32]byte{} {
			return false
		}
	}
	return true
}

// Add new function to process RequestedToSubmitCo event
func (n *LeaderNode) processRequestedToSubmitCo(ctx context.Context, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processRequestedToSubmitCo.")
		return
	}

	fmt.Printf("RequestedToSubmitCo Event: Round %v, TrialNum %v, BlockTimestamp %v\n", round, trialNum, blockTimestamp)

	// Start monitoring for failToSubmitCo condition
	n.startFailToSubmitCoMonitoring(ctx, round.String(), trialNum.String(), blockTimestamp)
}

// Add new function to process RequestedToSubmitCv event
func (n *LeaderNode) processRequestedToSubmitCv(ctx context.Context, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processRequestedToSubmitCv.")
		return
	}

	// Stop the requestToSubmitCv monitoring since the request has been made
	if n.GetRequestToSubmitCvMonitoringActive() {
		log.Printf("RequestedToSubmitCv event received, stopping requestToSubmitCv monitoring for round %s", round.String())
		n.stopRequestToSubmitCvMonitoring()
	}

	// Start monitoring for failToSubmitCv condition
	n.startFailToSubmitCvMonitoring(ctx, round.String(), trialNum.String(), blockTimestamp)
}

// Add function to start monitoring for failToSubmitCo condition
func (n *LeaderNode) startFailToSubmitCoMonitoring(ctx context.Context, round string, trialNum string, requestedToSubmitCoTimestamp *big.Int) {
	if requestedToSubmitCoTimestamp == nil {
		log.Printf("requestedToSubmitCoTimestamp is nil, cannot start monitoring")
		return
	}

	// Use atomic compare-and-swap to prevent double start
	if !atomic.CompareAndSwapInt32(&n.requestedToSubmitCoMonitoringActive, 0, 1) {
		log.Printf("FailToSubmitCo monitoring already active for round %s with trail %s", round, trialNum)
		return
	}

	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	// Stop existing timer if present
	if n.requestedToSubmitCoMonitoringTimer != nil {
		n.requestedToSubmitCoMonitoringTimer.Stop()
		n.requestedToSubmitCoMonitoringTimer = nil
	}

	// Calculate deadline: requestedToSubmitCoTimestamp + s_onChainSubmissionPeriod
	deadline := new(big.Int).Add(requestedToSubmitCoTimestamp, appconfig.GetContractPeriods().OnChainSubmissionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	if duration <= 0 {
		log.Printf("Deadline has already passed for round %s with trail %s, calling failToSubmitCo immediately", round, trialNum)
		atomic.StoreInt32(&n.requestedToSubmitCoMonitoringActive, 0)
		n.callFailToSubmitCo(ctx, round, trialNum)
		return
	}

	log.Printf("Starting failToSubmitCo monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	n.requestedToSubmitCoMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToSubmitCo", round)
		n.callFailToSubmitCo(ctx, round, trialNum)
		atomic.StoreInt32(&n.requestedToSubmitCoMonitoringActive, 0)
	})
}

// Add function to stop failToSubmitCo monitoring
func (n *LeaderNode) stopFailToSubmitCoMonitoring() {
	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	if n.requestedToSubmitCoMonitoringTimer != nil {
		n.requestedToSubmitCoMonitoringTimer.Stop()
		n.requestedToSubmitCoMonitoringTimer = nil
	}
	atomic.StoreInt32(&n.requestedToSubmitCoMonitoringActive, 0)
	log.Printf("Stopped failToSubmitCo monitoring")
}

// Add function to call failToSubmitCo on chain
func (n *LeaderNode) callFailToSubmitCo(ctx context.Context, round string, trialNum string) {
	clientUtils, err := utils.NewLeaderClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create leader client: %v", err)
		return
	}

	_, _, err = n.ethService.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"failToSubmitCo",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to call failToSubmitCo for round %s with trail %s: %v", round, trialNum, err)
		return
	}

	log.Printf("Successfully called failToSubmitCo for round %s with trail %s", round, trialNum)
}

// Add function to check if all COS values are received and stop monitoring
func (n *LeaderNode) checkAndStopFailToSubmitCoMonitoring(round string, trialNum string) {
	// Only check if monitoring is active for this round/trial
	if !n.GetRequestedToSubmitCoMonitoringActive() {
		return
	}

	// Check if all activated operators have submitted COS values
	if n.AllCosReceivedUnlocked(utils.GetUniqueKey(round, trialNum)) {
		log.Printf("All COS values received for round %s trial %s, stopping failToSubmitCo monitoring", round, trialNum)
		n.stopFailToSubmitCoMonitoring()
	}
}

// Add function to check if all CVS values are received and stop monitoring
func (n *LeaderNode) checkAndStopFailToSubmitCvMonitoring(round string, trialNum string) {
	// Only check if monitoring is active for this round/trial
	if !n.GetRequestedToSubmitCvMonitoringActive() {
		return
	}

	// Check if all activated operators have submitted CVS values
	if n.AllCvsReceivedUnlocked(utils.GetUniqueKey(round, trialNum)) {
		log.Printf("All CVS values received for round %s trial %s, stopping failToSubmitCv monitoring", round, trialNum)
		n.stopFailToSubmitCvMonitoring()
	}
}

// Add function to start monitoring for failToSubmitCv condition
func (n *LeaderNode) startFailToSubmitCvMonitoring(ctx context.Context, round string, trialNum string, requestedToSubmitCvTimestamp *big.Int) {
	if requestedToSubmitCvTimestamp == nil {
		log.Printf("requestedToSubmitCvTimestamp is nil, cannot start monitoring")
		return
	}

	// Use atomic compare-and-swap to prevent double start
	if !atomic.CompareAndSwapInt32(&n.requestedToSubmitCvMonitoringActive, 0, 1) {
		log.Printf("FailToSubmitCv monitoring already active for round %s with trail %s", round, trialNum)
		return
	}

	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	// Stop existing timer if present
	if n.requestedToSubmitCvMonitoringTimer != nil {
		n.requestedToSubmitCvMonitoringTimer.Stop()
		n.requestedToSubmitCvMonitoringTimer = nil
	}

	// Get timing parameters from config
	periods := appconfig.GetContractPeriods()

	// Calculate deadline: requestedToSubmitCvTimestamp + s_onChainSubmissionPeriod
	deadline := new(big.Int).Add(requestedToSubmitCvTimestamp, periods.OnChainSubmissionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting failToSubmitCv monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	n.requestedToSubmitCvMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("⚠️ Deadline reached for round %s, calling failToSubmitCv", round)
		n.callFailToSubmitCv(ctx, round, trialNum)
		atomic.StoreInt32(&n.requestedToSubmitCvMonitoringActive, 0)
	})
}

// Add function to stop failToSubmitCv monitoring
func (n *LeaderNode) stopFailToSubmitCvMonitoring() {
	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	if n.requestedToSubmitCvMonitoringTimer != nil {
		n.requestedToSubmitCvMonitoringTimer.Stop()
		n.requestedToSubmitCvMonitoringTimer = nil
	}
	atomic.StoreInt32(&n.requestedToSubmitCvMonitoringActive, 0)
	log.Printf("Stopped failToSubmitCv monitoring")
}

// Add function to call failToSubmitCv on chain
func (n *LeaderNode) callFailToSubmitCv(ctx context.Context, round string, trialNum string) {
	clientUtils, err := utils.NewLeaderClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create leader client: %v", err)
		return
	}

	_, _, err = n.ethService.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"failToSubmitCv",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to call failToSubmitCv for round %s with trail %s: %v", round, trialNum, err)
		return
	}

	log.Printf("Successfully called failToSubmitCv for round %s with trail %s", round, trialNum)
}

// ResetCosAndCvsMonitoringState resets COS and CVS monitoring variables
func (n *LeaderNode) ResetCosAndCvsMonitoringState(round string, trialNum string) {
	// Stop COS monitoring (this uses proper synchronization)
	n.stopFailToSubmitCoMonitoring()

	// Stop CVS monitoring (failToSubmitCv) (this uses proper synchronization)
	n.stopFailToSubmitCvMonitoring()

	// Stop requestToSubmitCv monitoring (this uses proper synchronization)
	n.stopRequestToSubmitCvMonitoring()

	// The individual stop functions already handle proper timer cleanup with mutex protection
	// No need for duplicate timer operations here since they're handled in the stop functions

	log.Printf("Reset COS and CVS monitoring state for round %s", round)
}

func (n *LeaderNode) CheckHaltedState(ctx context.Context) {
	// Load contract ABI and address
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}

	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		log.Printf("CONTRACT_ADDRESS is not set in environment variables.")
		return
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	// Check s_isInProcess storage variable
	result, err := n.ethService.CallSmartContract(ctx, n.fallbackEthClient, parsedABI, "s_isInProcess", contractAddress)
	if err != nil {
		log.Printf("Failed to call s_isInProcess: %v", err)
		return
	}

	isInProcess, ok := result.(*big.Int)
	if !ok {
		log.Printf("Unexpected type for s_isInProcess: %T", result)
		return
	}

	log.Printf("Current s_isInProcess value: %v", isInProcess)

	// If s_isInProcess equals 3, call resume function
	if isInProcess.Cmp(big.NewInt(3)) == 0 {
		// set Halted to 1 as protocol is halted
		n.SetHalted(true)
		n.resuming(ctx)
	} else {
		log.Printf("s_isInProcess is %v, no action needed", isInProcess)
	}
}

// Add function to start monitoring for automatic requestToSubmitCv
func (n *LeaderNode) startRequestToSubmitCvMonitoring(ctx context.Context, round string, trialNum string, startTime *big.Int) {
	if startTime == nil {
		log.Printf("StartTime is nil, cannot start requestToSubmitCv monitoring")
		return
	}

	// Use atomic compare-and-swap to prevent double start
	if !atomic.CompareAndSwapInt32(&n.requestToSubmitCvMonitoringActive, 0, 1) {
		log.Printf("RequestToSubmitCv monitoring already active for round %s with trail %s", round, trialNum)
		return
	}

	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	// Stop existing timer if present
	if n.requestToSubmitCvMonitoringTimer != nil {
		n.requestToSubmitCvMonitoringTimer.Stop()
		n.requestToSubmitCvMonitoringTimer = nil
	}

	// Get timing parameters from config
	periods := appconfig.GetContractPeriods()

	// Calculate deadline: startTime + s_offChainSubmissionPeriod + s_requestOrSubmitOrFailDecisionPeriod
	totalPeriod := new(big.Int).Add(periods.OffChainSubmissionPeriod, periods.RequestOrSubmitOrFailDecisionPeriod)
	totalPeriod = new(big.Int).Sub(totalPeriod, big.NewInt(20)) // 20 seconds can vary according to block chain network
	deadline := new(big.Int).Add(startTime, totalPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting requestToSubmitCv monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)
	log.Printf("Parameters - StartTime: %v, offChainPeriod: %v, requestOrSubmitPeriod: %v",
		startTime, periods.OffChainSubmissionPeriod, periods.RequestOrSubmitOrFailDecisionPeriod)

	// Set timer to call the function when deadline is reached
	n.requestToSubmitCvMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("⚠️ Deadline reached for round %s, calling requestToSubmitCv", round)
		n.callRequestToSubmitCv(ctx, round, trialNum)
		// Use atomic store to safely set to false
		atomic.StoreInt32(&n.requestToSubmitCvMonitoringActive, 0)
	})
}

// Add function to stop requestToSubmitCv monitoring
func (n *LeaderNode) stopRequestToSubmitCvMonitoring() {
	// Protect timer operations with mutex
	n.timerMutex.Lock()
	defer n.timerMutex.Unlock()

	if n.requestToSubmitCvMonitoringTimer != nil {
		n.requestToSubmitCvMonitoringTimer.Stop()
		n.requestToSubmitCvMonitoringTimer = nil
	}
	// Use atomic store to safely set to false
	atomic.StoreInt32(&n.requestToSubmitCvMonitoringActive, 0)
	log.Printf("Stopped requestToSubmitCv monitoring")
}

// Add function to call requestToSubmitCv when regular nodes haven't submitted CVS
func (n *LeaderNode) callRequestToSubmitCv(ctx context.Context, round string, trialNum string) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping callRequestToSubmitCv.")
		return
	}

	log.Printf("Calling requestToSubmitCv for round %s with trial %s due to missing CVS submissions", round, trialNum)

	// Get the list of missing operators (those who haven't submitted CVS)
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	missingOperators := n.getMissingCvsOperators(uniqueKey)

	if len(missingOperators) == 0 {
		log.Printf("All CVS received for round %s, no need to call requestToSubmitCv", round)
		return
	}

	log.Printf("Missing CVS from operators: %v", missingOperators)

	// Implement the same logic as handleMissingCV from leaderNode.go
	n.SetCvOnChain(uniqueKey, true)
	activatedOperators := n.ethService.GetActivatedOperatorsCached()
	i := big.NewInt(0)
	for _, op := range activatedOperators {
		for _, missingOp := range missingOperators {
			if op.Hex() == missingOp {
				n.AppendToIndices(i)
			}
		}
		i.Add(i, big.NewInt(1))
	}

	// Get the current indices and sort them
	indices := n.GetIndices()
	sort.Slice(indices, func(i, j int) bool {
		return indices[i].Cmp(indices[j]) < 0
	})

	// Update the sorted indices back
	n.SetIndices(indices)

	packedIndices := PackIndices(indices)

	clientUtils, err := utils.NewLeaderClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create leader client: %v", err)
		return
	}

	_, _, err = n.ethService.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"requestToSubmitCv",
		big.NewInt(0),
		packedIndices,
	)
	if err != nil {
		log.Printf("Failed to submit commit request root for round %s with trail %s: %v", round, trialNum, err)
		return
	}

	log.Printf("Successfully submitted commit request for round %s with trail %s and indices %v", round, trialNum, indices)
}

// Helper function to get missing CVS operators
func (n *LeaderNode) getMissingCvsOperators(uniqueKey string) []string {
	var missingOperators []string
	ops := n.ethService.GetActivatedOperatorsCached()

	roundCommits, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists {
		// No commits at all, all operators are missing
		for _, op := range ops {
			missingOperators = append(missingOperators, op.Hex())
		}
		return missingOperators
	}

	// Check which operators haven't submitted CVS
	for _, op := range ops {
		data, ok := roundCommits[op]
		if !ok || data.Cvs == [32]byte{} {
			missingOperators = append(missingOperators, op.Hex())
		}
	}

	return missingOperators
}

// GenerateMerkleRoot generates and submits merkle root for the given round and trial
func (n *LeaderNode) GenerateMerkleRoot(ctx context.Context, roundNum string, trialNum string) {
	if appconfig.Get().DisableMerkleRootSubmission {
		log.Println("Merkle root submission is disabled via configuration. Skipping GenerateMerkleRoot once.")
		appconfig.Get().DisableMerkleRootSubmission = false
		return
	}
	if n.GetHalted() {
		log.Println("System is halted. Skipping GenerateMerkleRoot.")
		return
	}

	if !n.CompareAndSwapSubmittingMerkleRoot(false, true) {
		log.Printf("Merkle root generation already in progress for round %s with trail %s, skipping.", roundNum, trialNum)
		return
	}

	shouldResetFlag := true
	defer func() {
		if shouldResetFlag {
			n.SetSubmittingMerkleRoot(false)
		}
	}()

	n.commitMu.Lock()
	uniqueKey := utils.GetUniqueKey(roundNum, trialNum)

	// Check if already done via round data
	roundData, exists := n.GetRoundData(uniqueKey)
	if exists && roundData.MerkleRoot {
		log.Printf("Merkle root already submitted for round %s with trail %s, skipping.", roundNum, trialNum)
		n.commitMu.Unlock()
		return
	}
	n.commitMu.Unlock()

	log.Printf("Generating Merkle root for round %s with trail %s...", roundNum, trialNum)

	activatedOperatorsList := n.ethService.GetActivatedOperatorsCached()

	log.Printf("Activated operators for round %s with trail %s in order: %v", roundNum, trialNum, activatedOperatorsList)

	n.commitMu.Lock()
	roundMap, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists || len(roundMap) == 0 {
		log.Printf("No commits found in-memory for round %s with trail %s, cannot generate Merkle root.", roundNum, trialNum)
		n.commitMu.Unlock()
		return
	}

	var leaves [][]byte
	for _, opAddr := range activatedOperatorsList {
		data, ok := roundMap[opAddr]
		if !ok || data.Cvs == [32]byte{} {
			log.Printf("Missing CVS for operator %s in round %s with trail %s", opAddr.Hex(), roundNum, trialNum)
			n.commitMu.Unlock()
			return
		}
		leaves = append(leaves, data.Cvs[:])
		log.Printf("Added CVS from operator %s for round %s with trail %s", opAddr.Hex(), roundNum, trialNum)
	}

	n.commitMu.Unlock()

	if len(leaves) == 0 {
		log.Printf("Error: No CVS commits found for round %s with trail %s. Cannot generate Merkle root.", roundNum, trialNum)
		return
	}

	log.Printf("Leaves for Merkle tree for round %s with trail %s: %v", roundNum, trialNum, leaves)

	merkleRoot, err := commitreveal2.CreateMerkleTree(leaves)
	if err != nil {
		log.Printf("Failed to create Merkle tree for round %s with trail %s: %v", roundNum, trialNum, err)
		return
	}

	shouldResetFlag = false
	n.SubmitMerkleRoot(ctx, roundNum, trialNum, merkleRoot)
}

// SubmitMerkleRoot submits the merkle root to the blockchain
func (n *LeaderNode) SubmitMerkleRoot(ctx context.Context, roundNum string, trialNum string, merkleRoot []byte) {
	var merkleRootBytes32 [32]byte
	copy(merkleRootBytes32[:], merkleRoot)

	clientUtils, err := utils.NewLeaderClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create leader client: %v", err)
		n.SetSubmittingMerkleRoot(false) // Reset flag on failure
		return
	}

	_, _, err = n.ethService.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"submitMerkleRoot",
		big.NewInt(0),
		merkleRootBytes32,
	)
	if err != nil {
		log.Printf("Failed to submit Merkle root for round %s with trail %s: %v", roundNum, trialNum, err)
		n.SetSubmittingMerkleRoot(false) // Reset flag on failure
		return
	}

	log.Printf("Successfully submitted Merkle root for round %s with trail %s", roundNum, trialNum)
	n.SetSubmittingMerkleRoot(false) // Reset flag on success
	uniqueKey := utils.GetUniqueKey(roundNum, trialNum)
	roundData, exists := n.GetRoundData(uniqueKey)
	if !exists {
		roundData = RoundData{}
	}
	roundData.MerkleRoot = true
	n.SetRoundData(uniqueKey, roundData)
	n.updateCommitDataAfterSubmit(ctx, uniqueKey)
}

// updateCommitDataAfterSubmit updates commit data after successful merkle root submission
func (n *LeaderNode) updateCommitDataAfterSubmit(ctx context.Context, uniqueKey string) {
	roundMap, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists {
		return
	}

	for eoaAddress, data := range roundMap {
		if data.SubmitMerkleRootDone {
			continue
		} else {
			data.SubmitMerkleRootDone = true
			utils.SetCommittedNodeData(uniqueKey, eoaAddress, data)
			n.leaderCommitRepository.UpdateLeaderCommit(ctx, &data)
		}
	}

}

// startRequestToSubmitCoMonitoring starts monitoring for automatic requestToSubmitCo
func (n *LeaderNode) startRequestToSubmitCoMonitoring(ctx context.Context, roundNum string, trialNum string, merkleRootSubmittedTime *big.Int) {
	if merkleRootSubmittedTime == nil {
		log.Printf("merkleRootSubmittedTime is nil, cannot start requestToSubmitCo monitoring")
		return
	}

	uniqueKey := utils.GetUniqueKey(roundNum, trialNum)

	// Check if monitoring is already active for this round
	if n.GetRequestToSubmitCoTimerMonitoringActive() {
		log.Printf("RequestToSubmitCo monitoring already active for round %s with trail %s", roundNum, trialNum)
		return
	}

	n.SetRequestToSubmitCoTimerMonitoringActive(true)
	log.Printf("Started requestToSubmitCo monitoring for round %s with trail %s", roundNum, trialNum)

	// Get timing parameters from config
	periods := appconfig.GetContractPeriods()

	// Calculate the deadline: merkleRootSubmittedTime + s_offChainSubmissionPeriod + s_requestOrSubmitOrFailDecisionPeriod
	deadline := new(big.Int).Add(merkleRootSubmittedTime, periods.OffChainSubmissionPeriod)
	deadline.Add(deadline, periods.RequestOrSubmitOrFailDecisionPeriod)

	currentTime := big.NewInt(time.Now().Unix())

	// Determine chainID from client and compute seconds buffer from configured block time
	chainID, err := n.fallbackEthClient.NetworkID(ctx)
	if err != nil {
		log.Printf("Failed to get network ID: %v", err)
		return
	}
	blockTime := constants.Chains[chainID.Uint64()].BlockTime
	bufferSeconds := int64(5*3) * int64(blockTime.Seconds())
	buffer := big.NewInt(bufferSeconds)
	// Calculate how long to wait
	duration := new(big.Int).Sub(deadline, currentTime)
	waitDuration := new(big.Int).Sub(duration, buffer)

	if waitDuration.Cmp(big.NewInt(0)) <= 0 {
		log.Printf("Deadline already passed for requestToSubmitCo in round %s with trail %s", roundNum, trialNum)
		n.callRequestToSubmitCoIfNeeded(ctx, roundNum, trialNum, uniqueKey)
		return
	}

	waitSeconds := waitDuration.Int64()
	log.Printf("Waiting %d seconds before checking COS for round %s with trail %s", waitSeconds, roundNum, trialNum)

	// Use time.AfterFunc for the timer
	n.requestToSubmitCoTimerMonitoringTimer = time.AfterFunc(time.Duration(waitSeconds)*time.Second, func() {
		if n.GetRequestToSubmitCoTimerMonitoringActive() {
			n.callRequestToSubmitCoIfNeeded(ctx, roundNum, trialNum, uniqueKey)
		}
		n.SetRequestToSubmitCoTimerMonitoringActive(false)
	})
}

// callRequestToSubmitCoIfNeeded checks if COS are missing and calls requestToSubmitCo
func (n *LeaderNode) callRequestToSubmitCoIfNeeded(ctx context.Context, roundNum string, trialNum string, uniqueKey string) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping callRequestToSubmitCoIfNeeded.")
		return
	}

	n.commitMu.Lock()
	defer n.commitMu.Unlock()

	ops := n.ethService.GetActivatedOperatorsCached()
	roundCommits, roundExists := utils.GetCommittedNodes(uniqueKey)
	if !roundExists {
		log.Printf("No round data found for COS check in round %s with trail %s", roundNum, trialNum)
		return
	}

	var missingIndices []*big.Int
	for i, op := range ops {
		data, ok := roundCommits[op]
		if !ok || data.Cos == [32]byte{} {
			missingIndices = append(missingIndices, big.NewInt(int64(i)))
			log.Printf("Missing COS for operator %s at index %d for round %s with trail %s", op.Hex(), i, roundNum, trialNum)
		}
	}

	if len(missingIndices) > 0 {
		log.Printf("🔄 Requesting on-chain for missing COS indices: %v for round %s with trail %s", missingIndices, roundNum, trialNum)
		n.requestToSubmitCo(ctx, roundNum, trialNum, missingIndices)
	} else {
		log.Printf("✅ All COS values received for round %s with trail %s, no requestToSubmitCo needed", roundNum, trialNum)
	}
}

// stopRequestToSubmitCoMonitoring stops the requestToSubmitCo monitoring
func (n *LeaderNode) stopRequestToSubmitCoMonitoring() {
	if n.GetRequestToSubmitCoTimerMonitoringActive() {
		n.SetRequestToSubmitCoTimerMonitoringActive(false)
		if n.requestToSubmitCoTimerMonitoringTimer != nil {
			n.requestToSubmitCoTimerMonitoringTimer.Stop()
			n.requestToSubmitCoTimerMonitoringTimer = nil
		}
		log.Printf("Stopped requestToSubmitCo monitoring")
	}
	n.SetRequestToSubmitCoTimerMonitoringActive(false)
}

// CleanupRoundDataByUniqueKey cleans up all map entries for a specific uniqueKey
func (n *LeaderNode) CleanupRoundDataByUniqueKey(uniqueKey string) {
	// Clean up RoundsData
	n.DeleteRoundsData(uniqueKey)
	// Clean up CommittedNodes
	utils.DeleteCommittedNodes(uniqueKey)

	// Clean up other maps that use uniqueKey
	n.DeleteCvOnChain(uniqueKey)
	n.DeleteRevealRequestStatus(uniqueKey)
	n.DeleteRoundSecrets(uniqueKey)
	n.DeleteRoundSecret(uniqueKey)
	n.DeleteSecretsOnChain(uniqueKey)
	// DeleteStrictOrderWhileSecretRequest(uniqueKey)
	// DeleteSubmittedCvIndices(uniqueKey)

	log.Printf("Cleaned up data for uniqueKey: %s", uniqueKey)
}

// Define the types needed for COS requests (moved from leaderNode.go)
type SigRS struct {
	R [32]byte
	S [32]byte
}

type CvAndSigRS struct {
	Cv [32]byte
	Rs SigRS
}

// requestToSubmitCo submits on-chain request for missing COS values
func (n *LeaderNode) requestToSubmitCo(ctx context.Context, roundNum string, trialNum string, missingIndices []*big.Int) {
	cvNotOnChainCvAndSigRS, packedVs, indicesLength, packedOrederedIndices := n.prepareArgumentsForRequestToSubmitCo(ctx, roundNum, trialNum, missingIndices)

	clientUtils, err := utils.NewLeaderClient("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to create leader client: %v", err)
		return
	}

	_, _, err = n.ethService.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"requestToSubmitCo",
		big.NewInt(0),
		cvNotOnChainCvAndSigRS,
		packedVs,
		indicesLength,
		packedOrederedIndices,
	)
	if err != nil {
		log.Printf("Failed to submit commit request root for round %s with trail %s: %v", roundNum, trialNum, err)
		return
	}

	log.Printf("Successfully submitted cos request for round %s with trail %s and indices %v", roundNum, trialNum, missingIndices)
}

// prepareArgumentsForRequestToSubmitCo prepares arguments for COS request
func (n *LeaderNode) prepareArgumentsForRequestToSubmitCo(ctx context.Context, roundNum string, trialNum string, missingIndices []*big.Int) ([]CvAndSigRS, *big.Int, *big.Int, *big.Int) {
	cvs, _, _, vs, rs, ss := n.LoadNodeData(ctx, roundNum, trialNum)
	indicesLength := big.NewInt(int64(len(missingIndices)))

	notOnChainIndices, onChainIndices := n.orderedPackedIndices(missingIndices)

	allOrderedIndices := append(notOnChainIndices, onChainIndices...)
	packedOrderedIndices := PackIndices(allOrderedIndices)
	var cvNotOnChainCvAndSigRS []CvAndSigRS
	var vsForNotOnChain []*big.Int
	for _, i := range notOnChainIndices {
		index := int(i.Int64())

		vsForNotOnChain = append(vsForNotOnChain, big.NewInt(int64(vs[index])))
		var cv32 [32]byte
		copy(cv32[:], cvs[index])
		var r32, s32 [32]byte
		copy(r32[:], rs[index].Bytes())
		copy(s32[:], ss[index].Bytes())
		cvAndSigRS := CvAndSigRS{
			Cv: cv32,
			Rs: SigRS{
				R: r32,
				S: s32,
			},
		}
		cvNotOnChainCvAndSigRS = append(cvNotOnChainCvAndSigRS, cvAndSigRS)
	}
	packedVs := PackIndices(vsForNotOnChain)
	return cvNotOnChainCvAndSigRS, packedVs, indicesLength, packedOrderedIndices
}

// orderedPackedIndices separates indices into on-chain and not-on-chain
func (n *LeaderNode) orderedPackedIndices(missingIndices []*big.Int) ([]*big.Int, []*big.Int) {
	onChainCvIndices := make(map[int64]struct{})
	indices := n.GetIndices()
	for _, idx := range indices {
		onChainCvIndices[idx.Int64()] = struct{}{}
	}

	var notOnChain []*big.Int
	var onChain []*big.Int

	for _, idx := range missingIndices {
		if _, isOnChain := onChainCvIndices[idx.Int64()]; !isOnChain {
			notOnChain = append(notOnChain, idx)
		} else {
			onChain = append(onChain, idx)
		}
	}
	return notOnChain, onChain
}

func (n *LeaderNode) getLastProcessedCoords() (uint64, uint, uint) {
	n.lastProcessedCoordsMu.RLock()
	defer n.lastProcessedCoordsMu.RUnlock()
	return n.lastProcessedBlock, n.lastProcessedTxIndex, n.lastProcessedLogIndex
}

func (n *LeaderNode) updateLastProcessedCoords(blockNumber uint64, txIndex uint, logIndex uint) {
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

func (n *LeaderNode) processEventLog(ctx context.Context, vLog types.Log, parsedABI abi.ABI) {
	isReorg := vLog.Removed
	if isReorg {
		log.Printf("Reorg detected. Skipping event: %v", vLog.TxHash)
		return
	}

	StatusSig := parsedABI.Events["Status"].ID
	isCriticalEvent := vLog.Topics[0] == StatusSig

	if !isCriticalEvent {
		if n.GetHalted() {
			log.Println("System is halted. Skipping event processing.")
			return
		}
	}

	CvsEventSig := parsedABI.Events["CvSubmitted"].ID
	CoSubmittedSig := parsedABI.Events["CoSubmitted"].ID
	SSubmittedSig := parsedABI.Events["SSubmitted"].ID
	RequestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID
	RequestedToSubmitCvSig := parsedABI.Events["RequestedToSubmitCv"].ID
	MerkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID
	RequestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID
	DeactivatedSig := parsedABI.Events["DeActivated"].ID
	switch vLog.Topics[0] {
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

		err = n.processCVS(ctx, eventData.Round, eventData.TrialNum, eventData.Cv, eventData.Index)
		if err != nil {
			log.Printf("Failed to process CVS for round %s, trial %s: %v",
				eventData.Round.String(), eventData.TrialNum.String(), err)
			return
		}

	case CoSubmittedSig:
		eventData := struct {
			Round    *big.Int
			TrialNum *big.Int
			Co       [32]byte
			Index    *big.Int
		}{}
		err := parsedABI.UnpackIntoInterface(&eventData, "CoSubmitted", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode CoSubmitted event log: %v", err)
			return
		}

		fmt.Printf("CoSubmitted Event: Fetched successfully")

		err = n.processCOS(ctx, eventData.Round, eventData.TrialNum, eventData.Co, eventData.Index)
		if err != nil {
			log.Printf("Failed to process COS for round %s, trial %s: %v",
				eventData.Round.String(), eventData.TrialNum.String(), err)
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

		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}
		n.stopRequestToSubmitCvMonitoring()
		n.startRequestToSubmitCoMonitoring(ctx, eventData.Round.String(), eventData.TrialNum.String(), big.NewInt(int64(blockTimestamp)))

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

		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}

		n.processRequestedToSubmitCo(ctx, big.NewInt(int64(blockTimestamp)), eventData.Round, eventData.TrialNum)

	case RequestedToSubmitCvSig:
		eventData := struct {
			Round                         *big.Int
			TrialNum                      *big.Int
			PackedIndicesAscendingFromLSB *big.Int
		}{}
		err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCv", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode RequestedToSubmitCv event log: %v", err)
			return
		}

		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
			return
		}

		fmt.Printf("\033[34mRequestedToSubmitCv Event: Round %v, TrialNum %v, BlockTimestamp %v\033[0m\n", eventData.Round, eventData.TrialNum, blockTimestamp)
		n.processRequestedToSubmitCv(ctx, big.NewInt(int64(blockTimestamp)), eventData.Round, eventData.TrialNum)

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

		n.processRandomRequestNumber(ctx, big.NewInt(int64(blockTimestamp)), eventData.CurRound, eventData.CurTrialNum, eventData.CurState)
		n.stopRequestToSubmitCoMonitoring()

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
		n.stopRequestToSubmitCoMonitoring()

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

		blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(ctx, big.NewInt(int64(vLog.BlockNumber)))
		if err != nil {
			log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
		} else {
			n.UpdateLastSubmitSTimestamp(ctx, big.NewInt(int64(blockTimestamp)), eventData.Round.String(), eventData.TrialNum.String())
		}

		n.processSubmittedSecretRequest(ctx, eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)

	case DeactivatedSig:
		eventData := struct {
			Operator common.Address
		}{}
		err := parsedABI.UnpackIntoInterface(&eventData, "DeActivated", vLog.Data)
		if err != nil {
			log.Printf("Failed to decode DeActivated event log: %v", err)
			return
		}
		n.processDeactivated(ctx, eventData.Operator)
	}

	n.updateLastProcessedCoords(vLog.BlockNumber, vLog.TxIndex, vLog.Index) // need to check
}

func (n *LeaderNode) catchUpMissedEvents(ctx context.Context, contractAddr common.Address, parsedABI abi.ABI) error {
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
		if missedLogs[i].TxIndex != missedLogs[j].TxIndex {
			return missedLogs[i].TxIndex < missedLogs[j].TxIndex
		}
		return missedLogs[i].Index < missedLogs[j].Index
	})

	log.Printf("Processing %d missed events in blockchain order", len(missedLogs))

	for _, eventLog := range missedLogs {
		lastProcessedBlock, lastProcessedTxIndex, lastProcessedLogIndex := n.getLastProcessedCoords()

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
