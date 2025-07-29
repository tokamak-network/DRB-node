package leaderNode_helper

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
var StartTime *big.Int
var Execution bool

// var ActivatedOperator []string

var Halted int32 // 0 = false, 1 = true

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

var RoundsData map[string]RoundData

// In case last SSubmitted event also get's emmitted with Status event and curState is IN_PROGRESS then CurrentRound vairable will not be consistent
var SecretRequestSentForWhichRound string
var CurrentRound string
var Req RandomRequest

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

		reconnect := false
		for {
			select {
			case err := <-sub.Err():
				log.Printf("Error in event subscription: %v", err)

				if err != nil && (strings.Contains(err.Error(), "websocket: close 1006") || strings.Contains(err.Error(), "unexpected EOF")) {
					log.Printf("Websocket closed abnormally. Attempting to reconnect in 1 seconds...")
					reconnect = true
					time.Sleep(1 * time.Second)
				} else {
					log.Printf("Fatal error in event subscription: %v", err)
					reconnect = true
					time.Sleep(1 * time.Second)
				}
				
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
					processSubmittedSecretRequest(eventData.Round, eventData.TrialNum, eventData.S, eventData.Index)
				}
			}
			if reconnect {
				break // break inner for loop to reconnect
			}
		}
	}
}

func processSubmittedSecretRequest(round *big.Int, trialNum *big.Int, secret [32]byte, index *big.Int) {
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processSubmittedSecretRequest.")
		return
	}
	fmt.Printf("Round %v, TrialNum %v, index %v\n", round, trialNum, index)
	intValue := int(index.Int64())
	regularNodeAddress := eth.ActivatedOperators[intValue]
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())

	leaderCommits, err := database.GetLeaderCommitByRoundAndEoaAddr(SecretRequestSentForWhichRound, regularNodeAddress.Hex(), uniqueKey, regularNodeAddress.Hex())
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
	ReliableBroadCastS(libp2putils.HostInstance, SecretRequestSentForWhichRound, trialNum.String(), regularNodeAddress.Hex(), secret)
}

func processRandomRequestNumber(fallbackEthClient *fallback_ethclient.FallbackRPCClient, blockTimestamp *big.Int, round *big.Int, trialNum *big.Int, state *big.Int) {
	fmt.Printf("Round %v, TrialNum %v, state %v\n", round, trialNum, state)
	CurrentRound = round.String()
	uniqueKey := utils.GetUniqueKey(round.String(), trialNum.String())
	req := RandomRequest{
		Round:     round,
		TrialNum:  trialNum,
		StartTime: blockTimestamp,
		State:     state,
	}
	if state.Cmp(big.NewInt(1)) == 0 {
		// Set Halted to 0 to resume the round
		atomic.StoreInt32(&Halted, 0)

		fmt.Printf("Status Event:\n StartTime: %v\n State: %v\n Round: %v\n",
			blockTimestamp, state, round)
		Req = req
		// Update the activated operators
		eth.UpdateActivatedOperators(fallbackEthClient)
		// Reset the indices for the new round
		ResetIndicesForNewRound()
		log.Printf("Reset Indices array for new round %s with trail %s", CurrentRound, trialNum.String())
		Execution = true
	}
	if state.Cmp(big.NewInt(2)) == 0 {
		if RoundsData == nil {
			RoundsData = make(map[string]RoundData)
		}
		data := RoundsData[uniqueKey]
		data.RandomNumber = true
		RoundsData[uniqueKey] = data
		Execution = false
	}

	if state.Cmp(big.NewInt(3)) == 0 {
		// Set Execution to false to stop the round
		Execution = false
		// Set Halted to 1 to halt the round
		atomic.StoreInt32(&Halted, 1)
		// Delete round and trial data from database
		database.DeleteRoundTrialDataForLeaderNode(CurrentRound, trialNum.String())
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
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processCOS.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, activatedOperatorIndex %v\n", round, trialNum, activatedOperatorIndex)

	eoa := eth.ActivatedOperators[activatedOperatorIndex.Int64()]
	cosHex := hex.EncodeToString(cos[:])
	roundStr := round.String()
	trialNumStr := trialNum.String()
	uniqueKey := utils.GetUniqueKey(roundStr, trialNumStr)

	leaderCommitData, err := database.GetLeaderCommitByRoundAndEoaAddr(roundStr, trialNumStr, uniqueKey, eoa.Hex())
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

	return nil
}

func updateCOS(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, trialNum string, uniqueKey string, eoa common.Address, cos [32]byte) {
	if _, exists := utils.CommittedNodes[uniqueKey]; !exists {
		utils.CommittedNodes[uniqueKey] = make(map[common.Address]utils.LeaderCommitData)
	}

	commitData, exists := utils.CommittedNodes[uniqueKey][eoa]
	if !exists {
		commitData = utils.LeaderCommitData{}
	}

	commitData.EOAAddress = eoa.Hex()
	commitData.Round = round
	commitData.Cos = cos
	cosHex := hex.EncodeToString(cos[:])
	commitData.CosHex = cosHex
	utils.CommittedNodes[uniqueKey][eoa] = commitData

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
	if atomic.LoadInt32(&Halted) == 1 {
		log.Println("System is halted. Skipping processCVS.")
		return nil
	}
	fmt.Printf("Round %v, TrialNum %v, activatedOperatorIndex %v\n", round, trialNum, activatedOperatorIndex)
	roundStr := round.String()
	trialNumStr := trialNum.String()
	eoa := eth.ActivatedOperators[activatedOperatorIndex.Int64()]
	cvsHex := hex.EncodeToString(cvs[:])

	uniqueKey := utils.GetUniqueKey(roundStr, trialNumStr)
	leaderCommitData, err := database.GetLeaderCommitByRoundAndEoaAddr(roundStr, trialNumStr, uniqueKey, eoa.Hex())
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

	// Broadcast the CVS value to all activated regular nodes
	ReliableBroadCastCVS(libp2putils.HostInstance, roundStr, trialNumStr, eoa, cvs)

	return nil
}

func updateCVS(round string, uniqueKey string, eoa common.Address, cvs [32]byte) {
	if _, exists := utils.CommittedNodes[uniqueKey]; !exists {
		utils.CommittedNodes[uniqueKey] = make(map[common.Address]utils.LeaderCommitData)
	}

	commitData, exists := utils.CommittedNodes[uniqueKey][eoa]
	if !exists {
		commitData = utils.LeaderCommitData{}
	}

	commitData.EOAAddress = eoa.Hex()
	commitData.Round = round
	commitData.Cvs = cvs
	cvsHex := hex.EncodeToString(cvs[:])
	commitData.CvsHex = cvsHex
	utils.CommittedNodes[uniqueKey][eoa] = commitData

}

func AllCosReceivedUnlocked(uniqueKey string) bool {
	ops := eth.ActivatedOperators
	if len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := utils.CommittedNodes[uniqueKey]
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
