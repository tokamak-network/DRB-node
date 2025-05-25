package nodes

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/nodes/leaderNode_helper"
	"github.com/tokamak-network/DRB-node/utils"
)

var submittingMerkleRoot = false
var commitMu sync.Mutex
var firstRequest leaderNode_helper.RandomRequest

// committedNodes and activatedOperators are authoritative in-memory states.
// var committedNodes = make(map[string]map[common.Address]utils.LeaderCommitData)
// var activatedOperators = make(map[string]map[common.Address]bool)

var roundFlag = make(map[string]uint64)
var sendCommitRequest = make(map[string]bool)
var onChainExecution = make(map[string]map[string]map[string]int)
var flag = make(map[string]bool)
var dispute = make(map[string]bool)
var requestCv = true

func RunLeaderNode() {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT is not set in environment variables.")
	}

	h, peerID, err := libp2putils.CreateHost(port)
	if err != nil {
		log.Fatalf("Error creating host: %v", err)
	}
	defer h.Close()

	h.SetStreamHandler("/register", handleRegistrationRequest)
	h.SetStreamHandler("/cvs", handleCommitRequest)
	h.SetStreamHandler("/cos", func(s network.Stream) {
		handleCOSRequest(h, s)
	})
	h.SetStreamHandler("/secretValue", func(s network.Stream) {
		leaderNode_helper.AcceptSecretValue(h, s)
	})

	log.Printf("Leader node running on: %s", h.Addrs())
	log.Printf("Leader node PeerID: %s", peerID.String())


	go leaderNode_helper.MonitorCommits()
	go leaderNode_helper.ReceiveCommit()
	for {
		if !leaderNode_helper.Execution {
			time.Sleep(10 * time.Second)
			continue
		}
		firstRequest = leaderNode_helper.Req
		log.Printf("Executing request: %v", firstRequest)
		processRounds(firstRequest)
		time.Sleep(30 * time.Second)
	}
}

func handleRegistrationRequest(s network.Stream) {
	defer s.Close()
	filePath := "registered_nodes.json"
	if err := leaderNode_helper.RegisterNode(s, filePath, "contract/abi/Commit2RevealDRB.json"); err != nil {
		log.Printf("Failed to handle registration request: %v", err)
		return
	}
	log.Println("\033[32mNode registration completed.\033[0m")
}

func handleCommitRequest(s network.Stream) {
	defer s.Close()

	var req utils.CommitRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode commit request: %v", err)
		return
	}

	commitVerificationRequest := utils.Request{Round: req.Round, EOAAddress: req.EOAAddress, Signature: req.Signature}

	if !VerifySignatureAndCheckActivation(commitVerificationRequest, "commit") {
		return
	}

	roundNum := req.Round
	eoaAddress := common.HexToAddress(req.EOAAddress)

	commitMu.Lock()
	defer commitMu.Unlock()

	commitData := getOrCreateLeaderCommitData(roundNum, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		commitData.Cvs = req.Cvs
		commitData.CvsHex = hex.EncodeToString(req.Cvs[:])
		commitData.Sign = req.Sign
		log.Printf("Storing CVS and signature for round %s EOA %s", roundNum, eoaAddress.Hex())
	}

	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		log.Printf("Error saving commit data for round %s EOA %s: %v", roundNum, eoaAddress.Hex(), err)
		return
	}
	updateInMemoryData(roundNum, eoaAddress, *commitData)

	// Check if all commits are ready after this update
	if !isMerkleRootSubmitted(roundNum) && allCommitsReceivedUnlocked(roundNum) {
		log.Printf("\033[32mAll CVS received for round %s.\033[0m", roundNum)
		commitMu.Unlock() // Unlock before calling generateMerkleRoot
		generateMerkleRoot(roundNum)
		commitMu.Lock() // Re-lock if needed
	}
}

