package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-pg/pg/v10"
	"github.com/tokamak-network/DRB-node/utils"
)

type RevealOrderRepository struct {
	db *pg.DB
}

func NewRevealOrderRepository(db *pg.DB) *RevealOrderRepository {
	return &RevealOrderRepository{db: db}
}

func (r *RevealOrderRepository) AddRevealOrder(ctx context.Context, order *utils.RevealOrderData) error {
	// Validate required fields
	if order.Round == "" {
		return errors.New("round cannot be empty")
	}
	if order.TrialNum == "" {
		return errors.New("trialNum cannot be empty")
	}
	if order.RV == "" {
		return errors.New("RV cannot be empty")
	}

	// Validate arrays are not empty
	if len(order.OrderedNodes) == 0 {
		return errors.New("orderedNodes cannot be empty")
	}
	if len(order.RevealOrder) == 0 {
		return errors.New("revealOrder cannot be empty")
	}

	// validate array lengths match
	if len(order.OrderedNodes) != len(order.RevealOrder) {
		return fmt.Errorf("orderedNodes length (%d) must match revealOrder length (%d)",
			len(order.OrderedNodes), len(order.RevealOrder))
	}

	// validate negative indices
	maxIndex := len(order.OrderedNodes) - 1

	seen := make(map[int]bool)
	for i, idx := range order.RevealOrder {
		if idx < 0 {
			return fmt.Errorf("revealOrder[%d] contains negative index: %d", i, idx)
		}

		if idx > maxIndex {
			return fmt.Errorf("revealOrder[%d] contains out-of-bounds index %d (max: %d)", i, idx, maxIndex)
		}

		if seen[idx] {
			return fmt.Errorf("revealOrder[%d] contains duplicate index: %d", i, idx)
		}
		seen[idx] = true

	}
	model := mapRevealOrderDataToScheme(order)
	_, err := r.db.Model(&model).Insert(ctx)
	return err
}

func (r *RevealOrderRepository) GetRevealOrder(ctx context.Context, round, trialNum string) (*utils.RevealOrderData, error) {
	var model RevealOrderScheme
	err := r.db.Model(&model).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Limit(1).
		Select(ctx)
	if err != nil {
		return nil, err
	}
	return mapRevealOrderSchemeToData(model), nil
}

// Helper: map DB model to utils.RevealOrderData
func mapRevealOrderSchemeToData(model RevealOrderScheme) *utils.RevealOrderData {
	return &utils.RevealOrderData{
		UniqueKey:    model.Round + "-" + model.TrialNum,
		Round:        model.Round,
		TrialNum:     model.TrialNum,
		OrderedNodes: model.OrderedNodes,
		RevealOrder:  model.RevealOrder,
		RV:           model.RV,
	}
}

// Helper: map utils.RevealOrderData to DB model
func mapRevealOrderDataToScheme(data *utils.RevealOrderData) RevealOrderScheme {
	return RevealOrderScheme{
		Round:        data.Round,
		TrialNum:     data.TrialNum,
		OrderedNodes: data.OrderedNodes,
		RevealOrder:  data.RevealOrder,
		RV:           data.RV,
	}
}
