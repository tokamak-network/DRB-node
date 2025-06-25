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
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/nodes/leaderNode_helper"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

var submittingMerkleRoot = false
var commitMu sync.Mutex
var firstRequest leaderNode_helper.RandomRequest
var cosTimerOnce = make(map[string]*sync.Once)

// committedNodes and activatedOperators are authoritative in-memory states.
// var committedNodes = make(map[string]map[common.Address]utils.LeaderCommitData)
// var activatedOperators = make(map[string]map[common.Address]bool)
type SigRS struct {
	R [32]byte
	S [32]byte
}
type CvAndSigRS struct {
	Cv [32]byte
	Rs SigRS
}

var roundFlag = make(map[string]uint64)
var sendCommitRequest = make(map[string]bool)
var onChainExecution = make(map[string]map[string]map[string]int)
var flag = make(map[string]bool)
var dispute = make(map[string]bool)
var requestCv = true

type Handler struct {
	fallbackEthClient *fallback_ethclient.FallbackRPCClient
}

func RunLeaderNode(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT is not set in environment variables.")
	}

	h, peerID, err := libp2putils.CreateHost(port)
	if err != nil {
		log.Fatalf("Error creating host: %v", err)
	}

	handler := &Handler{
		fallbackEthClient: fallbackEthClient,
	}

	defer h.Close()
	libp2putils.SetHost(h)
	h.SetStreamHandler("/register", handler.handleRegistrationRequest)
	h.SetStreamHandler("/cvs", handler.handleCommitRequest)
	h.SetStreamHandler("/cos", func(s network.Stream) {
		handleCOSRequest(fallbackEthClient, h, s)
	})
	h.SetStreamHandler("/secretValue", func(s network.Stream) {
		leaderNode_helper.AcceptSecretValue(h, s, fallbackEthClient)
	})
	h.SetStreamHandler("/acknowledgment", func(s network.Stream) {
		handleAcknowledgment(s)
	})

	log.Printf("Leader node running on: %s", h.Addrs())
	log.Printf("Leader node PeerID: %s", peerID.String())

	go leaderNode_helper.MonitorCommits(fallbackEthClient)
	go leaderNode_helper.ReceiveCommit(fallbackEthClient)
	leaderNode_helper.StartBroadcastCleanup()
	for {
		if !leaderNode_helper.Execution {
			time.Sleep(10 * time.Second)
			continue
		}
		firstRequest = leaderNode_helper.Req
		fmt.Printf("Executing request: %v", firstRequest)
		processRounds(fallbackEthClient, firstRequest)
		time.Sleep(30 * time.Second)
	}
}

func (h *Handler) handleRegistrationRequest(s network.Stream) {
	defer s.Close()
	filePath := "registered_nodes.json"
	if err := leaderNode_helper.RegisterNode(s, filePath, "contract/abi/Commit2RevealDRB.json"); err != nil {
		log.Printf("Failed to handle registration request: %v", err)
		return
	}
	log.Println("Node registration completed.")
}

func (h *Handler) handleCommitRequest(s network.Stream) {
	defer s.Close()

	fallbackEthClient := h.fallbackEthClient

	var req utils.CommitRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode commit request: %v", err)
		return
	}

	commitVerificationRequest := utils.Request{Round: req.Round, EOAAddress: req.EOAAddress, Signature: req.Signature}

	if !VerifySignatureAndCheckActivation(fallbackEthClient, commitVerificationRequest, "commit") {
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
	log.Printf("Commit data saved and updated in-memory for round %s EOA %s", roundNum, eoaAddress.Hex())
	leaderNode_helper.BroadCastCVS(libp2putils.HostInstance, roundNum, eoaAddress, commitData.Cvs)
	// Check if all commits are ready after this update
	if !isMerkleRootSubmitted(roundNum) && allCommitsReceivedUnlocked(roundNum) {
		log.Printf("All CVS received for round %s. Generating Merkle root...", roundNum)
		commitMu.Unlock() // Unlock before calling generateMerkleRoot
		generateMerkleRoot(fallbackEthClient, roundNum)
		commitMu.Lock() // Re-lock if needed
	}
}

func handleCOSRequest(fallbackEthClient *fallback_ethclient.FallbackRPCClient, h host.Host, s network.Stream) {
	defer s.Close()

	var req utils.CosRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode COS request: %v", err)
		return
	}

	cosVerificationRequest := utils.Request{Round: req.Round, EOAAddress: req.EOAAddress, Signature: req.Signature}

	if !VerifySignatureAndCheckActivation(fallbackEthClient, cosVerificationRequest, "COS") {
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
	log.Printf("Storing COS for round %s EOA %s", roundNum, eoaAddress.Hex())

	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		log.Printf("Error saving COS data for round %s EOA %s: %v", roundNum, eoaAddress.Hex(), err)
		return
	}
	updateInMemoryData(roundNum, eoaAddress, *commitData)
	log.Printf("COS data saved and updated in-memory for round %s EOA %s", roundNum, eoaAddress.Hex())
	leaderNode_helper.BroadCastCOS(libp2putils.HostInstance, roundNum, eoaAddress, commitData.Cos)
	// Check if all commits are ready after this COS
	if !isMerkleRootSubmitted(roundNum) && allCommitsReceivedUnlocked(roundNum) {
		log.Printf("All CVS received for round %s after COS, generating Merkle root...", roundNum)
		commitMu.Unlock()
		generateMerkleRoot(fallbackEthClient, roundNum)
		commitMu.Lock()
	}

	// Also, if all COS are received (if that matters), we determine reveal order as existing code:
	if allCosReceivedUnlocked(roundNum) {
		log.Printf("All COS received for round %s.", roundNum)
		err := commitreveal2.DetermineRevealOrder(roundNum, eth.ActivatedOperators)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s: %v", roundNum, err)
			return
		}
		leaderNode_helper.StartSecretValueRequests(h, fallbackEthClient, roundNum)
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

