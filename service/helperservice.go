package service

import (
	"context"
	"log"

	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-Node/utils" // Adjust the import path to your project structure
)

func IsOperator(operator string) (bool, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetIsOperatorRequest(operator)

	var respData struct {
		OperatorNumberChangeds []utils.OperatorNumberChanged `json:"operatorNumberChangeds"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		log.Printf("Failed to execute query: %v", err)
		return false, err
	}

	for _, record := range respData.OperatorNumberChangeds {
		return record.IsOperator, nil
	}

	return false, nil
}
