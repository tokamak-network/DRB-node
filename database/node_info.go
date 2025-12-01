package database

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-pg/pg/v10"
	"github.com/tokamak-network/DRB-node/utils"
)

type NodeInfoRepository struct {
	db *pg.DB
}

func NewNodeInfoRepository(db *pg.DB) *NodeInfoRepository {
	return &NodeInfoRepository{db: db}
}

func (r *NodeInfoRepository) AddAndUpdateNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error {
	if nodeInfo == nil {
		return fmt.Errorf("nodeInfo cannot be nil")
	}

	node := &NodeInfoScheme{
		IP:         nodeInfo.IP,
		Port:       nodeInfo.Port,
		PeerID:     nodeInfo.PeerID,
		EOAAddress: nodeInfo.EOAAddress,
	}

	// First, check if there's a record with the same IP but different EOA
	var existingByIP NodeInfoScheme
	errIP := r.db.WithContext(ctx).Model(&existingByIP).
		Where("ip = ?", nodeInfo.IP).
		Select()

	if errIP == nil && existingByIP.EOAAddress != nodeInfo.EOAAddress {
		log.Printf("IP %s exists with different EOA (%s), deleting old record",
			nodeInfo.IP, existingByIP.EOAAddress)
		_, err := r.db.WithContext(ctx).Model(&existingByIP).
			Where("ip = ?", nodeInfo.IP).
			Delete()
		if err != nil {
			log.Printf("Failed to delete old record with IP %s: %v", nodeInfo.IP, err)
		}
	}

	// Now try UPSERT based on eoa_address
	_, err := r.db.WithContext(ctx).Model(node).
		OnConflict("(eoa_address) DO UPDATE").
		Set("peer_id = EXCLUDED.peer_id").
		Set("ip = EXCLUDED.ip").
		Set("port = EXCLUDED.port").
		Insert()

	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "duplicate key") && strings.Contains(errStr, "ip_key") {
			log.Printf("Retrying after deleting conflicting IP record")
			_, delErr := r.db.WithContext(ctx).Model(&NodeInfoScheme{}).
				Where("ip = ? AND eoa_address != ?", nodeInfo.IP, nodeInfo.EOAAddress).
				Delete()
			if delErr == nil {
				_, err = r.db.WithContext(ctx).Model(node).
					OnConflict("(eoa_address) DO UPDATE").
					Set("peer_id = EXCLUDED.peer_id").
					Set("ip = EXCLUDED.ip").
					Set("port = EXCLUDED.port").
					Insert()
			}
		}
	}

	if err != nil {
		log.Printf("Database error occurred: %s", err.Error())
	} else {
		log.Printf("Successfully inserted/updated NodeInfo: EOA=%s, IP=%s, Port=%s, PeerID=%s",
			nodeInfo.EOAAddress, nodeInfo.IP, nodeInfo.Port, nodeInfo.PeerID)
	}

	return err
}

func (r *NodeInfoRepository) GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error) {
	var nodes []NodeInfoScheme
	if err := r.db.WithContext(ctx).Model(&nodes).Select(); err != nil {
		return nil, err
	}
	nodeInfos := make([]*utils.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		nodeInfos = append(nodeInfos, mapNodeInfoSchemeToNodeInfo(n))
	}
	return nodeInfos, nil
}

func (r *NodeInfoRepository) DeleteNodeInfoByEOA(ctx context.Context, eoaAddress string) error {
	_, err := r.db.WithContext(ctx).Model(&NodeInfoScheme{}).
		Where("eoa_address = ?", eoaAddress).
		ForceDelete()
	return err
}

// Helper function for mapping
func mapNodeInfoSchemeToNodeInfo(node NodeInfoScheme) *utils.NodeInfo {
	return &utils.NodeInfo{
		IP:         node.IP,
		Port:       node.Port,
		PeerID:     node.PeerID,
		EOAAddress: node.EOAAddress,
	}
}