func VerifySignatureAndCheckActivation(fallbackEthClient *fallback_ethclient.FallbackRPCClient, req utils.Request, reqType string) bool {
	verifyReq := utils.RegistrationRequest{EOAAddress: req.EOAAddress, Signature: req.Signature}
	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for round %s EOA %s", req.Round, req.EOAAddress)
		return false
	}

	eoaAddress := common.HexToAddress(req.EOAAddress)

	if !isEOAActivatedForRound(fallbackEthClient, eoaAddress) {
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

func UpdatedallCommitsReceivedUnlocked(fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string) map[string]bool {
	result := make(map[string]bool)
	ops, _ := eth.GetActivatedOperators(fallbackEthClient)

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
func generateMerkleRoot(fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string) {
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

	log.Printf("Activated operators for round %s in order: %v", roundNum, activatedOperatorsList)

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
		log.Printf("Added CVS from operator %s for round %s", opAddr.Hex(), roundNum)
	}

	commitMu.Unlock()

	if len(leaves) == 0 {
		log.Printf("Error: No CVS commits found for round %s. Cannot generate Merkle root.", roundNum)
		return
	}

	log.Printf("Leaves for Merkle tree for round %s: %v", roundNum, leaves)

	merkleRoot, err := commitreveal2.CreateMerkleTree(leaves)
	if err != nil {
		log.Printf("Failed to create Merkle tree for round %s: %v", roundNum, err)
		return
	}
	if !submittingMerkleRoot {
		submittingMerkleRoot = true
		submitMerkleRoot(fallbackEthClient, roundNum, merkleRoot)
	}
}

func submitMerkleRoot(fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string, merkleRoot []byte) {
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
		fallbackEthClient,
		"submitMerkleRoot",
		big.NewInt(0),
		merkleRootBytes32,
	)
	if err != nil {
		log.Printf("Failed to submit Merkle root for round %s: %v", roundNum, err)
		return
	}

	log.Printf("Successfully submitted Merkle root for round %s", roundNum)
	submittingMerkleRoot = false
	roundData := leaderNode_helper.RoundsData[roundNum]
	roundData.MerkleRoot = true
	if leaderNode_helper.RoundsData == nil {
		leaderNode_helper.RoundsData = make(map[string]leaderNode_helper.RoundData)
	}
	leaderNode_helper.RoundsData[roundNum] = roundData
	updateCommitDataAfterSubmit(roundNum)

	if _, exists := cosTimerOnce[roundNum]; !exists {
		cosTimerOnce[roundNum] = &sync.Once{}
	}

	cosTimerOnce[roundNum].Do(func() {
		go func(rn string) {
			log.Printf("Started 30s timer for COS for round %s", rn)
			time.Sleep(30 * time.Second)
			commitMu.Lock()
			defer commitMu.Unlock()
			ops := eth.ActivatedOperators
			roundCommits, roundExists := utils.CommittedNodes[rn]
			var missingIndices []*big.Int
			if roundExists {
				for idx, op := range ops {
					data, ok := roundCommits[op]
					if !ok || data.Cos == [32]byte{} {
						missingIndices = append(missingIndices, big.NewInt(int64(idx)))
					}
				}
			}
			if len(missingIndices) > 0 {
				log.Printf("Requesting on-chain for missing COS indices: %v for round %s", missingIndices, rn)
				requestToSubmitCo(fallbackEthClient, rn, missingIndices)
			}
		}(roundNum)
	})
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
		log.Printf("Setting submit_merkle_root_done = true for key: %s+%s", roundNum, eoaAddress.Hex())

		if err := utils.SaveLeaderCommitData(data); err != nil {
			log.Printf("Failed to save updated commit data for %s in round %s: %v", eoaAddress.Hex(), roundNum, err)
		} else {
			roundMap[eoaAddress] = data
		}
	}
}

func isEOAActivatedForRound(fallbackEthClient *fallback_ethclient.FallbackRPCClient, eoaAddress common.Address) bool {
	activatedOperators, err := eth.GetActivatedOperators(fallbackEthClient)
	if err != nil {
		log.Printf("Error fetching the activated operators %v", err)
	}

	for _, operator := range activatedOperators {
		if operator == eoaAddress {
			log.Printf("EOA address %s is activated", eoaAddress.Hex())
			return true
		}
	}

	log.Printf("EOA address %s is NOT activated", eoaAddress.Hex())
	return false
}

