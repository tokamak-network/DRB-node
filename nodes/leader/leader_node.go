package leader

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/logger"
	leaderNodeHelper "github.com/tokamak-network/DRB-node/nodes/leader/helper"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

type Node struct {
	host                 host.Host
	fallbackEthClient    *fallback_ethclient.FallbackRPCClient
	mu                   sync.Mutex
	config               *Config
	submittingMerkleRoot bool
	cosTimerOnce         map[string]*sync.Once
	onChainExecution     map[string]map[string]map[string]int
	sendCommitRequest    map[string]bool
	flag                 map[string]bool
	dispute              map[string]bool
	requestCv            bool
	roundFlag            map[string]uint64
	abi                  abi.ABI
}

func NewNode(
	config *Config,
	fallbackEthClient *fallback_ethclient.FallbackRPCClient,
) (*Node, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if config.Port == 0 {
		return nil, fmt.Errorf("port is required")
	}

	if config.ContractAddress == (common.Address{}) {
		return nil, fmt.Errorf("contract address is required")
	}

	if config.LeaderPrivateKey == nil {
		return nil, fmt.Errorf("leader private key is required")
	}

	if fallbackEthClient == nil {
		return nil, fmt.Errorf("fallback eth client is nil")
	}

	host, peerID, err := libp2putils.CreateHost(fmt.Sprintf("%d", config.Port))
	if err != nil {
		logger.Fatalf("Error creating host: %v", err)
	}

	logger.Infof("Leader node running on: %s", host.Addrs())
	logger.Infof("Leader node PeerID: %s", peerID.String())

	// Set the host to the libp2putils package
	libp2putils.SetHost(host)

	parsedAbi, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		logger.Fatalf("Failed to load contract ABI: %v", err)
	}

	node := &Node{
		config:               config,
		fallbackEthClient:    fallbackEthClient,
		host:                 host,
		submittingMerkleRoot: false,
		cosTimerOnce:         make(map[string]*sync.Once),
		onChainExecution:     make(map[string]map[string]map[string]int),
		sendCommitRequest:    make(map[string]bool),
		flag:                 make(map[string]bool),
		dispute:              make(map[string]bool),
		requestCv:            true,
		roundFlag:            make(map[string]uint64),
		abi:                  parsedAbi,
	}

	node.setHandlers()

	return node, nil
}

func (n *Node) Start(ctx context.Context) {
	go leaderNodeHelper.MonitorCommits(n.fallbackEthClient)
	go leaderNodeHelper.ReceiveCommit(n.fallbackEthClient)
	for {
		if !leaderNodeHelper.Execution {
			time.Sleep(10 * time.Second)
			continue
		}
		firstRequest := leaderNodeHelper.Req
		fmt.Printf("Executing request: %v", firstRequest)
		n.processRounds(firstRequest)
		time.Sleep(30 * time.Second)
	}
}

func (n *Node) Close() {
	n.host.Close()
}

func (n *Node) setHandlers() {
	n.host.SetStreamHandler("/register", n.handleRegistrationRequest)
	n.host.SetStreamHandler("/cvs", n.handleCommitRequest)
	n.host.SetStreamHandler("/cos", n.handleCOSRequest)
	n.host.SetStreamHandler("/secretValue", func(s network.Stream) {
		leaderNodeHelper.AcceptSecretValue(n.host, s, n.fallbackEthClient)
	})
}

