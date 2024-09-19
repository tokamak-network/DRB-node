package service

import (
	"context"
	"fmt"
	"os"

	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

func GetRandomWordRequested(client *utils.Client) (*utils.RoundResults, error) {
	config := utils.GetConfig()
	sgClient := graphql.NewClient(config.SubgraphURL)

	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		return nil, fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	req := utils.GetRandomWordsRequestedRequest()
	ctx := context.Background()

	var respData struct {
		RandomWordsRequested []utils.RandomWordRequestedStruct `json:"roundInfos"`
	}

	if err := sgClient.Run(ctx, req, &respData); err != nil {
		return nil, fmt.Errorf("error fetching random words requested: %v", err)
	}

	latestRounds := updateLatestRounds(respData.RandomWordsRequested)

	rounds := convertToRoundStruct(latestRounds)
	filteredRounds := filterRounds(rounds)

	results := &utils.RoundResults{
		RevealRounds: []string{},
		CommitRounds: []string{},
	}

	for _, round := range filteredRounds {
		data := round.Data

		isValid, err := IsValidOperator(data.Round, client)
		if err != nil || !isValid {
			continue
		}

		hasCommitted, err := HasOperatorCommitted(data.Round, walletAddress, sgClient)
		if err != nil {
			logger.Log.Errorf("Error checking if operator has committed for round %s: %v", data.Round, err)
			continue
		}

		hasRevealed, err := HasOperatorRevealed(data.Round, walletAddress, sgClient)
		if err != nil {
			logger.Log.Errorf("Error checking if operator has revealed for round %s: %v", data.Round, err)
			continue
		}

		if !hasCommitted && data.CommitCount < "2" {
			results.CommitRounds = append(results.CommitRounds, data.Round)
		} else if !hasRevealed && data.RevealCount < "2" && data.CommitCount >= "2" {
			results.RevealRounds = append(results.RevealRounds, data.Round)
		}
	}

	logResults(results)
	return results, nil
}
