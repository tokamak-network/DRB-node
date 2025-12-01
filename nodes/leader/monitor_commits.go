package leader_node

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

// Global variables with mutex protection for thread safety

// MonitorCommits continuously checks for rounds where all EOAs have submitted their secret values.
func (n *LeaderNode) MonitorCommits(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("MonitorCommits function received shutdown signal, stopping...")
			return
		case <-ticker.C:
			n.checkRoundsForCompletion(ctx)
		}
	}
}

type RevealOrderData struct {
	OrderedNodes []string   `json:"ordered_nodes"`
	RevealOrder  []*big.Int `json:"reveal_order"`
	RV           string     `json:"rv"`
}

type RevealOrders map[string]RevealOrderData

func (n *LeaderNode) checkRoundsForCompletion(ctx context.Context) {
	round := n.GetCurrentRound()
	trialNum := n.GetCurrentTrial()
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	if n.GetHalted() {
		log.Println("System is halted. Skipping checkRoundsForCompletion.")
		return
	}

	// Defensive check: skip if all random_number_generated are already true for this round
	leaderCommits, err := n.leaderCommitRepository.GetLeaderCommitsByRoundAndTrialNum(ctx, round, trialNum)
	if err == nil && len(leaderCommits) > 0 {
		allRandomNumberGenerated := true
		for _, lc := range leaderCommits {
			if !lc.RandomNumberGenerated {
				allRandomNumberGenerated = false
				break
			}
		}
		if allRandomNumberGenerated {
			return
		}
	}

	// Copy the activated operators from eth package
	operatorAddresses := eth.Service.GetActivatedOperatorsCached()
	if len(operatorAddresses) < 2 {
		return
	}
	// Collect secret values, signatures (v, r, s), and round info in the order of activated operators
	var secrets [][]byte
	var vs []uint8
	var rs []common.Hash
	var ss []common.Hash
	var index int
	allEOAsSubmitted := true
	for i, operator := range operatorAddresses {
		commitData, err := n.leaderCommitRepository.GetLeaderCommitByRoundAndEoaAddr(ctx, round, trialNum, operator.Hex())
		if err != nil {
			log.Printf("Either Data not found or Error getting commit data for operator %s: %v", operator.Hex(), err)
			return
		} else {
			if commitData.SecretValue == [32]byte{} {
				if !(commitData.Cos == [32]byte{}) {
					log.Printf("EOA %s has not submitted a secret value for round %s.", operator.Hex(), round)
				}
				return
			}
		}
		secrets = append(secrets, commitData.SecretValue[:])
		// if Cv values are on-chain, than check this condition
		cvOnChain, _ := n.GetCvOnChain(uniqueKey)
		if cvOnChain {
			indices := n.GetIndices()
			if index < len(indices) && int64(i) <= indices[index].Int64() {
				if int64(i) == indices[index].Int64() {
					index++
					continue
				}
			}
		}

		// Ensure the signature map contains valid data
		if commitData.Sign.V == "" || commitData.Sign.R == "" || commitData.Sign.S == "" {
			log.Printf("Incomplete signature for EOA %s in round %s", operator.Hex(), round)
			allEOAsSubmitted = false
			continue
		}

		// Parse and validate signature components
		vStr := commitData.Sign.V
		vValue, err := strconv.ParseUint(vStr, 10, 8)
		if err != nil {
			log.Printf("Error parsing v value for EOA %s in round %s: %v", operator.Hex(), round, err)
			allEOAsSubmitted = false
			continue
		}

		vs = append(vs, uint8(vValue))
		rs = append(rs, common.HexToHash(commitData.Sign.R))
		ss = append(ss, common.HexToHash(commitData.Sign.S))
	}
	// If all EOAs have submitted, trigger the random number generation transaction
	if allEOAsSubmitted {
		log.Printf("All EOAs have submitted for round %s with trail %s. Initiating random number generation.", round, trialNum)
		secretsOnChainValue, _ := n.GetSecretsOnChain(uniqueKey)

		if !secretsOnChainValue {
			cvOnChain, _ := n.GetCvOnChain(uniqueKey)
			if !cvOnChain {
				err = n.generateRandomNumberTransaction(ctx, round, trialNum, secrets, vs, rs, ss)
			} else {
				err = n.generateRandomNumberTransactionSomeCvOnChain(ctx, round, trialNum, secrets, vs, rs, ss)
			}
		}
		if err != nil {
			log.Printf("Failed to execute random number generation transaction for round %s with trail %s: %v", round, trialNum, err)
		} else {
			err = n.completeRound(ctx, round, trialNum)
			if err != nil {
				log.Printf("Failed to mark round %s as completed: %v", round, err)
			}
		}
	}
}

