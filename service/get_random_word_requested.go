package service

import (
	"context"
	"fmt"
	"math/big"
	"os"

	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/service/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

func GetRandomWordRequested(pofClient *utils.Client) (*utils.RoundResults, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		return nil, fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	req := utils.GetRandomWordsRequestedRequest()
	ctx := context.Background()

	var respData struct {
		RandomWordsRequested []utils.RandomWordRequestedStruct `json:"roundInfos"`
	}

	if err := client.Run(ctx, req, &respData); err != nil {
		return nil, fmt.Errorf("error fetching random words requested: %v", err)
	}

	latestRounds := updateLatestRounds(respData.RandomWordsRequested)

	rounds := convertToRoundStruct(latestRounds)
	filteredRounds := filterRounds(rounds, pofClient)

	results := &utils.RoundResults{
		RevealRounds: []string{},
		CommitRounds: []string{},
	}

	for _, round := range filteredRounds {
		data := round.Data

		isValid, err := IsValidOperator(data.Round, pofClient)
		if err != nil || !isValid {
			continue
		}

		hasCommitted, err := HasOperatorCommitted(data.Round, walletAddress, client)
		if err != nil {
			logger.Log.Errorf("Error checking if operator has committed for round %s: %v", data.Round, err)
			continue
		}

		hasRevealed, err := HasOperatorRevealed(data.Round, walletAddress, client)
		if err != nil {
			logger.Log.Errorf("Error checking if operator has revealed for round %s: %v", data.Round, err)
			continue
		}

		// Convert data.Round (string) to *big.Int
		roundInt := new(big.Int)
		roundInt, ok := roundInt.SetString(data.Round, 10) // base 10
		if !ok {
			logger.Log.Errorf("Failed to convert round string to *big.Int: %s", data.Round)
			return nil, fmt.Errorf("invalid round value: %s", data.Round)
		}

		// Assume ctx and pofClient are already available in your function scope.
		activatedOperators, err := transactions.GetActivatedOperatorsAtRound(ctx, roundInt, pofClient)
		if err != nil {
			logger.Log.Errorf("Error fetching activated operators: %v", err)
			return nil, err
		}

		// Subtract 1 to account for the zero address in the array.
		operatorCount := len(activatedOperators) - 1

		// Now replace the static "2" with operatorCount in your conditions
		if !hasCommitted && data.CommitCount < fmt.Sprintf("%d", operatorCount) {
			results.CommitRounds = append(results.CommitRounds, data.Round)
		} else if !hasRevealed && data.CommitCount >= "2" &&  data.RevealCount < fmt.Sprintf("%d", operatorCount) && (data.CommitCount >= fmt.Sprintf("%d", operatorCount) || commitDurationOver(data.RequestedTimestamp)) {
			results.RevealRounds = append(results.RevealRounds, data.Round)
		}
	}

	logResults(results)
	return results, nil
}
