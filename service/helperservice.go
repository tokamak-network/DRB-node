package service

import (
	"context"
	"strings"

	"github.com/tokamak-network/DRB-node/logger"

	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/utils"
)

func IsOperator(operator string) (bool, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetIsOperatorRequest()
	req.Header.Set("Content-Type", "application/json")

	// Define the response struct
	var respData struct {
		ActivatedOperatorsCollection []struct {
			Operators      []string `json:"operators"`
			OperatorsCount string   `json:"operatorsCount"`
		} `json:"activatedOperators_collection"`
		ActivatedOperators struct {
			Operators      []string `json:"operators"`
			OperatorsCount string   `json:"operatorsCount"`
		} `json:"activatedOperators"`
	}

	// Execute the query
	ctx := context.Background()
	err := client.Run(ctx, req, &respData)
	if err != nil {
		logger.Log.Printf("GraphQL query failed with error: %v", err)
		return false, err
	}

	// logger.Log the raw response data
	logger.Log.Printf("Raw GraphQL Response: %+v\n", respData)

	// Check if data is populated for both collections
	if len(respData.ActivatedOperatorsCollection) == 0 {
		logger.Log.Printf("No operators received in activatedOperators_collection")
	} else {
		for _, collection := range respData.ActivatedOperatorsCollection {
			logger.Log.Printf("Operators received in activatedOperators_collection: %+v", collection.Operators)
		}
	}

	if len(respData.ActivatedOperators.Operators) == 0 {
		logger.Log.Printf("No operators received in activatedOperators")
	} else {
		logger.Log.Printf("Operators received in activatedOperators: %+v", respData.ActivatedOperators.Operators)
	}

	// Determine if the operator exists in the `activatedOperators` list
	isOperator := false
	for _, op := range respData.ActivatedOperators.Operators {
		if strings.ToLower(op) == strings.ToLower(operator) {
			isOperator = true
			break
		}
	}

	logger.Log.Printf("Is operator %s: %v", operator, isOperator)

	return isOperator, nil
}
