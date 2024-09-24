package service

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ethereum/go-ethereum/common"
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
	logger.Log.Printf("Raw GraphQL Response: %+v", respData)

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
		if strings.EqualFold(op, operator) {
			isOperator = true
			break
		}
	}

	logger.Log.Printf("Is operator %s: %v", operator, isOperator)

	return isOperator, nil
}

// Helper function to check if the operator has already committed for the round
func HasOperatorCommitted(round string, walletAddress string, client *graphql.Client) (bool, error) {
	req := utils.GetCommitDataRequest(round)
	var respData struct {
		Commits []struct {
			Operator string `json:"operator"`
		} `json:"commits"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		return false, err
	}

	// Convert wallet address to the standard format (checksummed format)
	walletAddr := common.HexToAddress(walletAddress)

	for _, commit := range respData.Commits {
		commitAddr := common.HexToAddress(commit.Operator)

		// Compare the wallet address and operator address in checksummed format
		if strings.EqualFold(commitAddr.Hex(), walletAddr.Hex()) {
			return true, nil
		}
	}

	return false, nil
}

// Helper function to check if the operator has already revealed for the round
func HasOperatorRevealed(round string, walletAddress string, client *graphql.Client) (bool, error) {
	req := utils.GetRevealDataRequest(round)
	var respData struct {
		Reveals []struct {
			Operator string `json:"operator"`
		} `json:"reveals"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		return false, err
	}

	// Convert wallet address to the standard format (checksummed format)
	walletAddr := common.HexToAddress(walletAddress)

	for _, reveal := range respData.Reveals {
		revealAddr := common.HexToAddress(reveal.Operator)

		// Compare the wallet address and operator address in checksummed format
		if strings.EqualFold(revealAddr.Hex(), walletAddr.Hex()) {
			return true, nil
		}
	}

	return false, nil
}

func GetActivatedOperatorsAtRound(round *big.Int) ([]common.Address, error) {
	// Convert *big.Int to int (assuming it can safely fit)
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	// Pass the converted int value to the GraphQL request
	req := utils.GetActivatedOperatorsAtRoundRequest(round.String())

	var respData struct {
		RandomNumberRequesteds []struct {
			ActivatedOperators []string `json:"activatedOperators"`
			Round              string   `json:"round"`
		} `json:"randomNumberRequesteds"`
	}

	err := client.Run(context.Background(), req, &respData)
	if err != nil {
		return nil, err
	}

	var activatedOperators []common.Address
	for _, operator := range respData.RandomNumberRequesteds[0].ActivatedOperators {
		activatedOperators = append(activatedOperators, common.HexToAddress(operator))
	}

	return activatedOperators, nil
}

// IsValidOperator checks if the given walletAddress is a valid operator for a specific round
func IsValidOperator(round string, client *utils.Client) (bool, error) {
	// Get wallet address from environment variable
	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		logger.Log.Error("WALLET_ADDRESS environment variable is not set")
		return false, nil
	}

	// Convert the wallet address to checksummed format
	walletAddr := common.HexToAddress(walletAddress)

	// Convert round to *big.Int
	roundInt, ok := new(big.Int).SetString(round, 10)
	if !ok {
		logger.Log.Errorf("Invalid round value: %s", round)
		return false, nil
	}

	// Fetch the activated operators for the specified round
	activatedOperators, err := GetActivatedOperatorsAtRound(roundInt)
	if err != nil {
		logger.Log.Errorf("Error fetching activated operators for round %s: %v", round, err)
		return false, err
	}

	// Compare the walletAddress with the list of activated operators (using normalized addresses)
	for _, operator := range activatedOperators {
		// Convert each operator address to checksummed format
		operatorAddr := common.HexToAddress(operator.Hex())

		// Compare the normalized wallet address and operator address
		if operatorAddr.Hex() == walletAddr.Hex() {
			// Wallet is a valid operator
			return true, nil
		}
	}

	// Wallet is not a valid operator
	return false, nil
}

