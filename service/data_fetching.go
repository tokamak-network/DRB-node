package service

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/machinebox/graphql"
	"github.com/sirupsen/logrus"
	crr "github.com/tokamak-network/DRB-node/dependencies/commit_reveal_recover"
	"github.com/tokamak-network/DRB-node/logger" // Use the custom logger package
	"github.com/tokamak-network/DRB-node/utils"
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
		logger.Log.Errorf("Failed to execute query: %v", err) // Replacing logrus with logger.Log
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

	var respData struct {
		CommitCs []utils.CommitData `json:"commitCs"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Failed to execute query: %v", err) // Replacing logrus with logger.Log
		return nil, err
	}

	return respData.CommitCs, nil
}

// GetFulfillRandomnessData fetches fulfill randomness data for a given round.
func GetFulfillRandomnessData(round string) ([]utils.FulfillRandomnessData, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetFulfillRandomnessDataRequest(round)

	var respData struct {
		FulfillRandomnesses []struct {
			MsgSender      string `json:"msgSender"`
			BlockTimestamp string `json:"blockTimestamp"`
			Success        bool   `json:"success"`
		} `json:"fulfillRandomnesses"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Failed to execute GetFulfillRandomnessData query: %v", err) // Replacing logrus with logger.Log
		return nil, err
	}

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
	logger.Log.Info("Starting BeforeRecoverPhase...") // Replacing logrus with logger.Log

	setupValues := utils.GetSetupValue()

	commitDataList, err := GetCommitData(round)
	if err != nil {
		logger.Log.Errorf("Error retrieving commit-reveal data: %v", err) // Replacing logrus with logger.Log
		return utils.RecoveryResult{}, err
	}

	logger.Log.Info("Processing commit data...") // Replacing logrus with logger.Log

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
				logger.Log.Warnf("Failed to convert commit val to big.Int: %s", commitData.CommitVal) // Replacing logrus with logger.Log
				continue
			}
			commits = append(commits, commitBigInt)
		}
	}

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

	logger.Log.Info("BeforeRecoverPhase completed successfully") // Replacing logrus with logger.Log
	return result, nil
}

// FindOffChainLeaderAtRound determines if the local node is the leader for the given round based on the recovered Omega value.
func FindOffChainLeaderAtRound(round string, OmegaRecov *big.Int) (bool, common.Address, error) {
	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		logger.Log.Error("WALLET_ADDRESS environment variable is not set")
		return false, common.Address{}, fmt.Errorf("WALLET_ADDRESS environment variable is not set")
	}

	mySender := common.HexToAddress(walletAddress)
	commitDataList, err := GetCommitData(round)
	if err != nil {
		logger.Log.Errorf("Error fetching commit data for round %s: %v", round, err) // Replacing logrus with logger.Log
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
		logger.Log.WithFields(logrus.Fields{ // Corrected fields usage
			"round": round,
		}).Info("My sender's address has the min hash - I am the leader") // Replacing logrus with logger.Log
	} else {
		logger.Log.WithFields(logrus.Fields{ // Corrected fields usage
			"round": round,
		}).Info("My sender's address does not have the min hash - I am not the leader") // Replacing logrus with logger.Log
		time.Sleep(10 * time.Second)
	}

	return isMyAddressLeader, leaderAddress, nil
}
