package regularNode_helper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	commitreveal2 "github.com/tokamak-network/DRB-node/commit-reveal2"
	"github.com/tokamak-network/DRB-node/utils"
)

var peerNodeInfo map[string]utils.PeerCommitData
var CosRecevied = make(map[string]map[string]bool)

var revealOrderLock sync.Mutex
var strictOrderWhileReceiving = make(map[string][]string)

// HandleSecret processes incoming secret values and ensures they are accepted in the reveal order.
func HandleCvs(s network.Stream) {
	defer s.Close()

	var message utils.PeerCommitData
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode CVS message: %v", err)
		return
	}
	log.Printf("Received CVS for round %s from EOA %s", message.Round, message.EOAAddress)

	filePath := "peerNodeInfo.json"

	if _, err := os.Stat(filePath); err == nil {
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Failed to open peerNodeInfo.json: %v", err)
			return
		}
		defer file.Close()

		if err := json.NewDecoder(file).Decode(&peerNodeInfo); err != nil {
			log.Printf("Failed to decode peerNodeInfo.json: %v", err)
			return
		}
	} else if os.IsNotExist(err) {
		log.Printf("peerNodeInfo.json does not exist. Creating a new file.")
		peerNodeInfo = make(map[string]utils.PeerCommitData)
	} else {
		log.Printf("Error checking peerNodeInfo.json: %v", err)
		return
	}

	key := fmt.Sprintf("%s+%s", message.Round, message.EOAAddress)
	peerNodeInfo[key] = message

	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create peerNodeInfo.json: %v", err)
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(peerNodeInfo); err != nil {
		log.Printf("Failed to write to peerNodeInfo.json: %v", err)
		return
	}

	log.Printf("Successfully saved CVS data for round %s and EOA %s into peerNodeInfo.json", message.Round, message.EOAAddress)
}

func HandleCos(s network.Stream) {
	defer s.Close()

	var message utils.PeerCommitData

	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode COS message: %v", err)
		return
	}

	log.Printf("Received COS for round %s from EOA %s", message.Round, message.EOAAddress)
	if CosRecevied[message.Round] == nil {
		CosRecevied[message.Round] = make(map[string]bool)
	}
	CosRecevied[message.Round][message.EOAAddress] = true
	filePath := "peerNodeInfo.json"

	if _, err := os.Stat(filePath); err == nil {
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Failed to open peerNodeInfo.json: %v", err)
			return
		}
		defer file.Close()

		if err := json.NewDecoder(file).Decode(&peerNodeInfo); err != nil {
			log.Printf("Failed to decode peerNodeInfo.json: %v", err)
			return
		}
	} else if os.IsNotExist(err) {
		log.Printf("peerNodeInfo.json does not exist. Creating a new file.")
		peerNodeInfo = make(map[string]utils.PeerCommitData)
	} else {
		log.Printf("Error checking peerNodeInfo.json: %v", err)
		return
	}

	key := fmt.Sprintf("%s+%s", message.Round, message.EOAAddress)
	data := peerNodeInfo[key]
	data.Cos = message.Cos
	peerNodeInfo[key] = data
	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create peerNodeInfo.json: %v", err)
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(peerNodeInfo); err != nil {
		log.Printf("Failed to write to peerNodeInfo.json: %v", err)
		return
	}

	log.Printf("Successfully saved COS data for round %s and EOA %s into peerNodeInfo.json", message.Round, message.EOAAddress)
}

func HandleSecret(s network.Stream) {
	defer s.Close()

	var message utils.PeerCommitData
	if err := json.NewDecoder(s).Decode(&message); err != nil {
		log.Printf("Failed to decode secret message: %v", err)
		return
	}

	log.Printf("Received secret value for round %s from EOA %s", message.Round, message.EOAAddress)

	revealOrderLock.Lock()
	defer revealOrderLock.Unlock()

	filePath := "regular_reveal_order.json"
	for {
		data, err := commitreveal2.LoadRevealOrders(filePath)
		if err != nil {
			log.Printf("Failed to load reveal order: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		roundData, exists := data[message.Round]
		if exists {
			if strictOrderWhileReceiving[message.Round] == nil {
				strictOrderWhileReceiving[message.Round] = roundData.OrderedNodes
			}
			break
		}

		log.Printf("Reveal order not yet calculated for round %s. Waiting...", message.Round)
		time.Sleep(2 * time.Second)
	}

	log.Printf("Processing secret value for EOA %s in round %s", message.EOAAddress, message.Round)

	if len(strictOrderWhileReceiving[message.Round]) == 0 || strictOrderWhileReceiving[message.Round][0] != message.EOAAddress {
		log.Printf("EOA %s is not next in the reveal order for broadcasting for round %s", message.EOAAddress, message.Round)
		return
	}

	log.Printf("Received Secret for round %s from EOA %s", message.Round, message.EOAAddress)

	filePath = "peerNodeInfo.json"

	if _, err := os.Stat(filePath); err == nil {
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Failed to open peerNodeInfo.json: %v", err)
			return
		}
		defer file.Close()

		if err := json.NewDecoder(file).Decode(&peerNodeInfo); err != nil {
			log.Printf("Failed to decode peerNodeInfo.json: %v", err)
			return
		}
	} else if os.IsNotExist(err) {
		log.Printf("peerNodeInfo.json does not exist. Creating a new file.")
		peerNodeInfo = make(map[string]utils.PeerCommitData)
	} else {
		log.Printf("Error checking peerNodeInfo.json: %v", err)
		return
	}

	key := fmt.Sprintf("%s+%s", message.Round, message.EOAAddress)
	data := peerNodeInfo[key]
	data.SecretValue = message.SecretValue
	peerNodeInfo[key] = data
	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create peerNodeInfo.json: %v", err)
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(peerNodeInfo); err != nil {
		log.Printf("Failed to write to peerNodeInfo.json: %v", err)
		return
	}

	log.Printf("Successfully saved Secret for round %s and EOA %s into peerNodeInfo.json", message.Round, message.EOAAddress)
	strictOrderWhileReceiving[message.Round] = strictOrderWhileReceiving[message.Round][1:]
}
