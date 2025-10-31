package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadContractABI(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "abi_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("valid ABI file", func(t *testing.T) {
		// Create a valid ABI file
		validABI := map[string]interface{}{
			"abi": []interface{}{
				map[string]interface{}{
					"type": "function",
					"name": "testFunction",
					"inputs": []interface{}{
						map[string]interface{}{
							"name": "param1",
							"type": "uint256",
						},
					},
					"outputs": []interface{}{
						map[string]interface{}{
							"name": "result",
							"type": "uint256",
						},
					},
				},
			},
		}

		abiBytes, err := json.Marshal(validABI)
		require.NoError(t, err)

		abiFile := filepath.Join(tempDir, "valid.json")
		err = os.WriteFile(abiFile, abiBytes, 0644)
		require.NoError(t, err)

		// Test loading the ABI
		parsedABI, err := LoadContractABI(abiFile)
		assert.NoError(t, err)
		assert.NotNil(t, parsedABI)

		// Verify the ABI contains our test function
		method, exists := parsedABI.Methods["testFunction"]
		assert.True(t, exists)
		assert.Equal(t, "testFunction", method.RawName)
	})

	t.Run("non-existent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tempDir, "non_existent.json")
		
		_, err := LoadContractABI(nonExistentFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read ABI file")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		invalidJSONFile := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(invalidJSONFile, []byte("invalid json content"), 0644)
		require.NoError(t, err)

		_, err = LoadContractABI(invalidJSONFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal ABI JSON")
	})

	t.Run("missing abi field", func(t *testing.T) {
		missingABIField := map[string]interface{}{
			"someOtherField": "value",
		}

		abiBytes, err := json.Marshal(missingABIField)
		require.NoError(t, err)

		missingABIFile := filepath.Join(tempDir, "missing_abi.json")
		err = os.WriteFile(missingABIFile, abiBytes, 0644)
		require.NoError(t, err)

		_, err = LoadContractABI(missingABIFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse contract ABI")
	})

	t.Run("invalid ABI format", func(t *testing.T) {
		invalidABIFormat := map[string]interface{}{
			"abi": "invalid abi format",
		}

		abiBytes, err := json.Marshal(invalidABIFormat)
		require.NoError(t, err)

		invalidABIFile := filepath.Join(tempDir, "invalid_abi.json")
		err = os.WriteFile(invalidABIFile, abiBytes, 0644)
		require.NoError(t, err)

		_, err = LoadContractABI(invalidABIFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse contract ABI")
	})

	t.Run("empty ABI", func(t *testing.T) {
		emptyABI := map[string]interface{}{
			"abi": []interface{}{},
		}

		abiBytes, err := json.Marshal(emptyABI)
		require.NoError(t, err)

		emptyABIFile := filepath.Join(tempDir, "empty_abi.json")
		err = os.WriteFile(emptyABIFile, abiBytes, 0644)
		require.NoError(t, err)

		parsedABI, err := LoadContractABI(emptyABIFile)
		assert.NoError(t, err)
		assert.NotNil(t, parsedABI)
		assert.Empty(t, parsedABI.Methods)
	})
}