func handleCOSRequest(h host.Host, s network.Stream) {
	defer s.Close()

	var req utils.CosRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode COS request: %v", err)
		return
	}

	cosVerificationRequest := utils.Request{Round: req.Round, EOAAddress: req.EOAAddress, Signature: req.Signature}

	if !VerifySignatureAndCheckActivation(cosVerificationRequest, "COS") {
		return
	}

	roundNum := req.Round
	eoaAddress := common.HexToAddress(req.EOAAddress)

	commitMu.Lock()
	defer commitMu.Unlock()

	commitData := getOrCreateLeaderCommitData(roundNum, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		log.Printf("No CVS found for round %s EOA %s, rejecting COS.", roundNum, eoaAddress.Hex())
		return
	}

	recalculatedCvs := commitreveal2.Keccak256(req.Cos[:])
	if !bytes.Equal(recalculatedCvs, commitData.Cvs[:]) {
		log.Printf("COS hash mismatch for round %s EOA %s. Rejecting COS.", roundNum, eoaAddress.Hex())
		return
	}

	if commitData.Cos != [32]byte{} {
		log.Printf("COS already received for round %s EOA %s. Skipping.", roundNum, eoaAddress.Hex())
		return
	}

	commitData.Cos = req.Cos
	commitData.CosHex = hex.EncodeToString(req.Cos[:])

	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		log.Printf("Error saving COS data for round %s EOA %s: %v", roundNum, eoaAddress.Hex(), err)
		return
	}
	updateInMemoryData(roundNum, eoaAddress, *commitData)
	log.Printf("Received and Stored COS for round %s EOA %s", roundNum, eoaAddress.Hex())

	// Check if all commits are ready after this COS
	if !isMerkleRootSubmitted(roundNum) && allCommitsReceivedUnlocked(roundNum) {
		log.Printf("All CVS received for round %s after COS, generating Merkle root...", roundNum)
		commitMu.Unlock()
		generateMerkleRoot(roundNum)
		commitMu.Lock()
	}

	// Also, if all COS are received (if that matters), we determine reveal order as existing code:
	if allCosReceivedUnlocked(roundNum) {
		log.Printf("\033[32mAll COS received for round %s.\033[0m", roundNum)
		err := commitreveal2.DetermineRevealOrder(roundNum, eth.ActivatedOperators)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s: %v", roundNum, err)
			return
		}
		leaderNode_helper.AllCosReceived = true
		leaderNode_helper.StartSecretValueRequests(h, roundNum)
	}
}

func isMerkleRootSubmitted(roundNum string) bool {
	// Call with commitMu locked or ensure commitMu is locked outside
	roundMap, exists := utils.CommittedNodes[roundNum]
	if !exists || len(roundMap) == 0 {
		return false
	}

	// Check any operator to see if SubmitMerkleRootDone is set
	for _, data := range roundMap {
		if data.SubmitMerkleRootDone {
			return true
		}
	}
	return false
}

func VerifySignatureAndCheckActivation(req utils.Request, reqType string) bool {
	verifyReq := utils.RegistrationRequest{EOAAddress: req.EOAAddress, Signature: req.Signature}
	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for round %s EOA %s", req.Round, req.EOAAddress)
		return false
	}

	eoaAddress := common.HexToAddress(req.EOAAddress)

	if !isEOAActivatedForRound(eoaAddress) {
		log.Printf("EOA %s not activated, skipping %v.", eoaAddress.Hex(), reqType)
		return false
	}
	return true
}

