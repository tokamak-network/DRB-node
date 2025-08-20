package database

import (
	"encoding/hex"
	"time"

	"github.com/tokamak-network/DRB-node/utils"
)

func AddLeaderCommit(commitData *utils.LeaderCommitData) error {
	leaderCommit := mapLeaderCommitDataToScheme(commitData)
	leaderCommit.CreatedAt = time.Now().Unix()

	// Encode hex fields if empty but byte slices are present
	if leaderCommit.CvsHex == "" && len(leaderCommit.Cvs) > 0 {
		leaderCommit.CvsHex = hex.EncodeToString(leaderCommit.Cvs)
	}
	if leaderCommit.CosHex == "" && len(leaderCommit.Cos) > 0 {
		leaderCommit.CosHex = hex.EncodeToString(leaderCommit.Cos)
	}
	if leaderCommit.SecretValueHex == "" && len(leaderCommit.SecretValue) > 0 {
		leaderCommit.SecretValueHex = hex.EncodeToString(leaderCommit.SecretValue)
	}

	_, err := GetDB().Model(&leaderCommit).Insert()
	return err
}

func GetLeaderCommitByRoundAndEoaAddr(round, trialNum, eoaAddr string) (*utils.LeaderCommitData, error) {
	var model LeaderCommitScheme
	err := GetDB().Model(&model).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddr).
		Limit(1).
		Select()
	if err != nil {
		return nil, err
	}
	return mapLeaderCommitSchemeToData(model), nil
}

func GetLeaderCommitsByRoundAndTrialNum(round, trialNum string) ([]*utils.LeaderCommitData, error) {
	var models []LeaderCommitScheme
	if err := GetDB().Model(&models).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Select(); err != nil {
		return nil, err
	}
	commits := make([]*utils.LeaderCommitData, 0, len(models))
	for _, m := range models {
		commits = append(commits, mapLeaderCommitSchemeToData(m))
	}
	return commits, nil
}

func GetRoundsToProcess() ([]*utils.LeaderCommitData, error) {
	var models []LeaderCommitScheme
	if err := GetDB().Model(&models).
		Where("random_number_generated = ? AND submit_merkle_root_done = ?", false, true).
		Select(); err != nil {
		return nil, err
	}
	commits := make([]*utils.LeaderCommitData, 0, len(models))
	for _, m := range models {
		commits = append(commits, mapLeaderCommitSchemeToData(m))
	}
	return commits, nil
}

func UpdateLeaderCommit(leaderCommit *utils.LeaderCommitData) error {
	model := mapLeaderCommitDataToScheme(leaderCommit)
	model.CreatedAt = leaderCommit.CreatedAt

	_, err := GetDB().Model(&model).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", model.Round, model.TrialNum, model.EOAAddress).
		Update()
	return err
}

func UpdateLeaderCommitRandomNumberGenerated(round, trialNum string) error {
	model := LeaderCommitScheme{RandomNumberGenerated: true}
	_, err := GetDB().Model(&model).
		Column("random_number_generated").
		Where("round = ? AND trial_num = ?", round, trialNum).
		Update()
	return err
}

// mapLeaderCommitSchemeToData converts DB model to utils.LeaderCommitData
func mapLeaderCommitSchemeToData(model LeaderCommitScheme) *utils.LeaderCommitData {
	return &utils.LeaderCommitData{
		UniqueKey:             model.Round + "-" + model.TrialNum,
		Round:                 model.Round,
		TrialNum:              model.TrialNum,
		EOAAddress:            model.EOAAddress,
		Cvs:                   utils.ConvertByteArray(model.Cvs),
		CvsHex:                model.CvsHex,
		Cos:                   utils.ConvertByteArray(model.Cos),
		CosHex:                model.CosHex,
		SecretValue:           utils.ConvertByteArray(model.SecretValue),
		SecretValueHex:        model.SecretValueHex,
		Sign:                  utils.SignInfo{R: model.SignR, S: model.SignS, V: model.SignV},
		SubmitMerkleRootDone:  model.SubmitMerkleRootDone,
		RandomNumberGenerated: model.RandomNumberGenerated,
		CreatedAt:             model.CreatedAt,
	}
}

// mapLeaderCommitDataToScheme converts utils.LeaderCommitData to DB model
func mapLeaderCommitDataToScheme(data *utils.LeaderCommitData) LeaderCommitScheme {
	return LeaderCommitScheme{
		Round:                 data.Round,
		TrialNum:              data.TrialNum,
		EOAAddress:            data.EOAAddress,
		Cvs:                   data.Cvs[:],
		CvsHex:                data.CvsHex,
		Cos:                   data.Cos[:],
		CosHex:                data.CosHex,
		SecretValue:           data.SecretValue[:],
		SecretValueHex:        data.SecretValueHex,
		SignR:                 data.Sign.R,
		SignS:                 data.Sign.S,
		SignV:                 data.Sign.V,
		SubmitMerkleRootDone:  data.SubmitMerkleRootDone,
		RandomNumberGenerated: data.RandomNumberGenerated,
		CreatedAt:             data.CreatedAt,
	}
}
