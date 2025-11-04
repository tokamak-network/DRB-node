package commitreveal2

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/utils"
)

type RevealOrderService struct {
	revealOrderRepository  database.IRevealOrderRepository
	peerCommitRepository   database.IPeerCommitRepository
	leaderCommitRepository database.ILeaderCommitRepository
}

func NewRevealOrderService(
	revealOrderRepository database.IRevealOrderRepository,
	peerCommitRepository database.IPeerCommitRepository,
	leaderCommitRepository database.ILeaderCommitRepository,
) *RevealOrderService {
	return &RevealOrderService{
		revealOrderRepository:  revealOrderRepository,
		peerCommitRepository:   peerCommitRepository,
		leaderCommitRepository: leaderCommitRepository,
	}
}

// DetermineRevealOrder determines the reveal order for leader nodes
func (s *RevealOrderService) DetermineRevealOrder(ctx context.Context, round, trialNum string, activatedOps []common.Address) (bool, error) {
	return DetermineRevealOrder(round, trialNum, activatedOps)
}

// DetermineRegularRevealOrder determines the reveal order for regular nodes
func (s *RevealOrderService) DetermineRegularRevealOrder(ctx context.Context, round, trialNum string, activatedOps []common.Address) (bool, error) {
	return DetermineRegularRevealOrder(round, trialNum, activatedOps)
}

type RevealOrder struct {
	OrderedNodes []string `json:"ordered_nodes"`
	RevealOrder  []int    `json:"reveal_order"`
	RV           string   `json:"rv"`
}

// calculateRV hashes all COS values into a single RV value
func calculateRV(cosValues [][]byte) [32]byte {
	var concatenated []byte
	for _, cos := range cosValues {
		concatenated = append(concatenated, cos...)
	}

	hashed := Keccak256(concatenated)
	var rv [32]byte
	copy(rv[:], hashed)
	return rv
}

// determineOrder calculates the reveal order by comparing COS values with RV
func determineOrder(rv [32]byte, cvsValues [][]byte) []int {
	type revealOrderEntry struct {
		index int
		value [32]byte // Use hash as value
	}

	var entries []revealOrderEntry
	for i, cvs := range cvsValues {
		concatenated := append(rv[:], cvs...) // rv || cv
		hash := Keccak256(concatenated)       // hash(rv || cv)
		var hash32 [32]byte
		copy(hash32[:], hash)
		entries = append(entries, revealOrderEntry{index: i, value: hash32})
	}

	// Sort by the hash value (descending, to match contract's RevealNotInDescendingOrder)
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].value[:], entries[j].value[:]) > 0
	})

	var order []int
	for _, entry := range entries {
		order = append(order, entry.index)
	}
	return order
}

func DetermineRevealOrder(roundNum string, trialNum string, activatedOps []common.Address) (bool, error) {
	ctx := context.Background()
	revealOrderRepo := database.NewRevealOrderRepository(database.GetDB())
	_, err := revealOrderRepo.GetRevealOrder(ctx, roundNum, trialNum)
	if err == nil {
		log.Printf("Reveal order already exists for round %s with trail %s. Skipping calculation.", roundNum, trialNum)
		return true, nil
	}

	log.Printf("Determining reveal order for round %s with trail %s...", roundNum, trialNum)

	if len(activatedOps) == 0 {
		log.Printf("No activated operators found for round %s with trail %s", roundNum, trialNum)
		return false, fmt.Errorf("no activated operators found for round %s with trail %s", roundNum, trialNum)
	}

	var cvsValues [][]byte
	var cosValues [][]byte
	var addresses []string
	for _, eoaAddress := range activatedOps {
		eoaAddressStr := eoaAddress.Hex()

		leaderCommitRepo := database.NewLeaderCommitRepository(database.GetDB())
		commitData, err := leaderCommitRepo.GetLeaderCommitByRoundAndEoaAddr(ctx, roundNum, trialNum, eoaAddressStr)
		if err != nil {
			log.Printf("Failed to load leader commit for operator %s in round %s with trail %s: %v", eoaAddressStr, roundNum, trialNum, err)
			return false, fmt.Errorf("failed to load leader commit for operator %s in round %s with trail %s", eoaAddressStr, roundNum, trialNum)
		}

		if commitData.Cos == [32]byte{} {
			log.Printf("Missing leader commit for operator %s in round %s with trail %s", eoaAddressStr, roundNum, trialNum)
			return false, fmt.Errorf("missing leader commit for operator %s in round %s with trail %s", eoaAddressStr, roundNum, trialNum)
		}

		cvsValues = append(cvsValues, commitData.Cvs[:])
		cosValues = append(cosValues, commitData.Cos[:])
		addresses = append(addresses, eoaAddressStr)
	}

	// Calculate the RV and determine the reveal order
	rv := calculateRV(cosValues)
	revealOrder := determineOrder(rv, cvsValues)

	// Reorder addresses based on reveal order
	orderedAddresses := make([]string, len(addresses))
	for i, index := range revealOrder {
		orderedAddresses[i] = addresses[index]
	}

	revealOrderData := utils.RevealOrderData{
		UniqueKey:    roundNum + "-" + trialNum,
		Round:        roundNum,
		TrialNum:     trialNum,
		RevealOrder:  revealOrder,
		OrderedNodes: orderedAddresses,
		RV:           hex.EncodeToString(rv[:]),
	}

	err = revealOrderRepo.AddRevealOrder(ctx, &revealOrderData)
	if err != nil {
		log.Printf("Failed to save reveal order for round %s with trail %s: %v", roundNum, trialNum, err)
		return false, fmt.Errorf("failed to save reveal order for round %s with trail %s", roundNum, trialNum)
	}

	log.Printf("Reveal order determined and stored for round %s with trail %s", roundNum, trialNum)
	return true, nil
}

