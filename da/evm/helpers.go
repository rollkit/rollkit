package evm

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func BuildTxOpts(client *ethclient.Client, sender common.Address, privateKey *ecdsa.PrivateKey) *bind.TransactOpts {
	// Get the nonce from the RPC.
	nonce, err := client.PendingNonceAt(context.Background(), sender)
	if err != nil {
		panic("REEE 1")
	}

	// Get the ChainID from the RPC.
	chainID, err := client.ChainID(context.Background())
	if err != nil {
		panic("REEE 2")
	}

	// Build transaction opts object.
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		panic("REEE 3")
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0) // in wei
	auth.GasLimit = 6000000
	return auth
}

// ExpectSuccessReceipt waits for the transaction to be mined and returns the receipt.
// It also checks that the transaction was successful.
func WaitForMined(
	client *ethclient.Client,
	tx *coretypes.Transaction,
) (*coretypes.Receipt, error) {
	// Wait for the transaction to be mined.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return nil, err
	}
	// Verify the receipt is good.
	receipt, err := client.TransactionReceipt(context.Background(), tx.Hash())
	if err != nil {
		return nil, err
	}
	return receipt, nil
}
