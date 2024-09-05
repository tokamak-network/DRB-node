package service

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

func GetRandomWordRequested(pofClient *utils.PoFClient) (*utils.RoundResults, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		logger.Log.Error("WALLET_ADDRESS environment variable is not set")
		return nil, fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	req := utils.GetRandomWordsRequestedRequest()
	ctx := context.Background()

	var respData struct {
		RandomWordsRequested []utils.RandomWordRequestedStruct `json:"randomWordsRequesteds"`
	}

	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Error fetching random words requested: %v", err)
		return nil, err
	}

	latestRounds := make(map[string]utils.RandomWordRequestedStruct)
	for _, item := range respData.RandomWordsRequested {
		if existing, ok := latestRounds[item.Round]; ok {
			existingTimestamp, _ := strconv.Atoi(existing.BlockTimestamp)
			currentTimestamp, _ := strconv.Atoi(item.BlockTimestamp)
			if currentTimestamp > existingTimestamp {
				latestRounds[item.Round] = item
			}
		} else {
			latestRounds[item.Round] = item
		}
	}

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

	var filteredRounds []struct {
		RoundInt int
		Data     utils.RandomWordRequestedStruct
	}

	for _, round := range rounds {
		if !round.Data.RoundInfo.IsFulfillExecuted {
			filteredRounds = append(filteredRounds, round)
		}
	}

	sort.Slice(filteredRounds, func(i, j int) bool {
		return filteredRounds[i].RoundInt < filteredRounds[j].RoundInt
	})

	results := &utils.RoundResults{
		RecoverableRounds:           []string{},
		CommittableRounds:           []string{},
		FulfillableRounds:           []string{},
		ReRequestableRounds:         []string{},
		RecoverDisputeableRounds:    []string{},
		LeadershipDisputeableRounds: []string{},
		CompleteRounds:              []string{},
		RecoveryData:                []utils.RecoveryResult{},
	}

	var roundStatus sync.Map

	for _, round := range filteredRounds {
		item := round.Data

		reqOne := utils.GetCommitCsRequest(item.Round, walletAddress)
		var respOneData struct {
			CommitCs []struct {
				BlockTimestamp string `json:"blockTimestamp"`
				CommitVal      string `json:"commitVal"`
			} `json:"commitCs"`
		}

		if err := client.Run(ctx, reqOne, &respOneData); err != nil {
			logger.Log.Errorf("Error running commitCs query for round %s: %v", item.Round, err)
			continue
		}

		var myCommitBlockTimestamp time.Time
		for _, data := range respOneData.CommitCs {
			myCommitBlockTimestampInt, err := strconv.ParseInt(data.BlockTimestamp, 10, 64)
			if err != nil {
				logger.Log.Errorf("Error converting block timestamp to int64: %v", err)
				continue
			}
			myCommitBlockTimestamp = time.Unix(myCommitBlockTimestampInt, 0)
		}

		validCommitCount, err := strconv.Atoi(item.RoundInfo.ValidCommitCount)
		if err != nil {
			logger.Log.Errorf("Error converting ValidCommitCount to int: %v", err)
			continue
		}

		recoveredData, err := GetRecoveredData(item.Round)
		if err != nil {
			logger.Log.Errorf("Error retrieving recovered data for round %s: %v", item.Round, err)
		}

		var recoverPhaseEndTime time.Time
		var isRecovered bool
		var msgSender string
		var omega string

		for _, data := range recoveredData {
			myRecoverBlockTimestampInt, err := strconv.ParseInt(data.BlockTimestamp, 10, 64)
			if err != nil {
				log.Printf("Failed to parse block timestamp for round %s: %v", item.Round, err)
				continue
			}

			isRecovered = data.IsRecovered
			omega = data.Omega
			msgSender = data.MsgSender
			blockTime := time.Unix(myRecoverBlockTimestampInt, 0)
			recoverPhaseEndTime = blockTime.Add(time.Duration(utils.RecoverDuration) * time.Second)
		}

		fulfillData, err := GetFulfillRandomnessData(item.Round)
		if err != nil {
			logger.Log.Errorf("Error retrieving fulfill randomness data for round %s: %v", item.Round, err)
		}

		var fulfillSender string
		for _, data := range fulfillData {
			if data.Success {
				fulfillSender = data.MsgSender
				break
			}
		}

		requestBlockTimestampStr := item.BlockTimestamp
		requestBlockTimestampInt, err := strconv.ParseInt(requestBlockTimestampStr, 10, 64)
		if err != nil {
			logger.Log.Errorf("Error converting block timestamp to int64: %v", err)
			return nil, err
		}
		requestBlockTimestamp := time.Unix(requestBlockTimestampInt, 0)

		getCommitData, err := GetCommitData(item.Round)
		if err != nil {
			logger.Log.Errorf("Error retrieving commit data for round %s: %v", item.Round, err)
		}

		var commitSenders []common.Address
		var isCommitSender bool
		var commitTimeStampStr string

		for _, data := range getCommitData {
			commitSender := common.HexToAddress(data.MsgSender)
			commitSenders = append(commitSenders, commitSender)
			commitTimeStampStr = data.BlockTimestamp
		}

		for _, commitSender := range commitSenders {
			if commitSender == common.HexToAddress(walletAddress) {
				isCommitSender = true
				break
			}
		}

		var isMyAddressLeader bool
		var leaderAddress common.Address
		var recoverData utils.RecoveryResult

		var isPreviousRoundRecovered bool
		previousRoundInt, err := strconv.Atoi(item.Round)
		if err != nil {
			logger.Log.Errorf("Error converting round to int: %v", err)
			continue
		}
		previousRound := strconv.Itoa(previousRoundInt - 1)

		previousRoundData, err := GetRecoveredData(previousRound)
		if err != nil {
			logger.Log.Errorf("Error retrieving recovered data for previous round %s: %v", previousRound, err)
		} else {
			isPreviousRoundRecovered = false
			for _, data := range previousRoundData {
				if data.IsRecovered {
					isPreviousRoundRecovered = true
					break
				}
			}
		}

		if commitTimeStampStr == "" {
			commitTimeStampStr = "0"
		}

		commitTimeStampInt, err := strconv.ParseInt(commitTimeStampStr, 10, 64)
		if err != nil {
			logger.Log.Errorf("Error converting commit timestamp to int64: %v", err)
			return nil, err
		}
		commitTimeStampTime := time.Unix(commitTimeStampInt, 0)
		commitPhaseEndTime := commitTimeStampTime.Add(time.Duration(utils.CommitDuration) * time.Second)
		reRequestTime := commitPhaseEndTime.Add(time.Duration(utils.ReRequestDuration) * time.Second)

		if item.Round == "0" {
			isPreviousRoundRecovered = true
		}
		roundStr := item.Round

		if isPreviousRoundRecovered && !item.RoundInfo.IsRecovered && requestBlockTimestamp.After(myCommitBlockTimestamp) {
			if _, exists := roundStatus.Load(roundStr + ":Committed"); !exists {
				results.CommittableRounds = append(results.CommittableRounds, roundStr)
				roundStatus.Store(roundStr+":Committed", "Processed")
				if _, reRequestExists := roundStatus.Load(roundStr + ":ReRequested"); reRequestExists {
					roundStatus.Delete(roundStr + ":ReRequested")
				}
			}
		}

		recoverDataMap := make(map[string]utils.RecoveryResult)

		if validCommitCount >= 2 && !isRecovered {
			recoverData, err = BeforeRecoverPhase(roundStr, pofClient)
			if err != nil {
				logger.Log.Errorf("Error processing BeforeRecoverPhase for round %s: %v", roundStr, err)
				continue
			}

			if recoverData.OmegaRecov == nil {
				logger.Log.Errorf("OmegaRecov is nil for round %s", roundStr)
				continue
			}

			recoverDataMap[roundStr] = recoverData
			isMyAddressLeader, leaderAddress, _ = FindOffChainLeaderAtRound(roundStr, recoverData.OmegaRecov)
			results.RecoveryData = append(results.RecoveryData, recoverData)
		} else if validCommitCount >= 2 && isRecovered {
			if strings.ToLower(walletAddress) == msgSender {
				isMyAddressLeader = true
			} else {
				isMyAddressLeader = false
			}
		}

		// Recover
		if !isRecovered && isMyAddressLeader && isCommitSender && commitPhaseEndTime.Before(time.Now()) && !item.RoundInfo.IsRecovered && !item.RoundInfo.IsFulfillExecuted && validCommitCount > 1 {
			if _, exists := roundStatus.Load(roundStr + ":Recovered"); !exists {
				results.RecoverableRounds = append(results.RecoverableRounds, roundStr)
				roundStatus.Store(roundStr+":Recovered", "Processed")
			}
		}

		// Fulfill
		if isMyAddressLeader && isCommitSender && recoverPhaseEndTime.Before(time.Now()) && item.RoundInfo.IsRecovered && !item.RoundInfo.IsFulfillExecuted && validCommitCount > 1 {
			if _, exists := roundStatus.Load(roundStr + ":Fulfilled"); !exists {
				results.FulfillableRounds = append(results.FulfillableRounds, roundStr)
				roundStatus.Store(roundStr+":Fulfilled", "Processed")
			}
		}

		// Re-request
		if isPreviousRoundRecovered && reRequestTime.Before(time.Now()) && !item.RoundInfo.IsRecovered && validCommitCount < 2 && validCommitCount > 0 && commitTimeStampStr != "0" {
			isRoundAlreadyCommittable := false
			for _, committableRound := range results.CommittableRounds {
				if committableRound == roundStr {
					isRoundAlreadyCommittable = true
					break
				}
			}

			if !isRoundAlreadyCommittable {
				_, commitExists := roundStatus.Load(roundStr + ":Committed")
				if _, exists := roundStatus.Load(roundStr + ":ReRequested"); !exists {
					results.ReRequestableRounds = append(results.ReRequestableRounds, roundStr)
					roundStatus.Store(roundStr+":ReRequested", "Processed")

					if commitExists {
						roundStatus.Delete(roundStr + ":Committed")
					}
				}
			}
		}

		// Dispute Recover
		if !isCommitSender && time.Now().Before(recoverPhaseEndTime) && item.RoundInfo.IsRecovered && !item.RoundInfo.IsFulfillExecuted {
			roundBigInt := new(big.Int)
			roundBigInt.SetString(item.Round, 10)

			if data, exists := recoverDataMap[item.Round]; exists {
				omega = strings.TrimPrefix(omega, "0x")
				omegaBigInt := new(big.Int)
				if _, ok := omegaBigInt.SetString(omega, 16); !ok {
					log.Printf("Failed to parse omega: %s", omega)
				}

				if omegaBigInt.Cmp(data.OmegaRecov) != 0 {
					if _, exists := roundStatus.Load(item.Round + ":DisputeRecovered"); !exists {
						if !containsRound(results.RecoverDisputeableRounds, item.Round) {
							results.RecoverDisputeableRounds = append(results.RecoverDisputeableRounds, item.Round)
							roundStatus.Store(item.Round+":DisputeRecovered", "Processed")

							committedKey := item.Round + ":Committed"
							if _, exists := roundStatus.Load(committedKey); exists {
								roundStatus.Delete(committedKey)
							}
						}
					}
				}
			} else {
				log.Printf("No recovery data found for round %s", item.Round)
			}
		}

		// Dispute Leadership
		if !isCommitSender && time.Now().Before(recoverPhaseEndTime) && item.RoundInfo.IsRecovered && item.RoundInfo.IsFulfillExecuted {
			fulfillSenderAddress := common.HexToAddress(fulfillSender)

			if fulfillSenderAddress != leaderAddress {
				if _, exists := roundStatus.Load(roundStr + ":DisputeLeadershiped"); !exists {
					if !containsRound(results.LeadershipDisputeableRounds, roundStr) {
						results.LeadershipDisputeableRounds = append(results.LeadershipDisputeableRounds, roundStr)
						roundStatus.Store(roundStr+":DisputeLeadershiped", "Processed")
					}
				}
			}
		}

		// Complete Rounds
		if isRecovered && fulfillSender == walletAddress {
			if _, exists := roundStatus.Load(roundStr + ":Completed"); !exists {
				results.CompleteRounds = append(results.CompleteRounds, roundStr)
				roundStatus.Store(roundStr+":Completed", "Processed")
				logger.Log.Infof("Round %s added to complete rounds", roundStr)
			}
		}
	}

	// Logging the results
	logger.Log.Info("---------------------------------------------------------------------------")
	w := tabwriter.NewWriter(log.Writer(), 0, 0, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "Category\tRounds")
	fmt.Fprintln(w, "RecoverableRounds\t", results.RecoverableRounds)
	fmt.Fprintln(w, "CommittableRounds\t", results.CommittableRounds)
	fmt.Fprintln(w, "FulfillableRounds\t", results.FulfillableRounds)
	fmt.Fprintln(w, "ReRequestableRounds\t", results.ReRequestableRounds)
	fmt.Fprintln(w, "RecoverDisputeableRounds\t", results.RecoverDisputeableRounds)
	fmt.Fprintln(w, "LeadershipDisputeableRounds\t", results.LeadershipDisputeableRounds)
	w.Flush()
	logger.Log.Info("---------------------------------------------------------------------------")

	logger.Log.Info("Random words requested fetch completed successfully")

	return results, nil
}

func containsRound(rounds []string, round string) bool {
	for _, r := range rounds {
		if r == round {
			return true
		}
	}
	return false
}
