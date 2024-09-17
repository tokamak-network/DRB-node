package service

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"sort"

	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/service/transactions"
	"github.com/tokamak-network/DRB-node/utils"
)

// ProcessRoundResults handles committing and revealing rounds based on fetched results
func ProcessRoundResults(ctx context.Context, pofClient *utils.Client) error {
	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		logger.Log.Error("WALLET_ADDRESS environment variable is not set")
		return fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	isOperator, err := IsOperator(walletAddress)
	if err != nil {
		logger.Log.Errorf("Error fetching operator status: %v", err)
		return err
	}

	// If wallet address is not an operator, deposit and activate
	if !isOperator {
		_, _, err := transactions.OperatorDepositAndActivate(ctx, pofClient)
		if err != nil {
			logger.Log.Errorf("Error during OperatorDeposit: %v", err)
			return err
		}
	}

	// Fetch rounds for commit/reveal
	results, err := GetRandomWordRequested(pofClient)
	if err != nil {
		logger.Log.Errorf("Error fetching round results: %v", err)
		return err
	}

	// Convert string rounds to *big.Int for CommitRounds and RevealRounds
	commitRounds := make([]*big.Int, len(results.CommitRounds))
	revealRounds := make([]*big.Int, len(results.RevealRounds))

	for i, roundStr := range results.CommitRounds {
		round := new(big.Int)
		round.SetString(roundStr, 10) // Base 10 for string to big.Int conversion
		commitRounds[i] = round
	}

	for i, roundStr := range results.RevealRounds {
		round := new(big.Int)
		round.SetString(roundStr, 10)
		revealRounds[i] = round
	}

	// Sort CommitRounds and RevealRounds in ascending order
	sort.Slice(commitRounds, func(i, j int) bool {
		return commitRounds[i].Cmp(commitRounds[j]) < 0
	})
	sort.Slice(revealRounds, func(i, j int) bool {
		return revealRounds[i].Cmp(revealRounds[j]) < 0
	})

	// Track already committed and revealed rounds to avoid redundant actions
	committedRounds := make(map[string]bool)
	revealedRounds := make(map[string]bool)

	// Process reveal rounds first
	for _, revealRound := range revealRounds {
		roundStr := revealRound.String()

		// Check if the reveal for this round has already been done
		if revealedRounds[roundStr] {
			continue
		}

		// Call the Reveal function
		_, _, err := transactions.Reveal(ctx, revealRound, pofClient)
		if err != nil {
			logger.Log.Errorf("Error executing reveal for round %s: %v", roundStr, err)
			return err
		}

		logger.Log.Infof("Reveal successful for round %s", roundStr)
		revealedRounds[roundStr] = true
	}

	// Now process commit rounds
	for _, commitRound := range commitRounds {
		roundStr := commitRound.String()

		// Check if the commit for this round has already been done
		if committedRounds[roundStr] {
			continue
		}

		// Check if there's a lower-numbered reveal round that hasn't been processed yet
		for _, revealRound := range revealRounds {
			if revealRound.Cmp(commitRound) < 0 && !revealedRounds[revealRound.String()] {
				logger.Log.Infof("Skipping commit for round %s until reveal for round %s is processed", roundStr, revealRound.String())
				break
			}
		}

		// Call the Commit function
		_, _, err := transactions.Commit(ctx, commitRound, pofClient)
		if err != nil {
			logger.Log.Errorf("Error executing commit for round %s: %v", roundStr, err)
			return err
		}

		logger.Log.Infof("Commit successful for round %s", roundStr)
		committedRounds[roundStr] = true
	}

	return nil
}
