package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

type RegistrationRequest struct {
	EOAAddress string `json:"eoa_address"`
	Signature  []byte `json:"signature"`
	PeerID     string `json:"peer_id"`
}

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
}

func main() {
	nodeType := os.Getenv("NODE_TYPE") // Expecting 'leader' or 'regular'

	if nodeType == "leader" {
		runLeaderNode()
	} else if nodeType == "regular" {
		runRegularNode()
	} else {
		log.Fatal("NODE_TYPE must be set to either 'leader' or 'regular'")
	}
}

func runLeaderNode() {
	port := os.Getenv("LEADER_PORT")
	if port == "" {
		log.Fatal("LEADER_PORT not set in environment variables.")
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)),
	)
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer h.Close()

	h.SetStreamHandler("/register", handleRegistrationRequest)

	log.Printf("Leader node is running on %s with ID: %s\n", h.Addrs(), h.ID())

	select {}
}

func handleRegistrationRequest(s network.Stream) {
	defer s.Close()
	var req RegistrationRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("Failed to decode registration request: %v", err)
		return
	}

	if verifySignature(req) {
		log.Printf("Verified registration for PeerID: %s", req.PeerID)
	} else {
		log.Printf("Failed to verify registration for PeerID: %s", req.PeerID)
	}
}

func verifySignature(req RegistrationRequest) bool {
	hash := crypto.Keccak256Hash([]byte(req.EOAAddress))
	pubKey, err := crypto.SigToPub(hash.Bytes(), req.Signature)
	if err != nil {
		log.Printf("Error recovering public key: %v", err)
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()
	return recoveredAddress == req.EOAAddress
}

func runRegularNode() {
	ctx := context.Background()

	leaderIP := os.Getenv("LEADER_IP")
	leaderPort := os.Getenv("LEADER_PORT")
	leaderPeerID := os.Getenv("LEADER_PEER_ID")
	privateKeyHex := os.Getenv("EOA_PRIVATE_KEY")

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode Ethereum private key: %v", err)
	}

	eoaAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	data := crypto.Keccak256([]byte(eoaAddress))
	signature, err := crypto.Sign(data, privateKey)
	if err != nil {
		log.Fatalf("Failed to sign data: %v", err)
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer h.Close()

	leaderAddrString := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", leaderIP, leaderPort, leaderPeerID)
	leaderAddr, err := multiaddr.NewMultiaddr(leaderAddrString)
	if err != nil {
		log.Fatalf("Failed to parse leader multiaddr: %v", err)
	}

	leaderInfo, err := peer.AddrInfoFromP2pAddr(leaderAddr)
	if err != nil {
		log.Fatalf("Failed to create peer info from leader multiaddr: %v", err)
	}

	if err := h.Connect(ctx, *leaderInfo); err != nil {
		log.Fatalf("Failed to connect to leader: %v", err)
	}

	req := RegistrationRequest{
		EOAAddress: eoaAddress,
		Signature:  signature,
		PeerID:     h.ID().String(),
	}

	if err := sendRegistrationRequest(ctx, h, leaderInfo.ID, req); err != nil {
		log.Fatalf("Failed to send registration request: %v", err)
	}

	log.Println("Registration request sent successfully. Listening for further instructions...")

	select {}
}

func sendRegistrationRequest(ctx context.Context, h core.Host, leaderID peer.ID, req RegistrationRequest) error {
	s, err := h.NewStream(ctx, leaderID, "/register")
	if err != nil {
		return err
	}
	defer s.Close()

	return json.NewEncoder(s).Encode(req)
}