func processRounds(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round leaderNode_helper.RandomRequest) {
	roundNum := round.Round.String()
	if !leaderNode_helper.RoundsData[roundNum].MerkleRoot && !leaderNode_helper.RoundsData[roundNum].RandomNumber {
		log.Printf("LeaderNode for round %s is still waiting for commits...", roundNum)

		commitMu.Lock()
		ready := UpdatedallCommitsReceivedUnlocked(fallbackEthClient, roundNum)
		commitMu.Unlock()
		var missingOperators []string

		allReceived := true
		for op, submitted := range ready {
			if !submitted {
				allReceived = false
				log.Printf("Operator %s has not submitted CV.", op)
				if _, exists := onChainExecution[roundNum]; !exists {
					onChainExecution[roundNum] = make(map[string]map[string]int)
				}
				if _, exists := onChainExecution[roundNum]["CVS"]; !exists {
					onChainExecution[roundNum]["CVS"] = make(map[string]int)
				}
				if onChainExecution[roundNum]["CVS"][op] >= 3 {
					failToSubmitCv(fallbackEthClient)
					revert()
				} else if sendCommitRequest[roundNum] {
					missingOperators = append(missingOperators, op)
					onChainExecution[roundNum]["CVS"][op]++
					flag[roundNum] = true
					dispute[roundNum] = true
				}
			}
		}
		if flag[roundNum] {
			if requestCv {
				handleMissingCV(fallbackEthClient, missingOperators, roundNum)
			}
		}
		if allReceived {
			log.Printf("All CVS received for round %s. Generating Merkle root...", roundNum)
			flag[roundNum] = false
			generateMerkleRoot(fallbackEthClient, roundNum)
		} else {
			log.Printf("Not all CVS received for round %s. Waiting for remaining commits.", roundNum)
			roundFlag[roundNum]++
			if roundFlag[roundNum] >= 1 {
				sendCommitRequest[roundNum] = true
			}
		}
	}
}

func failToSubmitCv(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
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
		log.Printf("Failed to failToSubmitCv request root for round %s: %v", leaderNode_helper.CurrentRound, err)
		return
	}

	log.Printf("Successfully submitted failToSubmitCv request for round %s", leaderNode_helper.CurrentRound)
}

func prepareArgumentsForRequestToSubmitCo(roundNum string, missingIndices []*big.Int) ([]CvAndSigRS, *big.Int, *big.Int, *big.Int) {
	cvs, _, _, vs, rs, ss := leaderNode_helper.LoadNodeData(roundNum)
	indicesLength := big.NewInt(int64(len(missingIndices)))

	notOnChainIndices, onChainIndices := orderedPackedIndices(missingIndices)
	allOrderedIndices := append(notOnChainIndices, onChainIndices...)
	packedOrderedIndices := leaderNode_helper.PackIndices(allOrderedIndices)
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
	packedVs := leaderNode_helper.PackIndices(vsForNotOnChain)
	return cvNotOnChainCvAndSigRS, packedVs, indicesLength, packedOrderedIndices
}

func orderedPackedIndices(missingIndices []*big.Int) ([]*big.Int, []*big.Int) {
	onChainCvIndices := make(map[int64]struct{})
	for _, idx := range leaderNode_helper.Indices {
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

func requestToSubmitCo(fallbackEthClient *fallback_ethclient.FallbackRPCClient, roundNum string, missingIndices []*big.Int) {
	cvNotOnChainCvAndSigRS, packedVs, indicesLength, packedOrederedIndices := prepareArgumentsForRequestToSubmitCo(roundNum, missingIndices)

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
		"requestToSubmitCo",
		big.NewInt(0),
		cvNotOnChainCvAndSigRS,
		packedVs,
		indicesLength,
		packedOrederedIndices,
	)
	if err != nil {
		log.Printf("Failed to submit commit request root for round %s: %v", roundNum, err)
		return
	}

	log.Printf("Successfully submitted cos request for round %s and indices %v", roundNum, missingIndices)
}

func handleMissingCV(fallbackEthClient *fallback_ethclient.FallbackRPCClient, missingOperators []string, roundNum string) {
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

	packedIndices := leaderNode_helper.PackIndices(leaderNode_helper.Indices)

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
		log.Printf("Failed to submit commit request root for round %s: %v", roundNum, err)
		return
	}

	log.Printf("Successfully submitted commit request for round %s and indices %v", roundNum, leaderNode_helper.Indices)
	requestCv = false
}

func revert() {

}

func handleAcknowledgment(s network.Stream) {
	defer s.Close()

	var ack utils.AcknowledgmentMessage
	if err := json.NewDecoder(s).Decode(&ack); err != nil {
		log.Printf("Failed to decode acknowledgment message: %v", err)
		return
	}

	log.Printf("Received acknowledgment from %s for %s broadcast (message ID: %s, status: %s)",
		ack.EOAAddress, ack.Type, ack.MessageID, ack.Status)

	// Process the acknowledgment
	leaderNode_helper.HandleAcknowledgment(ack)
}
