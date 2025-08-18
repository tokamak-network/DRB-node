package database

import (
	"fmt"

	"github.com/tokamak-network/DRB-node/utils"
)

func AddBroadcastTracker(trackerData *utils.BroadcastTracker) error {
	tracker := mapBroadcastTrackerToScheme(trackerData)
	_, err := GetDB().Model(&tracker).Insert()
	return err
}

func GetBroadcastTrackers() ([]*utils.BroadcastTracker, error) {
	var models []BroadcastTrackerScheme
	if err := GetDB().Model(&models).Select(); err != nil {
		return nil, err
	}
	trackers := make([]*utils.BroadcastTracker, 0, len(models))
	for _, m := range models {
		trackers = append(trackers, mapBroadcastTrackerSchemeToData(m))
	}
	return trackers, nil
}

func UpdateBroadcastTracker(tracker *utils.BroadcastTracker) error {
	model := mapBroadcastTrackerToScheme(tracker)
	_, err := GetDB().Model(&model).
		Where("round = ? AND trial_num = ? AND eoa_address = ? AND type = ? AND message_id = ?",
			model.Round, model.TrialNum, model.EOAAddress, model.Type, model.MessageID).
		Update()
	return err
}

func AddAllBroadcastTrackers(trackers []*utils.BroadcastTracker) error {
	// Consider batch insert if supported by ORM for better performance
	for _, tracker := range trackers {
		if err := AddBroadcastTracker(tracker); err != nil {
			return fmt.Errorf("failed to add broadcast tracker data for %s_%s_%s_%s_%s: %w",
				tracker.Round, tracker.TrialNum, tracker.EOAAddress, tracker.Type, tracker.MessageID, err)
		}
	}
	return nil
}

func DeleteBroadcastTracker(tracker *utils.BroadcastTracker) error {
	model := mapBroadcastTrackerToScheme(tracker)
	_, err := GetDB().Model(&model).
		Where("round = ? AND trial_num = ? AND eoa_address = ? AND type = ? AND message_id = ?",
			model.Round, model.TrialNum, model.EOAAddress, model.Type, model.MessageID).
		Delete()
	return err
}

// mapBroadcastTrackerSchemeToData converts DB model to domain model
func mapBroadcastTrackerSchemeToData(model BroadcastTrackerScheme) *utils.BroadcastTracker {
	return &utils.BroadcastTracker{
		Round:        model.Round,
		TrialNum:     model.TrialNum,
		EOAAddress:   model.EOAAddress,
		Type:         model.Type,
		MessageID:    model.MessageID,
		Data:         utils.ConvertByteArray(model.Data),
		Attempts:     model.Attempts,
		MaxAttempts:  model.MaxAttempts,
		Acknowledged: model.Acknowledged,
		LastSent:     model.LastSent,
		Timeout:      model.Timeout,
	}
}

// mapBroadcastTrackerToScheme converts domain model to DB model
func mapBroadcastTrackerToScheme(tracker *utils.BroadcastTracker) BroadcastTrackerScheme {
	return BroadcastTrackerScheme{
		Round:        tracker.Round,
		TrialNum:     tracker.TrialNum,
		EOAAddress:   tracker.EOAAddress,
		Type:         tracker.Type,
		MessageID:    tracker.MessageID,
		Data:         tracker.Data[:],
		Attempts:     tracker.Attempts,
		MaxAttempts:  tracker.MaxAttempts,
		Acknowledged: tracker.Acknowledged,
		LastSent:     tracker.LastSent,
		Timeout:      tracker.Timeout,
	}
}
