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
	"github.com/tokamak-network/DRB-node/service/transactions"

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
	activatedOperators, err := transactions.GetActivatedOperatorsAtRound(context.Background(), roundInt, client)
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
	activatedOperators, err := transactions.GetActivatedOperatorsAtRound(context.Background(), roundInt, client)
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
		commitCount, _ := strconv.Atoi(data.CommitCount)
		revealCount, _ := strconv.Atoi(data.RevealCount)
		requestedTime := time.Unix(parseTimestamp(data.RequestedTimestamp), 0)

		// Fetch operator count for the specific round
		operatorCount, err := GetOperatorCountByRound(strconv.Itoa(round.RoundInt), client)
		if err != nil {
			logger.Log.Errorf("Failed to fetch operator count for round %d: %v", round.RoundInt, err)
			continue
		}

		// Use operatorCount in commitExpired and revealExpired functions
		if commitExpired(commitCount, operatorCount, currentTime, requestedTime) ||
			revealExpired(revealCount, operatorCount, currentTime, requestedTime) {
			continue
		}

		filteredRounds = append(filteredRounds, round)
	}
	return filteredRounds
}

func parseTimestamp(timestamp string) int64 {
	t, _ := strconv.ParseInt(timestamp, 10, 64)
	return t
}

func commitExpired(commitCount, operatorCount int, currentTime, requestedTime time.Time) bool {
	if commitCount <= (operatorCount - 1) {
		return currentTime.Sub(requestedTime) > 5*time.Minute+30*time.Second
	}
	return false
}

func revealExpired(revealCount, operatorCount int, currentTime, requestedTime time.Time) bool {
	if revealCount <= (operatorCount - 1) {
		return currentTime.Sub(requestedTime) > 15*time.Minute+30*time.Second
	}
	return false
}

func commitDurationOver(requestedTimestamp string) bool {
	// Parse the requested timestamp into a Unix time
	requestedTime := time.Unix(parseTimestamp(requestedTimestamp), 0)
	currentTime := time.Now()

	// Check if more than 5 minutes have passed
	return currentTime.Sub(requestedTime) > 5*time.Minute
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
