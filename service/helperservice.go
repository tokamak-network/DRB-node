package service

import (
	"context"
	"fmt"
	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
	"strings"
)

func IsOperator(operator string) (bool, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetIsOperatorRequest()

	var respData struct {
		Data struct {
			ActivatedOperatorsCollection []struct {
				Operators      []string `json:"operators"`
				OperatorsCount string   `json:"operatorsCount"`
			} `json:"activatedOperators_collection"`
			ActivatedOperators struct {
				Operators      []string `json:"operators"`
				OperatorsCount string   `json:"operatorsCount"`
			} `json:"activatedOperators"`
		} `json:"data"`
	}

	fmt.Println("req: ", req)
	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Failed to execute query: %v", err)
		return false, err
	}

	logger.Log.Infof("Activated Operators Collection Data: %+v", respData.Data.ActivatedOperatorsCollection)
	logger.Log.Infof("Activated Operators Data: %+v", respData.Data.ActivatedOperators)

	isOperator := false
	for _, op := range respData.Data.ActivatedOperators.Operators {
		if strings.ToLower(op) == strings.ToLower(operator) {
			isOperator = true
			break
		}
	}

	logger.Log.Infof("Operator status for %s: %v", operator, isOperator)

	return isOperator, nil
}
