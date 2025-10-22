package leader_node

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
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

func (n *LeaderNode) ReceiveCommit() {
	n.receiveCommit()
}

func (n *LeaderNode) receiveCommit() {
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
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
		sub, err := n.fallbackEthClient.SubscribeFilterLogs(context.Background(), query, logs)
		if err != nil {
			log.Printf("Failed to subscribe to logs: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		CvsEventSig := parsedABI.Events["CvSubmitted"].ID
		StatusSig := parsedABI.Events["Status"].ID
		CoSubmittedSig := parsedABI.Events["CoSubmitted"].ID
		SSubmittedSig := parsedABI.Events["SSubmitted"].ID
		RequestedToSubmitCoSig := parsedABI.Events["RequestedToSubmitCo"].ID
		RequestedToSubmitCvSig := parsedABI.Events["RequestedToSubmitCv"].ID
		MerkleRootSubmittedSig := parsedABI.Events["MerkleRootSubmitted"].ID
		RequestedToSubmitSFromIndexKSig := parsedABI.Events["RequestedToSubmitSFromIndexK"].ID

		reconnect := false
		for {
			select {
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
				isReorg := vLog.Removed
				if isReorg {
					log.Printf("Reorg detected. Skipping event: %v", vLog.TxHash)
					continue
				}
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
						continue
					}
					fmt.Printf("CvSubmitted Event: Fetched successfully")

					n.processCVS(eventData.Round, eventData.TrialNum, eventData.Cv, eventData.Index)

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
						continue
					}
					fmt.Printf("CoSubmitted Event: Fetched successfully")

					n.processCOS(eventData.Round, eventData.TrialNum, eventData.Co, eventData.Index)

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
					blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))

					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
						continue
					}
					// stop requestToSubmitCv monitoring
					n.stopRequestToSubmitCvMonitoring()
					// Start requestToSubmitCo monitoring
					n.startRequestToSubmitCoMonitoring(eventData.Round.String(), eventData.TrialNum.String(), big.NewInt(int64(blockTimestamp)))

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

					blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
						continue
					}

					n.processRequestedToSubmitCo(big.NewInt(int64(blockTimestamp)), eventData.Round, eventData.TrialNum)

				case RequestedToSubmitCvSig:
					eventData := struct {
						Round                         *big.Int
						TrialNum                      *big.Int
						PackedIndicesAscendingFromLSB *big.Int
					}{}
					err := parsedABI.UnpackIntoInterface(&eventData, "RequestedToSubmitCv", vLog.Data)
					if err != nil {
						log.Printf("Failed to decode RequestedToSubmitCv event log: %v", err)
						continue
					}

					blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
						continue
					}

					fmt.Printf("\033[34mRequestedToSubmitCv Event: Round %v, TrialNum %v, BlockTimestamp %v\033[0m\n", eventData.Round, eventData.TrialNum, blockTimestamp)
					n.processRequestedToSubmitCv(big.NewInt(int64(blockTimestamp)), eventData.Round, eventData.TrialNum)

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

					blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
						continue
					}

					n.processRandomRequestNumber(big.NewInt(int64(blockTimestamp)), eventData.CurRound, eventData.CurTrialNum, eventData.CurState)

					// Stop requestToSubmitCo monitoring when Status event is received
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
						continue
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
						continue
					}
					fmt.Printf("SSubmitted Event:\n Round %v, TrialNum %v, Secret %v\n, indexK %v\n ", eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)

					// Get block timestamp and update the last submit S timestamp for monitoring
					blockTimestamp, err := n.fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
					} else {
						n.UpdateLastSubmitSTimestamp(big.NewInt(int64(blockTimestamp)), eventData.Round.String(), eventData.TrialNum.String())
					}

					n.processSubmittedSecretRequest(eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
				}
			}
			if reconnect {
				log.Printf("Reconnection triggered, breaking out of event loop to restart subscription...")
				break // break inner for loop to reconnect
			}
		}
	}
}

