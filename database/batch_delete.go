package database

import "fmt"

func DeleteRoundTrialDataForLeaderNode(round, trialNum string) error {
	db := GetDB()
	tables := []interface{}{
		(*LeaderCommitScheme)(nil),
		(*RevealOrderScheme)(nil),
		(*BroadcastTrackerScheme)(nil),
	}
	for _, table := range tables {
		if _, err := db.Model(table).Where("round = ? AND trial_num = ?", round, trialNum).Delete(); err != nil {
			return fmt.Errorf("failed to delete from %T: %w", table, err)
		}
	}
	return nil
}

func DeleteRoundTrialDataForRegularNode(round, trialNum string) error {
	db := GetDB()
	tables := []interface{}{
		(*CommitDataScheme)(nil),
		(*RevealOrderScheme)(nil),
		(*PeerCommitDataScheme)(nil),
		(*BroadcastTrackerScheme)(nil),
	}
	for _, table := range tables {
		if _, err := db.Model(table).Where("round = ? AND trial_num = ?", round, trialNum).Delete(); err != nil {
			return fmt.Errorf("failed to delete from %T: %w", table, err)
		}
	}
	return nil
}

// DeleteOldRoundDataForLeaderNode deletes all rows from all relevant tables in leader node except current round
func DeleteOldRoundDataForLeaderNode(currentRound string) error {
	db := GetDB()

	// Delete from leader_commit_schemes
	_, err := db.Model((*LeaderCommitScheme)(nil)).Where("round != ?", currentRound).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete from leader_commit_schemes: %w", err)
	}

	// Delete from reveal_order_schemes
	_, err = db.Model((*RevealOrderScheme)(nil)).Where("round != ?", currentRound).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete from reveal_order_schemes: %w", err)
	}

	// Delete from broadcast_tracker_schemes
	_, err = db.Model((*BroadcastTrackerScheme)(nil)).Where("round != ?", currentRound).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete from broadcast_tracker_schemes: %w", err)
	}

	return nil
}

// DeleteOldRoundDataForRegularNode deletes all rows from all relevant tables in regular node except current round
func DeleteOldRoundDataForRegularNode(currentRound string) error {
	db := GetDB()

	// Delete from commit_data_schemes
	_, err := db.Model((*CommitDataScheme)(nil)).Where("round != ?", currentRound).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete from commit_data_schemes: %w", err)
	}
	// Delete from reveal_order_schemes
	_, err = db.Model((*RevealOrderScheme)(nil)).Where("round != ?", currentRound).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete from reveal_order_schemes: %w", err)
	}
	// Delete from peer_commit_data_schemes
	_, err = db.Model((*PeerCommitDataScheme)(nil)).Where("round != ?", currentRound).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete from peer_commit_data_schemes: %w", err)
	}
	// Delete from broadcast_tracker_schemes
	_, err = db.Model((*BroadcastTrackerScheme)(nil)).Where("round != ?", currentRound).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete from broadcast_tracker_schemes: %w", err)
	}

	return nil
}
