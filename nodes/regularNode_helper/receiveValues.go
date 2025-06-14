package regularNode_helper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tokamak-network/DRB-node/utils"
)
var peerNodeInfo map[string]utils.PeerCommitData
var CosRecevied = make(map[string]map[string]bool)
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
	// CosRecevied[message.Round]["0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC"] = true
	filePath := "peerNodeInfo.json"
	// var peerNodeInfo map[string]utils.PeerCommitData

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
        log.Printf("Failed to decode COS message: %v", err)
        return
    }

    log.Printf("Received Secret for round %s from EOA %s", message.Round, message.EOAAddress)

    filePath := "peerNodeInfo.json"
    // var peerNodeInfo map[string]utils.PeerCommitData

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
}