func (n *LeaderNode) processSubmittedSecretRequest(round *big.Int, trialNum *big.Int, secret [32]byte, index *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	intValue := int(index.Int64())
	activatedOps := eth.GetActivatedOperatorsCached()
	if intValue >= len(activatedOps) {
		log.Printf("Index %d out of bounds for activated operators length %d", intValue, len(activatedOps))
		return
	}
	regularNodeAddress := activatedOps[intValue]
	fmt.Println("GetSecretRequestSentForWhichRound", n.GetSecretRequestSentForWhichRound())
	leaderCommits, err := database.GetLeaderCommitByRoundAndEoaAddr(n.GetSecretRequestSentForWhichRound(), trialNum.String(), regularNodeAddress.Hex())
	if err != nil {
		log.Printf("Failed to get leadercommit data from database by round and eoaAddress %v", err)
		return
	}

	// Update leader commit data with secretValue
	leaderCommits.SecretValue = secret
	secretHex := hex.EncodeToString(secret[:])
	leaderCommits.SecretValueHex = secretHex

	err = database.UpdateLeaderCommit(leaderCommits)
	if err != nil {
		log.Printf("Failed to save updated leader commits: %v", err)
	}

	// Broadcast the secret value to all activated regular nodes
	n.ReliableBroadCastSSync(libp2putils.HostInstance, n.GetSecretRequestSentForWhichRound(), trialNum.String(), regularNodeAddress.Hex(), secret, activatedOps)
}

func (n *LeaderNode) processRandomRequestNumber(blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, state %v\n", round, trialNum, state)
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	// internally calls the cleanup function
	n.EnqueueUniqueKeyForCleanup(uniqueKey)

	// update the current round and trial
	n.SetCurrentRound(round.String())
	n.SetCurrentTrial(trialNum.String())

	// Reset leader monitoring state for new round or trail
	n.ResetLeaderMonitoringState(round.String(), trialNum.String())
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
		err := database.DeleteOldRoundDataForLeaderNode(round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}

		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		// Update the activated operators
		eth.UpdateActivatedOperators(n.fallbackEthClient)
		// Reset the indices for the new round
		n.ResetIndicesForNewRound()
		log.Printf("Reset Indices array for new round %s with trail %s", n.GetCurrentRound(), n.GetCurrentTrial())

		// Start monitoring for automatic requestToSubmitCv
		n.startRequestToSubmitCvMonitoring(round.String(), trialNum.String(), blockTimestamp)

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
		err := database.DeleteOldRoundDataForLeaderNode(round.String())
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
		database.DeleteRoundTrialDataForLeaderNode(n.GetCurrentRound(), n.GetCurrentTrial())

		// resume the round
		n.resuming()
	}
}

func (n *LeaderNode) resuming() {
	// Load contract ABI and address
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}
	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode leader private key: %v", err)
	}
	leaderEOA := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Check deposit amount
	depositResult, err := eth.CallSmartContract(n.fallbackEthClient, parsedABI, "s_depositAmount", contractAddress, leaderEOA)
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
		_, _, err := eth.ExecuteTransaction(
			context.Background(),
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
		opsLenResult, err := eth.CallSmartContract(n.fallbackEthClient, parsedABI, "getActivatedOperatorsLength", contractAddress)
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
			_, _, err := eth.ExecuteTransaction(
				context.Background(),
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

func (n *LeaderNode) processCOS(round *big.Int, trialNum *big.Int, cos [32]byte, activatedOperatorIndex *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCOS.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, activatedOperatorIndex %v\n", round, trialNum, activatedOperatorIndex)

	activatedOps := eth.GetActivatedOperatorsCached()
	if activatedOperatorIndex.Int64() >= int64(len(activatedOps)) {
		log.Printf("Index %d out of bounds for activated operators length %d", activatedOperatorIndex.Int64(), len(activatedOps))
		return nil
	}
	eoa := activatedOps[activatedOperatorIndex.Int64()]
	cosHex := hex.EncodeToString(cos[:])
	roundStr := round.String()
	trialNumStr := trialNum.String()
	uniqueKey := utils.GetUniqueKey(roundStr, trialNumStr)

	leaderCommitData, err := database.GetLeaderCommitByRoundAndEoaAddr(roundStr, trialNumStr, eoa.Hex())
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
		err := database.AddLeaderCommit(&leaderCommit)
		if err != nil {
			fmt.Printf("Failed to add leader commit: %v", err)
		}
	} else {
		leaderCommitData.Cos = cos
		leaderCommitData.CosHex = cosHex

		err := database.UpdateLeaderCommit(leaderCommitData)
		if err != nil {
			fmt.Printf("Failed to update leader commit: %v", err)
		}
	}

	n.updateCOS(roundStr, trialNumStr, uniqueKey, eoa, cos)
	fmt.Printf("Successfully stored COS for Round %s with Trail %s, EOA %s\n", roundStr, trialNumStr, eoa.Hex())

	// Broadcast the COS value to all activated regular nodes
	n.ReliableBroadCastCOS(libp2putils.HostInstance, roundStr, trialNumStr, eoa, cos, activatedOps)

	// Check if all COS values are received and stop monitoring if so
	n.checkAndStopFailToSubmitCoMonitoring(roundStr, trialNumStr)

	return nil
}

func (n *LeaderNode) updateCOS(round string, trialNum string, uniqueKey string, eoa common.Address, cos [32]byte) {
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

	activatedOps := eth.GetActivatedOperatorsCached()

	if n.AllCosReceivedUnlocked(uniqueKey) {
		log.Printf("All COS received for round %s with trail %s.", round, trialNum)
		_, err := commitreveal2.DetermineRevealOrder(round, trialNum, activatedOps)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s with trail %s: %v", round, trialNum, err)
			return
		}
		n.StartSecretValueRequests(libp2putils.HostInstance, round, trialNum)
	}
}

