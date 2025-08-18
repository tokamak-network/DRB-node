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
