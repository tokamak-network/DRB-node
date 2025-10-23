package database

import "github.com/tokamak-network/DRB-node/utils"

type IRevealOrderRepository interface {
	GetRevealOrder(round, trialNum string) (*utils.RevealOrderData, error)
	AddRevealOrder(order *utils.RevealOrderData) error
}

type IPeerCommitRepository interface {
	GetPeerCommitData(round, trialNum, eoaAddress string) (*PeerCommitDataScheme, error)
	AddPeerCommitData(commitData *PeerCommitDataScheme) error
	UpdatePeerCommitData(commitData *PeerCommitDataScheme) error
}

type ILeaderCommitRepository interface {
	GetLeaderCommitByRoundAndEoaAddr(round, trialNum, eoaAddress string) (*utils.LeaderCommitData, error)
	UpdateLeaderCommit(commitData *utils.LeaderCommitData) error
	AddLeaderCommit(commitData *utils.LeaderCommitData) error
	GetLeaderCommitsByRoundAndTrialNum(round, trialNum string) ([]*utils.LeaderCommitData, error)
	UpdateLeaderCommitRandomNumberGenerated(round, trialNum string) error
}

type IBatchRepository interface {
	DeleteOldRoundDataForLeaderNode(currentRound string) error
	DeleteRoundTrialDataForLeaderNode(round, trialNum string) error
	DeleteOldRoundDataForRegularNode(currentRound string) error
	DeleteRoundTrialDataForRegularNode(round, trialNum string) error
}

type IBroadcastTrackerRepository interface {
	AddBroadcastTracker(tracker *utils.BroadcastTracker) error
	UpdateBroadcastTracker(tracker *utils.BroadcastTracker) error
}

type INodeInfoRepository interface {
	AddNodeInfo(nodeInfo *utils.NodeInfo) error
	GetNodeInfos() ([]*utils.NodeInfo, error)
}

type IRegularCommitRepository interface {
	AddCommit(commit *utils.CommitData) error
	UpdateCommit(commit *utils.CommitData) error
	GetCommitByRound(round, trialNum string) (*utils.CommitData, error)
}
