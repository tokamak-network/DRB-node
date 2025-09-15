package leaderNode_helper

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

var CommitMu sync.Mutex

// Atomic variables for thread safety
var Execution int32 // 0 = false, 1 = true
var Halted int32    // 0 = false, 1 = true

// Monitoring active flags - using atomic for thread safety
var RequestedToSubmitCoMonitoringActive int32 // 0 = false, 1 = true
var RequestedToSubmitCvMonitoringActive int32 // 0 = false, 1 = true
var RequestToSubmitCvMonitoringActive int32   // 0 = false, 1 = true

// Timer variables (already safe as they're pointers)
var RequestedToSubmitCoMonitoringTimer *time.Timer
var RequestedToSubmitCvMonitoringTimer *time.Timer
var RequestToSubmitCvMonitoringTimer *time.Timer

// In case last SSubmitted event also get's emmitted with Status event and curState is IN_PROGRESS then CurrentRound vairable will not be consistent
// Note: SecretRequestSentForWhichRound, CurrentRound, CurrentTrial, and Req are now handled with atomic operations
// String variables - using atomic with unsafe.Pointer
var SecretRequestSentForWhichRound unsafe.Pointer // *string
var CurrentRound unsafe.Pointer                   // *string
var CurrentTrial unsafe.Pointer                   // *string

// Map variables with mutex protection
var RoundsData map[string]RoundData
var RoundsDataMu sync.RWMutex

// Req is a struct so it needs mutex protection
var Req RandomRequest
var ReqMu sync.RWMutex

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

func ReceiveCommit(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	receiveCommit(fallbackEthClient)
}

func receiveCommit(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
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
		sub, err := fallbackEthClient.SubscribeFilterLogs(context.Background(), query, logs)
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

					processCVS(eventData.Round, eventData.TrialNum, eventData.Cv, eventData.Index)

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

					processCOS(fallbackEthClient, eventData.Round, eventData.TrialNum, eventData.Co, eventData.Index)

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

					blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
						continue
					}

					processRequestedToSubmitCo(fallbackEthClient, big.NewInt(int64(blockTimestamp)), eventData.Round, eventData.TrialNum)

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

					blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
						continue
					}

					fmt.Printf("\033[34mRequestedToSubmitCv Event: Round %v, TrialNum %v, BlockTimestamp %v\033[0m\n", eventData.Round, eventData.TrialNum, blockTimestamp)
					processRequestedToSubmitCv(fallbackEthClient, big.NewInt(int64(blockTimestamp)), eventData.Round, eventData.TrialNum)

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

					blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
						continue
					}

					processRandomRequestNumber(fallbackEthClient, big.NewInt(int64(blockTimestamp)), eventData.CurRound, eventData.CurTrialNum, eventData.CurState)

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
					blockTimestamp, err := fallbackEthClient.BlockTimestamp(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
					if err != nil {
						log.Printf("Failed to get block timestamp for block %d: %v", vLog.BlockNumber, err)
					} else {
						UpdateLastSubmitSTimestamp(big.NewInt(int64(blockTimestamp)), eventData.Round.String(), eventData.TrialNum.String())
					}

					processSubmittedSecretRequest(eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
				}
			}
			if reconnect {
				log.Printf("Reconnection triggered, breaking out of event loop to restart subscription...")
				break // break inner for loop to reconnect
			}
		}
	}
}