func (n *Node) handleCOSRequest(s network.Stream) {
	defer s.Close()

	var req utils.CosRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		logger.Infof("Failed to decode COS request: %v", err)
		return
	}

	cosVerificationRequest := utils.Request{Round: req.Round, EOAAddress: req.EOAAddress, Signature: req.Signature}

	if !n.verifySignatureAndCheckActivation(cosVerificationRequest, "COS") {
		return
	}

	roundNum := req.Round
	eoaAddress := common.HexToAddress(req.EOAAddress)

	n.mu.Lock()
	defer n.mu.Unlock()

	commitData := n.getOrCreateLeaderCommitData(roundNum, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		logger.Infof("No CVS found for round %s EOA %s, rejecting COS.", roundNum, eoaAddress.Hex())
		return
	}

	recalculatedCvs := commitreveal2.Keccak256(req.Cos[:])
	if !bytes.Equal(recalculatedCvs, commitData.Cvs[:]) {
		logger.Infof("COS hash mismatch for round %s EOA %s. Rejecting COS.", roundNum, eoaAddress.Hex())
		return
	}

	if commitData.Cos != [32]byte{} {
		logger.Infof("COS already received for round %s EOA %s. Skipping.", roundNum, eoaAddress.Hex())
		return
	}

	commitData.Cos = req.Cos
	commitData.CosHex = hex.EncodeToString(req.Cos[:])
	logger.Infof("Storing COS for round %s EOA %s", roundNum, eoaAddress.Hex())

	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		logger.Infof("Error saving COS data for round %s EOA %s: %v", roundNum, eoaAddress.Hex(), err)
		return
	}
	n.updateInMemoryData(roundNum, eoaAddress, *commitData)
	logger.Infof("COS data saved and updated in-memory for round %s EOA %s", roundNum, eoaAddress.Hex())
	leaderNodeHelper.BroadCastCOS(n.host, roundNum, eoaAddress, commitData.Cos)
	// Check if all commits are ready after this COS
	if !n.isMerkleRootSubmitted(roundNum) && n.allCommitsReceivedUnlocked(roundNum) {
		logger.Infof("All CVS received for round %s after COS, generating Merkle root...", roundNum)
		n.mu.Unlock() // Unlock before calling generateMerkleRoot
		n.generateMerkleRoot(roundNum)
		n.mu.Lock() // Re-lock if needed
	}

	// Also, if all COS are received (if that matters), we determine reveal order as existing code:
	if n.allCosReceivedUnlocked(roundNum) {
		logger.Infof("All COS received for round %s.", roundNum)
		err := commitreveal2.DetermineRevealOrder(roundNum, eth.ActivatedOperators)
		if err != nil {
			logger.Infof("Failed to determine reveal order for round %s: %v", roundNum, err)
			return
		}
		leaderNodeHelper.StartSecretValueRequests(n.host, n.fallbackEthClient, roundNum)
	}
}

func (n *Node) handleRegistrationRequest(s network.Stream) {
	defer s.Close()
	filePath := "registered_nodes.json"
	if err := leaderNodeHelper.RegisterNode(s, filePath, "contract/abi/Commit2RevealDRB.json"); err != nil {
		logger.Infof("Failed to handle registration request: %v", err)
		return
	}
	logger.Infof("Node registration completed.")
}

func (n *Node) handleCommitRequest(s network.Stream) {
	defer s.Close()

	var req utils.CommitRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		logger.Infof("Failed to decode commit request: %v", err)
		return
	}

	commitVerificationRequest := utils.Request{Round: req.Round, EOAAddress: req.EOAAddress, Signature: req.Signature}

	if !n.verifySignatureAndCheckActivation(commitVerificationRequest, "commit") {
		return
	}

	roundNum := req.Round
	eoaAddress := common.HexToAddress(req.EOAAddress)

	n.mu.Lock()
	defer n.mu.Unlock()

	commitData := n.getOrCreateLeaderCommitData(roundNum, eoaAddress)
	if commitData.Cvs == [32]byte{} {
		commitData.Cvs = req.Cvs
		commitData.CvsHex = hex.EncodeToString(req.Cvs[:])
		commitData.Sign = req.Sign
		logger.Infof("Storing CVS and signature for round %s EOA %s", roundNum, eoaAddress.Hex())
	}

	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		logger.Infof("Error saving commit data for round %s EOA %s: %v", roundNum, eoaAddress.Hex(), err)
		return
	}
	n.updateInMemoryData(roundNum, eoaAddress, *commitData)
	logger.Infof("Commit data saved and updated in-memory for round %s EOA %s", roundNum, eoaAddress.Hex())
	leaderNodeHelper.BroadCastCVS(n.host, roundNum, eoaAddress, commitData.Cvs)
	// Check if all commits are ready after this update
	if !n.isMerkleRootSubmitted(roundNum) && n.allCommitsReceivedUnlocked(roundNum) {
		logger.Infof("All CVS received for round %s. Generating Merkle root...", roundNum)
		n.mu.Unlock() // Unlock before calling generateMerkleRoot
		n.generateMerkleRoot(roundNum)
		n.mu.Lock() // Re-lock if needed
	}
}

// ------------------------------------------------------------
// Internal functions
// ------------------------------------------------------------

