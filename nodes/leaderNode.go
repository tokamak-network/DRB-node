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
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/machinebox/graphql"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/nodes/leaderNode_helper"
	"github.com/tokamak-network/DRB-node/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

// Struct for the GraphQL response
type RoundData struct {
	MerkleRootSubmitted struct {
		MerkleRoot interface{} `json:"merkleRoot"`
	} `json:"merkleRootSubmitted"`
	Round interface{} `json:"round"`
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

type CommitData struct {
    Cvs       [32]byte `json:"cvs"`
    CvsHex    string   `json:"cvs_hex,omitempty"`  // Add the CvsHex field to store the hex string
}

// Local storage for commits and activated operators for each round
var committedNodes = make(map[string]map[common.Address]utils.LeaderCommitData)
var activatedOperators = make(map[string]map[common.Address]bool) // Tracks activated operators for each round

// RunLeaderNode starts the leader node and listens for registration and commit requests
func RunLeaderNode() {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT not set in environment variables.")
	}

	// Check if PeerID already exists, if not create and save it
	privKey, peerID, err := utils.LoadPeerID() 
	if err != nil {
		log.Println("PeerID not found, generating new one.")
		privKey, _, err = libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 0)
		if err != nil {
			log.Fatalf("Failed to generate libp2p private key: %v", err)
		}

		err = utils.SavePeerID(privKey)
		if err != nil {
			log.Fatalf("Failed to save PeerID: %v", err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			log.Fatalf("Failed to get PeerID from private key: %v", err)
		}
	}

	log.Printf("Loaded or generated PeerID: %s", peerID.String())

	// Create the libp2p host
	h, err := libp2p.New(libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)), libp2p.Identity(privKey))
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer h.Close()

	h.SetStreamHandler("/register", func(s network.Stream) {
		handleRegistrationRequest(s)
	})

	h.SetStreamHandler("/commit", func(s network.Stream) {
		handleCommitRequest(s)
	})

	h.SetStreamHandler("/cos", func(s network.Stream) {
		handleCOSRequest(h, s) // New stream handler for COS
	})

	h.SetStreamHandler("/secretValue", func(s network.Stream) {
		leaderNode_helper.AcceptSecretValue(h, s) // New stream handler for COS
	})

	log.Printf("Leader node is running on addresses: %s\n", h.Addrs())
	log.Printf("Leader node PeerID: %s\n", peerID.String())

	// Continuously call fetchRoundsData() every 30 seconds
	for {
		roundsData, err := fetchRoundsData()
		if err != nil {
			log.Printf("Error fetching rounds data: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// Process the rounds and commit data
		processRounds(roundsData)

		// Wait 30 seconds before fetching the data again
		time.Sleep(30 * time.Second)
	}
}

// Function to fetch rounds data using GraphQL query
func fetchRoundsData() (*GraphQLResponse, error) {
	// Create a new GraphQL client (adjust the URL to your GraphQL server)
	client := graphql.NewClient(os.Getenv("SUBGRAPH_URL"))

	// Create a context
	ctx := context.Background()

	// Create the request object
	req := utils.GetRoundsRequest()

	// Execute the request
	var resp GraphQLResponse
	if err := client.Run(ctx, req, &resp); err != nil {
		log.Fatalf("Failed to execute GraphQL request: %v", err)
	}

	// Return the response containing round data
	return &resp, nil
}

func handleRegistrationRequest(s network.Stream) {
	defer s.Close()

	// Path to the JSON file where registered nodes are stored
	filePath := "registered_nodes.json"

	// Use the helper function to handle the registration
	err := leaderNode_helper.RegisterNode(s, filePath, abiFilePath)
	if err != nil {
		log.Printf("Failed to handle registration request: %v", err)
		return
	}

	log.Println("Node registration and activation completed successfully.")
}

// Handle commit request from regular nodes
func handleCommitRequest(s network.Stream) {
    defer s.Close()

    var req utils.CommitRequest
    if err := json.NewDecoder(s).Decode(&req); err != nil {
        log.Printf("Failed to decode commit request: %v", err)
        return
    }

    roundNum := req.Round
    eoaAddress := common.HexToAddress(req.EOAAddress)

    log.Printf("Received commit for Round: %s", roundNum)
    log.Printf("CVS (bytes32): 0x%x\n", req.Cvs)
    log.Printf("EOA Address: %s", eoaAddress.Hex())

    // Check if the EOA address is activated for the current round
    if !isEOAActivatedForRound(roundNum, eoaAddress) {
        log.Printf("EOA address %s is NOT activated for round %s. Skipping commit.", eoaAddress.Hex(), roundNum)
        return
    }

    // Load existing commit data or initialize a new one
    commitData, err := utils.LoadLeaderCommitData(roundNum, eoaAddress.Hex())
    if err != nil {
        commitData = &utils.LeaderCommitData{
            Round:      roundNum,
            EOAAddress: eoaAddress.Hex(),
        }
    }

    // Save the CVS value
    if commitData.Cvs == [32]byte{} {
        commitData.Cvs = req.Cvs
        log.Printf("Storing CVS for round %s from %s", roundNum, eoaAddress.Hex())
    }

    // Persist commit data
    if err := utils.SaveLeaderCommitData(*commitData); err != nil {
        log.Printf("Failed to save commit data: %v", err)
        return
    }

    // Optionally update the in-memory map
    if _, exists := committedNodes[roundNum]; !exists {
        committedNodes[roundNum] = make(map[common.Address]utils.LeaderCommitData)
    }
    committedNodes[roundNum][eoaAddress] = *commitData
}

// Handle COS commit request from regular nodes
func handleCOSRequest(h host.Host, s network.Stream) {
	defer s.Close()

	var req utils.CosRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode COS commit request: %v", err)
		return
	}

	roundNum := req.Round
	eoaAddress := common.HexToAddress(req.EOAAddress)

	log.Printf("Received COS for Round: %s", roundNum)
	log.Printf("COS (bytes32): 0x%x\n", req.Cos)
	log.Printf("EOA Address: %s", eoaAddress.Hex())

	// Check if the EOA address is activated for the current round
	if !isEOAActivatedForRound(roundNum, eoaAddress) {
		log.Printf("EOA address %s is NOT activated for round %s. Skipping COS.", eoaAddress.Hex(), roundNum)
		return
	}

	// Load the leader commit data for the round and EOA
	commitData, err := utils.LoadLeaderCommitData(roundNum, eoaAddress.Hex())
	if err != nil || commitData.Cvs == [32]byte{} {
		// Reject the COS request if no CVS exists for this round and EOA
		log.Printf("No existing CVS found for EOA address %s in round %s. Rejecting COS commit.", eoaAddress.Hex(), roundNum)
		return
	}

	// Recalculate the CVS by hashing the received COS value
	recalculatedCvs := commitreveal2.Keccak256(req.Cos[:])

	// Compare recalculated CVS with the stored CVS
	if !bytes.Equal(recalculatedCvs, commitData.Cvs[:]) {
		log.Printf("COS hash does not match CVS for EOA address %s in round %s. Rejecting COS commit.", eoaAddress.Hex(), roundNum)
		return
	}

	// Check if COS is already stored for this round and EOA
	if commitData.Cos != [32]byte{} {
		// COS has already been stored for this EOA and round
		log.Printf("COS already received for round %s from %s. Skipping save.", roundNum, eoaAddress.Hex())
		return
	}

	// Store COS
	commitData.Cos = req.Cos
	log.Printf("Storing COS for round %s from %s", roundNum, eoaAddress.Hex())

	// Convert the COS byte array to a hex string and store it in CosHex
	commitData.CosHex = hex.EncodeToString(commitData.Cos[:]) // Convert the byte array to hex string

	// Save the updated commit data to the file
	if err := utils.SaveLeaderCommitData(*commitData); err != nil {
		log.Printf("Failed to save commit data: %v", err)
		return
	}

	// Now, populate the committedNodes map with LeaderCommitData
	if _, exists := committedNodes[roundNum]; !exists {
		committedNodes[roundNum] = make(map[common.Address]utils.LeaderCommitData)
	}
	committedNodes[roundNum][eoaAddress] = *commitData // Store LeaderCommitData

	// Log the COS has been stored successfully
	log.Printf("COS stored for round %s from %s", roundNum, eoaAddress.Hex())

	// Check if all COS values are received for this round
	if allCommitsReceived(roundNum, "COS") {
		log.Printf("All COS received for round %s. Determining reveal order...", roundNum)
		// Determine the reveal order and save it
		err := commitreveal2.DetermineRevealOrder(roundNum, activatedOperators)
		if err != nil {
			log.Printf("Failed to determine reveal order for round %s: %v", roundNum, err)
			return
		}

		log.Printf("Reveal order successfully determined and stored for round %s.", roundNum)

		// Start requesting secret values sequentially
		leaderNode_helper.StartSecretValueRequests(h, roundNum)
	}
}