// Fetch activated operators for a specific round
func (n *LeaderNode) FetchActivatedOperators(ctx context.Context, fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string) ([]string, error) {
	var result []string
	activatedOperators, err := eth.Service.GetActivatedOperators(ctx, fallbackEthClient)
	if err != nil {
		log.Printf("Error fetching the activated operators %v", err)
		return result, err
	}
	strAddresses := make([]string, len(activatedOperators))
	for i, addr := range activatedOperators {
		strAddresses[i] = addr.Hex()
	}
	return strAddresses, nil
}

func (n *LeaderNode) LoadNodeData(ctx context.Context, round string, trialNum string) ([][]byte, [][]byte, [][]byte, []uint8, []common.Hash, []common.Hash) {

	leaderCommits, err := n.leaderCommitRepository.GetLeaderCommitsByRoundAndTrialNum(ctx, round, trialNum)
	if err != nil {
		log.Printf("Failed to load leader commits: %v", err)
	}

	// Sort leaderCommits based on ActivatedOperators order
	sortedLeaderCommits := sortLeaderCommitsByActivatedOperators(leaderCommits)

	// Collect secret values, signatures (v, r, s), and round info in the order of activated operators
	var secrets [][]byte
	var cos [][]byte
	var cvs [][]byte
	var vs []uint8
	var rs []common.Hash
	var ss []common.Hash

	for _, commitData := range sortedLeaderCommits {

		secrets = append(secrets, commitData.SecretValue[:])
		cvs = append(cvs, commitData.Cvs[:])
		cos = append(cos, commitData.Cos[:])

		if len(commitData.Sign.V) == 0 {
			log.Printf("Empty 'v' value for EOA %s in round %s", commitData.EOAAddress, round)
			vs = append(vs, 0)
		} else {
			vStr := commitData.Sign.V
			vValue, err := strconv.ParseUint(vStr, 10, 8)
			if err != nil {
				log.Printf("Error parsing v value for EOA %s in round %s: %v", commitData.EOAAddress, round, err)
				vs = append(vs, 0)
			} else {
				vs = append(vs, uint8(vValue))
			}
		}

		if len(commitData.Sign.R) == 0 {
			log.Printf("Empty 'r' value for EOA %s in round %s", commitData.EOAAddress, round)
			rs = append(rs, common.Hash{})
		} else {
			rs = append(rs, common.HexToHash(commitData.Sign.R))
		}

		if len(commitData.Sign.S) == 0 {
			log.Printf("Empty 's' value for EOA %s in round %s", commitData.EOAAddress, round)
			ss = append(ss, common.Hash{})
		} else {
			ss = append(ss, common.HexToHash(commitData.Sign.S))
		}
	}

	return cvs, cos, secrets, vs, rs, ss
}

// Updated function to use utils.LeaderCommitData
func sortLeaderCommitsByActivatedOperators(leaderCommits []*utils.LeaderCommitData) []*utils.LeaderCommitData {
	// Create a map for quick lookup of activated operators order
	activatedOperatorsOrder := eth.Service.GetActivatedOperatorsCached()

	log.Printf("Activated operators order: %v", activatedOperatorsOrder)

	// Create a map of leaderCommits by EOA address for quick lookup
	leaderCommitsMap := make(map[string]*utils.LeaderCommitData)
	for _, commit := range leaderCommits {
		leaderCommitsMap[commit.EOAAddress] = commit
	}

	// Create sorted array based on activated operators order
	var sortedLeaderCommits []*utils.LeaderCommitData

	for i, operator := range activatedOperatorsOrder {
		operatorAddr := operator.Hex()
		log.Printf("Looking for operator %s at position %d", operatorAddr, i)

		if commit, exists := leaderCommitsMap[operatorAddr]; exists {
			log.Printf("Found commit data for operator %s", operatorAddr)
			// No need to convert, already utils.LeaderCommitData
			sortedLeaderCommits = append(sortedLeaderCommits, commit)
		}
	}

	log.Printf("Sorted leader commits length: %d", len(sortedLeaderCommits))
	return sortedLeaderCommits
}

