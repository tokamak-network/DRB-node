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

func broadCastS(h host.Host, roundNum string, eoaAddress string, secret [32]byte) {
	eoa := common.HexToAddress(eoaAddress)
	nodeInfo := libp2putils.GetConnectedPeers()

	message := struct {
		Secret [32]byte `json:"secret_value"`
	}{
		Secret: secret,
	}
	for _, op := range eth.ActivatedOperators {
		if eoa == op {
			continue
		}
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