func (n *Node) verifySignatureAndCheckActivation(req utils.Request, reqType string) bool {
	verifyReq := utils.RegistrationRequest{EOAAddress: req.EOAAddress, Signature: req.Signature}
	if !utils.VerifySignature(verifyReq) {
		logger.Infof("Signature verification failed for round %s EOA %s", req.Round, req.EOAAddress)
		return false
	}

	eoaAddress := common.HexToAddress(req.EOAAddress)

	if !n.isEOAActivatedForRound(eoaAddress) {
		logger.Infof("EOA %s not activated, skipping %v.", eoaAddress.Hex(), reqType)
		return false
	}
	return true
}

func (n *Node) isEOAActivatedForRound(eoaAddress common.Address) bool {
	activatedOperators, err := eth.GetActivatedOperators(n.fallbackEthClient)
	if err != nil {
		logger.Infof("Error fetching the activated operators %v", err)
	}

	for _, operator := range activatedOperators {
		if operator == eoaAddress {
			logger.Infof("EOA address %s is activated", eoaAddress.Hex())
			return true
		}
	}

	logger.Infof("EOA address %s is NOT activated", eoaAddress.Hex())
	return false
}

// getOrCreateLeaderCommitData returns commitData from in-memory map or creates a new one.
// Called with commitMu locked.
func (n *Node) getOrCreateLeaderCommitData(roundNum string, eoaAddress common.Address) *utils.LeaderCommitData {
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
func (n *Node) updateInMemoryData(roundNum string, eoaAddress common.Address, commitData utils.LeaderCommitData) {
	roundMap, exists := utils.CommittedNodes[roundNum]
	if !exists {
		roundMap = make(map[common.Address]utils.LeaderCommitData)
		utils.CommittedNodes[roundNum] = roundMap
	}
	roundMap[eoaAddress] = commitData
}

func (n *Node) isMerkleRootSubmitted(roundNum string) bool {
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

// allCommitsReceivedUnlocked checks if all operators have CVS in-memory.
// Called with commitMu locked.
func (n *Node) allCommitsReceivedUnlocked(roundNum string) bool {
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

// generateMerkleRoot doesn't lock; it locks inside to read from memory
func (n *Node) generateMerkleRoot(roundNum string) {
	n.mu.Lock()
	// Check if merkle root is already done before proceeding
	if n.isMerkleRootSubmitted(roundNum) {
		logger.Infof("Merkle root already submitted for round %s, skipping.", roundNum)
		n.mu.Unlock()
		return
	}
	n.mu.Unlock()

	logger.Infof("Generating Merkle root for round %s...", roundNum)

	activatedOperatorsList := leaderNodeHelper.ActivatedOperator

	logger.Infof("Activated operators for round %s in order: %v", roundNum, activatedOperatorsList)

	n.mu.Lock()
	roundMap, roundExists := utils.CommittedNodes[roundNum]
	if !roundExists || len(roundMap) == 0 {
		logger.Infof("No commits found in-memory for round %s, cannot generate Merkle root.", roundNum)
		n.mu.Unlock()
		return
	}

	var leaves [][]byte
	for _, op := range activatedOperatorsList {
		opAddr := common.HexToAddress(op)
		data, ok := roundMap[opAddr]
		if !ok || data.Cvs == [32]byte{} {
			logger.Infof("Missing CVS for operator %s in round %s", opAddr.Hex(), roundNum)
			n.mu.Unlock()
			return
		}
		leaves = append(leaves, data.Cvs[:])
		logger.Infof("Added CVS from operator %s for round %s", opAddr.Hex(), roundNum)
	}

	n.mu.Unlock()

	if len(leaves) == 0 {
		logger.Infof("Error: No CVS commits found for round %s. Cannot generate Merkle root.", roundNum)
		return
	}

	logger.Infof("Leaves for Merkle tree for round %s: %v", roundNum, leaves)

	merkleRoot, err := commitreveal2.CreateMerkleTree(leaves)
	if err != nil {
		logger.Infof("Failed to create Merkle tree for round %s: %v", roundNum, err)
		return
	}
	if !n.submittingMerkleRoot {
		n.submittingMerkleRoot = true
		n.submitMerkleRoot(roundNum, merkleRoot)
	}
}

func (n *Node) submitMerkleRoot(roundNum string, merkleRoot []byte) {
	var merkleRootBytes32 [32]byte
	copy(merkleRootBytes32[:], merkleRoot)

	clientUtils := &utils.Client{
		ContractAddress: n.config.ContractAddress,
		PrivateKey:      n.config.LeaderPrivateKey,
		ContractABI:     n.abi,
	}

	_, _, err := eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		n.fallbackEthClient,
		"submitMerkleRoot",
		big.NewInt(0),
		merkleRootBytes32,
	)
	if err != nil {
		logger.Infof("Failed to submit Merkle root for round %s: %v", roundNum, err)
		return
	}

	logger.Infof("Successfully submitted Merkle root for round %s", roundNum)
	n.submittingMerkleRoot = false
	roundData := leaderNodeHelper.RoundsData[roundNum]
	roundData.MerkleRoot = true
	if leaderNodeHelper.RoundsData == nil {
		leaderNodeHelper.RoundsData = make(map[string]leaderNodeHelper.RoundData)
	}
	leaderNodeHelper.RoundsData[roundNum] = roundData
	n.updateCommitDataAfterSubmit(roundNum)

	if _, exists := n.cosTimerOnce[roundNum]; !exists {
		n.cosTimerOnce[roundNum] = &sync.Once{}
	}

	n.cosTimerOnce[roundNum].Do(func() {
		go func(rn string) {
			logger.Infof("Started 30s timer for COS for round %s", rn)
			time.Sleep(30 * time.Second)
			n.mu.Lock()
			defer n.mu.Unlock()
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
				logger.Infof("Requesting on-chain for missing COS indices: %v for round %s", missingIndices, rn)
				n.requestToSubmitCo(rn, missingIndices)
			}
		}(roundNum)
	})
}

func (n *Node) requestToSubmitCo(roundNum string, missingIndices []*big.Int) {
	cvNotOnChainCvAndSigRS, packedVs, indicesLength, packedOrederedIndices := n.prepareArgumentsForRequestToSubmitCo(roundNum, missingIndices)

	clientUtils := &utils.Client{
		ContractAddress: n.config.ContractAddress,
		PrivateKey:      n.config.LeaderPrivateKey,
		ContractABI:     n.abi,
	}

	_, _, err := eth.ExecuteTransaction(
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
		logger.Infof("Failed to submit commit request root for round %s: %v", roundNum, err)
		return
	}

	logger.Infof("Successfully submitted cos request for round %s and indices %v", roundNum, missingIndices)
}

func (n *Node) prepareArgumentsForRequestToSubmitCo(roundNum string, missingIndices []*big.Int) ([]CvAndSigRS, *big.Int, *big.Int, *big.Int) {
	cvs, _, _, vs, rs, ss := leaderNodeHelper.LoadNodeData(roundNum)
	indicesLength := big.NewInt(int64(len(missingIndices)))

	notOnChainIndices, onChainIndices := n.orderedPackedIndices(missingIndices)
	allOrderedIndices := append(notOnChainIndices, onChainIndices...)
	packedOrderedIndices := leaderNodeHelper.PackIndices(allOrderedIndices)
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
	packedVs := leaderNodeHelper.PackIndices(vsForNotOnChain)
	return cvNotOnChainCvAndSigRS, packedVs, indicesLength, packedOrderedIndices
}

func (n *Node) orderedPackedIndices(missingIndices []*big.Int) ([]*big.Int, []*big.Int) {
	onChainCvIndices := make(map[int64]struct{})
	for _, idx := range leaderNodeHelper.Indices {
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

func (n *Node) updateCommitDataAfterSubmit(roundNum string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	roundMap, exists := utils.CommittedNodes[roundNum]
	if !exists {
		return
	}

	for eoaAddress, data := range roundMap {
		data.SubmitMerkleRootDone = true
		logger.Infof("Setting submit_merkle_root_done = true for key: %s+%s", roundNum, eoaAddress.Hex())

		if err := utils.SaveLeaderCommitData(data); err != nil {
			logger.Infof("Failed to save updated commit data for %s in round %s: %v", eoaAddress.Hex(), roundNum, err)
		} else {
			roundMap[eoaAddress] = data
		}
	}
}

func (n *Node) allCosReceivedUnlocked(roundNum string) bool {
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

func (n *Node) processRounds(round leaderNodeHelper.RandomRequest) {
	roundNum := round.Round.String()
	if !leaderNodeHelper.RoundsData[roundNum].MerkleRoot && !leaderNodeHelper.RoundsData[roundNum].RandomNumber {
		logger.Infof("LeaderNode for round %s is still waiting for commits...", roundNum)

		n.mu.Lock()
		ready := n.updatedallCommitsReceivedUnlocked(roundNum)
		n.mu.Unlock()
		var missingOperators []string

		allReceived := true
		for op, submitted := range ready {
			if !submitted {
				allReceived = false
				logger.Infof("Operator %s has not submitted CV.", op)
				if _, exists := n.onChainExecution[roundNum]; !exists {
					n.onChainExecution[roundNum] = make(map[string]map[string]int)
				}
				if _, exists := n.onChainExecution[roundNum]["CVS"]; !exists {
					n.onChainExecution[roundNum]["CVS"] = make(map[string]int)
				}
				if n.onChainExecution[roundNum]["CVS"][op] >= 3 {
					n.failToSubmitCv()
					n.revert()
				} else if n.sendCommitRequest[roundNum] {
					missingOperators = append(missingOperators, op)
					n.onChainExecution[roundNum]["CVS"][op]++
					n.flag[roundNum] = true
					n.dispute[roundNum] = true
				}
			}
		}
		if n.flag[roundNum] {
			if n.requestCv {
				n.handleMissingCV(missingOperators, roundNum)
			}
		}
		if allReceived {
			logger.Infof("All CVS received for round %s. Generating Merkle root...", roundNum)
			n.flag[roundNum] = false
			n.generateMerkleRoot(roundNum)
		} else {
			logger.Infof("Not all CVS received for round %s. Waiting for remaining commits.", roundNum)
			n.roundFlag[roundNum]++
			if n.roundFlag[roundNum] >= 1 {
				n.sendCommitRequest[roundNum] = true
			}
		}
	}
}

func (n *Node) updatedallCommitsReceivedUnlocked(roundNum string) map[string]bool {
	result := make(map[string]bool)
	ops, _ := eth.GetActivatedOperators(n.fallbackEthClient)

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

func (n *Node) failToSubmitCv() {
	clientUtils := &utils.Client{
		ContractAddress: n.config.ContractAddress,
		PrivateKey:      n.config.LeaderPrivateKey,
		ContractABI:     n.abi,
	}

	_, _, err := eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		n.fallbackEthClient,
		"failToSubmitCv",
		big.NewInt(0),
	)
	if err != nil {
		logger.Infof("Failed to failToSubmitCv request root for round %s: %v", leaderNodeHelper.CurrentRound, err)
		return
	}

	logger.Infof("Successfully submitted failToSubmitCv request for round %s", leaderNodeHelper.CurrentRound)
}

func (n *Node) revert() {

}

func (n *Node) handleMissingCV(missingOperators []string, roundNum string) {
	leaderNodeHelper.CvOnChain = true
	activatedOperators := leaderNodeHelper.ActivatedOperator
	i := big.NewInt(0)
	for _, op := range activatedOperators {
		for _, missingOp := range missingOperators {
			if op == missingOp {
				leaderNodeHelper.Indices = append(leaderNodeHelper.Indices, new(big.Int).Set(i))
			}
		}
		i.Add(i, big.NewInt(1))
	}
	sort.Slice(leaderNodeHelper.Indices, func(i, j int) bool {
		return leaderNodeHelper.Indices[i].Cmp(leaderNodeHelper.Indices[j]) < 0
	})

	packedIndices := leaderNodeHelper.PackIndices(leaderNodeHelper.Indices)

	clientUtils := &utils.Client{
		ContractAddress: n.config.ContractAddress,
		PrivateKey:      n.config.LeaderPrivateKey,
		ContractABI:     n.abi,
	}

	_, _, err := eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		n.fallbackEthClient,
		"requestToSubmitCv",
		big.NewInt(0),
		packedIndices,
	)
	if err != nil {
		logger.Infof("Failed to submit commit request root for round %s: %v", roundNum, err)
		return
	}

	logger.Infof("Successfully submitted commit request for round %s and indices %v", roundNum, leaderNodeHelper.Indices)
	n.requestCv = false
}
