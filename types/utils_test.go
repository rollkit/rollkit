package types

import (
	"testing"
)

func TestGetRandomTx(t *testing.T) {
	// Generate 100 random transactions and check that they are all unique
	txSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tx := GetRandomTx()
		key := string(tx[:])
		if _, ok := txSet[key]; ok {
			t.Errorf("Duplicate transaction generated: %v", tx)
		}
		txSet[key] = true
	}
}

func TestGetRandomBytes(t *testing.T) {
	// Generate 100 random byte slices of length 32 and check that they are all unique
	byteSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		bytes := GetRandomBytes(32)
		key := string(bytes[:])
		if _, ok := byteSet[key]; ok {
			t.Errorf("Duplicate byte slice generated: %v", bytes)
		}
		byteSet[key] = true
	}
}

func TestGetRandomValidatorSet(t *testing.T) {
	// Generate 100 random validator sets and check that they are all unique
	valSetSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		valSet := GetRandomValidatorSet()
		valSetHash := string(valSet.Hash())
		if _, ok := valSetSet[valSetHash]; ok {
			t.Errorf("Duplicate validator set generated: %v", valSet)
		}
		valSetSet[valSetHash] = true
	}
}

func TestGetRandomHeader(t *testing.T) {
	// Generate 100 random headers and check that they are all unique
	headerSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		header := GetRandomHeader()
		headerHash := header.Hash().String()
		if _, ok := headerSet[headerHash]; ok {
			t.Errorf("Duplicate header generated: %v", header)
		}
		headerSet[headerHash] = true
	}
}
