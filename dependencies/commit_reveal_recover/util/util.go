package util

import (
	"crypto/rand"
	"fmt"
	"math/big"

	prover "github.com/tokamak-network/DRB-Node/dependencies/pieterzak_vdf_prover"
)

func ConstructClaim(N, x, y *big.Int, T int) prover.Claim {
	var THalf int
	if T%2 == 0 {
		THalf = T / 2
	} else {
		THalf = (T + 1) / 2
	}

	v := prover.CalV(N, x, THalf)

	return prover.Claim{
		N: N,
		X: x,
		Y: y,
		T: T,
		V: v,
	}
}

func XGenerator(N *big.Int) *big.Int {
	x, err := rand.Int(rand.Reader, N)
	if err != nil {
		fmt.Println("Error generating random number:", err)
		return nil
	}

	// x^2 mod N
	x.Exp(x, big.NewInt(2), N)
	return x
}
