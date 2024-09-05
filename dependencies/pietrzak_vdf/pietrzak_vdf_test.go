package vdf_test

import (
	"math/big"
	"testing"

	vdf "github.com/tokamak-network/DRB-node/dependencies/pietrzak_vdf"
)

func TestCalV(t *testing.T) {
	N := big.NewInt(101) // a small prime modulus
	x := big.NewInt(5)   // base
	T := 32              // number of iterations

	v := vdf.CalV(N, x, T)
	expected := new(big.Int).Exp(x, new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(T)), nil), N)

	if v.Cmp(expected) != 0 {
		t.Errorf("CalV failed, expected %s, got %s", expected, v)
	}
}

func TestRecHalveProof(t *testing.T) {
	N := big.NewInt(101)
	x := big.NewInt(5)
	y := big.NewInt(25)
	T := 32
	v := vdf.CalV(N, x, T)
	claim := vdf.Claim{
		N: N,
		X: x,
		Y: y,
		T: T,
		V: v,
	}

	proofList := vdf.RecHalveProof(claim)
	if len(proofList) == 0 {
		t.Errorf("RecHalveProof failed, proof list is empty")
	}
}

func TestHalveProof(t *testing.T) {
	N := big.NewInt(101)
	x := big.NewInt(5)
	y := big.NewInt(25)
	T := 32
	v := vdf.CalV(N, x, T)
	claim := vdf.Claim{
		N: N,
		X: x,
		Y: y,
		T: T,
		V: v,
	}

	halvedClaim := vdf.HalveProof(claim)
	if halvedClaim.T >= claim.T {
		t.Errorf("HalveProof failed, T was not halved properly")
	}
}

//func TestVerifyProof(t *testing.T) {
//	N := big.NewInt(101)
//	x := big.NewInt(5)
//	y := big.NewInt(25)
//	T := 32
//	v := vdf.CalV(N, x, T)
//	claim := vdf.Claim{
//		N: N,
//		X: x,
//		Y: y,
//		T: T,
//		V: v,
//	}
//
//	proofList := vdf.RecHalveProof(claim)
//	if !vdf.VerifyProof(proofList) {
//		t.Errorf("VerifyProof failed, proof was not verified correctly")
//	}
//}

func TestRecHalveProofWithDelta(t *testing.T) {
	N := big.NewInt(101)
	x := big.NewInt(5)
	y := big.NewInt(25)
	T := 32
	v := vdf.CalV(N, x, T)
	claim := vdf.Claim{
		N: N,
		X: x,
		Y: y,
		T: T,
		V: v,
	}

	proofList := vdf.RecHalveProofWithDelta(claim)
	if len(proofList) == 0 {
		t.Errorf("RecHalveProofWithDelta failed, proof list is empty")
	}
}
