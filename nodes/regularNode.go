package nodes

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/libp2p/go-libp2p"
	core "github.com/libp2p/go-libp2p/core"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/nodes/regularNode_helper"
	"github.com/tokamak-network/DRB-node/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

const abiFilePath = "contract/abi/Commit2RevealDRB.json"

// RunRegularNode handles the behavior for a regular node
func RunRegularNode() {
	ctx := context.Background()

	// Check if PeerID already exists, if not create and save it
	privKey, peerID, err := utils.LoadPeerID()
	if err != nil {
		log.Println("PeerID not found, generating new one.")

		// Generate a new deterministic libp2p private key (for example, from a fixed seed)
		privKey, _, err = libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, 0)
		if err != nil {
			log.Fatalf("Failed to generate libp2p private key: %v", err)
		}

		// Save the generated PeerID for future restarts
		err = utils.SavePeerID(privKey)
		if err != nil {
			log.Fatalf("Failed to save PeerID: %v", err)
		}

		peerID, err = peer.IDFromPrivateKey(privKey)
		if err != nil {
			log.Fatalf("Failed to get PeerID from private key: %v", err)
		}
	}

	// Create the libp2p host with the loaded or generated PeerID
	h, err := libp2p.New(libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", os.Getenv("PORT"))), libp2p.Identity(privKey))
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer h.Close()

	h.SetStreamHandler("/sendSecretValue", func(s network.Stream) {
		regularNode_helper.HandleSecretValueRequest(h, s)
	})

	// Get leader's multiaddress
	leaderIP := os.Getenv("LEADER_IP")
	leaderPort := os.Getenv("LEADER_PORT")
	leaderPeerID := os.Getenv("LEADER_PEER_ID")

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}

	// The Ethereum private key is used separately for Ethereum transactions
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode Ethereum private key: %v", err)
	}

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	log.Printf("EOA Address: %s", eoaAddress)

	// Get the local IP address of the node
	ip := utils.GetLocalIP() // Use dynamic IP retrieval
	port := os.Getenv("PORT")

	// Save the node's information (IP, Port, PeerID, EOA address)
	nodeInfo := utils.NodeInfo{
		IP:         ip,
		Port:       port,
		PeerID:     peerID.String(),
		EOAAddress: eoaAddress,
	}

	if err := utils.SaveNodeInfo([]utils.NodeInfo{nodeInfo}); err != nil {
		log.Printf("Failed to save node info: %v", err)
	}

	// Connect to the leader
	leaderAddrString := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	log.Printf("Leader multiaddress: %s", leaderAddrString)

	leaderAddr, err := multiaddr.NewMultiaddr(leaderAddrString)
	if err != nil {
		log.Fatalf("Failed to parse leader multiaddress: %v", err)
	}

	leaderInfo, err := peer.AddrInfoFromP2pAddr(leaderAddr)
	if err != nil {
		log.Fatalf("Failed to create peer info from leader multiaddress: %v", err)
	}

	h.Peerstore().AddAddrs(leaderInfo.ID, leaderInfo.Addrs, peerstore.PermanentAddrTTL)

	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	clientUtils := &utils.Client{
		Client:          client,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}

	for {
		// Fetch round data
		roundsData, err := fetchRoundsData()
		if err != nil {
			log.Printf("Error fetching rounds data: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// Check activation status
		isActivated := checkActivationStatus(clientUtils, eoaAddress)
		if isActivated {
			log.Println("Node is activated. No further action required.")
		} else {
			log.Println("Node is not activated. Checking deposit amount...")

			// Check and ensure deposit is sufficient
			depositSufficient, err := checkDepositAmount(clientUtils, eoaAddress)
			if err != nil {
				log.Printf("Error checking deposit amount: %v", err)
				time.Sleep(30 * time.Second)
				continue
			}

			if !depositSufficient {
				log.Println("Deposit insufficient. Initiating deposit transaction...")
				txSent, err := depositAndCheckActivation(ctx, eoaAddress, privateKey)
				if err != nil {
					log.Printf("Error during deposit transaction: %v", err)
					time.Sleep(30 * time.Second)
					continue
				}
				if txSent {
					log.Println("Deposit transaction sent. Waiting for confirmation...")
					time.Sleep(30 * time.Second)
					continue
				}
			}

			// Send registration request to leader
			log.Println("Deposit sufficient. Sending registration request to leader...")
			sendRegistrationRequestToLeader(ctx, h, leaderInfo.ID, eoaAddress, privateKey)
		}

		for _, round := range roundsData.Rounds {
			log.Printf("Checking round...")

			// Check if Merkle Root and Random Number are already generated (not nil)
			if round.MerkleRootSubmitted.MerkleRoot != nil && round.RandomNumberGenerated.RandomNumber != nil {
				// If both MerkleRoot and RandomNumber are generated, skip this round
				log.Printf("Round %s already has Merkle Root AND Random Number generated. Skipping commit generation.", round.Round)
				continue
			}

// Check if this node's EOA is in the activated operators for the round
if isEOAActivated(round, eoaAddress) {
	log.Printf("EOA %s is activated in this round, generating commit...", eoaAddress)

	// Type assertion to extract round number as a string
	roundNum, ok := round.Round.(string) // Round is a string in the response
	if !ok {
		log.Println("Error: Round is not of type string.")
		continue
	}

	// Check if this round has already been committed (store it locally)
	commitData, err := utils.LoadCommitData(roundNum)
	if err != nil && err.Error() != "commit not found" {
		log.Printf("Error loading commit data: %v", err)
		continue
	}

	// If commitData exists, we should only skip the round if both MerkleRoot and RandomNumber are nil
	if commitData != nil && round.MerkleRootSubmitted.MerkleRoot == nil && round.RandomNumberGenerated.RandomNumber == nil {
		log.Printf("Commit data already exists for round %s, but both Merkle Root and Random Number are nil. Skipping commit generation.", roundNum)
		continue
	}

	// If Merkle Root and Random Number are nil, generate commit
	if round.MerkleRootSubmitted.MerkleRoot == nil && round.RandomNumberGenerated.RandomNumber == nil {
		// Generate commit
		secretValue, cos, cvs, err := commitreveal2.GenerateCommit(roundNum, eoaAddress)
		if err != nil {
			log.Printf("Error generating commit: %v", err)
			continue
		}

		// Prepare commit data
		commitData := utils.CommitData{
			Round:          roundNum,
			SecretValue:    secretValue,
			Cos:            cos,
			Cvs:            cvs,
			SendToLeader:   true,  // Mark commit to be sent to leader
			SendCosToLeader: false, // Initially false, to allow sending COS
		}

		// Save commit data locally to prevent resending
		err = utils.SaveCommitData(commitData)
		if err != nil {
			log.Printf("Error saving commit data: %v", err)
			continue
		}

		// Send commit to leader
		sendCommitToLeader(ctx, h, leaderInfo.ID, commitData, eoaAddress)
	}

	// If commit data exists and SendCosToLeader is false, send COS to leader
	if commitData != nil && !commitData.SendCosToLeader {
		// If Merkle Root is set but Random Number is nil, check and send COS
		if round.MerkleRootSubmitted.MerkleRoot != nil && round.RandomNumberGenerated.RandomNumber == nil {
			log.Printf("Merkle Root is set but Random Number is not. Sending COS for round %s.", roundNum)

			// Send COS to leader
			sendCosToLeader(ctx, h, leaderInfo.ID, *commitData, eoaAddress, privateKey)

			// Update SendCosToLeader flag
			commitData.SendCosToLeader = true

			// Save updated commit data to prevent re-sending COS
			err := utils.SaveCommitData(*commitData)
			if err != nil {
				log.Printf("Error saving updated commit data after sending COS: %v", err)
			}
		}
		continue
	}
}			
		}

		// Wait before rechecking activation status
		time.Sleep(30 * time.Second)
	}
}

// sendCOSToLeader sends the COS to the leader node
func sendCosToLeader(ctx context.Context, h core.Host, leaderID peer.ID, commitData utils.CommitData, eoaAddress string, privateKey *ecdsa.PrivateKey) {
	// Create commit request structure with signed COS and round data
	req := utils.CosRequest{
		Round:     commitData.Round,
		Cos:       commitData.Cos,
		EOAAddress: eoaAddress, // Include EOA address to verify
	}

	// Sign the request (just the round value here)
	signedRequest := utils.SignData(eoaAddress, privateKey)

	// Send the COS commit to leader with the signed request
	req.Signature = signedRequest

	// Send the commit to leader
	s, err := h.NewStream(ctx, leaderID, "/cos")
	if err != nil {
		log.Printf("Failed to create stream to leader: %v", err)
		return
	}
	defer s.Close()

	// Encode and send the commit request
	if err := json.NewEncoder(s).Encode(req); err != nil {
		log.Printf("Failed to send COS commit to leader: %v", err)
	} else {
		log.Printf("COS commit sent to leader for round %s", commitData.Round)
	}
}

// isEOAActivated checks if the current regular node's EOA address is in the activated operators list for the round
func isEOAActivated(round RoundData, eoaAddress string) bool {
	// Convert eoaAddress string to common.Address
	eoaAddr := common.HexToAddress(eoaAddress)

	// Compare with activated operators
	for _, operator := range round.RandomNumberRequested.ActivatedOperators {
		// Convert operator (string) to common.Address
		operatorAddr := common.HexToAddress(operator)

		// Compare operatorAddr with eoaAddr
		if operatorAddr == eoaAddr {
			return true
		}
	}
	return false
}

func checkActivationStatus(client *utils.Client, eoaAddress string) bool {
	activatedOperatorsResult, err := transactions.CallSmartContract(client.Client, client.ContractABI, "getActivatedOperators", client.ContractAddress)
	if err != nil {
		log.Printf("Failed to call getActivatedOperators: %v", err)
		return false
	}

	activatedOperators := activatedOperatorsResult.([]common.Address)
	for _, operator := range activatedOperators {
		if operator.Hex() == eoaAddress {
			return true
		}
	}

	return false
}

// sendRegistrationRequestToLeader sends the registration request to the leader node
func sendRegistrationRequestToLeader(ctx context.Context, h core.Host, leaderID peer.ID, eoaAddress string, privateKey *ecdsa.PrivateKey) {
	req := utils.RegistrationRequest{
		EOAAddress: eoaAddress,
		Signature:  utils.SignData(eoaAddress, privateKey),
		PeerID:     h.ID().String(),
	}

	s, err := h.NewStream(ctx, leaderID, "/register")
	if err != nil {
		log.Printf("Failed to create stream to leader: %v", err)
		h.Peerstore().AddAddrs(leaderID, h.Peerstore().Addrs(leaderID), peerstore.PermanentAddrTTL)
		return
	}
	defer s.Close()

	if err := json.NewEncoder(s).Encode(req); err != nil {
		log.Printf("Failed to send registration request: %v", err)
	} else {
		log.Println("Registration request sent to leader.")
	}
}

func depositAndCheckActivation(ctx context.Context, eoaAddress string, privateKey *ecdsa.PrivateKey) (bool, error) {
	client, err := ethclient.Dial(os.Getenv("ETH_RPC_URL"))
	if err != nil {
		return false, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	// Fetch deposit amount
	depositAmountResult, err := transactions.CallSmartContract(client, parsedABI, "s_depositAmount", contractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := transactions.CallSmartContract(client, parsedABI, "s_activationThreshold", contractAddress)
	if err != nil {
		return false, fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	// If deposit is insufficient, we calculate the remaining amount and proceed with the deposit
	if depositAmount.Cmp(activationThreshold) < 0 {
		remaining := new(big.Int).Sub(activationThreshold, depositAmount)
		log.Printf("Deposit insufficient. Adding remaining: %s", remaining.String())

		// Check account balance
		balance, err := client.BalanceAt(ctx, common.HexToAddress(eoaAddress), nil)
		if err != nil {
			return false, fmt.Errorf("failed to fetch account balance: %v", err)
		}
		log.Printf("Account balance: %s", balance.String())

		if balance.Cmp(remaining) < 0 {
			return false, fmt.Errorf("insufficient balance: required %s, available %s", remaining.String(), balance.String())
		}

		// Create and send deposit transaction
		_, _, err = transactions.ExecuteTransaction(
			ctx,
			&utils.Client{
				Client:       client,
				ContractAddress: contractAddress,
				PrivateKey:   privateKey,
				ContractABI:  parsedABI,
			},
			"deposit",
			remaining,
		)
		if err != nil {
			return false, fmt.Errorf("failed to send deposit transaction: %v", err)
		}

		log.Println("Deposit transaction sent.")
		return true, nil
	}

	log.Println("Deposit amount is sufficient. No additional deposit required.")
	return false, nil
}

func checkDepositAmount(client *utils.Client, eoaAddress string) (bool, error) {
	// Fetch deposit amount
	depositAmountResult, err := transactions.CallSmartContract(client.Client, client.ContractABI, "s_depositAmount", client.ContractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := transactions.CallSmartContract(client.Client, client.ContractABI, "s_activationThreshold", client.ContractAddress)
	if err != nil {
		return false, fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	log.Printf("Deposit amount: %s, Activation threshold: %s", depositAmount.String(), activationThreshold.String())

	// Check if deposit is sufficient
	if depositAmount.Cmp(activationThreshold) >= 0 {
		return true, nil
	}

	return false, nil
}


// sendCommitToLeader sends the generated commit to the leader node
func sendCommitToLeader(ctx context.Context, h core.Host, leaderID peer.ID, commitData utils.CommitData, eoaAddress string) {
	// Create commit request structure with signed round value and CVS
	req := utils.CommitRequest{
		Round:      commitData.Round,
		Cvs:        commitData.Cvs,
		EOAAddress: eoaAddress,
	}

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode leader private key: %v", err)
		return
	}

	// Sign the request (round + EOA address)
	signedRequest := utils.SignData(eoaAddress, privateKey)

	req.Signature = signedRequest

	// Generate v, r, s for the CVS using the helper function
	v, r, s, err := regularNode_helper.GenerateCvsSignature(req.Round, req.Cvs)
	if err != nil {
		log.Printf("Failed to generate v, r, s for CVS: %v", err)
		return
	}

	// Add signature values to the commit request
	req.Sign = map[string]string{
		"v": fmt.Sprintf("%d", v),
		"r": r,
		"s": s,
	}

	// Save commit data locally with v, r, s
	commitData.Sign = req.Sign
	if err := utils.SaveCommitData(commitData); err != nil {
		log.Printf("Failed to save commit data locally: %v", err)
		return
	}

	// Send the commit to the leader
	send, err := h.NewStream(ctx, leaderID, "/cvs")
	if err != nil {
		log.Printf("Failed to create stream to leader: %v", err)
		return
	}
	defer send.Close()

	// Encode and send the commit request
	if err := json.NewEncoder(send).Encode(req); err != nil {
		log.Printf("Failed to send commit to leader for round %s: %v", req.Round, err)
	} else {
		log.Printf("Commit successfully sent to leader for round %s", req.Round)
	}
}
