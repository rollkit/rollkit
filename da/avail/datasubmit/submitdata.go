package datasubmit

import (
	"errors"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// submit data submits the extrinsic through substrate api
func SubmitData(apiURL string, seed string, appID int, data []byte) error {
	api, err := gsrpc.NewSubstrateAPI(apiURL)
	if err != nil {
		return err
	}

	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		return err
	}

	//if app id is greater than 0 then it must be created before submitting data
	err = errors.New("AppID cant be 0")
	if appID == 0 {
		return err
	}

	c, err := types.NewCall(meta, "DataAvailability.submit_data", data)
	if err != nil {
		return err
	}

	//Create the extrinsic
	ext := types.NewExtrinsic(c)

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		return err
	}

	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		return err
	}

	keyringPair, err := signature.KeyringPairFromSecret(seed, 42)
	if err != nil {
		return err
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", keyringPair.PublicKey)
	if err != nil {
		return err
	}

	var accountInfo types.AccountInfo
	ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	if err != nil || !ok {
		return err
	}

	nonce := uint32(accountInfo.Nonce)
	o := types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		AppID:              types.NewUCompactFromUInt(uint64(appID)),
		TransactionVersion: rv.TransactionVersion,
	}

	// Sign the transaction using Alice's default account
	err = ext.Sign(keyringPair, o)
	if err != nil {
		return err
	}

	// Send the extrinsic
	_, err = api.RPC.Author.SubmitExtrinsic(ext)
	if err != nil {
		return err
	}

	return nil

}
