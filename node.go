package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
)

const topicName = "random-value-topic"
const keyFileName = "node_key"
const storageFileName = "values.json"

type RandomValue struct {
	Value    int64  `json:"value"`
	SenderID string `json:"sender_id"`
}

type Node struct {
	ctx     context.Context
	host    host.Host
	ps      *pubsub.PubSub
	topic   *pubsub.Topic
	sub     *pubsub.Subscription
	storage sync.Map // Stores the random values for each sender ID
}

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
		return
	}

	peerID := os.Getenv("PEER_ID")
	if peerID == "" {
		fmt.Println("PEER_ID must be set in the .env file.")
		return
	}

	portStr := os.Getenv("PORT")
	if portStr == "" {
		fmt.Println("PORT must be set in the .env file.")
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Println("Invalid port number:", err)
		return
	}

	ctx := context.Background()
	node, err := NewNode(ctx, peerID, port)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Node started with ID: %s\n", node.host.ID().String())

	for _, addr := range node.host.Addrs() {
		fmt.Printf("Listening at %s/p2p/%s\n", addr, node.host.ID().String())
	}

	// Connect to other nodes if known addresses are provided
	if len(os.Args) > 2 {
		for _, peerAddr := range os.Args[2:] {
			node.Connect(peerAddr)
		}
	}

	err = node.JoinTopic()
	if err != nil {
		panic(err)
	}

	go node.MonitorPeers() // Monitor peer connections
	go node.ReadLoop()
	node.GenerateAndPublishValues()
}

func NewNode(ctx context.Context, peerID string, port int) (*Node, error) {
	privateKey, err := loadOrCreateKey(peerID)
	if err != nil {
		return nil, err
	}

	host, err := libp2p.New(
		libp2p.Identity(privateKey),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)), // Use the specified port
	)
	if err != nil {
		return nil, err
	}

	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		return nil, err
	}

	return &Node{
		ctx:     ctx,
		host:    host,
		ps:      ps,
		storage: sync.Map{},
	}, nil
}

func loadOrCreateKey(peerID string) (crypto.PrivKey, error) {
	keyPath := filepath.Join(".", keyFileName+"_"+peerID)

	if _, err := os.Stat(keyPath); err == nil {
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		privateKey, err := crypto.UnmarshalPrivateKey(keyBytes)
		if err != nil {
			return nil, err
		}
		return privateKey, nil
	}

	privateKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, err
	}

	keyBytes, err := crypto.MarshalPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(keyPath, keyBytes, 0600)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

func (n *Node) Connect(peerAddr string) {
	peerInfo, err := peer.AddrInfoFromString(peerAddr)
	if err != nil {
		fmt.Println("Failed to parse peer address:", err)
		return
	}

	// Check if the peer address is the same as the current node's address
	if peerInfo.ID == n.host.ID() {
		fmt.Println("Attempted to connect to self, skipping...")
		return
	}

	n.host.Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, peerstore.PermanentAddrTTL)
	err = n.host.Connect(n.ctx, *peerInfo)
	if err != nil {
		fmt.Println("Failed to connect to peer:", err)
	} else {
		fmt.Println("Connected to peer:", peerAddr)
	}
}

func (n *Node) JoinTopic() error {
	topic, err := n.ps.Join(topicName)
	if err != nil {
		return err
	}
	sub, err := topic.Subscribe()
	if err != nil {
		return err
	}

	n.topic = topic
	n.sub = sub
	fmt.Println("Successfully subscribed to topic:", topicName)
	return nil
}

func (n *Node) GenerateAndPublishValues() {
	for {
		time.Sleep(5 * time.Second)
		randomValue := generateRandomValue()
		fmt.Printf("Node %s generated value: %d\n", n.host.ID().String(), randomValue)

		msg := RandomValue{
			Value:    randomValue,
			SenderID: n.host.ID().String(),
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			fmt.Println("Error encoding message:", err)
			continue
		}

		// Store the random value in the new format
		n.storeValue(msg)

		err = n.topic.Publish(n.ctx, msgBytes)
		if err != nil {
			fmt.Println("Error publishing message:", err)
		} else {
			fmt.Printf("Node %s published value: %d\n", n.host.ID().String(), randomValue)
			n.storeValueInFile() // Store in file
		}
	}
}

func (n *Node) storeValue(value RandomValue) {
	// Use sync.Map to store values in a map format: {<peerId>: [randomValues]}
	peerValues, _ := n.storage.LoadOrStore(value.SenderID, []int64{})
	values := peerValues.([]int64)

	// Append the new value
	values = append(values, value.Value)
	n.storage.Store(value.SenderID, values)
}

func (n *Node) ReadLoop() {
	for {
		msg, err := n.sub.Next(n.ctx)
		if err != nil {
			fmt.Println("Error reading message:", err)
			return
		}

		if msg.ReceivedFrom == n.host.ID() {
			continue
		}

		var receivedValue RandomValue
		err = json.Unmarshal(msg.Data, &receivedValue)
		if err != nil {
			fmt.Println("Error decoding message:", err)
			continue
		}

		fmt.Printf("Node %s received value: %d from %s\n", n.host.ID().String(), receivedValue.Value, receivedValue.SenderID)
		n.storeValue(receivedValue) // Store received value in the new format

		n.storeValueInFile() // Store received value in file
	}
}

func (n *Node) storeValueInFile() {
	// Create a map to hold the aggregated values
	data := make(map[string][]int64)

	// Load existing data from the file
	if _, err := os.Stat(storageFileName); err == nil {
		file, err := os.OpenFile(storageFileName, os.O_RDONLY, 0666)
		if err == nil {
			defer file.Close()

			decoder := json.NewDecoder(file)
			err = decoder.Decode(&data)
			if err != nil {
				fmt.Println("Error decoding existing data:", err)
				return
			}
		}
	}

	// Update the data map with current values from the storage
	n.storage.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok {
			if v, ok := value.([]int64); ok {
				data[k] = v // Aggregate values for each peer
			}
		}
		return true
	})

	// Save the updated data back to the file
	file, err := os.OpenFile(storageFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Println("Error opening file for writing:", err)
		return
	}
	defer file.Close()

	// Encode and write the updated data to the file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Optional: to format the JSON
	err = encoder.Encode(data)
	if err != nil {
		fmt.Println("Error encoding data to JSON:", err)
	}
}

func (n *Node) MonitorPeers() {
	for {
		time.Sleep(10 * time.Second)
		peers := n.host.Network().Peers()
		fmt.Printf("Node %s connected to peers: %v\n", n.host.ID().String(), peers)
	}
}

func generateRandomValue() int64 {
	n, _ := rand.Int(rand.Reader, big.NewInt(100))
	return n.Int64()
}