func generateMerkleRoot(roundNum string) {
    log.Printf("Generating Merkle root for round %s...", roundNum)

    // Ensure activated operators exist for the round
    operators, exists := activatedOperators[roundNum]
    if !exists || len(operators) == 0 {
        log.Printf("No activated operators found for round %s. Cannot generate Merkle root.", roundNum)
        return
    }

    var leaves [][]byte
    for eoaAddress := range operators {
        // Load commit data from file for each operator
        commitData, err := utils.LoadLeaderCommitData(roundNum, eoaAddress.Hex())
        if err != nil {
            log.Printf("Failed to load CVS for operator %s in round %s: %v", eoaAddress.Hex(), roundNum, err)
            continue
        }

        if commitData.Cvs == [32]byte{} {
            log.Printf("Missing CVS for operator %s in round %s", eoaAddress.Hex(), roundNum)
            continue
        }

        leaves = append(leaves, commitData.Cvs[:])
        log.Printf("Added CVS from operator %s for round %s", eoaAddress.Hex(), roundNum)
    }

    if len(leaves) == 0 {
        log.Printf("Error: No CVS commits found for round %s. Cannot generate Merkle root.", roundNum)
        return
    }

    // Create Merkle tree with CVS values
    merkleRoot, err := commitreveal2.CREATE_MERKLE_TREE(leaves)
    if err != nil {
        log.Printf("Failed to create Merkle tree for round %s: %v", roundNum, err)
        return
    }

    // Submit the Merkle root
    submitMerkleRoot(roundNum, merkleRoot)
}

