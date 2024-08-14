package service

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tokamak-network/DRB-Node/service/transactions"
	"github.com/tokamak-network/DRB-Node/utils"
)

func ProcessRoundResults(ctx context.Context, pofClient *utils.PoFClient) error {
	config := utils.GetConfig()
	isOperator, err := IsOperator(config.WalletAddress)
	if err != nil {
		log.Printf("Error fetching isOperator results: %v", err)
		return err
	}

	if !isOperator {
		_,_,err := transactions.OperatorDeposit(ctx, pofClient) // Call as function
        if err != nil {
            log.Printf("Error during OperatorDeposit: %v", err)
            return err
        }
	}

	results, err := GetRandomWordRequested(pofClient)
    if err != nil {
        log.Printf("Error fetching round results: %v", err)
        return err
    }

	if len(results.RecoverableRounds) > 0 {
		fmt.Println("Processing Recoverable Rounds...")
		processedRounds := make(map[string]bool)

		for _, roundStr := range results.RecoverableRounds {
			if processedRounds[roundStr] {
				continue
			}

			for _, recoveryData := range results.RecoveryData {
				isMyAddressLeader, _, _ := FindOffChainLeaderAtRound(roundStr, recoveryData.OmegaRecov)
				if isMyAddressLeader {
					round := new(big.Int)
					round, ok := round.SetString(roundStr, 10)
					if !ok {
						log.Printf("Failed to convert round string to big.Int: %s", roundStr)
						continue
					}

					err := transactions.Recover(ctx, round, recoveryData.Y, pofClient)
					if err != nil {
						log.Printf("Failed to recover round: %s, error: %v", roundStr, err)
					} else {
						fmt.Printf("Processing recoverable round: %s\n", roundStr)
						processedRounds[roundStr] = true
					}
					time.Sleep(3 * time.Second)
					break
				}
			}

			if !processedRounds[roundStr] {
				fmt.Printf("Not recoverable round: %s\n", roundStr)
			}
		}
	}

	if len(results.CommittableRounds) > 0 {
		fmt.Println("Processing Committable Rounds...")
		processedRounds := make(map[string]bool)

		for _, roundStr := range results.CommittableRounds {
			if processedRounds[roundStr] {
				continue
			}

			round := new(big.Int)
			round, ok := round.SetString(roundStr, 10)
			if !ok {
				log.Printf("Failed to convert round string to big.Int: %s", roundStr)
				continue
			}

			address, byteData, err := transactions.Commit(ctx, round, pofClient)
			if err != nil {
				log.Printf("Failed to commit round: %v", err)
			} else {
				fmt.Printf("Commit successful for round %s!\nAddress: %s\nData: %x\n", round.String(), address.Hex(), byteData)
			}
			processedRounds[roundStr] = true
		}
	}

	if len(results.FulfillableRounds) > 0 {
		fmt.Println("Processing Fulfillable Rounds...")
		for _, roundStr := range results.FulfillableRounds {
			round := new(big.Int)
			round, ok := round.SetString(roundStr, 10)
			if !ok {
				log.Printf("Failed to convert round string to big.Int: %s", roundStr)
				continue
			}

			tx, err := transactions.FulfillRandomness(ctx, round, pofClient)
			if err != nil {
				log.Printf("Failed to fulfill randomness for round: %v", err)
			} else {
				log.Printf("FulfillRandomness successful! Tx Hash: %s\n", tx.Hash().Hex())
			}
		}
	}

	if len(results.ReRequestableRounds) > 0 {
		fmt.Println("Processing ReRequestable Rounds...")
		for _, roundStr := range results.ReRequestableRounds {
			round := new(big.Int)
			round, ok := round.SetString(roundStr, 10)
			if !ok {
				log.Printf("Failed to convert round string to big.Int: %s", roundStr)
				continue
			}

			err := transactions.ReRequestRandomWordAtRound(ctx, round, pofClient)
			if err != nil {
				log.Printf("Failed to re-request random word at round: %v", err)
			} else {
				log.Printf("Re-request successful for round %s", round.String())
			}
		}
	}

	if len(results.RecoverDisputeableRounds) > 0 {
		fmt.Println("Processing Recover Disputeable Rounds...")
		for _, roundStr := range results.RecoverDisputeableRounds {
			recoveredData, err := GetRecoveredData(roundStr)
			if err != nil {
				log.Printf("Error retrieving recovered data for round %s: %v", roundStr, err)
				continue
			}

			round := new(big.Int)
			round, ok := round.SetString(roundStr, 10)
			if !ok {
				log.Printf("Failed to convert round string to big.Int: %s", roundStr)
				continue
			}

			disputeInitiated := false

			for _, data := range recoveredData {
				msgSender := common.HexToAddress(data.MsgSender)
				omega := new(big.Int)
				omega, ok := omega.SetString(data.Omega[2:], 16)
				if !ok {
					log.Printf("Failed to parse omega for round %s: %s", roundStr, data.Omega)
					continue
				}

				fmt.Printf("Recovered Data - MsgSender: %s, Omega: %s\n", msgSender.Hex(), omega.String())

				for _, recoveryData := range results.RecoveryData {
					// Check if the dispute conditions are met
					if recoveryData.OmegaRecov.Cmp(omega) != 0 && !disputeInitiated {
						// Call DisputeRecover with correct parameters
						tx, err := transactions.DisputeRecover(ctx, round, recoveryData.V, recoveryData.X, recoveryData.Y, pofClient)
						if err != nil {
							log.Printf("Failed to initiate dispute recovery: %v", err)
							return err // Or handle the error accordingly
						}
						log.Printf("Dispute recovery initiated successfully. Tx Hash: %s", tx.Hash().Hex())
						
						// Set the flag to true to prevent multiple disputes
						disputeInitiated = true
					}
				}

				if disputeInitiated {
					fmt.Printf("Processing disputeable round: %s\n", roundStr)
					break
				}
			}

			if !disputeInitiated {
				fmt.Printf("No disputes initiated for round: %s\n", roundStr)
			}
		}
	}

	if len(results.LeadershipDisputeableRounds) > 0 {
		fmt.Println("Processing Leadership Disputeable Rounds...")
		for i, roundStr := range results.LeadershipDisputeableRounds {
			recoveredData, err := GetRecoveredData(roundStr)
			if err != nil {
				log.Printf("Error retrieving recovered data for round %s: %v", roundStr, err)
				continue
			}

			round := new(big.Int)
			round, ok := round.SetString(roundStr, 10)
			if !ok {
				log.Printf("Failed to convert round string to big.Int: %s", roundStr)
				continue
			}

			var msgSender common.Address

			for _, data := range recoveredData {
				msgSender = common.HexToAddress(data.MsgSender)
				fmt.Printf("Recovered Data - MsgSender: %s\n", msgSender.Hex())
			}

			if i < len(results.RecoveryData) {
				isMyAddressLeader, leaderAddress, _ := FindOffChainLeaderAtRound(roundStr, results.RecoveryData[i].OmegaRecov)

				if msgSender != leaderAddress {
					// If the current address is supposed to be the leader
					if isMyAddressLeader {
						// Call the DisputeLeadershipAtRound function
						err := transactions.DisputeLeadershipAtRound(ctx, round, pofClient)
						if err != nil {
							return fmt.Errorf("failed to initiate dispute leadership at round: %w", err)
						}
			
						// Log the information
						fmt.Printf("MsgSender %s is not the leader for round %s\n", msgSender.Hex(), roundStr)
					}
				}

				fmt.Printf("Processing disputeable round: %s\n", roundStr)
			} else {
				log.Printf("No recovery data available for round: %s", roundStr)
			}
		}
	}

	return nil
}
