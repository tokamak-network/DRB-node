package database

import (
	"github.com/go-pg/pg/v10"
	"github.com/tokamak-network/DRB-node/utils"
)

type RevealOrderRepository struct {
	db *pg.DB
}

func NewRevealOrderRepository(db *pg.DB) *RevealOrderRepository {
	return &RevealOrderRepository{db: db}
}

func (r *RevealOrderRepository) AddRevealOrder(order *utils.RevealOrderData) error {
	model := mapRevealOrderDataToScheme(order)
	_, err := r.db.Model(&model).Insert()
	return err
}

func (r *RevealOrderRepository) GetRevealOrder(round, trialNum string) (*utils.RevealOrderData, error) {
	var model RevealOrderScheme
	err := r.db.Model(&model).
		Where("round = ? AND trial_num = ?", round, trialNum).
		Limit(1).
		Select()
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
