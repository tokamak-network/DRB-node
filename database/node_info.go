package database

import (
	"github.com/go-pg/pg/v10"
	"github.com/tokamak-network/DRB-node/utils"
)

type NodeInfoRepository struct {
	db *pg.DB
}

func NewNodeInfoRepository(db *pg.DB) *NodeInfoRepository {
	return &NodeInfoRepository{db: db}
}

func (r *NodeInfoRepository) AddNodeInfo(nodeInfo *utils.NodeInfo) error {
	node := &NodeInfoScheme{
		IP:         nodeInfo.IP,
		Port:       nodeInfo.Port,
		PeerID:     nodeInfo.PeerID,
		EOAAddress: nodeInfo.EOAAddress,
	}
	_, err := r.db.Model(node).Insert()
	return err
}

func (r *NodeInfoRepository) UpdateNodeInfo(nodeInfo *utils.NodeInfo) error {
	node := &NodeInfoScheme{
		IP:         nodeInfo.IP,
		Port:       nodeInfo.Port,
		PeerID:     nodeInfo.PeerID,
		EOAAddress: nodeInfo.EOAAddress,
	}
	_, err := r.db.Model(node).WherePK().Update()
	return err
}

func (r *NodeInfoRepository) GetNodeInfos() ([]*utils.NodeInfo, error) {
	var nodes []NodeInfoScheme
	if err := r.db.Model(&nodes).Select(); err != nil {
		return nil, err
	}
	nodeInfos := make([]*utils.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		nodeInfos = append(nodeInfos, mapNodeInfoSchemeToNodeInfo(n))
	}
	return nodeInfos, nil
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