func (n *LeaderNode) processCVS(round *big.Int, trialNum *big.Int, cvs [32]byte, activatedOperatorIndex *big.Int) error {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processCVS.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, activatedOperatorIndex %v\n", round, trialNum, activatedOperatorIndex)
	roundStr := round.String()
	trialNumStr := trialNum.String()
	activatedOps := eth.GetActivatedOperatorsCached()
	if activatedOperatorIndex.Int64() >= int64(len(activatedOps)) {
		log.Printf("Index %d out of bounds for activated operators length %d", activatedOperatorIndex.Int64(), len(activatedOps))
		return nil
	}
	eoa := activatedOps[activatedOperatorIndex.Int64()]
	cvsHex := hex.EncodeToString(cvs[:])

	uniqueKey := utils.GetUniqueKey(roundStr, trialNumStr)
	leaderCommitData, err := database.GetLeaderCommitByRoundAndEoaAddr(roundStr, trialNumStr, eoa.Hex())
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
		err := database.AddLeaderCommit(&leaderCommit)
		if err != nil {
			fmt.Printf("Failed to add leader commit: %v", err)
		}
	} else {
		leaderCommitData.Cvs = cvs
		leaderCommitData.CvsHex = cvsHex

		err := database.UpdateLeaderCommit(leaderCommitData)
		if err != nil {
			fmt.Printf("Failed to update leader commit: %v", err)
		}
	}

	n.updateCVS(roundStr, uniqueKey, eoa, cvs)
	fmt.Printf("Successfully stored CVS for Round %s with Trail %s, EOA %s\n", roundStr, trialNumStr, eoa.Hex())

	// Check if all CVS values are received and stop monitoring if needed
	n.checkAndStopFailToSubmitCvMonitoring(roundStr, trialNumStr)

	// Broadcast the CVS value to all activated regular nodes
	n.ReliableBroadCastCVS(libp2putils.HostInstance, roundStr, trialNumStr, eoa, cvs, activatedOps)
	if n.AllCvsReceivedUnlocked(uniqueKey) {
		n.GenerateMerkleRoot(roundStr, trialNumStr)
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
	ops := eth.GetActivatedOperatorsCached()
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
	ops := eth.GetActivatedOperatorsCached()
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
func (n *LeaderNode) processRequestedToSubmitCo(blockTimestamp *big.Int, round *big.Int, trialNum *big.Int) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping processRequestedToSubmitCo.")
		return
	}

	fmt.Printf("RequestedToSubmitCo Event: Round %v, TrialNum %v, BlockTimestamp %v\n", round, trialNum, blockTimestamp)

	// Start monitoring for failToSubmitCo condition
	n.startFailToSubmitCoMonitoring(round.String(), trialNum.String(), blockTimestamp)
}

// Add new function to process RequestedToSubmitCv event
func (n *LeaderNode) processRequestedToSubmitCv(blockTimestamp *big.Int, round *big.Int, trialNum *big.Int) {
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
	n.startFailToSubmitCvMonitoring(round.String(), trialNum.String(), blockTimestamp)
}

