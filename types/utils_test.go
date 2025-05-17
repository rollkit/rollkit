package types

import (
	"testing"
)

func TestGetRandomTx(t *testing.T) {
	// Generate 100 random transactions and check that they are all unique
	txSet := make(map[string]bool)
	for range 100 {
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
	for range 100 {
		bytes := GetRandomBytes(32)
		key := string(bytes[:])
		if _, ok := byteSet[key]; ok {
			t.Errorf("Duplicate byte slice generated: %v", bytes)
		}
		byteSet[key] = true
	}
}

func TestGetRandomHeader(t *testing.T) {
	// Generate 100 random headers and check that they are all unique
	headerSet := make(map[string]bool)
	for range 100 {
		appHashBytes := GetRandomBytes(32)
		header := GetRandomHeader("TestGetRandomHeader", appHashBytes)
		headerHash := header.Hash().String()
		if _, ok := headerSet[headerHash]; ok {
			t.Errorf("Duplicate header generated: %v", header)
		}
		headerSet[headerHash] = true
	}
}
