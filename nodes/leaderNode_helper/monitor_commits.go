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
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

// MonitorCommits continuously checks for rounds where all EOAs have submitted their secret values.
func MonitorCommits(h host.Host) {
    for {
        checkRoundsForCompletion(h)
        time.Sleep(10 * time.Second) // Adjust the interval as needed
    }
}

func checkRoundsForCompletion(h host.Host) {
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

        // Fetch activated operators for the round
        activatedOperators, err := FetchActivatedOperators(round)
        if err != nil {
            log.Printf("Failed to fetch activated operators for round %s: %v", round, err)
            continue
        }

        // Filter out the `0x0000000000000000000000000000000000000000` address
        filteredOperators := filterOperators(activatedOperators)

        // Convert filteredOperators from []string to []common.Address
        var operatorAddresses []common.Address
        for _, operator := range filteredOperators {
            operatorAddresses = append(operatorAddresses, common.HexToAddress(operator))
        }

        // Collect secret values, signatures (v, r, s), and round info in the order of activated operators
        var secrets [][]byte
        var vs []uint8
        var rs []common.Hash
        var ss []common.Hash

        allEOAsSubmitted := true
        for _, operator := range operatorAddresses {
            commitData, exists := leaderCommits[round+"+"+operator.Hex()]
            if !exists || commitData.SecretValue == [32]byte{} {
                log.Printf("EOA %s has not submitted a secret value for round %s. Initiating request.", operator.Hex(), round)

                // Initiate a request for the missing secret value
                nodeInfo, err := fetchNodeInfo(operator.Hex())
                if err != nil {
                    log.Printf("Failed to fetch node info for EOA %s: %v", operator.Hex(), err)
                    continue
                }

                sendSecretValueRequestToNode(h, round, operator.Hex(), nodeInfo)
                allEOAsSubmitted = false
                break
            }

            // Ensure the signature map contains valid data
            if len(commitData.Sign["v"]) == 0 || len(commitData.Sign["r"]) == 0 || len(commitData.Sign["s"]) == 0 {
                log.Printf("Incomplete signature for EOA %s in round %s", operator.Hex(), round)
                allEOAsSubmitted = false
                break
            }

            // Parse and validate signature components
            vStr := commitData.Sign["v"]
            vValue, err := strconv.ParseUint(vStr, 10, 8)
            if err != nil {
                log.Printf("Error parsing v value for EOA %s in round %s: %v", operator.Hex(), round, err)
                allEOAsSubmitted = false
                break
            }

            secrets = append(secrets, commitData.SecretValue[:])
            vs = append(vs, uint8(vValue))
            rs = append(rs, common.HexToHash(commitData.Sign["r"]))
            ss = append(ss, common.HexToHash(commitData.Sign["s"]))
        }

        // If all EOAs have submitted, trigger the random number generation transaction
        if allEOAsSubmitted {
            log.Printf("All EOAs have submitted for round %s. Initiating random number generation.", round)
            err := generateRandomNumberTransaction(round, secrets, vs, rs, ss, operatorAddresses)
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

func fetchNodeInfo(eoa string) (NodeInfo, error) {
    filePath := "registered_nodes.json"
    nodes, err := LoadRegisteredNodes(filePath)
    if err != nil {
        return NodeInfo{}, fmt.Errorf("failed to load registered nodes: %v", err)
    }

    nodeInfo, exists := nodes[eoa]
    if !exists {
        return NodeInfo{}, fmt.Errorf("node info for EOA %s not found", eoa)
    }

    return nodeInfo, nil
}


// Helper: Filter out `0x0000000000000000000000000000000000000000` from the list of operators.
func filterOperators(operators []string) []string {
	var filtered []string
	for _, operator := range operators {
		if operator != "0x0000000000000000000000000000000000000000" {
			filtered = append(filtered, operator)
		}
	}
	return filtered
}

// Fetch activated operators for a specific round
func FetchActivatedOperators(round string) ([]string, error) {
    subGraphURL := os.Getenv("SUBGRAPH_URL")
	if subGraphURL == "" {
		log.Fatal("SUBGRAPH_URL is not set in environment variables.")
	}
	client := graphql.NewClient(subGraphURL)
	req := utils.GetActivatedOperatorsAtRoundRequest(roundToInt(round))

	var resp struct {
		RandomNumberRequesteds []struct {
			ActivatedOperators []string `json:"activatedOperators"`
		} `json:"randomNumberRequesteds"`
	}

	err := client.Run(context.Background(), req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch activated operators: %v", err)
	}

	if len(resp.RandomNumberRequesteds) == 0 {
		return nil, fmt.Errorf("no activated operators found for round %s", round)
	}

	return resp.RandomNumberRequesteds[0].ActivatedOperators, nil
}

// Helper: Convert round string to int
func roundToInt(round string) int {
	roundInt, _ := strconv.Atoi(round)
	return roundInt
}

// generateRandomNumberTransaction sends a transaction to generate a random number for a round.
func generateRandomNumberTransaction(round string, secrets [][]byte, vs []uint8, rs []common.Hash, ss []common.Hash, eoas []common.Address) error {
    log.Printf("Preparing to execute generateRandomNumber...")

    // Convert `secrets` from [][]byte to []common.Hash
    var secretsHashes []common.Hash
    for _, secret := range secrets {
        var secretHash common.Hash
        copy(secretHash[:], secret)
        secretsHashes = append(secretsHashes, secretHash)
    }

    // Check if secretsHashes, vs, rs, or ss are empty
    if len(secretsHashes) == 0 || len(vs) == 0 || len(rs) == 0 || len(ss) == 0 {
        return fmt.Errorf("one or more of the required arrays (secretsHashes, vs, rs, ss) are empty")
    }

    // Debugging: Log EOA order
    for i, eoa := range eoas {
        log.Printf("EOA Position %d: %s", i+1, eoa.Hex())
    }

    // Prepare the round number
    roundNum, ok := new(big.Int).SetString(round, 10)
    if !ok {
        return fmt.Errorf("invalid round number: %s", round)
    }

    // Load Ethereum client and private key
    ethRPCURL := os.Getenv("ETH_RPC_URL")
	if ethRPCURL == "" {
		log.Fatal("ETH_RPC_URL is not set in environment variables.")
	}
    client, err := ethclient.Dial(ethRPCURL)
    if err != nil {
        return fmt.Errorf("failed to connect to Ethereum client: %v", err)
    }
    defer client.Close()

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
        Client:          client,
        ContractAddress: contractAddress,
        PrivateKey:      privateKey,
        ContractABI:     parsedABI,
    }

    // Debugging: Log all inputs before executing the transaction
    log.Printf("Secrets: %v", secretsHashes)
    log.Printf("VS: %v", vs)
    log.Printf("RS: %v", rs)
    log.Printf("SS: %v", ss)

    // Prepare the function call to generateRandomNumber
    tx, _, err := eth.ExecuteTransaction(
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
        return err
    }

    log.Printf("Transaction submitted. TX Hash: %s", tx.Hash().Hex())
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