// Add function to start monitoring for failToSubmitCo condition
func (n *LeaderNode) startFailToSubmitCoMonitoring(round string, trialNum string, requestedToSubmitCoTimestamp *big.Int) {
	if requestedToSubmitCoTimestamp == nil {
		log.Printf("requestedToSubmitCoTimestamp is nil, cannot start monitoring")
		return
	}

	n.SetRequestedToSubmitCoMonitoringActive(true)

	// Get s_onChainSubmissionPeriod from contract
	onChainSubmissionPeriod := big.NewInt(120)

	// Calculate deadline: requestedToSubmitCoTimestamp + s_onChainSubmissionPeriod
	deadline := new(big.Int).Add(requestedToSubmitCoTimestamp, onChainSubmissionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	if duration <= 0 {
		log.Printf("Deadline has already passed for round %s with trail %s, calling failToSubmitCo immediately", round, trialNum)
		n.callFailToSubmitCo(round, trialNum)
		return
	}

	log.Printf("Starting failToSubmitCo monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	n.requestedToSubmitCoMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToSubmitCo", round)
		n.callFailToSubmitCo(round, trialNum)
		n.SetRequestedToSubmitCoMonitoringActive(false)
	})
}

// Add function to stop failToSubmitCo monitoring
func (n *LeaderNode) stopFailToSubmitCoMonitoring() {
	if n.requestedToSubmitCoMonitoringTimer != nil {
		n.requestedToSubmitCoMonitoringTimer.Stop()
		n.requestedToSubmitCoMonitoringTimer = nil
	}
	n.SetRequestedToSubmitCoMonitoringActive(false)
	log.Printf("Stopped failToSubmitCo monitoring")
}

// Add function to call failToSubmitCo on chain
func (n *LeaderNode) callFailToSubmitCo(round string, trialNum string) {
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
func (n *LeaderNode) startFailToSubmitCvMonitoring(round string, trialNum string, requestedToSubmitCvTimestamp *big.Int) {
	if requestedToSubmitCvTimestamp == nil {
		log.Printf("requestedToSubmitCvTimestamp is nil, cannot start monitoring")
		return
	}

	n.SetRequestedToSubmitCvMonitoringActive(true)

	// Get s_onChainSubmissionPeriod from contract (60 seconds as specified)
	onChainSubmissionPeriod := big.NewInt(60)

	// Calculate deadline: requestedToSubmitCvTimestamp + s_onChainSubmissionPeriod
	deadline := new(big.Int).Add(requestedToSubmitCvTimestamp, onChainSubmissionPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting failToSubmitCv monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	n.requestedToSubmitCvMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("⚠️ Deadline reached for round %s, calling failToSubmitCv", round)
		n.callFailToSubmitCv(round, trialNum)
		n.SetRequestedToSubmitCvMonitoringActive(false)
	})
}

// Add function to stop failToSubmitCv monitoring
func (n *LeaderNode) stopFailToSubmitCvMonitoring() {
	if n.requestedToSubmitCvMonitoringTimer != nil {
		n.requestedToSubmitCvMonitoringTimer.Stop()
		n.requestedToSubmitCvMonitoringTimer = nil
	}
	n.SetRequestedToSubmitCvMonitoringActive(false)
	log.Printf("Stopped failToSubmitCv monitoring")
}

// Add function to call failToSubmitCv on chain
func (n *LeaderNode) callFailToSubmitCv(round string, trialNum string) {
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
	// Stop COS monitoring
	n.stopFailToSubmitCoMonitoring()

	// Stop CVS monitoring (failToSubmitCv)
	n.stopFailToSubmitCvMonitoring()

	// Stop requestToSubmitCv monitoring
	n.stopRequestToSubmitCvMonitoring()

	// Reset COS monitoring variables
	n.SetRequestedToSubmitCoMonitoringActive(false)
	if n.requestedToSubmitCoMonitoringTimer != nil {
		n.requestedToSubmitCoMonitoringTimer.Stop()
		n.requestedToSubmitCoMonitoringTimer = nil
	}

	// Reset CVS monitoring variables (failToSubmitCv)
	n.SetRequestedToSubmitCvMonitoringActive(false)
	if n.requestedToSubmitCvMonitoringTimer != nil {
		n.requestedToSubmitCvMonitoringTimer.Stop()
		n.requestedToSubmitCvMonitoringTimer = nil
	}

	// Reset requestToSubmitCv monitoring variables
	n.SetRequestToSubmitCvMonitoringActive(false)
	if n.requestToSubmitCvMonitoringTimer != nil {
		n.requestToSubmitCvMonitoringTimer.Stop()
		n.requestToSubmitCvMonitoringTimer = nil
	}

	// Reset secret request tracking variable
	n.SetSecretRequestSentForWhichRound("")

	log.Printf("Reset COS and CVS monitoring state for round %s", round)
}

