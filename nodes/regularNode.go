package nodes

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	core "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/nodes/regularNode_helper"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

const abiFilePath = "contract/abi/Commit2RevealDRB.json"

// Flags to track whether deposit and activate have been called in current run
var (
	depositCalledInThisRun  bool = false
	activateCalledInThisRun bool = false
)

// RunRegularNode handles the behavior for a regular node
func RunRegularNode(fallbackEthClient *fallback_ethclient.FallbackRPCClient) {
	ctx := context.Background()

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT not set in environment variables.")
	}

	nodeType := os.Getenv("NODE_TYPE")
	if nodeType == "" {
		log.Fatal("NODE_TYPE is not set in environment variables.")
	}

	h, peerID, err := libp2putils.CreateHost(port, nodeType)
	if err != nil {
		log.Fatalf("Error creating host: %v", err)
	}

	defer h.Close()

	h.SetStreamHandler("/sendSecretValue", func(s network.Stream) {
		regularNode_helper.HandleSecretValueRequest(h, s)
	})
	h.SetStreamHandler("/cvsBroadcast", func(s network.Stream) {
		regularNode_helper.HandleCvs(h, s)
	})
	h.SetStreamHandler("/cosBroadcast", func(s network.Stream) {
		regularNode_helper.HandleCos(h, s)
	})
	h.SetStreamHandler("/secretBroadcast", func(s network.Stream) {
		regularNode_helper.HandleSecret(h, s)
	})

	go regularNode_helper.MonitorCommitRequest(fallbackEthClient)

	// Get leader's multiaddress
	leaderIP := os.Getenv("LEADER_IP")
	if leaderIP == "" {
		log.Fatal("LEADER_IP is not set in environment variables.")
	}

	leaderPort := os.Getenv("LEADER_PORT")
	if leaderPort == "" {
		log.Fatal("LEADER_PORT is not set in environment variables.")
	}

	leaderPeerID := os.Getenv("LEADER_PEER_ID")
	if leaderPeerID == "" {
		log.Fatal("LEADER_PEER_ID is not set in environment variables.")
	}

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
	regularNode_helper.SetRegularNodeEOA(eoaAddress)
	log.Printf("EOA Address: %s", eoaAddress)

	// Get the local IP address of the node
	ip := utils.GetLocalIP() // Use dynamic IP retrieval

	// Save the node's information (IP, Port, PeerID, EOA address)
	nodeInfo := utils.NodeInfo{
		IP:         ip,
		Port:       port,
		PeerID:     peerID.String(),
		EOAAddress: eoaAddress,
	}

	if err := database.AddNodeInfo(&nodeInfo); err != nil {
		log.Printf("Failed to save node info: %v", err)
	}

	// Connect to the leader
	leaderInfo, err := libp2putils.ConnectToPeer(h, leaderIP, leaderPort, leaderPeerID)
	if err != nil {
		log.Fatalf("Error connecting to leader: %v", err)
	}

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}
	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		log.Fatalf("Failed to load contract ABI: %v", err)
	}

	clientUtils := &utils.Client{
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		ContractABI:     parsedABI,
	}
	for {
		if err != nil {
			log.Printf("Error fetching rounds data: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// Check activation status
		isActivated := checkActivationStatus(fallbackEthClient, clientUtils, eoaAddress)
		if isActivated {
			log.Println("Node is activated. No further action required.")
			activateCalledInThisRun = true
		} else {
			log.Println("Node is not activated. Checking deposit amount...")
			// Check and ensure deposit is sufficient (only if not already called this run)
			// This ensures deposit is called only once per program run
			if !depositCalledInThisRun {
				depositSufficient, err := checkDepositAmount(fallbackEthClient, clientUtils, eoaAddress)
				if err != nil {
					log.Printf("Error checking deposit amount: %v", err)
					time.Sleep(30 * time.Second)
					continue
				}

				if !depositSufficient {
					depositCalledInThisRun = true // Mark deposit as called this run
					log.Println("Deposit insufficient. Initiating deposit transaction...")
					txSent, err := deposit(ctx, fallbackEthClient, eoaAddress, privateKey)
					if err != nil {
						log.Printf("Error during deposit transaction: %v", err)
						// time.Sleep(30 * time.Second)
						depositCalledInThisRun = false
						continue
					}
					if txSent {
						log.Println("\033[32mDeposit successful\033[0m")
						// depositCalledInThisRun = true 
						continue
					}
				}
			} else {
				log.Println("Deposit already attempted this run. Skipping deposit check.")
			}

			// Call activate only if not already called this run
			if !activateCalledInThisRun {
				err = activateOnChain(fallbackEthClient, abiFilePath)
				if err != nil {
					log.Printf("failed to activate EOA %s on-chain: %v", eoaAddress, err)
					time.Sleep(30 * time.Second)
					continue
				}
				activateCalledInThisRun = true // Mark activate as called this run
				log.Println("\033[32mActivation successful\033[0m")
				// Send registration request to leader
				log.Println("Sending registration request to leader...")
				sendRegistrationRequestToLeader(ctx, h, leaderInfo.ID, eoaAddress, privateKey)
			} else {
				log.Println("Activation already attempted this run. Skipping activation.")
			}
		}

		if !regularNode_helper.Execution {
			time.Sleep(10 * time.Second)
			continue
		}
		time.Sleep(5 * time.Second)

		// Check and start leader monitoring
		round := regularNode_helper.CurrentRound
		trialNum := regularNode_helper.CurrentTrialNum
		regularNode_helper.CheckAndStartMonitoring(fallbackEthClient, round, trialNum)

		uniqueKey := utils.GetUniqueKey(round, trialNum)
		if atomic.LoadInt32(&regularNode_helper.Halted) == 1 {
			log.Println("System is halted. Skipping checkAndStartMonitoring.")
			continue
		}
		merkleRootSubmitted := regularNode_helper.RoundsData[uniqueKey].MerkleRoot
		randomNumberSubmitted := regularNode_helper.RoundsData[uniqueKey].RandomNumber

		// log.Printf("Checking round %s with trial %s ...", round, trialNum)
		// Check if Merkle Root and Random Number are already generated (not nil)
		if merkleRootSubmitted && randomNumberSubmitted {
			// If both MerkleRoot and RandomNumber are generated, skip this round
			log.Printf("Round %s with trial %s already has Merkle Root AND Random Number generated. Skipping commit generation.", round, trialNum)
			continue
		}

		// Check if this node's EOA is in the activated operators for the round
		if isEOAActivated(eoaAddress) {

			// Check if this round has already been committed (store it locally)
			commitData, err := database.GetCommitByRound(round, trialNum)
			if err != nil && err.Error() != "pg: no rows in result set" {
				log.Printf("Error loading commit data: %v", err)
				continue
			}

			// If commitData exists, we should only skip the round if both MerkleRoot and RandomNumber are nil
			if commitData != nil && !merkleRootSubmitted && !randomNumberSubmitted {
				// log.Printf("Commit data already exists for round %s with trial %s, but both Merkle Root and Random Number are nil. Skipping commit generation.", round, trialNum)
				continue
			}

			// If Merkle Root and Random Number are nil, generate commit
			if !merkleRootSubmitted && !randomNumberSubmitted {
				// Generate commit
				log.Printf("EOA %s is activated in this round, generating commit...", eoaAddress)
				secretValue, cos, cvs, err := commitreveal2.GenerateCommit(round, eoaAddress)
				if err != nil {
					log.Printf("Error generating commit: %v", err)
					continue
				}

				// Prepare commit data
				commitData := utils.CommitData{
					UniqueKey:       uniqueKey,
					Round:           round,
					TrialNum:        trialNum,
					SecretValue:     secretValue,
					Cos:             cos,
					Cvs:             cvs,
					SendToLeader:    true,  // Initially false, will be set to true after sending
					SendCosToLeader: false, // Initially false, to allow sending COS
				}

				// Save commit data locally to prevent resending
				err = database.AddCommit(&commitData)
				if err != nil {
					log.Printf("Error saving commit data: %v", err)
					continue
				}

				// Send commit to leader
				sendCommitToLeader(ctx, h, leaderInfo.ID, commitData, round, trialNum, eoaAddress)
			}

			// If commit data exists and SendCosToLeader is false, send COS to leader
			if commitData != nil && !commitData.SendCosToLeader {
				// If Merkle Root is set but Random Number is nil, check and send COS
				if merkleRootSubmitted && !randomNumberSubmitted {
					log.Printf("Merkle Root is set but Random Number is not. Sending COS for round %s.", round)
					// Send COS to leader
					sendCosToLeader(ctx, h, leaderInfo.ID, *commitData, eoaAddress, privateKey)
				}
				continue
			}
		}

		// Wait before rechecking activation status
		time.Sleep(10 * time.Second)
	}
}

func sendCosToLeader(ctx context.Context, h core.Host, leaderID peer.ID, commitData utils.CommitData, eoaAddress string, privateKey *ecdsa.PrivateKey) {
	// Create commit request structure with signed COS and round data
	req := utils.CosRequest{
		UniqueKey:  commitData.UniqueKey,
		Round:      commitData.Round,
		TrialNum:   commitData.TrialNum,
		Cos:        commitData.Cos,
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
		log.Printf("\033[32mCOS commit sent to leader for round %s\033[0m", commitData.Round)

		// Update SendCosToLeader flag to true after successful send
		commitData.SendCosToLeader = true
		if err := database.UpdateCommit(&commitData); err != nil {
			log.Printf("Failed to update SendCosToLeader flag: %v", err)
		}
	}
}

// isEOAActivated checks if the current regular node's EOA address is in the activated operators list for the round
func isEOAActivated(eoaAddress string) bool {
	address := regularNode_helper.ActivatedOperator

	// Compare with activated operators
	for _, operator := range address {
		// Compare operatorAddr with eoaAddr
		if operator == eoaAddress {
			return true
		}
	}
	return false
}

func checkActivationStatus(fallbackEthClient *fallback_ethclient.FallbackRPCClient, client *utils.Client, eoaAddress string) bool {
	activatedOperatorsResult, err := eth.CallSmartContract(fallbackEthClient, client.ContractABI, "getActivatedOperators", client.ContractAddress)
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
		log.Println("\033[32mRegistration request sent to leader.\033[0m")
	}
}

