package regular_node

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	appconfig "github.com/tokamak-network/DRB-node/config"
	"github.com/tokamak-network/DRB-node/utils"
)

// GenerateCvsSignature generates the EIP-712 signature components (v, r, s) for a given round, trialNum and CVS value.
func (n *RegularNode) GenerateCvsSignature(round *big.Int, trialNum *big.Int, cvs [32]byte) (uint8, string, string, error) {
	// Convert CVS to string for internal usage (optional, depending on use case)
	cvsString := hex.EncodeToString(cvs[:])
	log.Printf("Received CVS as [32]byte: %x", cvs)
	log.Printf("Converted CVS to String: %s", cvsString)

	// Load the private key
	envCfg := appconfig.Get()
	privateKeyHex := envCfg.EOAPrivateKey
	if privateKeyHex == "" {
		log.Fatal("EOA_PRIVATE_KEY is not set in the environment variables")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to decode private key: %v", err)
	}

	typedDataHash, err := utils.ComputeCvsEIP712TypedDataHash(round, trialNum, cvs)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to compute EIP-712 typed data hash: %v", err)
	}
	log.Printf("Typed Data Hash: %s", typedDataHash.Hex())

	// Sign the typed data hash
	signature, err := crypto.Sign(typedDataHash.Bytes(), privateKey)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to sign typed data: %v", err)
	}

	// Split the signature into r, s, and v
	r := hex.EncodeToString(signature[:32])
	s := hex.EncodeToString(signature[32:64])
	v := uint8(signature[64]) + 27 // Adjust for Ethereum recovery ID

	log.Printf("Generated EIP-712 signature: v=%d, r=%s, s=%s", v, r, s)
	return v, r, s, nil
}
