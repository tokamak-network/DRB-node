package nodes

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
	"github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/machinebox/graphql"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
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
	Cvs [32]byte
}

// Local storage for commits and activated operators for each round
var committedNodes = make(map[string]map[common.Address]CommitData) // This now tracks the commit data (CVS) for each round and operator (EOA)
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

	var req utils.RegistrationRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode registration request: %v", err)
		return
	}

	if !utils.VerifySignature(req) {
		log.Printf("Failed to verify signature for PeerID: %s", req.PeerID)
		return
	}

	log.Printf("Verified registration for PeerID: %s", req.PeerID)

	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	operatorAddress := common.HexToAddress(req.EOAAddress)

	// Verify if the operator is activated
	activatedOperatorsResult, err := transactions.CallSmartContract(client, parsedABI, "getActivatedOperators", contractAddress)
	if err != nil {
		log.Printf("Failed to call getActivatedOperators: %v", err)
		return
	}

	activatedOperators := activatedOperatorsResult.([]common.Address)
	isActivated := false
	for _, operator := range activatedOperators {
		if operator == operatorAddress {
			isActivated = true
			break
		}
	}

	if isActivated {
		log.Println("Operator is already activated.")
		return
	}

	// Continue with contract interaction for activation
	depositAmountResult, err := transactions.CallSmartContract(client, parsedABI, "s_depositAmount", contractAddress, operatorAddress)
	if err != nil {
		log.Printf("Failed to call s_depositAmount: %v", err)
		return
	}
	depositAmount := depositAmountResult.(*big.Int)

	activationThresholdResult, err := transactions.CallSmartContract(client, parsedABI, "s_activationThreshold", contractAddress)
	if err != nil {
		log.Printf("Failed to call s_activationThreshold: %v", err)
		return
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	if depositAmount.Cmp(activationThreshold) < 0 {
		log.Printf("Deposit amount is insufficient. Deposit: %s, Threshold: %s", depositAmount, activationThreshold)
		return
	}

	// Prepare for transaction execution using ExecuteTransaction from transactions package
	privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode leader private key: %v", err)
	}

	clientUtils := &utils.Client{
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	_, _, err = transactions.ExecuteTransaction(
		context.Background(),
		clientUtils,
		"activate",
		big.NewInt(0),
		operatorAddress,
	)
	if err != nil {
		log.Printf("Failed to activate operator: %v", err)
		return
	}

	log.Println("Operator activated successfully.")
}

// Store commit locally only if the operator has sent a commit and is activated
func storeCommitLocally(roundNum string, eoaAddress common.Address, commitData utils.CommitRequest) {
	// Initialize the map for the round if it doesn't exist
	if _, exists := committedNodes[roundNum]; !exists {
		committedNodes[roundNum] = make(map[common.Address]CommitData)
	}

	// Store the commit status and CVS value for the EOA address
	if _, exists := committedNodes[roundNum][eoaAddress]; !exists {
		committedNodes[roundNum][eoaAddress] = CommitData{
			Cvs: commitData.Cvs, // Store CVS value from the commit request
		}
		log.Printf("Stored commit from %s for round %s with CVS %s", eoaAddress.Hex(), roundNum, commitData.Cvs)
	} else {
		log.Printf("Commit already received from %s for round %s. Ignoring duplicate commit.", eoaAddress.Hex(), roundNum)
	}
}

