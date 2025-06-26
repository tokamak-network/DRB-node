package leaderNode_helper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

var CvOnChain bool

// MonitorCommits continuously checks for rounds where all EOAs have submitted their secret values.
func MonitorCommits(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	for {
		checkRoundsForCompletion(fallbackEthClient)
		time.Sleep(10 * time.Second) // Adjust the interval as needed
	}
}

var StartNextRound bool = true

type RevealOrderData struct {
	OrderedNodes []string   `json:"ordered_nodes"`
	RevealOrder  []*big.Int `json:"reveal_order"`
	RV           string     `json:"rv"`
}

type RevealOrders map[string]RevealOrderData

func checkRoundsForCompletion(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	// Fetch EOAs for each round
	eoasForRounds := getEOAsForRounds()

	for round := range eoasForRounds {
		// Load the leader commits for the round
		leaderCommits, err := loadLeaderCommits("leader_commits.json")
		if err != nil {
			log.Printf("Failed to load leader commits: %v", err)
			continue
		}

		// Check if the round has already generated a random number
		if isRoundCompleted(leaderCommits, round) {
			continue
		}

		// Check if the Merkle root has been submitted
		if !isMerkleRootSubmitted(leaderCommits, round) {
			log.Printf("Merkle root not submitted for round %s. Skipping random number generation.", round)
			continue
		}

		// Convert ActivatedOperator from []string to []common.Address
		var operatorAddresses []common.Address
		for _, operator := range ActivatedOperator {
			operatorAddresses = append(operatorAddresses, common.HexToAddress(operator))
		}

		// Collect secret values, signatures (v, r, s), and round info in the order of activated operators
		var secrets [][]byte
		var vs []uint8
		var rs []common.Hash
		var ss []common.Hash
		var index int
		allEOAsSubmitted := true
		for i, operator := range operatorAddresses {
			commitData, exists := leaderCommits[round+"+"+operator.Hex()]
			if !exists || commitData.SecretValue == [32]byte{} {
				log.Printf("EOA %s has not submitted a secret value for round %s.", operator.Hex(), round)
				allEOAsSubmitted = false
				break
			}

			secrets = append(secrets, commitData.SecretValue[:])
			// if Cv values are on-chain, than check this condition
			if CvOnChain {
				if index < len(Indices) && int64(i) <= Indices[index].Int64() {
					if int64(i) == Indices[index].Int64() {
						index++
						continue
					}
				}
			}

			// Ensure the signature map contains valid data
			if len(commitData.Sign["v"]) == 0 || len(commitData.Sign["r"]) == 0 || len(commitData.Sign["s"]) == 0 {
				log.Printf("Incomplete signature for EOA %s in round %s", operator.Hex(), round)
				allEOAsSubmitted = false
				continue
			}

			// Parse and validate signature components
			vStr := commitData.Sign["v"]
			vValue, err := strconv.ParseUint(vStr, 10, 8)
			if err != nil {
				log.Printf("Error parsing v value for EOA %s in round %s: %v", operator.Hex(), round, err)
				allEOAsSubmitted = false
				continue
			}

			vs = append(vs, uint8(vValue))
			rs = append(rs, common.HexToHash(commitData.Sign["r"]))
			ss = append(ss, common.HexToHash(commitData.Sign["s"]))
		}
		// If all EOAs have submitted, trigger the random number generation transaction
		if allEOAsSubmitted {
			log.Printf("All EOAs have submitted for round %s. Initiating random number generation.", round)
			var err error
			if !secretsOnChain[round] {
				if !CvOnChain {
					err = generateRandomNumberTransaction(fallbackEthClient, round, secrets, vs, rs, ss)
				} else {
					err = generateRandomNumberTransactionSomeCvOnChain(fallbackEthClient, round, secrets, vs, rs, ss)
				}
			}
			if err != nil {
				log.Printf("Failed to execute random number generation transaction for round %s: %v", round, err)
			} else {
				markRoundCompleted(leaderCommits, round)
			}
		}
	}
}

