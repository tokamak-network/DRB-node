package nodes

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/machinebox/graphql"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/nodes/leaderNode_helper"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

var commitMu sync.Mutex

type RoundData struct {
	MerkleRootSubmitted struct {
		MerkleRoot interface{} `json:"merkleRoot"`
	} `json:"merkleRootSubmitted"`
	Round                 interface{} `json:"round"`
	RandomNumberGenerated struct {
		RandomNumber interface{} `json:"randomNumber"`
	} `json:"randomNumberGenerated"`
	RandomNumberRequested struct {
		ActivatedOperators []string `json:"activatedOperators"`
	} `json:"randomNumberRequested"`
}

type GraphQLResponse struct {
	Rounds []RoundData `json:"rounds"`
}

// committedNodes and activatedOperators are authoritative in-memory states.
var committedNodes = make(map[string]map[common.Address]utils.LeaderCommitData)
var activatedOperators = make(map[string]map[common.Address]bool)

func RunLeaderNode() {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT not set in environment variables.")
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

	go leaderNode_helper.MonitorCommits(h)

	for {
		roundsData, err := fetchRoundsData()
		if err != nil {
			log.Printf("Error fetching rounds data: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		processRounds(roundsData)
		time.Sleep(30 * time.Second)
	}
}

func fetchRoundsData() (*GraphQLResponse, error) {
	client := graphql.NewClient(os.Getenv("SUBGRAPH_URL"))
	ctx := context.Background()
	req := utils.GetRoundsRequest()

	var resp GraphQLResponse
	if err := client.Run(ctx, req, &resp); err != nil {
		log.Fatalf("Failed to execute GraphQL request: %v", err)
	}
	return &resp, nil
}

func handleRegistrationRequest(s network.Stream) {
	defer s.Close()
	filePath := "registered_nodes.json"
	if err := leaderNode_helper.RegisterNode(s, filePath, "contract/abi/Commit2RevealDRB.json"); err != nil {
		log.Printf("Failed to handle registration request: %v", err)
		return
	}
	log.Println("Node registration and activation completed.")
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
	log.Printf("Commit data saved and updated in-memory for round %s EOA %s", roundNum, eoaAddress.Hex())

	// Check if all commits are ready after this update
	if !isMerkleRootSubmitted(roundNum) && allCommitsReceivedUnlocked(roundNum) {
		log.Printf("All CVS received for round %s. Generating Merkle root...", roundNum)
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
	log.Printf("Storing COS for round %s EOA %s", roundNum, eoaAddress.Hex())

	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		log.Printf("Error saving COS data for round %s EOA %s: %v", roundNum, eoaAddress.Hex(), err)
		return
	}
	updateInMemoryData(roundNum, eoaAddress, *commitData)
	log.Printf("COS data saved and updated in-memory for round %s EOA %s", roundNum, eoaAddress.Hex())

	// Check if all commits are ready after this COS
	if !isMerkleRootSubmitted(roundNum) && allCommitsReceivedUnlocked(roundNum) {
		log.Printf("All CVS received for round %s after COS, generating Merkle root...", roundNum)
		commitMu.Unlock()
		generateMerkleRoot(roundNum)
		commitMu.Lock()
	}

	// Also, if all COS are received (if that matters), we determine reveal order as existing code:
	if allCommitsReceivedUnlocked(roundNum) {
		log.Printf("All COS received for round %s. Determining reveal order...", roundNum)
		err := commitreveal2.DetermineRevealOrder(roundNum, activatedOperators)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s: %v", roundNum, err)
			return
		}
		log.Printf("Reveal order determined for round %s.", roundNum)
		leaderNode_helper.StartSecretValueRequests(h, roundNum)
	}
}

func isMerkleRootSubmitted(roundNum string) bool {
	// Call with commitMu locked or ensure commitMu is locked outside
	roundMap, exists := committedNodes[roundNum]
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

func VerifySignatureAndCheckActivation(temp utils.Request, reqType string, ) bool {
	verifyReq := utils.RegistrationRequest{EOAAddress: temp.EOAAddress, Signature: temp.Signature}
	if !utils.VerifySignature(verifyReq) {
		log.Printf("Signature verification failed for round %s EOA %s", temp.Round, temp.EOAAddress)
		return false
	}

	roundNum := temp.Round
	eoaAddress := common.HexToAddress(temp.EOAAddress)

	if !isEOAActivatedForRound(roundNum, eoaAddress) {
		log.Printf("EOA %s not activated for round %s, skipping %s.", eoaAddress.Hex(), roundNum, reqType)
		return false
	}
	return true
}

// allCommitsReceivedUnlocked checks if all operators have CVS in-memory.
// Called with commitMu locked.
func allCommitsReceivedUnlocked(roundNum string) bool {
	ops, exists := activatedOperators[roundNum]
	if !exists || len(ops) == 0 {
		return false
	}

	roundCommits, roundExists := committedNodes[roundNum]
	if !roundExists || len(roundCommits) == 0 {
		return false
	}

	for op := range ops {
		data, ok := roundCommits[op]
		if !ok || data.Cvs == [32]byte{} {
			return false
		}
	}
	return true
}

// getOrCreateLeaderCommitData returns commitData from in-memory map or creates a new one.
// Called with commitMu locked.
func getOrCreateLeaderCommitData(roundNum string, eoaAddress common.Address) *utils.LeaderCommitData {
	roundMap, exists := committedNodes[roundNum]
	if !exists {
		roundMap = make(map[common.Address]utils.LeaderCommitData)
		committedNodes[roundNum] = roundMap
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
	roundMap, exists := committedNodes[roundNum]
	if !exists {
		roundMap = make(map[common.Address]utils.LeaderCommitData)
		committedNodes[roundNum] = roundMap
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

	activatedOperatorsList, err := leaderNode_helper.FetchActivatedOperators(roundNum)
	if err != nil {
		log.Printf("Failed to fetch activated operators for round %s: %v", roundNum, err)
		return
	}

	var filteredOperators []string
	for _, operator := range activatedOperatorsList {
		if operator != "0x0000000000000000000000000000000000000000" {
			filteredOperators = append(filteredOperators, operator)
		}
	}

	log.Printf("Activated operators for round %s in order: %v", roundNum, filteredOperators)
	if len(filteredOperators) == 0 {
		log.Printf("No valid activated operators found for round %s. Cannot generate Merkle root.", roundNum)
		return
	}

	commitMu.Lock()
	roundMap, roundExists := committedNodes[roundNum]
	if !roundExists || len(roundMap) == 0 {
		log.Printf("No commits found in-memory for round %s, cannot generate Merkle root.", roundNum)
		commitMu.Unlock()
		return
	}

	var leaves [][]byte
	for _, op := range filteredOperators {
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

	submitMerkleRoot(roundNum, merkleRoot)
}

func submitMerkleRoot(roundNum string, merkleRoot []byte) {
	var merkleRootBytes32 [32]byte
	copy(merkleRootBytes32[:], merkleRoot)

	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		log.Printf("Failed to connect to Ethereum client: %v", err)
		return
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		log.Printf("Failed to load contract ABI: %v", err)
		return
	}

	roundNumInt, err := strconv.ParseInt(roundNum, 10, 64)
	if err != nil {
		log.Printf("Failed to parse roundNum: %v", err)
		return
	}

	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
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
		big.NewInt(roundNumInt),
		merkleRootBytes32,
	)
	if err != nil {
		log.Printf("Failed to submit Merkle root for round %s: %v", roundNum, err)
		return
	}

	log.Printf("Successfully submitted Merkle root for round %s", roundNum)
	updateCommitDataAfterSubmit(roundNum)
}

func updateCommitDataAfterSubmit(roundNum string) {
	commitMu.Lock()
	defer commitMu.Unlock()

	roundMap, exists := committedNodes[roundNum]
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

func isEOAActivatedForRound(roundNum string, eoaAddress common.Address) bool {
	roundInt, err := strconv.Atoi(roundNum)
	if err != nil {
		log.Printf("Invalid round number %s: %v", roundNum, err)
		return false
	}

	client := graphql.NewClient(os.Getenv("SUBGRAPH_URL"))
	req := utils.GetActivatedOperatorsAtRoundRequest(roundInt)

	var resp map[string]interface{}
	ctx := context.Background()
	err = client.Run(ctx, req, &resp)
	if err != nil {
		log.Printf("Failed to execute GraphQL request for activated operators in round %d: %v", roundInt, err)
		return false
	}

	activatedOperatorsData, ok := resp["randomNumberRequesteds"].([]interface{})
	if !ok || len(activatedOperatorsData) == 0 {
		log.Printf("No activated operators found for round %d", roundInt)
		return false
	}

	activated := activatedOperatorsData[0].(map[string]interface{})["activatedOperators"].([]interface{})
	for _, operator := range activated {
		operatorAddress := common.HexToAddress(operator.(string))
		if operatorAddress == eoaAddress {
			log.Printf("EOA address %s is activated for round %d", eoaAddress.Hex(), roundInt)
			return true
		}
	}

	log.Printf("EOA address %s is NOT activated for round %d", eoaAddress.Hex(), roundInt)
	return false
}

func processRounds(roundsData *GraphQLResponse) {
	for _, round := range roundsData.Rounds {
		roundNum, ok := round.Round.(string)
		if !ok {
			log.Printf("Error: Round is not of type string.")
			continue
		}

		if len(round.RandomNumberRequested.ActivatedOperators) > 0 {
			commitMu.Lock()
			if _, exists := activatedOperators[roundNum]; !exists {
				activatedOperators[roundNum] = make(map[common.Address]bool)
			}
			for _, op := range round.RandomNumberRequested.ActivatedOperators {
				opAddr := common.HexToAddress(op)
				if opAddr == common.HexToAddress("0x0000000000000000000000000000000000000000") {
					continue
				}
				activatedOperators[roundNum][opAddr] = true
			}
			commitMu.Unlock()
		}

		if round.MerkleRootSubmitted.MerkleRoot == nil && round.RandomNumberGenerated.RandomNumber == nil {
			log.Printf("Round %s is still waiting for commits", roundNum)

			commitMu.Lock()
			ready := allCommitsReceivedUnlocked(roundNum)
			commitMu.Unlock()

			if ready {
				log.Printf("All CVS received for round %s. Generating Merkle root...", roundNum)
				generateMerkleRoot(roundNum)
			} else {
				log.Printf("Not all CVS received for round %s. Waiting for remaining commits.", roundNum)
			}
		}
	}
}