func DetermineRegularRevealOrder(roundNum string, trialNum string, activatedOps []common.Address) (bool, error) {
	ctx := context.Background()
	revealOrderRepo := database.NewRevealOrderRepository(database.GetDB())
	_, err := revealOrderRepo.GetRevealOrder(ctx, roundNum, trialNum)
	if err == nil {
		log.Printf("Reveal order already exists for round %s with trial %s. Skipping calculation.", roundNum, trialNum)
		return true, nil
	}

	log.Printf("Determining reveal order for round %s with trial %s...", roundNum, trialNum)

	if len(activatedOps) == 0 {
		log.Printf("No activated operators found for round %s", roundNum)
		return false, fmt.Errorf("no activated operators found for round %s", roundNum)
	}

	var cvsValues [][]byte
	var cosValues [][]byte
	var addresses []string
	for _, eoaAddress := range activatedOps {
		eoaAddressStr := eoaAddress.Hex()

		peerCommitRepo := database.NewPeerCommitRepository(database.GetDB())
		commitData, err := peerCommitRepo.GetPeerCommitData(ctx, roundNum, trialNum, eoaAddressStr)
		if err != nil {
			log.Printf("Failed to load leader commit for operator %s in round %s: %v", eoaAddressStr, roundNum, err)
			return false, fmt.Errorf("failed to load leader commit for operator %s", eoaAddressStr)
		}

		if utils.ConvertByteArray(commitData.Cos) == [32]byte{} {
			log.Printf("Missing leader commit for operator %s in round %s", eoaAddressStr, roundNum)
			return false, fmt.Errorf("missing leader commit for operator %s", eoaAddressStr)
		}

		cvsValues = append(cvsValues, commitData.Cvs)
		cosValues = append(cosValues, commitData.Cos)
		addresses = append(addresses, eoaAddressStr)
	}

	// Calculate the RV and determine the reveal order
	rv := calculateRV(cosValues)
	revealOrder := determineOrder(rv, cvsValues)

	// Reorder addresses based on reveal order
	orderedAddresses := make([]string, len(addresses))
	for i, index := range revealOrder {
		orderedAddresses[i] = addresses[index]
	}

	revealOrderData := utils.RevealOrderData{
		UniqueKey:    roundNum + "-" + trialNum,
		Round:        roundNum,
		TrialNum:     trialNum,
		RevealOrder:  revealOrder,
		OrderedNodes: orderedAddresses,
		RV:           hex.EncodeToString(rv[:]),
	}

	err = revealOrderRepo.AddRevealOrder(ctx, &revealOrderData)
	if err != nil {
		log.Printf("Failed to save reveal order for round %s: %v", roundNum, err)
		return false, fmt.Errorf("failed to save reveal order for round %s", roundNum)
	}

	log.Printf("Reveal order determined and stored for round %s", roundNum)
	return true, nil
}