func processSubmittedSecretRequest(round *big.Int, trialNum *big.Int, secret [32]byte, index *big.Int) {
	if GetHalted() {
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

	leaderCommits, err := database.GetLeaderCommitByRoundAndEoaAddr(GetSecretRequestSentForWhichRound(), regularNodeAddress.Hex(), regularNodeAddress.Hex())
	if err != nil {
		log.Printf("Failed to get leadercommit data from database by round and eoaAddress %v", err)
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
	ReliableBroadCastSSync(libp2putils.HostInstance, GetSecretRequestSentForWhichRound(), trialNum.String(), regularNodeAddress.Hex(), secret)
}

func processRandomRequestNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, state %v\n", round, trialNum, state)
	SetCurrentRound(round.String())
	SetCurrentTrial(trialNum.String())
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	// Reset leader monitoring state for new round or trail
	ResetLeaderMonitoringState(round.String(), trialNum.String())
	SetReq(RandomRequest{
		Round:     round,
		TrialNum:  trialNum,
		StartTime: blockTimestamp,
		State:     state,
	})
	if state.Cmp(big.NewInt(1)) == 0 {
		// Set Halted to 0 to resume the round
		atomic.StoreInt32(&Halted, 0)
		// Delete round and trial data from database
		err := database.DeleteOldRoundDataForLeaderNode(round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}

		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		// Update the activated operators
		eth.UpdateActivatedOperators(fallbackEthClient)
		// Reset the indices for the new round
		ResetIndicesForNewRound()
		log.Printf("Reset Indices array for new round %s with trail %s", GetCurrentRound(), GetCurrentTrial())

		// Start monitoring for automatic requestToSubmitCv
		startRequestToSubmitCvMonitoring(fallbackEthClient, round.String(), trialNum.String(), blockTimestamp)

		SetExecution(true)
	}
	if state.Cmp(big.NewInt(2)) == 0 {
		data, exists := GetRoundData(uniqueKey)
		if !exists {
			data = RoundData{}
		}
		data.RandomNumber = true
		SetRoundData(uniqueKey, data)
		SetExecution(false)

		// Delete round and trial data from database
		err := database.DeleteOldRoundDataForLeaderNode(round.String())
		if err != nil {
			log.Printf("Failed to delete old round data except round %v for regular node\n", round)
		}
	}

	if state.Cmp(big.NewInt(3)) == 0 {
		// Set Execution to false to stop the round
		SetExecution(false)
		// Set Halted to 1 to halt the round
		SetHalted(true)
		// Delete round and trial data from database
		database.DeleteRoundTrialDataForLeaderNode(GetCurrentRound(), GetCurrentTrial())
		// resume the round
		resuming(fallbackEthClient)
	}
}

func resuming(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
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
	depositResult, err := eth.CallSmartContract(fallbackEthClient, parsedABI, "s_depositAmount", contractAddress, leaderEOA)
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
			fallbackEthClient,
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
		opsLenResult, err := eth.CallSmartContract(fallbackEthClient, parsedABI, "getActivatedOperatorsLength", contractAddress)
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
				fallbackEthClient,
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

func processCOS(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round *big.Int, trialNum *big.Int, cos [32]byte, activatedOperatorIndex *big.Int) error {
	if GetHalted() {
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

	updateCOS(fallbackEthClient, roundStr, trialNumStr, uniqueKey, eoa, cos)
	fmt.Printf("Successfully stored COS for Round %s with Trail %s, EOA %s\n", roundStr, trialNumStr, eoa.Hex())

	// Broadcast the COS value to all activated regular nodes
	ReliableBroadCastCOS(libp2putils.HostInstance, roundStr, trialNumStr, eoa, cos)

	// Check if all COS values are received and stop monitoring if so
	checkAndStopFailToSubmitCoMonitoring(roundStr, trialNumStr)

	return nil
}

func updateCOS(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, uniqueKey string, eoa common.Address, cos [32]byte) {
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

	if AllCosReceivedUnlocked(uniqueKey) {
		log.Printf("All COS received for round %s with trail %s.", round, trialNum)
		_, err := commitreveal2.DetermineRevealOrder(round, trialNum, eth.ActivatedOperators)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s with trail %s: %v", round, trialNum, err)
			return
		}
		StartSecretValueRequests(libp2putils.HostInstance, fallbackEthClient, round, trialNum)
	}
}

func processCVS(round *big.Int, trialNum *big.Int, cvs [32]byte, activatedOperatorIndex *big.Int) error {
	if GetHalted() {
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

	updateCVS(roundStr, uniqueKey, eoa, cvs)
	fmt.Printf("Successfully stored CVS for Round %s with Trail %s, EOA %s\n", roundStr, trialNumStr, eoa.Hex())

	// Check if all CVS values are received and stop monitoring if needed
	checkAndStopFailToSubmitCvMonitoring(roundStr, trialNumStr)

	// Broadcast the CVS value to all activated regular nodes
	ReliableBroadCastCVS(libp2putils.HostInstance, roundStr, trialNumStr, eoa, cvs)

	return nil
}

func updateCVS(round string, uniqueKey string, eoa common.Address, cvs [32]byte) {
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

func AllCosReceivedUnlocked(uniqueKey string) bool {
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

func AllCvsReceivedUnlocked(uniqueKey string) bool {
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
func processRequestedToSubmitCo(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int) {
	if GetHalted() {
		log.Println("System is halted. Skipping processRequestedToSubmitCo.")
		return
	}

	fmt.Printf("RequestedToSubmitCo Event: Round %v, TrialNum %v, BlockTimestamp %v\n", round, trialNum, blockTimestamp)

	// Start monitoring for failToSubmitCo condition
	startFailToSubmitCoMonitoring(fallbackEthClient, round.String(), trialNum.String(), blockTimestamp)
}

// Add new function to process RequestedToSubmitCv event
func processRequestedToSubmitCv(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int) {
	if GetHalted() {
		log.Println("System is halted. Skipping processRequestedToSubmitCv.")
		return
	}

	// Stop the requestToSubmitCv monitoring since the request has been made
	if GetRequestToSubmitCvMonitoringActive() {
		log.Printf("RequestedToSubmitCv event received, stopping requestToSubmitCv monitoring for round %s", round.String())
		stopRequestToSubmitCvMonitoring()
	}

	// Start monitoring for failToSubmitCv condition
	startFailToSubmitCvMonitoring(fallbackEthClient, round.String(), trialNum.String(), blockTimestamp)
}

// Add function to start monitoring for failToSubmitCo condition
func startFailToSubmitCoMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, requestedToSubmitCoTimestamp *big.Int) {
	if requestedToSubmitCoTimestamp == nil {
		log.Printf("requestedToSubmitCoTimestamp is nil, cannot start monitoring")
		return
	}

	SetRequestedToSubmitCoMonitoringActive(true)

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
		callFailToSubmitCo(fallbackEthClient, round, trialNum)
		return
	}

	log.Printf("Starting failToSubmitCo monitoring for round %s, deadline: %v (in %v)", round, deadlineTime, duration)

	// Set timer to call the function when deadline is reached
	RequestedToSubmitCoMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("Deadline reached for round %s, calling failToSubmitCo", round)
		callFailToSubmitCo(fallbackEthClient, round, trialNum)
		SetRequestedToSubmitCoMonitoringActive(false)
	})
}

// Add function to stop failToSubmitCo monitoring
func stopFailToSubmitCoMonitoring() {
	if RequestedToSubmitCoMonitoringTimer != nil {
		RequestedToSubmitCoMonitoringTimer.Stop()
		RequestedToSubmitCoMonitoringTimer = nil
	}
	SetRequestedToSubmitCoMonitoringActive(false)
	log.Printf("Stopped failToSubmitCo monitoring")
}

// Add function to call failToSubmitCo on chain
func callFailToSubmitCo(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
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
func checkAndStopFailToSubmitCoMonitoring(round string, trialNum string) {
	// Only check if monitoring is active for this round/trial
	if !GetRequestedToSubmitCoMonitoringActive() {
		return
	}

	// Check if all activated operators have submitted COS values
	if AllCosReceivedUnlocked(utils.GetUniqueKey(round, trialNum)) {
		log.Printf("All COS values received for round %s trial %s, stopping failToSubmitCo monitoring", round, trialNum)
		stopFailToSubmitCoMonitoring()
	}
}

// Add function to check if all CVS values are received and stop monitoring
func checkAndStopFailToSubmitCvMonitoring(round string, trialNum string) {
	// Only check if monitoring is active for this round/trial
	if !GetRequestedToSubmitCvMonitoringActive() {
		return
	}

	// Check if all activated operators have submitted CVS values
	if AllCvsReceivedUnlocked(utils.GetUniqueKey(round, trialNum)) {
		log.Printf("All CVS values received for round %s trial %s, stopping failToSubmitCv monitoring", round, trialNum)
		stopFailToSubmitCvMonitoring()
	}
}

// Add function to start monitoring for failToSubmitCv condition
func startFailToSubmitCvMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, requestedToSubmitCvTimestamp *big.Int) {
	if requestedToSubmitCvTimestamp == nil {
		log.Printf("requestedToSubmitCvTimestamp is nil, cannot start monitoring")
		return
	}

	SetRequestedToSubmitCvMonitoringActive(true)

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
	RequestedToSubmitCvMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("⚠️ Deadline reached for round %s, calling failToSubmitCv", round)
		callFailToSubmitCv(fallbackEthClient, round, trialNum)
		SetRequestedToSubmitCvMonitoringActive(false)
	})
}

