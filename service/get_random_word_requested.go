package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-Node/logger"
	"github.com/tokamak-network/DRB-Node/utils"
)

func GetRandomWordRequested(pofClient *utils.PoFClient) (*utils.RoundResults, error) {
    config := utils.GetConfig()
    client := graphql.NewClient(config.SubgraphURL)

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

    roundStatus := make(map[string]string)

    for _, round := range filteredRounds {
        item := round.Data

        reqOne := utils.GetCommitCsRequest(item.Round, config.WalletAddress)
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

        var isRecovered bool
        var msgSender string

        for _, data := range recoveredData {
            isRecovered = data.IsRecovered
            msgSender = data.MsgSender
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
            if commitSender == common.HexToAddress(config.WalletAddress) {
                isCommitSender = true
                break
            }
        }

        var isMyAddressLeader bool
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
        recoverPhaseEndTime := commitPhaseEndTime.Add(time.Duration(utils.RecoverDuration) * time.Second)
        reRequestTime := commitPhaseEndTime.Add(time.Duration(utils.ReRequestDuration) * time.Second)

        if item.Round == "0" {
            isPreviousRoundRecovered = true
        }

        if isPreviousRoundRecovered && !item.RoundInfo.IsRecovered && requestBlockTimestamp.After(myCommitBlockTimestamp) {
            if _, exists := roundStatus[item.Round+":Committed"]; !exists {
                results.CommittableRounds = append(results.CommittableRounds, item.Round)
                roundStatus[item.Round+":Committed"] = "Processed"
                if _, reRequestExists := roundStatus[item.Round+":ReRequested"]; reRequestExists {
                    delete(roundStatus, item.Round+":ReRequested")
                }
            }
        }

        recoverDataMap := make(map[string]utils.RecoveryResult)

        if validCommitCount >= 2 && !isRecovered {
            recoverData, err = BeforeRecoverPhase(item.Round, pofClient)
            if err != nil {
                logger.Log.Errorf("Error processing BeforeRecoverPhase for round %s: %v", item.Round, err)
                continue
            }

            if recoverData.OmegaRecov == nil {
                logger.Log.Errorf("OmegaRecov is nil for round %s", item.Round)
                continue
            }

            recoverDataMap[item.Round] = recoverData
            isMyAddressLeader, _, _ = FindOffChainLeaderAtRound(item.Round, recoverData.OmegaRecov)
            results.RecoveryData = append(results.RecoveryData, recoverData)
        } else if validCommitCount >= 2 && isRecovered {
            if strings.ToLower(config.WalletAddress) == msgSender {
                isMyAddressLeader = true
            } else {
                isMyAddressLeader = false
            }
        }

        // Recover
        if !isRecovered && isMyAddressLeader && isCommitSender && commitPhaseEndTime.Before(time.Now()) && !item.RoundInfo.IsRecovered && !item.RoundInfo.IsFulfillExecuted && validCommitCount > 1 {
            if _, exists := roundStatus[item.Round+":Recovered"]; !exists {
                results.RecoverableRounds = append(results.RecoverableRounds, item.Round)
                roundStatus[item.Round+":Recovered"] = "Processed"
            }
        }

        // Fulfill
        if isMyAddressLeader && isCommitSender && recoverPhaseEndTime.Before(time.Now()) && item.RoundInfo.IsRecovered && !item.RoundInfo.IsFulfillExecuted && validCommitCount > 1 {
            if _, exists := roundStatus[item.Round+":Fulfilled"]; !exists {
                results.FulfillableRounds = append(results.FulfillableRounds, item.Round)
                roundStatus[item.Round+":Fulfilled"] = "Processed"
            }
        }

        // Re-request
        if isPreviousRoundRecovered && reRequestTime.Before(time.Now()) && !item.RoundInfo.IsRecovered && validCommitCount < 2 && validCommitCount > 0 && commitTimeStampStr != "0" {
            if _, exists := roundStatus[item.Round+":ReRequested"]; !exists {
                results.ReRequestableRounds = append(results.ReRequestableRounds, item.Round)
                roundStatus[item.Round+":ReRequested"] = "Processed"
            }
        }

        // Dispute Recover
        if !isCommitSender && time.Now().Before(recoverPhaseEndTime) && item.RoundInfo.IsRecovered && !item.RoundInfo.IsFulfillExecuted {
            if _, exists := roundStatus[item.Round+":DisputeRecover"]; !exists {
                results.RecoverDisputeableRounds = append(results.RecoverDisputeableRounds, item.Round)
                roundStatus[item.Round+":DisputeRecover"] = "Processed"
            }
        }

        // Dispute Leadership
        if !isCommitSender && time.Now().Before(recoverPhaseEndTime) && item.RoundInfo.IsRecovered && item.RoundInfo.IsFulfillExecuted {
            if _, exists := roundStatus[item.Round+":DisputeLeadership"]; !exists {
                results.LeadershipDisputeableRounds = append(results.LeadershipDisputeableRounds, item.Round)
                roundStatus[item.Round+":DisputeLeadership"] = "Processed"
            }
        }

        // Complete Rounds
        if isRecovered && fulfillSender == config.WalletAddress {
            if _, exists := roundStatus[item.Round+":Completed"]; !exists {
                results.CompleteRounds = append(results.CompleteRounds, item.Round)
                roundStatus[item.Round+":Completed"] = "Processed"
                logger.Log.Infof("Round %s added to complete rounds", item.Round)
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