func deposit(ctx context.Context, fallbackEthClient *fallback_ethclient.FallbackRPCClient, eoaAddress string, privateKey *ecdsa.PrivateKey) (bool, error) {
	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to load contract ABI: %v", err)
	}

	// Fetch deposit amount
	depositAmountResult, err := eth.CallSmartContract(fallbackEthClient, parsedABI, "s_depositAmount", contractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := eth.CallSmartContract(fallbackEthClient, parsedABI, "s_activationThreshold", contractAddress)
	if err != nil {
		return false, fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	// If deposit is insufficient, we calculate the remaining amount and proceed with the deposit
	if depositAmount.Cmp(activationThreshold) < 0 {
		remaining := new(big.Int).Sub(activationThreshold, depositAmount)
		log.Printf("Deposit insufficient. Adding remaining: %s", remaining.String())

		// Check account balance
		balance, err := fallbackEthClient.BalanceAt(ctx, common.HexToAddress(eoaAddress), nil)
		if err != nil {
			return false, fmt.Errorf("failed to fetch account balance: %v", err)
		}
		log.Printf("Account balance: %s", balance.String())

		if balance.Cmp(remaining) < 0 {
			return false, fmt.Errorf("insufficient balance: required %s, available %s", remaining.String(), balance.String())
		}

		// Create and send deposit transaction
		_, _, err = eth.ExecuteTransaction(
			ctx,
			&utils.Client{
				ContractAddress: contractAddress,
				PrivateKey:      privateKey,
				ContractABI:     parsedABI,
			},
			fallbackEthClient,
			"deposit",
			remaining,
		)
		if err != nil {
			return false, fmt.Errorf("failed to send deposit transaction: %v", err)
		}

		return true, nil
	}

	log.Println("Deposit amount is sufficient. No additional deposit required.")
	return false, nil
}

func checkDepositAmount(fallbackEthClient *fallback_ethclient.FallbackRPCClient, client *utils.Client, eoaAddress string) (bool, error) {
	// Fetch deposit amount
	depositAmountResult, err := eth.CallSmartContract(fallbackEthClient, client.ContractABI, "s_depositAmount", client.ContractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := eth.CallSmartContract(fallbackEthClient, client.ContractABI, "s_activationThreshold", client.ContractAddress)
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
func sendCommitToLeader(ctx context.Context, h core.Host, leaderID peer.ID, commitData utils.CommitData, round string, trialNum string, eoaAddress string) {
	// Create commit request structure with signed round value and CVS
	req := utils.CommitRequest{
		UniqueKey:  commitData.UniqueKey,
		Round:      round,
		TrialNum:   trialNum,
		Cvs:        commitData.Cvs,
		EOAAddress: eoaAddress,
	}

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to decode leader private key: %v", err)
		return
	}

	// Sign the request (round + EOA address)
	signedRequest := utils.SignData(eoaAddress, privateKey)

	req.Signature = signedRequest

	// Use the current round and trialNum from Status event instead of startTime
	if round == "" || trialNum == "" {
		log.Printf("Current round or trialNum not available, cannot generate signature")
		return
	}
	trialNumBigIntValue, _ := big.NewInt(0).SetString(trialNum, 10)
	roundBigIntValue, _ := big.NewInt(0).SetString(round, 10)
	v, r, s, err := regularNode_helper.GenerateCvsSignature(roundBigIntValue, trialNumBigIntValue, req.Cvs)
	if err != nil {
		log.Printf("Failed to generate v, r, s for CVS: %v", err)
		return
	}

	// Add signature values to the commit request
	req.Sign = utils.SignInfo{
		V: fmt.Sprintf("%d", v),
		R: r,
		S: s,
	}

	// Save commit data locally with v, r, s
	commitData.Sign = req.Sign
	if err := database.UpdateCommit(&commitData); err != nil {
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
		log.Printf("\033[32mCVS successfully sent to leader for round %s\033[0m", req.Round)

		// Update SendToLeader flag to true after successful send
		commitData.SendToLeader = true
		if err := database.UpdateCommit(&commitData); err != nil {
			log.Printf("Failed to update SendToLeader flag: %v", err)
		}
	}
}

func activateOnChain(fallbackEthClient *fallback_ethclient.FallbackRPCClient, abiFilePath string) error {
	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		log.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	contractAddress := common.HexToAddress(contractAddressStr)
	parsedABI, err := utils.LoadContractABI(abiFilePath)
	if err != nil {
		return fmt.Errorf("failed to load contract ABI: %v", err)
	}

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in environment variables.")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to decode leader private key: %v", err)
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
		"activate",
		big.NewInt(0),
	)
	if err != nil {
		return fmt.Errorf("failed to activate operator: %v", err)
	}

	return nil
}