// generateRandomNumberTransaction sends a transaction to generate a random number for a round.
func (n *LeaderNode) generateRandomNumberTransaction(ctx context.Context, round string, trialNum string, secrets [][]byte, vs []uint8, rs []common.Hash, ss []common.Hash) error {
	if appconfig.Get().MockGenerateRandomNumber {
		appconfig.Get().MockGenerateRandomNumber = false
		log.Println("Random number generation is mocked via configuration. Skipping on-chain submission.")
		return nil
	}
	log.Printf("Preparing to execute generateRandomNumber...")

	privateKeyHex := appconfig.Get().LeaderPrivateKey
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to load leader private key: %v", err)
	}

	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to load contract ABI: %v", err)
	}
	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	type SigRS struct {
		R [32]byte
		S [32]byte
	}

	type SecretAndSigRS struct {
		Secret [32]byte
		Rs     SigRS
	}
	var secretSigRSs []SecretAndSigRS
	for i := range secrets {
		var secret [32]byte
		copy(secret[:], secrets[i])

		var r32, s32 [32]byte
		copy(r32[:], rs[i].Bytes())
		copy(s32[:], ss[i].Bytes())

		secretSigRSs = append(secretSigRSs, SecretAndSigRS{
			Secret: secret,
			Rs:     SigRS{R: r32, S: s32},
		})
	}

	roundRevealData, err := n.reavealOrderRepository.GetRevealOrder(ctx, round, trialNum)
	if err != nil {
		log.Printf("Failed to load reveal order: %v", err)
		return err
	}

	order := roundRevealData.RevealOrder
	packedRevealOrder := packRevealOrder(order)
	packedVs := packVsValues(vs)

	tx, _, err := eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"generateRandomNumber",
		big.NewInt(0),
		secretSigRSs,
		packedVs,
		packedRevealOrder,
	)
	if err != nil {
		return err
	}

	log.Printf("Transaction submitted. TX Hash: %s", tx.Hash().Hex())
	return nil
}

func (n *LeaderNode) generateRandomNumberTransactionSomeCvOnChain(ctx context.Context, round string, trialNum string, secrets [][]byte, vs []uint8, rs []common.Hash, ss []common.Hash) error {
	log.Printf("Preparing to execute generateRandomNumberTransactionSomeCvOnChain...")

	privateKeyHex := appconfig.Get().LeaderPrivateKey
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to load leader private key: %v", err)
	}

	contractAddressStr := appconfig.Get().ContractAddress
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to load contract ABI: %v", err)
	}
	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	type SigRS struct {
		R [32]byte
		S [32]byte
	}

	var sigRSArray []SigRS
	var vsArray []uint8
	for i := range vs {
		sigRSArray = append(sigRSArray, SigRS{R: rs[i], S: ss[i]})
		vsArray = append(vsArray, vs[i])
	}
	var allSecrets [][32]byte
	for i := range secrets {
		var secret [32]byte
		copy(secret[:], secrets[i])
		allSecrets = append(allSecrets, secret)
	}

	roundRevealData, err := n.reavealOrderRepository.GetRevealOrder(ctx, round, trialNum)
	if err != nil {
		log.Printf("Failed to load reveal order: %v", err)
		return err
	}

	order := roundRevealData.RevealOrder
	packedRevealOrder := packRevealOrder(order)
	packedVs := packVsValues(vsArray)
	tx, _, err := eth.Service.ExecuteTransaction(
		ctx,
		clientUtils,
		n.fallbackEthClient,
		"generateRandomNumberWhenSomeCvsAreOnChain",
		big.NewInt(0),
		allSecrets,
		sigRSArray,
		packedVs,
		packedRevealOrder,
	)
	if err != nil {
		return err
	}

	log.Printf("Transaction submitted. TX Hash: %s", tx.Hash().Hex())
	return nil
}

func packRevealOrder(order []int) *big.Int {
	packedRevealOrder := big.NewInt(0)
	for i, v := range order {
		shift := uint(8 * i)
		part := new(big.Int).Lsh(big.NewInt(int64(v)), shift)
		packedRevealOrder.Or(packedRevealOrder, part)
	}
	return packedRevealOrder
}

func packVsValues(vs []uint8) *big.Int {
	result := big.NewInt(0)
	for i, v := range vs {
		shift := uint(8 * i)
		part := new(big.Int).Lsh(big.NewInt(int64(v)), shift)
		result.Or(result, part)
	}
	return result
}

// completeRound updates to mark a round as completed and deletes old round data
func (n *LeaderNode) completeRound(ctx context.Context, round string, trialNum string) error {
	err := n.leaderCommitRepository.UpdateLeaderCommitRandomNumberGenerated(ctx, round, trialNum)
	if err != nil {
		return err
	}

	data, exists := n.GetRoundData(round)
	if !exists {
		data = RoundData{}
	}
	data.RandomNumber = true
	n.SetRoundData(round, data)

	return nil
}