// isMerkleRootSubmitted checks if the Merkle root has been submitted for a given round.
func isMerkleRootSubmitted(leaderCommits map[string]utils.LeaderCommitData, round string) bool {
	for _, commitData := range leaderCommits {
		if commitData.Round == round {
			return commitData.SubmitMerkleRootDone
		}
	}
	return false
}

// Fetch activated operators for a specific round
func FetchActivatedOperators(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string) ([]string, error) {
	var result []string
	activatedOperators, err := eth.GetActivatedOperators(fallbackEthClient)
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

func LoadNodeData(round string) ([][]byte, [][]byte, [][]byte, []uint8, []common.Hash, []common.Hash) {
	leaderCommits, err := loadLeaderCommits("leader_commits.json")
	if err != nil {
		log.Printf("Failed to load leader commits: %v", err)

	}
	var operatorAddresses []common.Address
	for _, operator := range ActivatedOperator {
		operatorAddresses = append(operatorAddresses, common.HexToAddress(operator))
	}

	// Collect secret values, signatures (v, r, s), and round info in the order of activated operators
	var secrets [][]byte
	var cos [][]byte
	var cvs [][]byte
	var vs []uint8
	var rs []common.Hash
	var ss []common.Hash
	// var index int
	for _, operator := range operatorAddresses {
		commitData := leaderCommits[round+"+"+operator.Hex()]

		secrets = append(secrets, commitData.SecretValue[:])
		cvs = append(cvs, commitData.Cvs[:])
		cos = append(cos, commitData.Cos[:])

		if len(commitData.Sign["v"]) == 0 {
			log.Printf("Empty 'v' value for EOA %s in round %s", operator.Hex(), round)
			vs = append(vs, 0)
		} else {
			vStr := commitData.Sign["v"]
			vValue, err := strconv.ParseUint(vStr, 10, 8)
			if err != nil {
				log.Printf("Error parsing v value for EOA %s in round %s: %v", operator.Hex(), round, err)
				vs = append(vs, 0)
			} else {
				vs = append(vs, uint8(vValue))
			}
		}

		if len(commitData.Sign["r"]) == 0 {
			log.Printf("Empty 'r' value for EOA %s in round %s", operator.Hex(), round)
			rs = append(rs, common.Hash{})
		} else {
			rs = append(rs, common.HexToHash(commitData.Sign["r"]))
		}

		if len(commitData.Sign["s"]) == 0 {
			log.Printf("Empty 's' value for EOA %s in round %s", operator.Hex(), round)
			ss = append(ss, common.Hash{})
		} else {
			ss = append(ss, common.HexToHash(commitData.Sign["s"]))
		}

	}

	return cvs, cos, secrets, vs, rs, ss
}

// generateRandomNumberTransaction sends a transaction to generate a random number for a round.
func generateRandomNumberTransaction(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, secrets [][]byte, vs []uint8, rs []common.Hash, ss []common.Hash) error {
	log.Printf("Preparing to execute generateRandomNumber...")

	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to load leader private key: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
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

	revealOrders, err := loadRevealOrders("reveal_orders.json")
	if err != nil {
		log.Printf("Failed to load reveal orders: %v", err)
		return nil
	}

	roundRevealData, exists := revealOrders[round]
	if !exists {
		log.Printf("No reveal order found for round %s.", round)
		return nil
	}

	order := roundRevealData.RevealOrder
	packedRevealOrder := packRevealOrder(order)
	packedVs := packVsValues(vs)

	tx, _, err := eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
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

func generateRandomNumberTransactionSomeCvOnChain(fallbackEthClient *fallback_ethclient.FallbackRPCClient, round string, secrets [][]byte, vs []uint8, rs []common.Hash, ss []common.Hash) error {
	log.Printf("Preparing to execute generateRandomNumberTransactionSomeCvOnChain...")

	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("LEADER_PRIVATE_KEY is not set in environment variables.")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to load leader private key: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
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

	revealOrders, err := loadRevealOrders("reveal_orders.json")
	if err != nil {
		log.Printf("Failed to load reveal orders: %v", err)
		return nil
	}

	roundRevealData, exists := revealOrders[round]
	if !exists {
		log.Printf("No reveal order found for round %s.", round)
		return nil
	}
	order := roundRevealData.RevealOrder
	packedRevealOrder := packRevealOrder(order)
	packedVs := packVsValues(vsArray)
	tx, _, err := eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		fallbackEthClient,
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

func packRevealOrder(order []*big.Int) *big.Int {
	packedRevealOrder := big.NewInt(0)
	for i, v := range order {
		shift := uint(8 * i)
		part := new(big.Int).Lsh(big.NewInt(int64(v.Int64())), shift)
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

// markRoundCompleted updates the leader_commits.json file to mark a round as completed.
func markRoundCompleted(leaderCommits map[string]utils.LeaderCommitData, round string) {

	for key, commitData := range leaderCommits {
		if commitData.Round == round {
			commitData.RandomNumberGenerated = true
			leaderCommits[key] = commitData
		}
	}

	err := saveLeaderCommits("leader_commits.json", leaderCommits)
	if err != nil {
		log.Printf("Failed to save updated leader commits: %v", err)
	}

	if RoundsData == nil {
		RoundsData = make(map[string]RoundData)
	}
	data := RoundsData[round]
	data.RandomNumber = true
	RoundsData[round] = data
}

// isRoundCompleted checks if a round is already completed.
func isRoundCompleted(leaderCommits map[string]utils.LeaderCommitData, round string) bool {
	for _, commitData := range leaderCommits {
		if commitData.Round == round {
			return commitData.RandomNumberGenerated
		}
	}
	return false
}

// getEOAsForRounds fetches all EOAs for each round from leader_commits.json.
func getEOAsForRounds() map[string][]common.Address {
	eoasForRounds := make(map[string][]common.Address)

	leaderCommits, err := loadLeaderCommits("leader_commits.json")
	if err != nil {
		log.Printf("Failed to load leader commits: %v", err)
		return eoasForRounds
	}

	// Populate EOAs from leader commits
	for key := range leaderCommits {
		round, eoa := parseLeaderCommitKey(key)
		if round == "" || eoa == "" {
			continue
		}
		eoasForRounds[round] = appendIfNotExists(eoasForRounds[round], common.HexToAddress(eoa))
	}

	return eoasForRounds
}

// Helper: Parse leader commit key into round and EOA
func parseLeaderCommitKey(key string) (string, string) {
	split := len(key)
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '+' {
			split = i
			break
		}
	}
	if split == len(key) {
		return "", "" // Invalid key format
	}
	return key[:split], key[split+1:]
}

// Helper: Append EOA to a slice only if it doesn't already exist
func appendIfNotExists(slice []common.Address, eoa common.Address) []common.Address {
	for _, addr := range slice {
		if addr == eoa {
			return slice
		}
	}
	return append(slice, eoa)
}

// Helper: Load leader commits
func loadLeaderCommits(filePath string) (map[string]utils.LeaderCommitData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("leader commit file not found")
		}
		return nil, fmt.Errorf("failed to open leader commit file: %v", err)
	}
	defer file.Close()

	var data map[string]utils.LeaderCommitData
	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode leader commit file: %v", err)
	}

	return data, nil
}

// Helper: Save leader commits
func saveLeaderCommits(filePath string, data map[string]utils.LeaderCommitData) error {
	utils.LeaderCommitsMutex.Lock()
	defer utils.LeaderCommitsMutex.Unlock()

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create leader commit file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(data)
	if err != nil {
		return fmt.Errorf("failed to save leader commits: %v", err)
	}

	return nil
}
