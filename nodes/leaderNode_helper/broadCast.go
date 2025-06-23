package leaderNode_helper

import (
	"context"
	"encoding/json"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/libp2putils"
)

func BroadCastS(h host.Host, roundNum string, eoaAddress string, secret [32]byte) {
	nodeInfo := libp2putils.GetConnectedPeers()

	message := struct {
		Round      string   `json:"round"`
		EOAAddress string   `json:"eoa_address"`
		Secret     [32]byte `json:"secret_value"`
	}{
		Round:      roundNum,
		EOAAddress: eoaAddress,
		Secret:     secret,
	}
	for _, op := range eth.ActivatedOperators {
		stream, err := h.NewStream(context.Background(), nodeInfo[op.Hex()].PeerID, "/secretBroadcast")
		if err != nil {
			log.Printf("Failed to create stream to peer %s: %v", nodeInfo[op.Hex()].PeerID, err)
			continue
		}
		if err := json.NewEncoder(stream).Encode(message); err != nil {
			log.Printf("Failed to send Secret to regular node: %v", err)
		} else {
			log.Printf("Secret sent to regular node %v for round %s", op, roundNum)
		}
		stream.Close()
	}
}

func BroadCastCOS(h host.Host, roundNum string, eoaAddress common.Address, cos [32]byte) {
	nodeInfo := libp2putils.GetConnectedPeers()

	message := struct {
		Round      string   `json:"round"`
		EOAAddress string   `json:"eoa_address"`
		Cos        [32]byte `json:"cos"`
	}{
		Round:      roundNum,
		EOAAddress: eoaAddress.Hex(),
		Cos:        cos,
	}
	for _, op := range eth.ActivatedOperators {
		stream, err := h.NewStream(context.Background(), nodeInfo[op.Hex()].PeerID, "/cosBroadcast")
		if err != nil {
			log.Printf("Failed to create stream to peer %s: %v", nodeInfo[op.Hex()].PeerID, err)
			continue
		}
		if err := json.NewEncoder(stream).Encode(message); err != nil {
			log.Printf("Failed to send CO to regular node: %v", err)
		} else {
			log.Printf("CO sent to regular node %v for round %s", op, roundNum)
		}
		stream.Close()
	}
}

func BroadCastCVS(h host.Host, roundNum string, eoaAddress common.Address, cvs [32]byte) {
	nodeInfo := libp2putils.GetConnectedPeers()

	message := struct {
		Round      string   `json:"round"`
		EOAAddress string   `json:"eoa_address"`
		CVS        [32]byte `json:"cvs"`
	}{
		Round:      roundNum,
		EOAAddress: eoaAddress.Hex(),
		CVS:        cvs,
	}
	for _, op := range eth.ActivatedOperators {
		stream, err := h.NewStream(context.Background(), nodeInfo[op.Hex()].PeerID, "/cvsBroadcast")
		if err != nil {
			log.Printf("Failed to create stream to peer %s: %v", nodeInfo[op.Hex()].PeerID, err)
			continue
		}
		if err := json.NewEncoder(stream).Encode(message); err != nil {
			log.Printf("Failed to send CVS to regular node: %v", err)
		} else {
			log.Printf("CVS sent to regular node %v for round %s", op, roundNum)
		}
		stream.Close()
	}
}
