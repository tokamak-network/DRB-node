package database

import (
	"context"

	"github.com/go-pg/pg/v10"
)

type PeerCommitRepository struct {
	db *pg.DB
}

func NewPeerCommitRepository(db *pg.DB) *PeerCommitRepository {
	return &PeerCommitRepository{db: db}
}

func (r *PeerCommitRepository) AddPeerCommitData(ctx context.Context, peerData *PeerCommitDataScheme) error {
	_, err := r.db.WithContext(ctx).Model(peerData).Insert()
	return err
}

func (r *PeerCommitRepository) GetPeerCommitData(ctx context.Context, round, trialNum, eoaAddress string) (*PeerCommitDataScheme, error) {
	var peerCommit PeerCommitDataScheme
	err := r.db.WithContext(ctx).Model(&peerCommit).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddress).
		Select()
	if err != nil {
		return nil, err
	}
	return &peerCommit, nil
}

func (r *PeerCommitRepository) UpdatePeerCommitData(ctx context.Context, peerData *PeerCommitDataScheme) error {
	_, err := r.db.WithContext(ctx).Model(peerData).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", peerData.Round, peerData.TrialNum, peerData.EOAAddress).
		Update()
	return err
}