// allCommitsReceivedUnlocked checks if all operators have CVS in-memory.
// Called with commitMu locked.
func allCommitsReceivedUnlocked(roundNum string) bool {
	ops := eth.ActivatedOperators
	if len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := utils.CommittedNodes[roundNum]
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
func allCosReceivedUnlocked(roundNum string) bool {
	ops := eth.ActivatedOperators
	if len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := utils.CommittedNodes[roundNum]
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

func UpdatedallCommitsReceivedUnlocked(roundNum string) map[string]bool {
	result := make(map[string]bool)
	ops, _ := eth.GetActivatedOperators()

	roundCommits, roundExists := utils.CommittedNodes[roundNum]
	if !roundExists || len(roundCommits) == 0 {
		for _, op := range ops {
			result[op.Hex()] = false
		}
	}

	for _, op := range ops {
		data, ok := roundCommits[op]
		if ok {
			if data.Cvs != [32]byte{} {
				result[op.Hex()] = true
			} else {
				result[op.Hex()] = false
			}
		} else {
			result[op.Hex()] = false
		}
	}
	return result
}

// getOrCreateLeaderCommitData returns commitData from in-memory map or creates a new one.
// Called with commitMu locked.
func getOrCreateLeaderCommitData(roundNum string, eoaAddress common.Address) *utils.LeaderCommitData {
	roundMap, exists := utils.CommittedNodes[roundNum]
	if !exists {
		roundMap = make(map[common.Address]utils.LeaderCommitData)
		utils.CommittedNodes[roundNum] = roundMap
	}

	data, existsData := roundMap[eoaAddress]
	if !existsData {
		data = utils.LeaderCommitData{Round: roundNum, EOAAddress: eoaAddress.Hex()}
		roundMap[eoaAddress] = data
	}
	return &data
}

// updateInMemoryData updates committedNodes with the latest commitData.
// Called with commitMu locked.
func updateInMemoryData(roundNum string, eoaAddress common.Address, commitData utils.LeaderCommitData) {
	roundMap, exists := utils.CommittedNodes[roundNum]
	if !exists {
		roundMap = make(map[common.Address]utils.LeaderCommitData)
		utils.CommittedNodes[roundNum] = roundMap
	}
	roundMap[eoaAddress] = commitData
}

// generateMerkleRoot doesn't lock; it locks inside to read from memory
func generateMerkleRoot(roundNum string) {
	commitMu.Lock()
	// Check if merkle root is already done before proceeding
	if isMerkleRootSubmitted(roundNum) {
		log.Printf("Merkle root already submitted for round %s, skipping.", roundNum)
		commitMu.Unlock()
		return
	}
	commitMu.Unlock()

	log.Printf("Generating Merkle root for round %s...", roundNum)
	activatedOperatorsList := leaderNode_helper.ActivatedOperator


	commitMu.Lock()
	roundMap, roundExists := utils.CommittedNodes[roundNum]
	if !roundExists || len(roundMap) == 0 {
		log.Printf("No commits found in-memory for round %s, cannot generate Merkle root.", roundNum)
		commitMu.Unlock()
		return
	}

	var leaves [][]byte
	for _, op := range activatedOperatorsList {
		opAddr := common.HexToAddress(op)
		data, ok := roundMap[opAddr]
		if !ok || data.Cvs == [32]byte{} {
			log.Printf("Missing CVS for operator %s in round %s", opAddr.Hex(), roundNum)
			commitMu.Unlock()
			return
		}
		leaves = append(leaves, data.Cvs[:])
	}

	commitMu.Unlock()

	if len(leaves) == 0 {
		log.Printf("Error: No CVS commits found for round %s. Cannot generate Merkle root.", roundNum)
		return
	}

	merkleRoot, err := commitreveal2.CreateMerkleTree(leaves)
	if err != nil {
		log.Printf("Failed to create Merkle tree for round %s: %v", roundNum, err)
		return
	}
	if !submittingMerkleRoot {
		submittingMerkleRoot = true
		submitMerkleRoot(roundNum, merkleRoot)
	}
}

func submitMerkleRoot(roundNum string, merkleRoot []byte) {
	var merkleRootBytes32 [32]byte
	copy(merkleRootBytes32[:], merkleRoot)

	ethRPCURL := os.Getenv("ETH_RPC_URL")
	if ethRPCURL == "" {
		log.Fatal("ETH_RPC_URL is not set in environment variables.")
	}

	client, err := ethclient.Dial(ethRPCURL)
	if err != nil {
		log.Printf("Failed to connect to Ethereum client: %v", err)
		return
	}

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
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		"submitMerkleRoot",
		big.NewInt(0),
		merkleRootBytes32,
	)
	if err != nil {
		log.Printf("Failed to submit Merkle root for round %s: %v", roundNum, err)
		return
	}

	submittingMerkleRoot = false
	roundData := leaderNode_helper.RoundsData[roundNum]
	roundData.MerkleRoot = true
	if leaderNode_helper.RoundsData == nil {
		leaderNode_helper.RoundsData = make(map[string]leaderNode_helper.RoundData)
	}
	leaderNode_helper.RoundsData[roundNum] = roundData
	updateCommitDataAfterSubmit(roundNum)
}

func updateCommitDataAfterSubmit(roundNum string) {
	commitMu.Lock()
	defer commitMu.Unlock()

	roundMap, exists := utils.CommittedNodes[roundNum]
	if !exists {
		return
	}

	for eoaAddress, data := range roundMap {
		data.SubmitMerkleRootDone = true

		if err := utils.SaveLeaderCommitData(data); err != nil {
			log.Printf("Failed to save updated commit data for %s in round %s: %v", eoaAddress.Hex(), roundNum, err)
		} else {
			roundMap[eoaAddress] = data
		}
	}
}

func isEOAActivatedForRound(eoaAddress common.Address) bool {
	activatedOperators, err := eth.GetActivatedOperators()
	if err != nil {
		log.Printf("Error fetching the activated operators %v", err)
	}

	for _, operator := range activatedOperators {
		if operator == eoaAddress {
			return true
		}
	}

	log.Printf("EOA address %s is NOT activated", eoaAddress.Hex())
	return false
}

func processRounds(round leaderNode_helper.RandomRequest) {
	roundNum := round.Round.String()
	if !leaderNode_helper.RoundsData[roundNum].MerkleRoot && !leaderNode_helper.RoundsData[roundNum].RandomNumber {
		log.Printf("LeaderNode for round %s is still waiting for commits...", roundNum)

		commitMu.Lock()
		ready := UpdatedallCommitsReceivedUnlocked(roundNum)
		commitMu.Unlock()
		var missingOperators []string

		allReceived := true
		for op, submitted := range ready {
			if !submitted {
				allReceived = false
				log.Printf("⏳ \033[33mOperator %s has not submitted CV.\033[0m", op)
				if _, exists := onChainExecution[roundNum]; !exists {
					onChainExecution[roundNum] = make(map[string]map[string]int)
				}
				if _, exists := onChainExecution[roundNum]["CVS"]; !exists {
					onChainExecution[roundNum]["CVS"] = make(map[string]int)
				}
				if onChainExecution[roundNum]["CVS"][op] >= 3 {
					failToSubmitCv()
					revert()
				} else if sendCommitRequest[roundNum] {
					fmt.Println("inside sendCommitRequest[roundNum] condition")
					missingOperators = append(missingOperators, op)
					onChainExecution[roundNum]["CVS"][op]++
					flag[roundNum] = true
					dispute[roundNum] = true
				}
			}
		}
		if flag[roundNum] {
			if requestCv {
				handleMissingCV(missingOperators, roundNum)
			}
		}
		if allReceived {
			log.Printf("All CVS received for round %s. Generating Merkle root...", roundNum)
			flag[roundNum] = false
			generateMerkleRoot(roundNum)
		} else {
			log.Printf("Not all CVS received for round %s. Waiting for remaining commits.", roundNum)
			roundFlag[roundNum]++
			if roundFlag[roundNum] >= 2 {
				sendCommitRequest[roundNum] = true
			}
		}
	}
}

func failToSubmitCv() {
	ethRPCURL := os.Getenv("ETH_RPC_URL")
	if ethRPCURL == "" {
		log.Fatal("ETH_RPC_URL is not set in environment variables.")
	}

	client, err := ethclient.Dial(ethRPCURL)
	if err != nil {
		log.Printf("Failed to connect to Ethereum client: %v", err)
		return
	}

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
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		"failToSubmitCv",
		big.NewInt(0),
	)
	if err != nil {
		log.Printf("Failed to failToSubmitCv request root for round %s: %v", leaderNode_helper.CurrentRound, err)
		return
	}

	log.Printf("Successfully submitted failToSubmitCv request for round %s", leaderNode_helper.CurrentRound)
}

