package regular

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
	"github.com/tokamak-network/DRB-node/logger"
	regularNodeHelper "github.com/tokamak-network/DRB-node/nodes/regular/helper"
	"github.com/tokamak-network/DRB-node/pkg/fallback_ethclient"
	"github.com/tokamak-network/DRB-node/utils"
)

type Node struct {
	host              host.Host
	fallbackEthClient *fallback_ethclient.FallbackRPCClient
	config            *Config
	abi               abi.ABI
}

func NewNode(config *Config, fallbackEthClient *fallback_ethclient.FallbackRPCClient) (*Node, error) {
	// Get leader's multiaddress
	if config.LeaderIP == "" {
		logger.Fatal("LEADER_IP is not set in environment variables.")
	}

	if config.Port == "" {
		logger.Fatal("PORT is not set in environment variables.")
	}

	if config.LeaderPort == "" {
		logger.Fatal("LEADER_PORT is not set in environment variables.")
	}

	if config.LeaderPeerID == "" {
		logger.Fatal("LEADER_PEER_ID is not set in environment variables.")
	}

	if config.EOAPrivateKey == nil {
		logger.Fatal("EOA_PRIVATE_KEY is not set in environment variables.")
	}

	if config.ContractAddress == (common.Address{}) {
		logger.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	host, peerID, err := libp2putils.CreateHost(config.Port)
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
		fallbackEthClient: fallbackEthClient,
		host:              host,
		config:            config,
		abi:               parsedAbi,
	}

	node.setHandlers()

	return node, nil
}

