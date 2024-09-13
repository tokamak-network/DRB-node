package service

import (
	"context"
	"fmt"
	"github.com/tokamak-network/DRB-node/logger" // Import your logger package
	"github.com/tokamak-network/DRB-node/utils"
	"os"
)

func ProcessRoundResults(ctx context.Context, client *utils.Client) error {
	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		logger.Log.Error("WALLET_ADDRESS environment variable is not set")
		return fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	isOperator, err := IsOperator(walletAddress)
	if err != nil {
		logger.Log.Errorf("Error fetching isOperator results: %v", err)
		return err
	}

	fmt.Printf("isOperator: %v\n", isOperator)

	if !isOperator {
		//_, _, err := transactions.OperatorDepositAndActivate(ctx, client) // Call as function
		//if err != nil {
		//	logger.Log.Errorf("Error during OperatorDeposit: %v", err)
		//	return err
		//}
	}

	//results, err := GetRandomWordRequested(pofClient)
	//if err != nil {
	//	logger.Log.Errorf("Error fetching round results: %v", err)
	//	return err
	//}

	//if len(results.RecoverableRounds) > 0 {
	//	logger.Log.Info("Processing Recoverable Rounds...")
	//	processedRounds := make(map[string]bool)
	//
	//	for _, roundStr := range results.RecoverableRounds {
	//		if processedRounds[roundStr] {
	//			continue
	//		}
	//
	//		for _, recoveryData := range results.RecoveryData {
	//			isMyAddressLeader, _, _ := FindOffChainLeaderAtRound(roundStr, recoveryData.OmegaRecov)
	//			if isMyAddressLeader {
	//				round := new(big.Int)
	//				round, ok := round.SetString(roundStr, 10)
	//				if !ok {
	//					logger.Log.Errorf("Failed to convert round string to big.Int: %s", roundStr)
	//					continue
	//				}
	//
	//				err := transactions.Recover(ctx, round, recoveryData.Y, pofClient)
	//				if err != nil {
	//					logger.Log.Errorf("Failed to recover round: %s, error: %v", roundStr, err)
	//				} else {
	//					logger.Log.Infof("Processing recoverable round: %s", roundStr)
	//					processedRounds[roundStr] = true
	//				}
	//				time.Sleep(3 * time.Second)
	//				break
	//			}
	//		}
	//
	//		if !processedRounds[roundStr] {
	//			logger.Log.Infof("Not recoverable round: %s", roundStr)
	//		}
	//	}
	//}
	//
	//if len(results.CommittableRounds) > 0 {
	//	fmt.Println("Processing Committable Rounds...")
	//	logger.Log.Info("Processing Committable Rounds...")
	//	processedRounds := make(map[string]bool)
	//
	//	for _, roundStr := range results.CommittableRounds {
	//		if processedRounds[roundStr] {
	//			continue
	//		}
	//
	//		round := new(big.Int)
	//		round, ok := round.SetString(roundStr, 10)
	//		if !ok {
	//			logger.Log.Errorf("Failed to convert round string to big.Int: %s", roundStr)
	//			continue
	//		}
	//
	//		address, byteData, err := transactions.Commit(ctx, round, pofClient)
	//		if err != nil {
	//			logger.Log.Errorf("Failed to commit round: %v", err)
	//		} else {
	//			logger.Log.Infof("Commit successful for round %s!\nAddress: %s\nData: %x", round.String(), address.Hex(), byteData)
	//		}
	//		processedRounds[roundStr] = true
	//	}
	//}
	//
	//if len(results.FulfillableRounds) > 0 {
	//	logger.Log.Info("Processing Fulfillable Rounds...")
	//	for _, roundStr := range results.FulfillableRounds {
	//		round := new(big.Int)
	//		round, ok := round.SetString(roundStr, 10)
	//		if !ok {
	//			logger.Log.Errorf("Failed to convert round string to big.Int: %s", roundStr)
	//			continue
	//		}
	//
	//		tx, err := transactions.FulfillRandomness(ctx, round, pofClient)
	//		if err != nil {
	//			logger.Log.Errorf("Failed to fulfill randomness for round: %v", err)
	//		} else {
	//			logger.Log.Infof("FulfillRandomness successful! Tx Hash: %s", tx.Hash().Hex())
	//		}
	//	}
	//}
	//
	//if len(results.ReRequestableRounds) > 0 {
	//	logger.Log.Info("Processing ReRequestable Rounds...")
	//	for _, roundStr := range results.ReRequestableRounds {
	//		round := new(big.Int)
	//		round, ok := round.SetString(roundStr, 10)
	//		if !ok {
	//			logger.Log.Errorf("Failed to convert round string to big.Int: %s", roundStr)
	//			continue
	//		}
	//
	//		err := transactions.ReRequestRandomWordAtRound(ctx, round, pofClient)
	//		if err != nil {
	//			logger.Log.Errorf("Failed to re-request random word at round: %v", err)
	//		} else {
	//			logger.Log.Infof("Re-request successful for round %s", round.String())
	//		}
	//	}
	//}
	//
	//if len(results.RecoverDisputeableRounds) > 0 {
	//	logger.Log.Info("Processing Recover Disputeable Rounds...")
	//	for _, roundStr := range results.RecoverDisputeableRounds {
	//		recoveredData, err := GetRecoveredData(roundStr)
	//		if err != nil {
	//			logger.Log.Errorf("Error retrieving recovered data for round %s: %v", roundStr, err)
	//			continue
	//		}
	//
	//		round := new(big.Int)
	//		round, ok := round.SetString(roundStr, 10)
	//		if !ok {
	//			logger.Log.Errorf("Failed to convert round string to big.Int: %s", roundStr)
	//			continue
	//		}
	//
	//		disputeInitiated := false
	//
	//		for _, data := range recoveredData {
	//			msgSender := common.HexToAddress(data.MsgSender)
	//			omega := new(big.Int)
	//			omega, ok := omega.SetString(data.Omega[2:], 16)
	//			if !ok {
	//				logger.Log.Errorf("Failed to parse omega for round %s: %s", roundStr, data.Omega)
	//				continue
	//			}
	//
	//			logger.Log.Infof("Recovered Data - MsgSender: %s, Omega: %s", msgSender.Hex(), omega.String())
	//
	//			for _, recoveryData := range results.RecoveryData {
	//				// Check if the dispute conditions are met
	//				if recoveryData.OmegaRecov.Cmp(omega) != 0 && !disputeInitiated {
	//					// Call DisputeRecover with correct parameters
	//					tx, err := transactions.DisputeRecover(ctx, round, recoveryData.V, recoveryData.X, recoveryData.Y, pofClient)
	//					if err != nil {
	//						logger.Log.Errorf("Failed to initiate dispute recovery: %v", err)
	//						return err // Or handle the error accordingly
	//					}
	//					logger.Log.Infof("Dispute recovery initiated successfully. Tx Hash: %s", tx.Hash().Hex())
	//
	//					// Set the flag to true to prevent multiple disputes
	//					disputeInitiated = true
	//				}
	//			}
	//
	//			if disputeInitiated {
	//				logger.Log.Infof("Processing disputeable round: %s", roundStr)
	//				break
	//			}
	//		}
	//
	//		if !disputeInitiated {
	//			logger.Log.Infof("No disputes initiated for round: %s", roundStr)
	//		}
	//	}
	//}
	//
	//if len(results.LeadershipDisputeableRounds) > 0 {
	//	logger.Log.Info("Processing Leadership Disputeable Rounds...")
	//	for i, roundStr := range results.LeadershipDisputeableRounds {
	//		recoveredData, err := GetRecoveredData(roundStr)
	//		if err != nil {
	//			logger.Log.Errorf("Error retrieving recovered data for round %s: %v", roundStr, err)
	//			continue
	//		}
	//
	//		round := new(big.Int)
	//		round, ok := round.SetString(roundStr, 10)
	//		if !ok {
	//			logger.Log.Errorf("Failed to convert round string to big.Int: %s", roundStr)
	//			continue
	//		}
	//
	//		var msgSender common.Address
	//
	//		for _, data := range recoveredData {
	//			msgSender = common.HexToAddress(data.MsgSender)
	//			logger.Log.Infof("Recovered Data - MsgSender: %s", msgSender.Hex())
	//		}
	//
	//		if i < len(results.RecoveryData) {
	//			isMyAddressLeader, leaderAddress, _ := FindOffChainLeaderAtRound(roundStr, results.RecoveryData[i].OmegaRecov)
	//
	//			if msgSender != leaderAddress {
	//				// If the current address is supposed to be the leader
	//				if isMyAddressLeader {
	//					// Call the DisputeLeadershipAtRound function
	//					err := transactions.DisputeLeadershipAtRound(ctx, round, pofClient)
	//					if err != nil {
	//						return fmt.Errorf("failed to initiate dispute leadership at round: %w", err)
	//					}
	//
	//					// Log the information
	//					logger.Log.Infof("MsgSender %s is not the leader for round %s", msgSender.Hex(), roundStr)
	//				}
	//			}
	//
	//			logger.Log.Infof("Processing disputeable round: %s", roundStr)
	//		} else {
	//			logger.Log.Errorf("No recovery data available for round: %s", roundStr)
	//		}
	//	}
	//}

	return nil
}
