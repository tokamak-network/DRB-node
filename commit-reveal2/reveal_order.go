package commitreveal2

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tokamak-network/DRB-node/database"
	"github.com/tokamak-network/DRB-node/eth"
	"github.com/tokamak-network/DRB-node/utils"
)

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
		value *big.Int
	}

	var entries []revealOrderEntry
	rvValue := new(big.Int).SetBytes(rv[:])
	for i, cvs := range cvsValues {
		cvsValue := new(big.Int).SetBytes(cvs)
		diff := new(big.Int).Abs(new(big.Int).Sub(rvValue, cvsValue)) // Absolute difference
		entries = append(entries, revealOrderEntry{index: i, value: diff})
	}

	// Sort by the difference value
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].value.Cmp(entries[j].value) > 0
	})

	var order []int
	for _, entry := range entries {
		order = append(order, entry.index)
	}

	return order
}

func DetermineRevealOrder(roundNum string, activatedOperators []common.Address) (bool, error) {
	_, err := database.GetRevealOrder(roundNum)
	if err == nil {
		log.Printf("Reveal order already exists for round %s. Skipping calculation.", roundNum)
		return true, nil
	}

	log.Printf("Determining reveal order for round %s...", roundNum)

	operators := eth.ActivatedOperators

	if len(operators) == 0 {
		log.Printf("No activated operators found for round %s", roundNum)
		return false, fmt.Errorf("no activated operators found for round %s", roundNum)
	}

	var cvsValues [][]byte
	var cosValues [][]byte
	var addresses []string
	for _, eoaAddress := range operators {
		eoaAddressStr := eoaAddress.Hex()

		commitData, err := database.GetLeaderCommitByRoundAndEoaAddr(roundNum, eoaAddressStr)
		if err != nil {
			log.Printf("Failed to load leader commit for operator %s in round %s: %v", eoaAddressStr, roundNum, err)
			return false, fmt.Errorf("failed to load leader commit for operator %s", eoaAddressStr)
		}

		if commitData.Cos == [32]byte{} {
			log.Printf("Missing leader commit for operator %s in round %s", eoaAddressStr, roundNum)
			return false, fmt.Errorf("missing leader commit for operator %s", eoaAddressStr)
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
		Round:        roundNum,
		RevealOrder:  revealOrder,
		OrderedNodes: orderedAddresses,
		RV:           hex.EncodeToString(rv[:]),
	}

	err = database.AddRevealOrder(&revealOrderData)
	if err != nil {
		log.Printf("Failed to save reveal order for round %s: %v", roundNum, err)
		return false, fmt.Errorf("failed to save reveal order for round %s", roundNum)
	}

	log.Printf("Reveal order determined and stored for round %s", roundNum)
	return true, nil
}

func DetermineRegularRevealOrder(roundNum string, activatedOperators []common.Address) (bool, error) {
	_, err := database.GetRevealOrder(roundNum)
	if err == nil {
		log.Printf("Reveal order already exists for round %s. Skipping calculation.", roundNum)
		return true, nil
	}

	log.Printf("Determining reveal order for round %s...", roundNum)

	operators := eth.ActivatedOperators

	if len(operators) == 0 {
		log.Printf("No activated operators found for round %s", roundNum)
		return false, fmt.Errorf("no activated operators found for round %s", roundNum)
	}

	var cvsValues [][]byte
	var cosValues [][]byte
	var addresses []string
	for _, eoaAddress := range operators {
		eoaAddressStr := eoaAddress.Hex()

		commitData, err := database.GetPeerCommitData(roundNum, eoaAddressStr)
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
		Round:        roundNum,
		RevealOrder:  revealOrder,
		OrderedNodes: orderedAddresses,
		RV:           hex.EncodeToString(rv[:]),
	}

	err = database.AddRevealOrder(&revealOrderData)
	if err != nil {
		log.Printf("Failed to save reveal order for round %s: %v", roundNum, err)
		return false, fmt.Errorf("failed to save reveal order for round %s", roundNum)
	}

	log.Printf("Reveal order determined and stored for round %s", roundNum)
	return true, nil
}