func handleCommitRequest(s network.Stream) {
    defer s.Close()

    // Decode the commit request from the stream
    var req utils.CommitRequest
    if err := json.NewDecoder(s).Decode(&req); err != nil {
        log.Printf("Failed to decode commit request: %v", err)
        return
    }

    roundNum := req.Round // roundNum is a string
    eoaAddress := common.HexToAddress(req.EOAAddress)

    log.Printf("Received commit for Round: %s", roundNum)
    log.Printf("CVS (bytes32): 0x%x\n", req.Cvs)
    log.Printf("EOA Address: %s", eoaAddress.Hex())

    // Check if the commit is valid (i.e., a valid EOA has actually sent the commit)
    if eoaAddress == common.HexToAddress("0x0000000000000000000000000000000000000000") {
        log.Printf("Ignoring commit from invalid EOA address %s", eoaAddress.Hex())
        return
    }

    // Only proceed if the EOA address is part of the activated operators for the current round
    if !isEOAActivatedForRound(roundNum, eoaAddress) {
        log.Printf("EOA address %s is not activated for round %s. Skipping commit.", eoaAddress.Hex(), roundNum)
        return
    }

    // Check if the EOA address has already sent a commit for this round (to prevent duplicate commits)
    if isCommitReceived(roundNum, eoaAddress) {
        log.Printf("Commit already received from %s for round %s. Ignoring duplicate commit.", eoaAddress.Hex(), roundNum)
        return
    }

    // Store the commit locally if it's valid
    storeCommitLocally(roundNum, eoaAddress, req)

    // After storing, check if all commits for the round have been received
    if allCommitsReceived(roundNum) {
        log.Printf("All activated operators have committed for round %s. Creating Merkle tree...", roundNum)

        // Generate Merkle root here
        var leaves [][]byte
        for _, commitData := range committedNodes[roundNum] {
            // Convert the CVS value (as bytes32) to []byte before appending to leaves
            leaves = append(leaves, commitData.Cvs[:]) // Use [:] to convert to slice of bytes
        }

        // Create Merkle tree with CVS values
        merkleRoot, err := commitreveal2.CREATE_MERKLE_TREE(leaves)
        if err != nil {
            log.Printf("Failed to create Merkle tree: %v", err)
            return
        }

        // Log the Merkle root in bytes32 format (as []byte)
        log.Printf("Merkle Root for Round %s: 0x%x", roundNum, merkleRoot)
		
        // Fetch the Merkle root from the contract
        client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
        if err != nil {
            log.Fatalf("Failed to connect to Ethereum client: %v", err)
        }

        contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
        parsedABI, err := utils.LoadContractABI(abiFilePath)
        if err != nil {
            log.Fatalf("Failed to load contract ABI: %v", err)
        }

        // Parse roundNum as an integer before passing it
        roundNumInt, err := strconv.ParseInt(roundNum, 10, 64)
        if err != nil {
            log.Printf("Failed to parse roundNum: %v", err)
            return
        }

        // Initialize the clientUtils (make sure it's correctly initialized with your client, contract address, and private key)
        privateKeyHex := os.Getenv("LEADER_PRIVATE_KEY")
        privateKey, err := crypto.HexToECDSA(privateKeyHex)
        if err != nil {
            log.Fatalf("Failed to decode leader private key: %v", err)
        }

        clientUtils := &utils.Client{
            Client:          client,              // Ethereum client
            ContractAddress: contractAddress,    // Contract address
            PrivateKey:      privateKey,          // Private key
            ContractABI:     parsedABI,           // Contract ABI
        }

		merkleRootBytes32 := [32]byte{}
   	 	copy(merkleRootBytes32[:], merkleRoot)

        // Execute the transaction to submit the Merkle root
        _, _, err = transactions.ExecuteTransaction(
            context.Background(),
            clientUtils,
            "submitMerkleRoot",  // The function name in the contract
            big.NewInt(0),
            big.NewInt(roundNumInt), // The round number as uint256
            merkleRootBytes32,            // The Merkle root as bytes32 (already correctly passed as a slice)
        )
        if err != nil {
            log.Printf("Failed to submit Merkle root for round %s: %v", roundNum, err)
            return
        }

        log.Printf("Successfully submitted Merkle root for round %s", roundNum)
    } else {
        log.Printf("Waiting for more commits for round %s", roundNum)
    }
}

// Check if the EOA address has already sent a commit for the round
func isCommitReceived(roundNum string, eoaAddress common.Address) bool {
	_, exists := committedNodes[roundNum][eoaAddress]
	return exists
}

// Check if the EOA address is activated for the specific round
func isEOAActivatedForRound(roundNum string, eoaAddress common.Address) bool {
	// Fetch the list of activated operators for the round
	activatedOperatorsForRound, exists := activatedOperators[roundNum]
	if !exists {
		log.Printf("No activated operators found for round %s", roundNum)
		return false
	}

	// Check if the EOA address is in the list of activated operators
	if _, activated := activatedOperatorsForRound[eoaAddress]; activated {
		log.Printf("EOA address %s is activated for round %s", eoaAddress.Hex(), roundNum)
		return true
	}

	log.Printf("EOA address %s is NOT activated for round %s", eoaAddress.Hex(), roundNum)
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
			// Only log the round status, no commits should be stored here
			log.Printf("Round %s is still waiting for commits", roundNum)
		}
	}
}

// Check if all commits for a round have been received
func allCommitsReceived(roundNum string) bool {
	// Check if all activated operators have committed
	for eoaAddress := range activatedOperators[roundNum] {
		if _, exists := committedNodes[roundNum][eoaAddress]; !exists {
			log.Printf("Missing commit from operator: %s for round %s", eoaAddress.Hex(), roundNum)
			return false
		}
	}
	return true
}