func (n *LeaderNode) CheckHaltedState() {
	// Load contract ABI and address
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Printf("CONTRACT_ADDRESS is not set in environment variables.")
		return
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	// Check s_isInProcess storage variable
	result, err := eth.CallSmartContract(n.fallbackEthClient, parsedABI, "s_isInProcess", contractAddress)
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
		n.resuming()
	} else {
		log.Printf("s_isInProcess is %v, no action needed", isInProcess)
	}
}

// Add function to start monitoring for automatic requestToSubmitCv
func (n *LeaderNode) startRequestToSubmitCvMonitoring(round string, trialNum string, startTime *big.Int) {
	if startTime == nil {
		log.Printf("StartTime is nil, cannot start requestToSubmitCv monitoring")
		return
	}

	n.SetRequestToSubmitCvMonitoringActive(true)

	// Calculate deadline: startTime + s_offChainSubmissionPeriod + s_requestOrSubmitOrFailDecisionPeriod
	s_offChainSubmissionPeriod := big.NewInt(40)
	s_requestOrSubmitOrFailDecisionPeriod := big.NewInt(30)

	// deadline = startTime + 40 + 30 = startTime + 70 seconds
	totalPeriod := new(big.Int).Add(s_offChainSubmissionPeriod, s_requestOrSubmitOrFailDecisionPeriod)
	deadline := new(big.Int).Add(startTime, totalPeriod)

	// Convert deadline to time.Duration
	deadlineTime := time.Unix(deadline.Int64(), 0)
	now := time.Now()
	duration := deadlineTime.Sub(now)

	log.Printf("Starting requestToSubmitCv monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)
	log.Printf("Parameters - StartTime: %v, offChainPeriod: %v, requestOrSubmitPeriod: %v",
		startTime, s_offChainSubmissionPeriod, s_requestOrSubmitOrFailDecisionPeriod)

	// Set timer to call the function when deadline is reached
	n.requestToSubmitCvMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("⚠️ Deadline reached for round %s, calling requestToSubmitCv", round)
		n.callRequestToSubmitCv(round, trialNum)
		n.SetRequestToSubmitCvMonitoringActive(false)
	})
}

// Add function to stop requestToSubmitCv monitoring
func (n *LeaderNode) stopRequestToSubmitCvMonitoring() {
	if n.requestToSubmitCvMonitoringTimer != nil {
		n.requestToSubmitCvMonitoringTimer.Stop()
		n.requestToSubmitCvMonitoringTimer = nil
	}
	n.SetRequestToSubmitCvMonitoringActive(false)
	log.Printf("Stopped requestToSubmitCv monitoring")
}

// Add function to call requestToSubmitCv when regular nodes haven't submitted CVS
func (n *LeaderNode) callRequestToSubmitCv(round string, trialNum string) {
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
	activatedOperators := eth.GetActivatedOperatorsCached()
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
	ops := eth.GetActivatedOperatorsCached()

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
func (n *LeaderNode) GenerateMerkleRoot(roundNum string, trialNum string) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping GenerateMerkleRoot.")
		return
	}
	n.commitMu.Lock()
	// Check if merkle root is a-lready done before proceeding
	uniqueKey := utils.GetUniqueKey(roundNum, trialNum)

	// Check if merkle root is already submitted using atomic operation
	if n.GetSubmittingMerkleRoot() {
		log.Printf("Merkle root is already being submitted for round %s with trail %s, skipping.", roundNum, trialNum)
		n.commitMu.Unlock()
		return
	}

	// Check if already done via round data
	roundData, exists := n.GetRoundData(uniqueKey)
	if exists && roundData.MerkleRoot {
		log.Printf("Merkle root already submitted for round %s with trail %s, skipping.", roundNum, trialNum)
		n.commitMu.Unlock()
		return
	}
	n.commitMu.Unlock()

	log.Printf("Generating Merkle root for round %s with trail %s...", roundNum, trialNum)

	activatedOperatorsList := eth.GetActivatedOperatorsCached()

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
	// Use atomic compare-and-swap to prevent race condition
	if n.CompareAndSwapSubmittingMerkleRoot(false, true) {
		n.SubmitMerkleRoot(roundNum, trialNum, merkleRoot)
	}
}

