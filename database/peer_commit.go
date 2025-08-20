package database

func AddPeerCommitData(peerData *PeerCommitDataScheme) error {
	_, err := GetDB().Model(peerData).Insert()
	return err
}

func GetPeerCommitData(round, trialNum, eoaAddress string) (*PeerCommitDataScheme, error) {
	var peerCommit PeerCommitDataScheme
	err := GetDB().Model(&peerCommit).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", round, trialNum, eoaAddress).
		Select()
	if err != nil {
		return nil, err
	}
	return &peerCommit, nil
}

func UpdatePeerCommitData(peerData *PeerCommitDataScheme) error {
	_, err := GetDB().Model(peerData).
		Where("round = ? AND trial_num = ? AND eoa_address = ?", peerData.Round, peerData.TrialNum, peerData.EOAAddress).
		Update()
	return err
}
