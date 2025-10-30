package database

import (
	"context"
	"fmt"

	"github.com/go-pg/pg/v10"
	"github.com/tokamak-network/DRB-node/utils"
)

type BroadcastTrackerRepository struct {
	db *pg.DB
}

func NewBroadcastTrackerRepository(db *pg.DB) *BroadcastTrackerRepository {
	return &BroadcastTrackerRepository{db: db}
}

func (r *BroadcastTrackerRepository) AddBroadcastTracker(ctx context.Context, trackerData *utils.BroadcastTracker) error {
	tracker := mapBroadcastTrackerToScheme(trackerData)
	_, err := r.db.WithContext(ctx).Model(&tracker).Insert()
	return err
}

func (r *BroadcastTrackerRepository) GetBroadcastTrackers(ctx context.Context) ([]*utils.BroadcastTracker, error) {
	var models []BroadcastTrackerScheme
	if err := r.db.WithContext(ctx).Model(&models).Select(); err != nil {
		return nil, err
	}
	trackers := make([]*utils.BroadcastTracker, 0, len(models))
	for _, m := range models {
		trackers = append(trackers, mapBroadcastTrackerSchemeToData(m))
	}
	return trackers, nil
}

func (r *BroadcastTrackerRepository) UpdateBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	model := mapBroadcastTrackerToScheme(tracker)
	_, err := r.db.WithContext(ctx).Model(&model).
		Where("round = ? AND trial_num = ? AND eoa_address = ? AND type = ? AND message_id = ?",
			model.Round, model.TrialNum, model.EOAAddress, model.Type, model.MessageID).
		Update()
	return err
}

func (r *BroadcastTrackerRepository) AddAllBroadcastTrackers(ctx context.Context, trackers []*utils.BroadcastTracker) error {
	// Consider batch insert if supported by ORM for better performance
	for _, tracker := range trackers {
		if err := r.AddBroadcastTracker(ctx, tracker); err != nil {
			return fmt.Errorf("failed to add broadcast tracker data for %s_%s_%s_%s_%s: %w",
				tracker.Round, tracker.TrialNum, tracker.EOAAddress, tracker.Type, tracker.MessageID, err)
		}
	}
	return nil
}

func (r *BroadcastTrackerRepository) DeleteBroadcastTracker(ctx context.Context, tracker *utils.BroadcastTracker) error {
	model := mapBroadcastTrackerToScheme(tracker)
	_, err := r.db.WithContext(ctx).Model(&model).
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
