package main

import (
	"sync"
	"testing"
	"time"
)

// TestNewNode checks if a new node is initialized with the correct parameters.
func TestNewNode(t *testing.T) {
	nodeID := 1
	address := "localhost:8001"
	neighbors := []string{"localhost:8002", "localhost:8003"}
	gossipDelay := 500 * time.Millisecond
	node := NewNode(nodeID, address, neighbors, gossipDelay)

	if node.ID != nodeID {
		t.Errorf("Expected node ID %d, got %d", nodeID, node.ID)
	}
	if node.Address != address {
		t.Errorf("Expected address %s, got %s", address, node.Address)
	}
	if len(node.Neighbors) != len(neighbors) {
		t.Errorf("Expected %d neighbors, got %d", len(neighbors), len(node.Neighbors))
	}
	if node.GossipDelay != gossipDelay {
		t.Errorf("Expected gossip delay %v, got %v", gossipDelay, node.GossipDelay)
	}
}

// TestHandleMessage verifies that a node processes a message and updates its log.
func TestHandleMessage(t *testing.T) {
	node := NewNode(1, "localhost:8001", []string{"localhost:8002"}, time.Second)
	msg := Message{
		ID:      101,
		Content: "Test Message",
		TTL:     5,
	}

	node.handleMessage(msg)

	// Check if the message is logged
	if !node.MessageLog[msg.ID] {
		t.Errorf("Message ID %d not found in message log", msg.ID)
	}
}

// TestDuplicateMessage ensures that a duplicate message is not processed.
func TestDuplicateMessage(t *testing.T) {
	node := NewNode(1, "localhost:8001", []string{"localhost:8002"}, time.Second)
	msg := Message{
		ID:      101,
		Content: "Duplicate Message",
		TTL:     5,
	}

	node.handleMessage(msg)
	// Attempt to process the same message again
	node.handleMessage(msg)

	// Check if the message is only logged once
	if len(node.MessageLog) != 1 {
		t.Errorf("Duplicate message processed; expected message log length 1, got %d", len(node.MessageLog))
	}
}

// TestSelectNeighbors ensures that selectNeighbors returns a subset of neighbors.
func TestSelectNeighbors(t *testing.T) {
	node := NewNode(1, "localhost:8001", []string{"localhost:8002", "localhost:8003", "localhost:8004"}, time.Second)
	selected := node.selectNeighbors()

	if len(selected) == 0 {
		t.Errorf("No neighbors selected; expected at least one neighbor")
	}
	if len(selected) > len(node.Neighbors) {
		t.Errorf("Selected too many neighbors; expected at most %d, got %d", len(node.Neighbors), len(selected))
	}
}

// TestGossipTTL ensures that a message with TTL=0 is not forwarded.
func TestGossipTTL(t *testing.T) {
	node := NewNode(1, "localhost:8001", []string{"localhost:8002"}, time.Second)
	msg := Message{
		ID:      102,
		Content: "Message with TTL=0",
		TTL:     0,
	}

	// Gossip should not propagate a message with TTL=0
	node.gossip(msg)

	if node.MessageLog[msg.ID] {
		t.Errorf("Message with TTL=0 should not be gossiped")
	}
}

// TestParseMessage verifies that parseMessage correctly parses valid messages.
func TestParseMessage(t *testing.T) {
	raw := "103|Sample Message|5"
	expected := Message{
		ID:      103,
		Content: "Sample Message",
		TTL:     5,
	}

	msg, err := parseMessage(raw)
	if err != nil {
		t.Errorf("Error parsing message: %v", err)
	}

	if msg != expected {
		t.Errorf("Parsed message does not match expected. Got %+v, expected %+v", msg, expected)
	}
}

// TestParseMessageInvalidFormat ensures parseMessage returns an error for invalid formats.
func TestParseMessageInvalidFormat(t *testing.T) {
	invalidRaw := "invalid|message|format"
	_, err := parseMessage(invalidRaw)

	if err == nil {
		t.Error("Expected error for invalid message format, but got nil")
	}
}

// TestStop verifies that the node stops correctly.
func TestStop(t *testing.T) {
	node := NewNode(1, "localhost:8001", []string{"localhost:8002"}, time.Second)
	var wg sync.WaitGroup
	wg.Add(1)
	go node.Start(&wg)

	node.Stop()
	wg.Wait()

	select {
	case <-node.Shutdown:
		// Pass: Node stopped successfully
	default:
		t.Error("Node did not shut down correctly")
	}
}
