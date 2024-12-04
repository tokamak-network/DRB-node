package leaderNode_helper

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tokamak-network/DRB-node/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

// MonitorCommits continuously checks for rounds where all EOAs have submitted their secret values.
func MonitorCommits() {
	for {
		checkRoundsForCompletion()
		time.Sleep(10 * time.Second) // Adjust the interval as needed
	}
}

func checkRoundsForCompletion() {
	eoasForRounds := getEOAsForRounds()

	for round, eoas := range eoasForRounds {
		// Load the reveal orders and leader commits for the round
		leaderCommits, err := loadLeaderCommits("leader_commits.json")
		if err != nil {
			log.Printf("Failed to load leader commits: %v", err)
			continue
		}

		// Check if the round has already generated a random number
		if isRoundCompleted(leaderCommits, round) {
			// log.Printf("Round %s is already completed. Skipping.", round)
			continue
		}

		// Collect secret values, signatures (v, r, s), and round info
		var secrets [][]byte
		var vs []uint8
		var rs []common.Hash
		var ss []common.Hash

		allEOAsSubmitted := true
		for _, eoa := range eoas {
			commitData, exists := leaderCommits[round+"+"+eoa.Hex()]
			if !exists || commitData.SecretValue == [32]byte{} {
				log.Printf("EOA %s has not submitted a secret value for round %s", eoa.Hex(), round)
				allEOAsSubmitted = false
				break
			}

			// Ensure the signature map contains valid data
			if len(commitData.Sign["v"]) == 0 || len(commitData.Sign["r"]) == 0 || len(commitData.Sign["s"]) == 0 {
				log.Printf("Incomplete signature for EOA %s in round %s", eoa.Hex(), round)
				allEOAsSubmitted = false
				break
			}

			secrets = append(secrets, commitData.SecretValue[:])
			vs = append(vs, commitData.Sign["v"][0])
			rs = append(rs, common.HexToHash(commitData.Sign["r"]))
			ss = append(ss, common.HexToHash(commitData.Sign["s"]))
		}

		// If all EOAs have submitted, trigger the random number generation transaction
		if allEOAsSubmitted {
			log.Printf("All EOAs have submitted for round %s. Initiating random number generation.", round)
			err := generateRandomNumberTransaction(round, secrets, vs, rs, ss)
			if err != nil {
				log.Printf("Failed to execute random number generation transaction for round %s: %v", round, err)
			} else {
				markRoundCompleted(leaderCommits, round)
			}
		}
	}
}

// generateRandomNumberTransaction sends a transaction to generate a random number for a round.
func generateRandomNumberTransaction(round string, secrets [][]byte, vs []uint8, rs []common.Hash, ss []common.Hash) error {
	log.Printf("Preparing to execute generateRandomNumber...")

	// Convert `secrets` from [][]byte to []common.Hash
	var secretsHashes []common.Hash
	for _, secret := range secrets {
		var secretHash common.Hash
		copy(secretHash[:], secret)

		// Add "0x" prefix for logs
		log.Printf("Adding secret hash: 0x%s", hex.EncodeToString(secretHash[:]))
		secretsHashes = append(secretsHashes, secretHash)
	}

	// Prepare the round number
	roundNum, ok := new(big.Int).SetString(round, 10)
	if !ok {
		return fmt.Errorf("invalid round number: %s", round)
	}

	// Load Ethereum client and private key
	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		return fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}
	defer client.Close()

	privateKey, err := crypto.HexToECDSA(os.Getenv("LEADER_PRIVATE_KEY"))
	if err != nil {
		return fmt.Errorf("failed to load leader private key: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI("contract/abi/Commit2RevealDRB.json")
	if err != nil {
		return fmt.Errorf("failed to load contract ABI: %v", err)
	}

	clientUtils := &utils.Client{
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	// Prepare the function call to generateRandomNumber
	tx, receipt, err := transactions.ExecuteTransaction(
		context.Background(),
		clientUtils,
		"generateRandomNumber",
		big.NewInt(0),        // No Ether value
		roundNum,             // uint256 round
		secretsHashes,        // bytes32[] secrets
		vs,                   // uint8[] vs
		rs,                   // bytes32[] rs
		ss,                   // bytes32[] ss
	)

	if err != nil {
		return fmt.Errorf("failed to execute generateRandomNumber transaction: %v", err)
	}

	log.Printf("Transaction submitted. TX Hash: %s", tx.Hash().Hex())
	log.Printf("Transaction receipt: %+v", receipt)
	return nil
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

// getEOAsForRounds fetches all EOAs for each round from reveal_orders.json and leader_commits.json.
func getEOAsForRounds() map[string][]common.Address {
	eoasForRounds := make(map[string][]common.Address)

	revealOrders, err := loadRevealOrders("reveal_orders.json")
	if err != nil {
		log.Printf("Failed to load reveal orders: %v", err)
		return eoasForRounds
	}

	leaderCommits, err := loadLeaderCommits("leader_commits.json")
	if err != nil {
		log.Printf("Failed to load leader commits: %v", err)
		return eoasForRounds
	}

	// Fetch EOAs from reveal orders
	for round, roundData := range revealOrders {
		orderedNodes, ok := roundData["ordered_nodes"].([]interface{})
		if !ok {
			log.Printf("Invalid ordered_nodes format in round %s", round)
			continue
		}

		for _, eoa := range orderedNodes {
			eoaStr, ok := eoa.(string)
			if !ok {
				continue
			}
			eoasForRounds[round] = append(eoasForRounds[round], common.HexToAddress(eoaStr))
		}
	}

	// Add any additional EOAs from leader commits
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

// Helper: Load reveal orders
func loadRevealOrders(filePath string) (map[string]map[string]interface{}, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("reveal order file not found")
		}
		return nil, fmt.Errorf("failed to open reveal order file: %v", err)
	}
	defer file.Close()

	var data map[string]map[string]interface{}
	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode reveal order file: %v", err)
	}

	return data, nil
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