// Add function to stop failToSubmitCv monitoring
func stopFailToSubmitCvMonitoring() {
	if RequestedToSubmitCvMonitoringTimer != nil {
		RequestedToSubmitCvMonitoringTimer.Stop()
		RequestedToSubmitCvMonitoringTimer = nil
	}
	SetRequestedToSubmitCvMonitoringActive(false)
	log.Printf("Stopped failToSubmitCv monitoring")
}

// Add function to call failToSubmitCv on chain
func callFailToSubmitCv(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
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
func ResetCosAndCvsMonitoringState(round string, trialNum string) {
	// Stop COS monitoring
	stopFailToSubmitCoMonitoring()

	// Stop CVS monitoring (failToSubmitCv)
	stopFailToSubmitCvMonitoring()

	// Stop requestToSubmitCv monitoring
	stopRequestToSubmitCvMonitoring()

	// Reset COS monitoring variables
	SetRequestedToSubmitCoMonitoringActive(false)
	if RequestedToSubmitCoMonitoringTimer != nil {
		RequestedToSubmitCoMonitoringTimer.Stop()
		RequestedToSubmitCoMonitoringTimer = nil
	}

	// Reset CVS monitoring variables (failToSubmitCv)
	SetRequestedToSubmitCvMonitoringActive(false)
	if RequestedToSubmitCvMonitoringTimer != nil {
		RequestedToSubmitCvMonitoringTimer.Stop()
		RequestedToSubmitCvMonitoringTimer = nil
	}

	// Reset requestToSubmitCv monitoring variables
	SetRequestToSubmitCvMonitoringActive(false)
	if RequestToSubmitCvMonitoringTimer != nil {
		RequestToSubmitCvMonitoringTimer.Stop()
		RequestToSubmitCvMonitoringTimer = nil
	}

	// Reset secret request tracking variable
	SetSecretRequestSentForWhichRound("")

	log.Printf("Reset COS and CVS monitoring state for round %s", round)
}

func CheckHaltedState(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
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
	result, err := eth.CallSmartContract(fallbackEthClient, parsedABI, "s_isInProcess", contractAddress)
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
		atomic.StoreInt32(&Halted, 1)
		resuming(fallbackEthClient)
	} else {
		log.Printf("s_isInProcess is %v, no action needed", isInProcess)
	}
}

