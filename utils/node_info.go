package utils

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

// NodeInfo structure to store information about the node
type NodeInfo struct {
	IP         string `json:"ip"`
	Port       string `json:"port"`
	PeerID     string `json:"peer_id"`
	EOAAddress string `json:"eoa_address"`
}

// SaveNodeInfo saves the node information to a file
func SaveNodeInfo(nodeInfos []NodeInfo) error {
	fileName := "node_info.json"
	data, err := json.MarshalIndent(nodeInfos, "", "  ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(fileName, data, 0644)
	if err != nil {
		return err
	}
	log.Printf("Node info saved to %s", fileName)
	return nil
}

// LoadNodeInfo loads the node information from a file
func LoadNodeInfo() ([]NodeInfo, error) {
	fileName := "node_info.json"
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var nodeInfos []NodeInfo
	err = json.Unmarshal(data, &nodeInfos)
	if err != nil {
		return nil, err
	}

	return nodeInfos, nil
}
