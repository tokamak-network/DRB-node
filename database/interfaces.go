package database

import (
	"context"

	"github.com/tokamak-network/DRB-node/utils"
)

type IRevealOrderRepository interface {
	GetRevealOrder(ctx context.Context, round, trialNum string) (*utils.RevealOrderData, error)
	AddRevealOrder(ctx context.Context, order *utils.RevealOrderData) error
}

type IPeerCommitRepository interface {
	GetPeerCommitData(ctx context.Context, round, trialNum, eoaAddress string) (*PeerCommitDataScheme, error)
	AddPeerCommitData(ctx context.Context, commitData *PeerCommitDataScheme) error
	UpdatePeerCommitData(ctx context.Context, commitData *PeerCommitDataScheme) error
}

type ILeaderCommitRepository interface {
	GetLeaderCommitByRoundAndEoaAddr(ctx context.Context, round, trialNum, eoaAddress string) (*utils.LeaderCommitData, error)
	UpdateLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error
	AddLeaderCommit(ctx context.Context, commitData *utils.LeaderCommitData) error
	GetLeaderCommitsByRoundAndTrialNum(ctx context.Context, round, trialNum string) ([]*utils.LeaderCommitData, error)
	UpdateLeaderCommitRandomNumberGenerated(ctx context.Context, round, trialNum string) error
}

type IBatchRepository interface {
	DeleteOldRoundDataForLeaderNode(ctx context.Context, currentRound string) error
	DeleteRoundTrialDataForLeaderNode(ctx context.Context, round, trialNum string) error
	DeleteOldRoundDataForRegularNode(ctx context.Context, currentRound string) error
	DeleteRoundTrialDataForRegularNode(ctx context.Context, round, trialNum string) error
}

type IBroadcastTrackerRepository interface {
	AddBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error
	UpdateBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error
}

type INodeInfoRepository interface {
	AddNodeInfo(ctx context.Context, nodeInfo *utils.NodeInfo) error
	GetNodeInfos(ctx context.Context) ([]*utils.NodeInfo, error)
}

type IRegularCommitRepository interface {
	AddCommit(ctx context.Context, commit *utils.CommitData) error
	UpdateCommit(ctx context.Context, commit *utils.CommitData) error
	GetCommitByRound(ctx context.Context, round, trialNum string) (*utils.CommitData, error)
}