// Helper function to update latest rounds
func updateLatestRounds(data []utils.RandomWordRequestedStruct) map[string]utils.RandomWordRequestedStruct {
	latestRounds := make(map[string]utils.RandomWordRequestedStruct)
	for _, item := range data {
		existing, ok := latestRounds[item.Round]
		if !ok || isLaterTimestamp(item, existing) {
			latestRounds[item.Round] = item
		}
	}
	return latestRounds
}

// Helper function to check if timestamp is later
func isLaterTimestamp(a, b utils.RandomWordRequestedStruct) bool {
	existingTimestamp, _ := strconv.Atoi(b.RequestedTimestamp)
	currentTimestamp, _ := strconv.Atoi(a.RequestedTimestamp)
	return currentTimestamp > existingTimestamp
}

// Helper function to convert map to slice with round int
func convertToRoundStruct(latestRounds map[string]utils.RandomWordRequestedStruct) []struct {
	RoundInt int
	Data     utils.RandomWordRequestedStruct
} {
	var rounds []struct {
		RoundInt int
		Data     utils.RandomWordRequestedStruct
	}
	for round, data := range latestRounds {
		roundInt, err := strconv.Atoi(round)
		if err != nil {
			logger.Log.Errorf("Error converting round to int: %s, %v", round, err)
			continue
		}
		rounds = append(rounds, struct {
			RoundInt int
			Data     utils.RandomWordRequestedStruct
		}{RoundInt: roundInt, Data: data})
	}
	return rounds
}

func GetOperatorCountByRound(round string, client *utils.Client) (int, error) {
	roundInt, ok := new(big.Int).SetString(round, 10)
	if !ok {
		return 0, fmt.Errorf("invalid round value: %s", round)
	}

	// Use the existing GetActivatedOperatorsAtRound function
	activatedOperators, err := GetActivatedOperatorsAtRound(roundInt)
	if err != nil {
		logger.Log.Errorf("Error fetching activated operators for round %s: %v", round, err)
		return 0, err
	}

	operatorCount := len(activatedOperators)
	return operatorCount, nil
}

func filterRounds(rounds []struct {
	RoundInt int
	Data     utils.RandomWordRequestedStruct
}, client *utils.Client) []struct {
	RoundInt int
	Data     utils.RandomWordRequestedStruct
} {
	currentTime := time.Now()
	var filteredRounds []struct {
		RoundInt int
		Data     utils.RandomWordRequestedStruct
	}

	for _, round := range rounds {
		data := round.Data
		requestedTime := time.Unix(parseTimestamp(data.RequestedTimestamp), 0)

		// Check if both commit and reveal are expired
		if commitExpired(currentTime, requestedTime) && revealExpired(currentTime, requestedTime) {
			// If both commit and reveal phases are expired, skip this round
			continue
		}

		// If either phase is not expired, append the round
		filteredRounds = append(filteredRounds, round)
	}
	return filteredRounds
}


func parseTimestamp(timestamp string) int64 {
	t, _ := strconv.ParseInt(timestamp, 10, 64)
	return t
}

func commitExpired(currentTime, requestedTime time.Time) bool {
    // Commit phase is expired if the current time is more than 5 minutes after the requested time
    return currentTime.Sub(requestedTime) > (5*time.Minute + 10*time.Second)
}

func revealExpired(currentTime, requestedTime time.Time) bool {
    // Reveal phase is expired if the current time is more than 15 minutes after the requested time
    return currentTime.Sub(requestedTime) > (15*time.Minute + 10*time.Second)
}

func commitDurationOver(requestedTimestamp string) bool {
	// Parse the requested timestamp into a Unix time
	requestedTime := time.Unix(parseTimestamp(requestedTimestamp), 0)
	currentTime := time.Now()

	// Check if more than 5 minutes and 10 seconds have passed
	return currentTime.Sub(requestedTime) > (5*time.Minute + 10*time.Second)
}

func logResults(results *utils.RoundResults) {
	logger.Log.Info("---------------------------------------------------------------------------")
	w := tabwriter.NewWriter(log.Writer(), 0, 0, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "Category\tRounds")
	fmt.Fprintln(w, "RevealRounds\t", results.RevealRounds)
	fmt.Fprintln(w, "CommitRounds\t", results.CommitRounds)
	w.Flush()
	logger.Log.Info("---------------------------------------------------------------------------")
}