// Submit the Merkle root to the smart contract
func submitMerkleRoot(roundNum string, merkleRoot []byte) {
    // Convert the merkleRoot to a [32]byte, as the contract expects a bytes32 type
    var merkleRootBytes32 [32]byte
    copy(merkleRootBytes32[:], merkleRoot)

    // Connect to the Ethereum client
    client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
    if err != nil {
        log.Printf("Failed to connect to Ethereum client: %v", err)
        return
    }

    // Load the contract ABI
    contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
    parsedABI, err := utils.LoadContractABI(abiFilePath)
    if err != nil {
        log.Printf("Failed to load contract ABI: %v", err)
        return
    }

    // Parse roundNum as an integer (if necessary)
    roundNumInt, err := strconv.ParseInt(roundNum, 10, 64)
    if err != nil {
        log.Printf("Failed to parse roundNum: %v", err)
        return
    }

    // Prepare the transaction
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

    // Execute the contract function to submit the Merkle root
    _, _, err = transactions.ExecuteTransaction(
        context.Background(),
        clientUtils,
        "submitMerkleRoot",  // The function name in the contract
        big.NewInt(0),       // Any necessary value (e.g., gas)
        big.NewInt(roundNumInt), // Round number
        merkleRootBytes32,   // Merkle root (as bytes32)
    )
    if err != nil {
        log.Printf("Failed to submit Merkle root for round %s: %v", roundNum, err)
        return
    }

    log.Printf("Successfully submitted Merkle root for round %s", roundNum)

    // After submitting the Merkle root, update the commit data to set submit_merkle_root_done to true
    updateCommitDataAfterSubmit(roundNum)
}

// Function to update commit data after submitting the Merkle root
func updateCommitDataAfterSubmit(roundNum string) {
    // Get the list of all EOA addresses that have committed
    for eoaAddress := range committedNodes[roundNum] {
        // Load the commit data
        commitData, err := utils.LoadLeaderCommitData(roundNum, eoaAddress.Hex())
        if err != nil {
            log.Printf("Error loading commit data for %s in round %s: %v", eoaAddress.Hex(), roundNum, err)
            continue
        }

        // Set submit_merkle_root_done to true
        commitData.SubmitMerkleRootDone = true
        log.Printf("Setting submit_merkle_root_done = true for key: %s+%s", roundNum, eoaAddress.Hex())

        // Save the updated commit data to the file
        if err := utils.SaveLeaderCommitData(*commitData); err != nil {
            log.Printf("Failed to save updated commit data for %s in round %s: %v", eoaAddress.Hex(), roundNum, err)
        }
    }
}

