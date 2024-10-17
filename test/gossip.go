package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message represents the gossip message structure
type Message struct {
	ID      int
	Content string
	TTL     int
}

// Node represents each node in the network
type Node struct {
	ID          int
	Address     string
	Neighbors   []string
	MessageLog  map[int]bool
	Mutex       sync.Mutex
	Listener    net.Listener
	Incoming    chan Message
	Shutdown    chan bool
	GossipDelay time.Duration
}

// NewNode initializes a new node
func NewNode(id int, address string, neighbors []string, gossipDelay time.Duration) *Node {
	return &Node{
		ID:          id,
		Address:     address,
		Neighbors:   neighbors,
		MessageLog:  make(map[int]bool),
		Incoming:    make(chan Message, 10),
		Shutdown:    make(chan bool),
		GossipDelay: gossipDelay,
	}
}

// Start initiates the node's operations
func (n *Node) Start(wg *sync.WaitGroup) {
	defer wg.Done()
	var err error
	n.Listener, err = net.Listen("tcp", n.Address)
	if err != nil {
		fmt.Printf("Node %d failed to start listener: %v\n", n.ID, err)
		return
	}
	fmt.Printf("Node %d listening on %s\n", n.ID, n.Address)

	go n.acceptConnections()
	go n.processIncomingMessages()
	<-n.Shutdown
	n.Listener.Close()
	fmt.Printf("Node %d shutting down.\n", n.ID)
}

// acceptConnections handles incoming TCP connections
func (n *Node) acceptConnections() {
	for {
		conn, err := n.Listener.Accept()
		if err != nil {
			select {
			case <-n.Shutdown:
				return
			default:
				fmt.Printf("Node %d accept error: %v\n", n.ID, err)
				continue
			}
		}
		go n.handleConnection(conn)
	}
}

// handleConnection processes messages from a connection
func (n *Node) handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		msg, err := parseMessage(line)
		if err != nil {
			fmt.Printf("Node %d received invalid message: %v\n", n.ID, err)
			continue
		}
		n.Incoming <- msg
	}
}

// parseMessage converts a string to a Message
func parseMessage(s string) (Message, error) {
	s = strings.TrimSpace(s) // 개행 문자 및 공백 제거
	parts := strings.Split(s, "|")
	if len(parts) != 3 {
		return Message{}, fmt.Errorf("invalid message format")
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		return Message{}, fmt.Errorf("invalid ID: %v", err)
	}
	ttl, err := strconv.Atoi(parts[2])
	if err != nil {
		return Message{}, fmt.Errorf("invalid TTL: %v", err)
	}
	return Message{
		ID:      id,
		Content: parts[1],
		TTL:     ttl,
	}, nil
}

// processIncomingMessages handles messages received by the node
func (n *Node) processIncomingMessages() {
	for {
		select {
		case msg := <-n.Incoming:
			n.handleMessage(msg)
		case <-n.Shutdown:
			return
		}
	}
}

// handleMessage processes a single message
func (n *Node) handleMessage(msg Message) {
	n.Mutex.Lock()
	if n.MessageLog[msg.ID] || msg.TTL <= 0 {
		n.Mutex.Unlock()
		return
	}
	n.MessageLog[msg.ID] = true
	n.Mutex.Unlock()
	fmt.Printf("Node %d received message %d: %s\n", n.ID, msg.ID, msg.Content)
	go n.gossip(msg)
}

// gossip forwards the message to a subset of neighbors
func (n *Node) gossip(msg Message) {
	time.Sleep(n.GossipDelay)
	msg.TTL--
	neighbors := n.selectNeighbors()
	for _, neighbor := range neighbors {
		go n.sendMessage(neighbor, msg)
	}
}

// selectNeighbors randomly selects a subset of neighbors
func (n *Node) selectNeighbors() []string {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()
	count := rand.Intn(len(n.Neighbors)) + 1
	indices := rand.Perm(len(n.Neighbors))[:count]
	selected := make([]string, count)
	for i, idx := range indices {
		selected[i] = n.Neighbors[idx]
	}
	return selected
}

// sendMessage sends a message to a neighbor
func (n *Node) sendMessage(address string, msg Message) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Printf("Node %d failed to connect to %s: %v\n", n.ID, address, err)
		return
	}
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "%d|%s|%d\n", msg.ID, msg.Content, msg.TTL)
	if err != nil {
		fmt.Printf("Node %d failed to send message to %s: %v\n", n.ID, address, err)
	}
}

// Stop signals the node to shutdown
func (n *Node) Stop() {
	close(n.Shutdown)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run gossip.go <node_id> <address> <neighbor_addresses...>")
		return
	}
	id, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Printf("Invalid node ID: %v\n", err)
		return
	}
	address := os.Args[2]
	neighbors := os.Args[3:]
	node := NewNode(id, address, neighbors, time.Second)
	var wg sync.WaitGroup
	wg.Add(1)
	go node.Start(&wg)

	if id == 0 {
		time.Sleep(2 * time.Second)
		msg := Message{
			ID:      rand.Int(),
			Content: "Hello Gossip!",
			TTL:     5,
		}
		node.handleMessage(msg)
	}

	fmt.Println("Press ENTER to shutdown the node.")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	node.Stop()
	wg.Wait()
}
