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

// IRevealOrderService defines the interface for reveal order operations
type IRevealOrderService interface {
	DetermineRevealOrder(ctx context.Context, roundNum string, trialNum string, activatedOps []common.Address) (bool, error)
	DetermineRegularRevealOrder(ctx context.Context, roundNum string, trialNum string, activatedOps []common.Address) (bool, error)
}

type RevealOrderService struct {
	revealOrderRepository  database.IRevealOrderRepository
	peerCommitRepository   database.IPeerCommitRepository
	leaderCommitRepository database.ILeaderCommitRepository
}

type commitDataExtractor func(ctx context.Context, roundNum, trialNum, eoaAddressStr string) (cvs []byte, cos []byte, err error)

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

type RevealOrder struct {
	OrderedNodes []string `json:"ordered_nodes"`
	RevealOrder  []int    `json:"reveal_order"`
	RV           string   `json:"rv"`
}

// calculateRV hashes all COS values into a single RV value
func (s *RevealOrderService) calculateRV(cosValues [][]byte) [32]byte {
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
func (s *RevealOrderService) determineOrder(rv [32]byte, cvsValues [][]byte) []int {
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

// extractLeaderCommitData extracts Cvs and Cos from leader commit data
func (s *RevealOrderService) extractLeaderCommitData(ctx context.Context, roundNum, trialNum, eoaAddressStr string) ([]byte, []byte, error) {
	commitData, err := s.leaderCommitRepository.GetLeaderCommitByRoundAndEoaAddr(ctx, roundNum, trialNum, eoaAddressStr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load leader commit for operator %s in round %s with trial %s: %w", eoaAddressStr, roundNum, trialNum, err)
	}

	if commitData.Cos == [32]byte{} {
		return nil, nil, fmt.Errorf("missing leader commit for operator %s in round %s with trial %s", eoaAddressStr, roundNum, trialNum)
	}

	return commitData.Cvs[:], commitData.Cos[:], nil
}

// extractPeerCommitData extracts Cvs and Cos from peer commit data
func (s *RevealOrderService) extractPeerCommitData(ctx context.Context, roundNum, trialNum, eoaAddressStr string) ([]byte, []byte, error) {
	commitData, err := s.peerCommitRepository.GetPeerCommitData(ctx, roundNum, trialNum, eoaAddressStr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load peer commit for operator %s in round %s with trial %s: %w", eoaAddressStr, roundNum, trialNum, err)
	}

	if utils.ConvertByteArray(commitData.Cos) == [32]byte{} {
		return nil, nil, fmt.Errorf("missing peer commit for operator %s in round %s with trial %s", eoaAddressStr, roundNum, trialNum)
	}

	return commitData.Cvs, commitData.Cos, nil
}

// determineRevealOrderInternal contains the common logic for determining reveal order
func (s *RevealOrderService) determineRevealOrderInternal(
	ctx context.Context,
	roundNum string,
	trialNum string,
	activatedOps []common.Address,
	extractCommitData commitDataExtractor,
	nodeType string,
) (bool, error) {
	_, err := s.revealOrderRepository.GetRevealOrder(ctx, roundNum, trialNum)
	if err == nil {
		log.Printf("Reveal order already exists for round %s with trial %s. Skipping calculation.", roundNum, trialNum)
		return true, nil
	}

	log.Printf("Determining reveal order for round %s with trial %s...", roundNum, trialNum)

	if len(activatedOps) == 0 {
		log.Printf("No activated operators found for round %s with trial %s", roundNum, trialNum)
		return false, fmt.Errorf("no activated operators found for round %s with trial %s", roundNum, trialNum)
	}

	var cvsValues [][]byte
	var cosValues [][]byte
	var addresses []string

	for _, eoaAddress := range activatedOps {
		eoaAddressStr := eoaAddress.Hex()

		cvs, cos, err := extractCommitData(ctx, roundNum, trialNum, eoaAddressStr)
		if err != nil {
			log.Printf("Failed to load %s commit for operator %s in round %s with trial %s: %v", nodeType, eoaAddressStr, roundNum, trialNum, err)
			return false, err
		}

		cvsValues = append(cvsValues, cvs)
		cosValues = append(cosValues, cos)
		addresses = append(addresses, eoaAddressStr)
	}

	// Calculate the RV and determine the reveal order
	rv := s.calculateRV(cosValues)
	revealOrder := s.determineOrder(rv, cvsValues)

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

	err = s.revealOrderRepository.AddRevealOrder(ctx, &revealOrderData)
	if err != nil {
		log.Printf("Failed to save reveal order for round %s with trial %s: %v", roundNum, trialNum, err)
		return false, fmt.Errorf("failed to save reveal order for round %s with trial %s: %w", roundNum, trialNum, err)
	}

	log.Printf("Reveal order determined and stored for round %s with trial %s", roundNum, trialNum)
	return true, nil
}

func (s *RevealOrderService) DetermineRevealOrder(ctx context.Context, roundNum string, trialNum string, activatedOps []common.Address) (bool, error) {
	return s.determineRevealOrderInternal(
		ctx,
		roundNum,
		trialNum,
		activatedOps,
		s.extractLeaderCommitData,
		"leader",
	)
}

func (s *RevealOrderService) DetermineRegularRevealOrder(ctx context.Context, roundNum string, trialNum string, activatedOps []common.Address) (bool, error) {
	return s.determineRevealOrderInternal(
		ctx,
		roundNum,
		trialNum,
		activatedOps,
		s.extractPeerCommitData,
		"peer",
	)
}