// Check if the EOA address is activated for the specific round
func isEOAActivatedForRound(roundNum string, eoaAddress common.Address) bool {
	// Convert roundNum to integer
	roundInt, err := strconv.Atoi(roundNum) // Convert string roundNum to integer
	if err != nil {
		log.Printf("Invalid round number %s: %v", roundNum, err)
		return false
	}

	// Create a new GraphQL client (adjust the URL to your GraphQL server)
	client := graphql.NewClient(os.Getenv("SUBGRAPH_URL"))

	// Create the request object using the updated query with the round number
	req := utils.GetActivatedOperatorsAtRoundRequest(roundInt) // Pass integer round

	// Execute the request
	var resp map[string]interface{}
	ctx := context.Background()
	err = client.Run(ctx, req, &resp)
	if err != nil {
		log.Printf("Failed to execute GraphQL request for activated operators in round %d: %v", roundInt, err)
		return false
	}

	// Extract the activated operators from the response
	activatedOperatorsData, ok := resp["randomNumberRequesteds"].([]interface{})
	if !ok || len(activatedOperatorsData) == 0 {
		log.Printf("No activated operators found for round %d", roundInt)
		return false
	}

	// Extract the list of activated operators
	activatedOperators := activatedOperatorsData[0].(map[string]interface{})["activatedOperators"].([]interface{})
	for _, operator := range activatedOperators {
		operatorAddress := common.HexToAddress(operator.(string))
		if operatorAddress == eoaAddress {
			log.Printf("EOA address %s is activated for round %d", eoaAddress.Hex(), roundInt)
			return true
		}
	}

	log.Printf("EOA address %s is NOT activated for round %d", eoaAddress.Hex(), roundInt)
	return false
}

// Function to process rounds and track commits
func processRounds(roundsData *GraphQLResponse) {
	for _, round := range roundsData.Rounds {
		roundNum, ok := round.Round.(string) // Type assertion to string
		if !ok {
			log.Printf("Error: Round is not of type string.")
			continue
		}

		// Initialize activated operators for the round if not already done
		if len(round.RandomNumberRequested.ActivatedOperators) > 0 {
			if _, exists := activatedOperators[roundNum]; !exists {
				activatedOperators[roundNum] = make(map[common.Address]bool)
			}

			for _, operator := range round.RandomNumberRequested.ActivatedOperators {
				// Skip the zero address
				operatorAddress := common.HexToAddress(operator)
				if operatorAddress == common.HexToAddress("0x0000000000000000000000000000000000000000") {
					continue // Skip the zero address
				}

				activatedOperators[roundNum][operatorAddress] = true
			}
		}

		// Check if Merkle Root and Random Number are still nil
		if round.MerkleRootSubmitted.MerkleRoot == nil && round.RandomNumberGenerated.RandomNumber == nil {
			log.Printf("Round %s is still waiting for commits", roundNum)

			// Check leader_commits.json to verify if all CVS are received
			if allCommitsReceived(roundNum, "CVS") {
				log.Printf("All CVS received for round %s. Generating Merkle root...", roundNum)
				generateMerkleRoot(roundNum) // Generate and submit Merkle root
			} else {
				log.Printf("Not all CVS received for round %s. Waiting for remaining commits.", roundNum)
			}
		}
	}
}

// Check if all commits for a round have been received (CVS, COS, Secret)
func allCommitsReceived(roundNum string, phase string) bool {
    log.Printf("Checking if all commits are received for round %s, phase: %s", roundNum, phase)

    // Ensure activated operators exist for the round
    operators, exists := activatedOperators[roundNum]
    if !exists || len(operators) == 0 {
        log.Printf("No activated operators found for round %s", roundNum)
        return false
    }

    missingOperators := []string{}
    for eoaAddress := range operators {
        // Load commit data from file for each operator
        commitData, err := utils.LoadLeaderCommitData(roundNum, eoaAddress.Hex())
        if err != nil || commitData.Cvs == [32]byte{} {
            missingOperators = append(missingOperators, eoaAddress.Hex())
        }
    }

    if len(missingOperators) > 0 {
        log.Printf("Missing CVS for round %s from operators: %v", roundNum, missingOperators)
        return false
    }

    log.Printf("All CVS received for round %s", roundNum)
    return true
}
