package database

import "github.com/tokamak-network/DRB-node/utils"

func AddNodeInfo(nodeInfo *utils.NodeInfo) error {
	node := &NodeInfoScheme{
		IP:         nodeInfo.IP,
		Port:       nodeInfo.Port,
		PeerID:     nodeInfo.PeerID,
		EOAAddress: nodeInfo.EOAAddress,
	}
	_, err := GetDB().Model(node).Insert()
	return err
}

func UpdateNodeInfo(nodeInfo *utils.NodeInfo) error {
	node := &NodeInfoScheme{
		IP:         nodeInfo.IP,
		Port:       nodeInfo.Port,
		PeerID:     nodeInfo.PeerID,
		EOAAddress: nodeInfo.EOAAddress,
	}
	_, err := GetDB().Model(node).WherePK().Update()
	return err
}

func GetNodeInfos() ([]*utils.NodeInfo, error) {
	var nodes []NodeInfoScheme
	if err := GetDB().Model(&nodes).Select(); err != nil {
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