// Add function to start monitoring for automatic requestToSubmitCv
func startRequestToSubmitCvMonitoring(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, startTime *big.Int) {
	if startTime == nil {
		log.Printf("StartTime is nil, cannot start requestToSubmitCv monitoring")
		return
	}

	SetRequestToSubmitCvMonitoringActive(true)

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
	RequestToSubmitCvMonitoringTimer = time.AfterFunc(duration, func() {
		log.Printf("⚠️ Deadline reached for round %s, calling requestToSubmitCv", round)
		callRequestToSubmitCv(fallbackEthClient, round, trialNum)
		SetRequestToSubmitCvMonitoringActive(false)
	})
}

// Add function to stop requestToSubmitCv monitoring
func stopRequestToSubmitCvMonitoring() {
	if RequestToSubmitCvMonitoringTimer != nil {
		RequestToSubmitCvMonitoringTimer.Stop()
		RequestToSubmitCvMonitoringTimer = nil
	}
	SetRequestToSubmitCvMonitoringActive(false)
	log.Printf("Stopped requestToSubmitCv monitoring")
}

// Add function to call requestToSubmitCv when regular nodes haven't submitted CVS
func callRequestToSubmitCv(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string) {
	if GetHalted() {
		log.Println("System is halted. Skipping callRequestToSubmitCv.")
		return
	}

	log.Printf("Calling requestToSubmitCv for round %s with trial %s due to missing CVS submissions", round, trialNum)

	// Get the list of missing operators (those who haven't submitted CVS)
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	missingOperators := getMissingCvsOperators(uniqueKey)

	if len(missingOperators) == 0 {
		log.Printf("All CVS received for round %s, no need to call requestToSubmitCv", round)
		return
	}

	log.Printf("Missing CVS from operators: %v", missingOperators)

	// Implement the same logic as handleMissingCV from leaderNode.go
	SetCvOnChain(uniqueKey, true)
	activatedOperators := eth.GetActivatedOperatorsCached()
	i := big.NewInt(0)
	for _, op := range activatedOperators {
		for _, missingOp := range missingOperators {
			if op.Hex() == missingOp {
				AppendToIndices(i)
			}
		}
		i.Add(i, big.NewInt(1))
	}

	// Get the current indices and sort them
	indices := GetIndices()
	sort.Slice(indices, func(i, j int) bool {
		return indices[i].Cmp(indices[j]) < 0
	})

	// Update the sorted indices back
	SetIndices(indices)

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
		fallbackEthClient,
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
func getMissingCvsOperators(uniqueKey string) []string {
	var missingOperators []string
	ops := eth.ActivatedOperators

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
