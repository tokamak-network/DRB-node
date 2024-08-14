package service

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/machinebox/graphql"
	"github.com/sirupsen/logrus"
	crr "github.com/tokamak-network/DRB-Node/dependencies/commit_reveal_recover"
	"github.com/tokamak-network/DRB-Node/utils"
)

// GetRecoveredData fetches recovered data from a GraphQL endpoint
func GetRecoveredData(round string) ([]utils.RecoveredData, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetRecoveredDataRequest(round)

	var respData struct {
		Recovereds []struct {
			Round          string `json:"round"`
			BlockTimestamp string `json:"blockTimestamp"`
			ID             string `json:"id"`
			MsgSender      string `json:"msgSender"`
			Omega          string `json:"omega"`
			RoundInfo      struct {
				IsRecovered bool `json:"isRecovered"`
			} `json:"roundInfo"`
		} `json:"recovereds"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logrus.Errorf("Failed to execute query: %v", err)
		return nil, err
	}

	var recoveredData []utils.RecoveredData
	for _, item := range respData.Recovereds {
		recoveredData = append(recoveredData, utils.RecoveredData{
			Round:          item.Round,
			BlockTimestamp: item.BlockTimestamp,
			ID:             item.ID,
			MsgSender:      item.MsgSender,
			Omega:          item.Omega,
			IsRecovered:    item.RoundInfo.IsRecovered,
		})
	}

	return recoveredData, nil
}

// GetCommitData retrieves commit data for a given round and returns a slice of CommitData and an error
func GetCommitData(round string) ([]utils.CommitData, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetCommitDataRequest(round)

	// Define a structure to hold the query response
	var respData struct {
		CommitCs []utils.CommitData `json:"commitCs"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logrus.Errorf("Failed to execute query: %v", err)
		return nil, err
	}

	// Return the list of commit data and no error
	return respData.CommitCs, nil
}

// GetFulfillRandomnessData fetches fulfill randomness data for a given round.
func GetFulfillRandomnessData(round string) ([]utils.FulfillRandomnessData, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetFulfillRandomnessDataRequest(round)

	// Define a structure to hold the query response
	var respData struct {
		FulfillRandomnesses []struct {
			MsgSender      string `json:"msgSender"`
			BlockTimestamp string `json:"blockTimestamp"`
			Success        bool   `json:"success"`
		} `json:"fulfillRandomnesses"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logrus.Errorf("Failed to execute GetFulfillRandomnessData query: %v", err)
		return nil, err
	}

	// Map the response data to the FulfillRandomnessData struct
	var fulfillRandomnessData []utils.FulfillRandomnessData
	for _, item := range respData.FulfillRandomnesses {
		fulfillRandomnessData = append(fulfillRandomnessData, utils.FulfillRandomnessData{
			MsgSender:      item.MsgSender,
			BlockTimestamp: item.BlockTimestamp,
			Success:        item.Success,
		})
	}

	return fulfillRandomnessData, nil
}

// BeforeRecoverPhase checks if the local node is the leader by recovering the minimum hash and compares it against its own.
func BeforeRecoverPhase(round string, pofClient *utils.PoFClient) (utils.RecoveryResult, error) {
	logrus.Info("Starting BeforeRecoverPhase...")

	// Fetch setup values
	setupValues := utils.GetSetupValue()

	// Fetch commit data using the round number
	commitDataList, err := GetCommitData(round)
	if err != nil {
		logrus.Errorf("Error retrieving commit-reveal data: %v", err)
		return utils.RecoveryResult{}, err
	}

	logrus.Info("Processing commit data...")

	// Process commit data to extract commit values
	var commits []*big.Int
	for _, commitData := range commitDataList {
		if commitData.CommitVal != "" {
			var commitBigInt *big.Int
			var ok bool
			if strings.HasPrefix(commitData.CommitVal, "0x") {
				commitBigInt, ok = new(big.Int).SetString(commitData.CommitVal[2:], 16)
			} else {
				commitBigInt, ok = new(big.Int).SetString(commitData.CommitVal, 10)
			}

			if !ok {
				logrus.Warnf("Failed to convert commit val to big.Int: %s", commitData.CommitVal)
				continue
			}
			commits = append(commits, commitBigInt)
		}
	}

	// Assuming T and NVal are used directly from setupValues for recovery
	omegaRecov, proofListRecovery := crr.Recover(new(big.Int).SetBytes(setupValues.NVal), int(setupValues.T.Int64()), commits)
	if len(proofListRecovery) == 0 {
		return utils.RecoveryResult{}, fmt.Errorf("proofListRecovery is empty")
	}

	x := utils.BigNumber{
		Val:    proofListRecovery[0].X.Bytes(),
		Bitlen: big.NewInt(int64(proofListRecovery[0].X.BitLen())),
	}

	y := utils.BigNumber{
		Val:    proofListRecovery[0].Y.Bytes(),
		Bitlen: big.NewInt(int64(proofListRecovery[0].Y.BitLen())),
	}

	v := make([]utils.BigNumber, len(proofListRecovery))
	for i, proof := range proofListRecovery {
		v[i] = utils.BigNumber{
			Val:    proof.V.Bytes(),
			Bitlen: big.NewInt(int64(proof.V.BitLen())),
		}
	}

	result := utils.RecoveryResult{
		OmegaRecov: omegaRecov,
		X:          x,
		Y:          y,
		V:          v,
	}

	logrus.Info("BeforeRecoverPhase completed successfully")
	return result, nil
}

// FindOffChainLeaderAtRound determines if the local node is the leader for the given round based on the recovered Omega value.
func FindOffChainLeaderAtRound(round string, OmegaRecov *big.Int) (bool, common.Address, error) {
	config := utils.GetConfig()
	mySender := common.HexToAddress(config.WalletAddress)
	commitDataList, err := GetCommitData(round)
	if err != nil {
		logrus.Errorf("Error fetching commit data for round %s: %v", round, err)
		return false, common.Address{}, err
	}

	var minHash *big.Int
	var leaderAddress common.Address
	var myHash *big.Int

	for _, commit := range commitDataList {
		commitAddress := common.HexToAddress(commit.MsgSender)
		dataToHash := append(commitAddress.Bytes(), OmegaRecov.Bytes()...)
		currentHash := crypto.Keccak256Hash(dataToHash)
		currentHashInt := new(big.Int).SetBytes(currentHash.Bytes())

		if minHash == nil || currentHashInt.Cmp(minHash) < 0 {
			minHash = currentHashInt
			leaderAddress = commitAddress
		}

		if commitAddress == mySender {
			myHash = currentHashInt
		}
	}

	isMyAddressLeader := myHash != nil && myHash.Cmp(minHash) == 0 && mySender == leaderAddress
	if isMyAddressLeader {
		logrus.WithFields(logrus.Fields{
			"round": round,
		}).Info("My sender's address has the min hash - I am the leader")
	} else {
		logrus.WithFields(logrus.Fields{
			"round": round,
		}).Info("My sender's address does not have the min hash - I am not the leader")
		time.Sleep(10 * time.Second)
	}

	return isMyAddressLeader, leaderAddress, nil
}