func (n *Node) Start(ctx context.Context) {
	go regularNodeHelper.MonitorCommitRequest(n.fallbackEthClient)

	eoaAddress := crypto.PubkeyToAddress(n.config.EOAPrivateKey.PublicKey).Hex()

	regularNodeHelper.Setup(eoaAddress)

	logger.Infof("EOA Address: %s", eoaAddress)

	// Get the local IP address of the node
	ip := utils.GetLocalIP() // Use dynamic IP retrieval

	// Save the node's information (IP, Port, PeerID, EOA address)
	nodeInfo := utils.NodeInfo{
		IP:         ip,
		Port:       n.config.Port,
		PeerID:     n.host.ID().String(),
		EOAAddress: eoaAddress,
	}

	if err := utils.SaveNodeInfo([]utils.NodeInfo{nodeInfo}); err != nil {
		logger.Infof("Failed to save node info: %v", err)
	}

	// Connect to the leader
	leaderInfo, err := libp2putils.ConnectToPeer(n.host, n.config.LeaderIP, n.config.LeaderPort, n.config.LeaderPeerID)
	if err != nil {
		logger.Fatalf("Error connecting to leader: %v", err)
	}

	for {
		if err != nil {
			logger.Infof("Error fetching rounds data: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// Check activation status
		isActivated := n.checkActivationStatus(eoaAddress)
		if isActivated {
			logger.Info("Node is activated. No further action required.")
		} else {
			logger.Info("Node is not activated. Checking deposit amount...")

			// Check and ensure deposit is sufficient
			depositSufficient, err := n.checkDepositAmount(eoaAddress)
			if err != nil {
				logger.Infof("Error checking deposit amount: %v", err)
				time.Sleep(30 * time.Second)
				continue
			}

			if !depositSufficient {
				logger.Info("Deposit insufficient. Initiating deposit transaction...")
				txSent, err := n.depositAndCheckActivation(ctx, eoaAddress, n.config.EOAPrivateKey)
				if err != nil {
					logger.Infof("Error during deposit transaction: %v", err)
					time.Sleep(30 * time.Second)
					continue
				}
				if txSent {
					logger.Info("Deposit successful")
					continue
				}
			}

			err = n.activateOnChain()
			if err != nil {
				logger.Infof("failed to activate EOA %s on-chain: %v", eoaAddress, err)
				time.Sleep(30 * time.Second)
				continue
			}

			// Send registration request to leader
			logger.Info("Deposit sufficient. Sending registration request to leader...")
			n.sendRegistrationRequestToLeader(ctx, leaderInfo.ID, eoaAddress, n.config.EOAPrivateKey)
		}

		if !regularNodeHelper.Execution {
			time.Sleep(10 * time.Second)
			continue
		}
		time.Sleep(5 * time.Second)

		round := regularNodeHelper.CurrentRound
		merkleRootSubmitted := regularNodeHelper.RoundsData[round].MerkleRoot
		randomNumberSubmitted := regularNodeHelper.RoundsData[round].RandomNumber

		logger.Infof("Checking round...")
		// Check if Merkle Root and Random Number are already generated (not nil)
		if merkleRootSubmitted && randomNumberSubmitted {
			// If both MerkleRoot and RandomNumber are generated, skip this round
			logger.Infof("Round %s already has Merkle Root AND Random Number generated. Skipping commit generation.", round)
			continue
		}

		// Check if this node's EOA is in the activated operators for the round
		if n.isEOAActivated(eoaAddress) {

			// Check if this round has already been committed (store it locally)
			commitData, err := utils.LoadCommitData(round)
			if err != nil && err.Error() != "commit not found" {
				logger.Infof("Error loading commit data: %v", err)
				continue
			}

			// If commitData exists, we should only skip the round if both MerkleRoot and RandomNumber are nil
			if commitData != nil && !merkleRootSubmitted && !randomNumberSubmitted {
				logger.Infof("Commit data already exists for round %s, but both Merkle Root and Random Number are nil. Skipping commit generation.", round)
				continue
			}

			// If Merkle Root and Random Number are nil, generate commit
			if !merkleRootSubmitted && !randomNumberSubmitted {
				// Generate commit
				logger.Infof("EOA %s is activated in this round, generating commit...", eoaAddress)
				secretValue, cos, cvs, err := commitreveal2.GenerateCommit(round, eoaAddress)
				if err != nil {
					logger.Infof("Error generating commit: %v", err)
					continue
				}

				// Prepare commit data
				commitData := utils.CommitData{
					Round:           round,
					SecretValue:     secretValue,
					Cos:             cos,
					Cvs:             cvs,
					SendToLeader:    true,  // Mark commit to be sent to leader
					SendCosToLeader: false, // Initially false, to allow sending COS
				}

				// Save commit data locally to prevent resending
				err = utils.SaveCommitData(commitData)
				if err != nil {
					logger.Infof("Error saving commit data: %v", err)
					continue
				}

				// Send commit to leader
				n.sendCommitToLeader(ctx, leaderInfo.ID, commitData, eoaAddress)
			}

			// If commit data exists and SendCosToLeader is false, send COS to leader
			if commitData != nil && !commitData.SendCosToLeader {
				// If Merkle Root is set but Random Number is nil, check and send COS
				if merkleRootSubmitted && !randomNumberSubmitted {
					logger.Infof("Merkle Root is set but Random Number is not. Sending COS for round %s.", round)
					// Send COS to leader
					n.sendCosToLeader(ctx, leaderInfo.ID, *commitData, eoaAddress, n.config.EOAPrivateKey)

					// Update SendCosToLeader flag
					commitData.SendCosToLeader = true

					// Save updated commit data to prevent re-sending COS
					err := utils.SaveCommitData(*commitData)
					if err != nil {
						logger.Infof("Error saving updated commit data after sending COS: %v", err)
					}
				}
				continue
			}
		}

		// Wait before rechecking activation status
		time.Sleep(10 * time.Second)
	}
}

func (n *Node) Close() {
	n.host.Close()
}

func (n *Node) setHandlers() {
	n.host.SetStreamHandler("/sendSecretValue", func(s network.Stream) {
		regularNodeHelper.HandleSecretValueRequest(n.host, s)
	})
	n.host.SetStreamHandler("/cvsBroadcast", regularNodeHelper.HandleCvs)
	n.host.SetStreamHandler("/cosBroadcast", regularNodeHelper.HandleCos)
	n.host.SetStreamHandler("/secretBroadcast", regularNodeHelper.HandleSecret)
}

func (n *Node) checkActivationStatus(eoaAddress string) bool {
	activatedOperatorsResult, err := eth.CallSmartContract(n.fallbackEthClient, n.abi, "getActivatedOperators", n.config.ContractAddress)
	if err != nil {
		logger.Infof("Failed to call getActivatedOperators: %v", err)
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

func (n *Node) checkDepositAmount(eoaAddress string) (bool, error) {
	// Fetch deposit amount
	depositAmountResult, err := eth.CallSmartContract(n.fallbackEthClient, n.abi, "s_depositAmount", n.config.ContractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := eth.CallSmartContract(n.fallbackEthClient, n.abi, "s_activationThreshold", n.config.ContractAddress)
	if err != nil {
		return false, fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	logger.Infof("Deposit amount: %s, Activation threshold: %s", depositAmount.String(), activationThreshold.String())

	// Check if deposit is sufficient
	if depositAmount.Cmp(activationThreshold) >= 0 {
		return true, nil
	}

	return false, nil
}

func (n *Node) depositAndCheckActivation(ctx context.Context, eoaAddress string, privateKey *ecdsa.PrivateKey) (bool, error) {
	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		logger.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	contractAddress := common.HexToAddress(contractAddressStr)

	// Fetch deposit amount
	depositAmountResult, err := eth.CallSmartContract(n.fallbackEthClient, n.abi, "s_depositAmount", n.config.ContractAddress, common.HexToAddress(eoaAddress))
	if err != nil {
		return false, fmt.Errorf("failed to call s_depositAmount: %v", err)
	}
	depositAmount := depositAmountResult.(*big.Int)

	// Fetch activation threshold
	activationThresholdResult, err := eth.CallSmartContract(n.fallbackEthClient, n.abi, "s_activationThreshold", n.config.ContractAddress)
	if err != nil {
		return false, fmt.Errorf("failed to call s_activationThreshold: %v", err)
	}
	activationThreshold := activationThresholdResult.(*big.Int)

	// If deposit is insufficient, we calculate the remaining amount and proceed with the deposit
	if depositAmount.Cmp(activationThreshold) < 0 {
		remaining := new(big.Int).Sub(activationThreshold, depositAmount)
		logger.Infof("Deposit insufficient. Adding remaining: %s", remaining.String())

		// Check account balance
		balance, err := n.fallbackEthClient.BalanceAt(ctx, common.HexToAddress(eoaAddress), nil)
		if err != nil {
			return false, fmt.Errorf("failed to fetch account balance: %v", err)
		}
		logger.Infof("Account balance: %s", balance.String())

		if balance.Cmp(remaining) < 0 {
			return false, fmt.Errorf("insufficient balance: required %s, available %s", remaining.String(), balance.String())
		}

		// Create and send deposit transaction
		_, _, err = eth.ExecuteTransaction(
			ctx,
			&utils.Client{
				ContractAddress: contractAddress,
				PrivateKey:      privateKey,
				ContractABI:     n.abi,
			},
			n.fallbackEthClient,
			"deposit",
			remaining,
		)
		if err != nil {
			return false, fmt.Errorf("failed to send deposit transaction: %v", err)
		}

		return true, nil
	}

	logger.Info("Deposit amount is sufficient. No additional deposit required.")
	return false, nil
}

func (n *Node) activateOnChain() error {
	clientUtils := &utils.Client{
		ContractAddress: n.config.ContractAddress,
		PrivateKey:      n.config.EOAPrivateKey,
		ContractABI:     n.abi,
	}

	_, _, err := eth.ExecuteTransaction(
		context.Background(),
		clientUtils,
		n.fallbackEthClient,
		"activate",
		big.NewInt(0),
	)
	if err != nil {
		return fmt.Errorf("failed to activate operator: %v", err)
	}

	return nil
}

// sendRegistrationRequestToLeader sends the registration request to the leader node
func (n *Node) sendRegistrationRequestToLeader(ctx context.Context, leaderID peer.ID, eoaAddress string, privateKey *ecdsa.PrivateKey) {
	req := utils.RegistrationRequest{
		EOAAddress: eoaAddress,
		Signature:  utils.SignData(eoaAddress, privateKey),
		PeerID:     n.host.ID().String(),
	}

	s, err := n.host.NewStream(ctx, leaderID, "/register")
	if err != nil {
		logger.Infof("Failed to create stream to leader: %v", err)
		n.host.Peerstore().AddAddrs(leaderID, n.host.Peerstore().Addrs(leaderID), peerstore.PermanentAddrTTL)
		return
	}
	defer s.Close()

	if err := json.NewEncoder(s).Encode(req); err != nil {
		logger.Infof("Failed to send registration request: %v", err)
	} else {
		logger.Info("Registration request sent to leader.")
	}
}

// isEOAActivated checks if the current regular node's EOA address is in the activated operators list for the round
func (n *Node) isEOAActivated(eoaAddress string) bool {
	address := regularNodeHelper.ActivatedOperator

	// Compare with activated operators
	for _, operator := range address {
		// Compare operatorAddr with eoaAddr
		if operator == eoaAddress {
			return true
		}
	}
	return false
}

// sendCommitToLeader sends the generated commit to the leader node
func (n *Node) sendCommitToLeader(ctx context.Context, leaderID peer.ID, commitData utils.CommitData, eoaAddress string) {
	// Create commit request structure with signed round value and CVS
	req := utils.CommitRequest{
		Round:      commitData.Round,
		Cvs:        commitData.Cvs,
		EOAAddress: eoaAddress,
	}

	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")
	if privateKeyHex == "" {
		logger.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		logger.Infof("Failed to decode leader private key: %v", err)
		return
	}

	// Sign the request (round + EOA address)
	signedRequest := utils.SignData(eoaAddress, privateKey)

	req.Signature = signedRequest

	contractAddressStr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddressStr == "" {
		logger.Fatal("CONTRACT_ADDRESS is not set in environment variables.")
	}

	result, err := eth.CallSmartContract(n.fallbackEthClient, n.abi, "getCurStartTime", n.config.ContractAddress)
	if err != nil {
		fmt.Println("error", err)
	}
	startTime := result.(*big.Int).String()
	v, r, s, err := regularNodeHelper.GenerateCvsSignature(startTime, req.Cvs)
	if err != nil {
		logger.Infof("Failed to generate v, r, s for CVS: %v", err)
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
		logger.Infof("Failed to save commit data locally: %v", err)
		return
	}

	// Send the commit to the leader
	send, err := n.host.NewStream(ctx, leaderID, "/cvs")
	if err != nil {
		logger.Infof("Failed to create stream to leader: %v", err)
		return
	}
	defer send.Close()

	// Encode and send the commit request
	if err := json.NewEncoder(send).Encode(req); err != nil {
		logger.Infof("Failed to send commit to leader for round %s: %v", req.Round, err)
	} else {
		logger.Infof("Commit successfully sent to leader for round %s", req.Round)
	}
}

func (n *Node) sendCosToLeader(ctx context.Context, leaderID peer.ID, commitData utils.CommitData, eoaAddress string, privateKey *ecdsa.PrivateKey) {
	// Create commit request structure with signed COS and round data
	req := utils.CosRequest{
		Round:      commitData.Round,
		Cos:        commitData.Cos,
		EOAAddress: eoaAddress, // Include EOA address to verify
	}

	// Sign the request (just the round value here)
	signedRequest := utils.SignData(eoaAddress, privateKey)

	// Send the COS commit to leader with the signed request
	req.Signature = signedRequest

	// Send the commit to leader
	s, err := n.host.NewStream(ctx, leaderID, "/cos")
	if err != nil {
		logger.Infof("Failed to create stream to leader: %v", err)
		return
	}
	defer s.Close()

	// Encode and send the commit request
	if err := json.NewEncoder(s).Encode(req); err != nil {
		logger.Infof("Failed to send COS commit to leader: %v", err)
	} else {
		logger.Infof("COS commit sent to leader for round %s", commitData.Round)
	}
}
