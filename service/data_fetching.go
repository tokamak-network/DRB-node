package service

import (
	"context"

	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/logger" // Use the custom logger package
	"github.com/tokamak-network/DRB-node/utils"
)

// GetRecoveredData fetches recovered data from a GraphQL endpoint
func GetRecoveredData(round string) ([]utils.RecoveredData, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetRecoveredDataRequest(round)

	var respData struct {
		Recovereds []struct {
			Round          string `json:"round"`
			BlockTimestamp string `json:"blockTimestamp"`
			ID             string `json:"id"`
			MsgSender      string `json:"msgSender"`
			Omega          string `json:"omega"`
			RoundInfo      struct {
				IsRecovered bool `json:"isRecovered"`
			} `json:"roundInfo"`
		} `json:"recovereds"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Failed to execute query: %v", err) // Replacing logrus with logger.Log
		return nil, err
	}

	var recoveredData []utils.RecoveredData
	for _, item := range respData.Recovereds {
		recoveredData = append(recoveredData, utils.RecoveredData{
			Round:          item.Round,
			BlockTimestamp: item.BlockTimestamp,
			ID:             item.ID,
			MsgSender:      item.MsgSender,
			Omega:          item.Omega,
			IsRecovered:    item.RoundInfo.IsRecovered,
		})
	}

	return recoveredData, nil
}

// GetCommitData retrieves commit data for a given round and returns a slice of CommitData and an error
func GetCommitData(round string) ([]utils.CommitData, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetCommitDataRequest(round)

	var respData struct {
		CommitCs []utils.CommitData `json:"commitCs"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Failed to execute query: %v", err) // Replacing logrus with logger.Log
		return nil, err
	}

	return respData.CommitCs, nil
}

// GetFulfillRandomnessData fetches fulfill randomness data for a given round.
func GetFulfillRandomnessData(round string) ([]utils.FulfillRandomnessData, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetFulfillRandomnessDataRequest(round)

	var respData struct {
		FulfillRandomnesses []struct {
			MsgSender      string `json:"msgSender"`
			BlockTimestamp string `json:"blockTimestamp"`
			Success        bool   `json:"success"`
		} `json:"fulfillRandomnesses"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Failed to execute GetFulfillRandomnessData query: %v", err) // Replacing logrus with logger.Log
		return nil, err
	}

	var fulfillRandomnessData []utils.FulfillRandomnessData
	for _, item := range respData.FulfillRandomnesses {
		fulfillRandomnessData = append(fulfillRandomnessData, utils.FulfillRandomnessData{
			MsgSender:      item.MsgSender,
			BlockTimestamp: item.BlockTimestamp,
			Success:        item.Success,
		})
	}

	return fulfillRandomnessData, nil
}