// SubmitMerkleRoot submits the merkle root to the blockchain
func (n *LeaderNode) SubmitMerkleRoot(roundNum string, trialNum string, merkleRoot []byte) {
	var merkleRootBytes32 [32]byte
	copy(merkleRootBytes32[:], merkleRoot)

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
	updateCommitDataAfterSubmit(uniqueKey)
}

// updateCommitDataAfterSubmit updates commit data after successful merkle root submission
func updateCommitDataAfterSubmit(uniqueKey string) {
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
			database.UpdateLeaderCommit(&data)
		}
	}

}

// startRequestToSubmitCoMonitoring starts monitoring for automatic requestToSubmitCo
func (n *LeaderNode) startRequestToSubmitCoMonitoring(roundNum string, trialNum string, merkleRootSubmittedTime *big.Int) {
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

	// Calculate the deadline: merkleRootSubmittedTime + s_offChainSubmissionPeriod(40) + s_requestOrSubmitOrFailDecisionPeriod(30)
	s_offChainSubmissionPeriod := big.NewInt(40)
	s_requestOrSubmitOrFailDecisionPeriod := big.NewInt(30)

	deadline := new(big.Int).Add(merkleRootSubmittedTime, s_offChainSubmissionPeriod)
	deadline.Add(deadline, s_requestOrSubmitOrFailDecisionPeriod)

	currentTime := big.NewInt(time.Now().Unix())

	// Determine chainID from client and compute seconds buffer from configured block time
	chainID, err := n.fallbackEthClient.NetworkID(context.Background())
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
		n.callRequestToSubmitCoIfNeeded(roundNum, trialNum, uniqueKey)
		return
	}

	waitSeconds := waitDuration.Int64()
	log.Printf("Waiting %d seconds before checking COS for round %s with trail %s", waitSeconds, roundNum, trialNum)

	// Use time.AfterFunc for the timer
	n.requestToSubmitCoTimerMonitoringTimer = time.AfterFunc(time.Duration(waitSeconds)*time.Second, func() {
		if n.GetRequestToSubmitCoTimerMonitoringActive() {
			n.callRequestToSubmitCoIfNeeded(roundNum, trialNum, uniqueKey)
		}
		n.SetRequestToSubmitCoTimerMonitoringActive(false)
	})
}

// callRequestToSubmitCoIfNeeded checks if COS are missing and calls requestToSubmitCo
func (n *LeaderNode) callRequestToSubmitCoIfNeeded(roundNum string, trialNum string, uniqueKey string) {
	if n.GetHalted() {
		log.Println("System is halted. Skipping callRequestToSubmitCoIfNeeded.")
		return
	}

	n.commitMu.Lock()
	defer n.commitMu.Unlock()

	ops := eth.GetActivatedOperatorsCached()
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
		n.requestToSubmitCo(roundNum, trialNum, missingIndices)
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
	n.DeleteActiveBroadcasts(uniqueKey)
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
func (n *LeaderNode) requestToSubmitCo(roundNum string, trialNum string, missingIndices []*big.Int) {
	cvNotOnChainCvAndSigRS, packedVs, indicesLength, packedOrederedIndices := n.prepareArgumentsForRequestToSubmitCo(roundNum, trialNum, missingIndices)

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
func (n *LeaderNode) prepareArgumentsForRequestToSubmitCo(roundNum string, trialNum string, missingIndices []*big.Int) ([]CvAndSigRS, *big.Int, *big.Int, *big.Int) {
	cvs, _, _, vs, rs, ss := n.LoadNodeData(roundNum, trialNum)
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