func handleMissingCV(missingOperators []string, roundNum string) {
	leaderNode_helper.CvOnChain = true
	activatedOperators := leaderNode_helper.ActivatedOperator
	i := big.NewInt(0)
	for _, op := range activatedOperators {
		for _, missingOp := range missingOperators {
			if op == missingOp {
				leaderNode_helper.Indices = append(leaderNode_helper.Indices, new(big.Int).Set(i))
			}
		}
		i.Add(i, big.NewInt(1))
	}
	sort.Slice(leaderNode_helper.Indices, func(i, j int) bool {
		return leaderNode_helper.Indices[i].Cmp(leaderNode_helper.Indices[j]) < 0
	})

	packedIndices := packIndices(leaderNode_helper.Indices)
	ethRPCURL := os.Getenv("ETH_RPC_URL")
	if ethRPCURL == "" {
		log.Fatal("ETH_RPC_URL is not set in environment variables.")
	}

	client, err := ethclient.Dial(ethRPCURL)
	if err != nil {
		log.Printf("Failed to connect to Ethereum client: %v", err)
		return
	}

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
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		"requestToSubmitCv",
		big.NewInt(0),
		packedIndices,
	)
	if err != nil {
		log.Printf("Failed to submit commit request root for round %s: %v", roundNum, err)
		return
	}

	log.Printf("Successfully submitted commit request for round %s and indices %v", roundNum, leaderNode_helper.Indices)
	requestCv = false
}

func packIndices(indices []*big.Int) *big.Int {
	packed := big.NewInt(0)
	for i, index := range indices {
		packed.Or(packed, new(big.Int).Lsh(index, uint(8*i)))
	}
	return packed
}

func revert() {

}
