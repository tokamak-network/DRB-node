package commitreveal2

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tokamak-network/DRB-node/utils"
)

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

// DetermineOrder calculates the reveal order by comparing COS values with RV
func DetermineOrder(rv [32]byte, cosValues [][]byte) []int {
	type revealOrderEntry struct {
		index int
		value *big.Int
	}

	var entries []revealOrderEntry
	rvValue := new(big.Int).SetBytes(rv[:])

	for i, cos := range cosValues {
		cosValue := new(big.Int).SetBytes(cos)
		diff := new(big.Int).Abs(new(big.Int).Sub(rvValue, cosValue)) // Absolute difference
		entries = append(entries, revealOrderEntry{index: i, value: diff})
	}

	// Sort by the difference value
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].value.Cmp(entries[j].value) < 0
	})

	var order []int
	for _, entry := range entries {
		order = append(order, entry.index)
	}

	return order
}

// saveRevealOrder stores the RV and reveal order in a file
func saveRevealOrders(filePath string, data map[string]interface{}) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create reveal order file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(data)
	if err != nil {
		return fmt.Errorf("failed to write reveal order to file: %v", err)
	}

	return nil
}

func LoadRevealOrders(filePath string) (map[string]interface{}, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return an empty map if the file doesn't exist
			return make(map[string]interface{}), nil
		}
		return nil, fmt.Errorf("failed to open reveal order file: %v", err)
	}
	defer file.Close()

	var data map[string]interface{}
	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode reveal order file: %v", err)
	}

	return data, nil
}

func DetermineRevealOrder(roundNum string, activatedOperators map[string]map[common.Address]bool) error {
	// File path for reveal order storage
	filePath := "reveal_orders.json"

	// Load existing data
	data, err := LoadRevealOrders(filePath)
	if err != nil {
		log.Printf("Failed to load existing reveal orders: %v", err)
		return err
	}

	// Check if the round already exists
	if _, exists := data[roundNum]; exists {
		log.Printf("Reveal order already exists for round %s. Skipping calculation.", roundNum)
		return nil
	}

	log.Printf("Determining reveal order for round %s...", roundNum)

	// Ensure activated operators exist for the round
	operators, exists := activatedOperators[roundNum]
	if !exists || len(operators) == 0 {
		log.Printf("No activated operators found for round %s", roundNum)
		return fmt.Errorf("no activated operators found for round %s", roundNum)
	}

	var cosValues [][]byte
	var addresses []string
	for eoaAddress := range operators {
		eoaAddressStr := eoaAddress.Hex()

		commitData, err := utils.LoadLeaderCommitData(roundNum, eoaAddressStr)
		if err != nil {
			log.Printf("Failed to load COS for operator %s in round %s: %v", eoaAddressStr, roundNum, err)
			return fmt.Errorf("failed to load COS for operator %s", eoaAddressStr)
		}

		if commitData.Cos == [32]byte{} {
			log.Printf("Missing COS for operator %s in round %s", eoaAddressStr, roundNum)
			return fmt.Errorf("missing COS for operator %s", eoaAddressStr)
		}

		cosValues = append(cosValues, commitData.Cos[:])
		addresses = append(addresses, eoaAddressStr)
	}

	// Calculate the RV and determine the reveal order
	rv := calculateRV(cosValues)
	revealOrder := DetermineOrder(rv, cosValues)

	// Reorder addresses based on reveal order
	orderedAddresses := make([]string, len(addresses))
	for i, index := range revealOrder {
		orderedAddresses[i] = addresses[index]
	}

	// Add the new reveal order to the data map
	data[roundNum] = map[string]interface{}{
		"rv":            hex.EncodeToString(rv[:]),
		"reveal_order":  revealOrder,
		"ordered_nodes": orderedAddresses, // Save addresses in reveal order
	}

	// Save the updated data back to the file
	err = saveRevealOrders(filePath, data)
	if err != nil {
		log.Printf("Failed to save reveal order for round %s: %v", roundNum, err)
		return fmt.Errorf("failed to save reveal order for round %s", roundNum)
	}

	log.Printf("Reveal order determined and stored for round %s", roundNum)
	return nil
}

