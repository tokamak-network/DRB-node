package database

import "github.com/tokamak-network/DRB-node/utils"

func AddRevealOrder(order *utils.RevealOrderData) error {
	model := mapRevealOrderDataToScheme(order)
	_, err := GetDB().Model(&model).Insert()
	return err
}

func GetRevealOrders() ([]*utils.RevealOrderData, error) {
	var models []RevealOrderScheme
	if err := GetDB().Model(&models).Select(); err != nil {
		return nil, err
	}
	orders := make([]*utils.RevealOrderData, 0, len(models))
	for _, m := range models {
		orders = append(orders, mapRevealOrderSchemeToData(m))
	}
	return orders, nil
}

func GetRevealOrder(round, trialNum string) (*utils.RevealOrderData, error) {
	var model RevealOrderScheme
	err := GetDB().Model(&model).
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
