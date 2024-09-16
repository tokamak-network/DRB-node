package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-node/logger"
	"github.com/tokamak-network/DRB-node/utils"
)

func GetRandomWordRequested(pofClient *utils.Client) (*utils.RoundResults, error) {
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
        RandomWordsRequested []utils.RandomWordRequestedStruct `json:"roundInfos"`
    }

    if err := client.Run(ctx, req, &respData); err != nil {
        logger.Log.Errorf("Error fetching random words requested: %v", err)
        return nil, err
    }

    latestRounds := make(map[string]utils.RandomWordRequestedStruct)
    for _, item := range respData.RandomWordsRequested {
        if existing, ok := latestRounds[item.Round]; ok {
            existingTimestamp, _ := strconv.Atoi(existing.RequestedTimestamp)
            currentTimestamp, _ := strconv.Atoi(item.RequestedTimestamp)
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

    currentTime := time.Now()

    for _, round := range rounds {
        data := round.Data
        revealCount, _ := strconv.Atoi(data.RevealCount)
        commitCount, _ := strconv.Atoi(data.CommitCount)
        requestedTimestamp, _ := strconv.ParseInt(data.RequestedTimestamp, 10, 64)
        requestedTime := time.Unix(requestedTimestamp, 0)

        commitDurationPassed := currentTime.Sub(requestedTime) > 5*time.Minute
        revealDurationPassed := currentTime.Sub(requestedTime) > 10*time.Minute

        if commitCount < 2 && commitDurationPassed {
            // Round expired in commit phase
            continue
        } else if commitCount == 2 && revealCount < 2 && revealDurationPassed {
            // Round expired in reveal phase
            continue
        } else if commitCount == 2 && revealCount == 2 {
            // Round completed, no need to process further
            continue
        } else {
            // Valid round (either pending commits or reveals), add to filtered list
            filteredRounds = append(filteredRounds, round)
        }
    }

    log.Printf("filteredRounds", filteredRounds)

    sort.Slice(filteredRounds, func(i, j int) bool {
        return filteredRounds[i].RoundInt < filteredRounds[j].RoundInt
    })

    results := &utils.RoundResults{
        RevealRounds:           []string{},
        CommitRounds:           []string{},
    }

    // Add rounds that still need commits or reveals to the result
    for _, round := range filteredRounds {
        data := round.Data
        if data.CommitCount < "2" {
            // If round is still waiting for commits
            results.CommitRounds = append(results.CommitRounds, data.Round)
        } else if data.RevealCount < "2" {
            // If round is still waiting for reveals
            results.RevealRounds = append(results.RevealRounds, data.Round)
        }
    }

    // Logging the results
    logger.Log.Info("---------------------------------------------------------------------------")
    w := tabwriter.NewWriter(log.Writer(), 0, 0, 1, ' ', tabwriter.Debug)
    fmt.Fprintln(w, "Category\tRounds")
    fmt.Fprintln(w, "RevealRounds\t", results.RevealRounds)
    fmt.Fprintln(w, "CommitRounds\t", results.CommitRounds)
    w.Flush()
    logger.Log.Info("---------------------------------------------------------------------------")

    logger.Log.Info("Random words requested fetch completed successfully")

    return results, nil
}


