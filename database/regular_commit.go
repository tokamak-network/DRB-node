package database

import (
	"context"
	"log"

	"github.com/go-pg/pg/v10"
	"github.com/tokamak-network/DRB-node/utils"
)

type RegularCommitRepository struct {
	db *pg.DB
}

func NewRegularCommitRepository(db *pg.DB) *RegularCommitRepository {
	return &RegularCommitRepository{db: db}
}

func (r *RegularCommitRepository) AddCommit(ctx context.Context, commit *utils.CommitData) error {
	model := mapCommitDataToScheme(commit)
	_, err := r.db.Model(&model).Insert(ctx)
	return err
}

func (r *RegularCommitRepository) UpdateCommit(ctx context.Context, commit *utils.CommitData) error {
	model := mapCommitDataToScheme(commit)
	_, err := r.db.Model(&model).
		Where("round = ? AND trial_num = ?", model.Round, model.TrialNum).
		Update(ctx)
	return err
}

func (r *RegularCommitRepository) GetCommitByRound(ctx context.Context, round, trialNum string) (*utils.CommitData, error) {
	var model CommitDataScheme
	err := r.db.Model(&model).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Limit(1).
		Select(ctx)
	if err != nil {
		log.Printf("Failed to get commit info: %v", err)
		return nil, err
	}
	return mapCommitSchemeToData(model, round, trialNum), nil
}

// Helper: map utils.CommitData to DB model
func mapCommitDataToScheme(commit *utils.CommitData) CommitDataScheme {
	return CommitDataScheme{
		Round:           commit.Round,
		TrialNum:        commit.TrialNum,
		Cvs:             commit.Cvs[:],
		Cos:             commit.Cos[:],
		SecretValue:     commit.SecretValue[:],
		SignR:           commit.Sign.R,
		SignS:           commit.Sign.S,
		SignV:           commit.Sign.V,
		SendToLeader:    commit.SendToLeader,
		SendCosToLeader: commit.SendCosToLeader,
	}
}

// Helper: map DB model to utils.CommitData
func mapCommitSchemeToData(model CommitDataScheme, round, trialNum string) *utils.CommitData {
	var cvs, cos, secretValue [32]byte
	copy(cvs[:], model.Cvs)
	copy(cos[:], model.Cos)
	copy(secretValue[:], model.SecretValue)

	return &utils.CommitData{
		UniqueKey:   round + "-" + trialNum,
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
		Sign: utils.SignInfo{
			R: model.SignR,
			S: model.SignS,
			V: model.SignV,
		},
		SendToLeader:    model.SendToLeader,
		SendCosToLeader: model.SendCosToLeader,
	}
}
