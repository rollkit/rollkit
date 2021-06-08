package lazyledger

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pelletier/go-toml"
	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/lazyledger/lazyledger-app/app/params"
	apptypes "github.com/lazyledger/lazyledger-app/x/lazyledgerapp/types"

	"github.com/lazyledger/optimint/da"
	"github.com/lazyledger/optimint/log"
	"github.com/lazyledger/optimint/types"
)

type Config struct {
	// PayForMessage related params
	NamespaceID []byte
	PubKey      []byte
	BaseRateMax uint64 // currently not used
	TipRateMax  uint64 // currently not used
	From        string

	// temporary fee fields
	gasLimit  uint64
	feeAmount uint64

	// RPC related params
	RPCAddress string
	ChainID    string
	Timeout    time.Duration

	// keyring related params

	// KeyringAccName is the name of the account registered in the keyring
	// for the `From` address field
	KeyringAccName string
	// Backend is the backend of keyring that contains the KeyringAccName
	Backend string
	RootDir string
}

type LazyLedger struct {
	config Config
	logger log.Logger

	keyring keyring.Keyring

	rpcClient *grpc.ClientConn
}

var _ da.DataAvailabilityLayerClient = &LazyLedger{}

// Init is called once to allow DA client to read configuration and initialize resources.
func (ll *LazyLedger) Init(config []byte, logger log.Logger) error {
	ll.logger = logger
	err := toml.Unmarshal(config, &ll.config)
	if err != nil {
		return err
	}
	var userInput io.Reader
	// TODO(tzdybal): this means interactive reading from stdin - shouldn't we replace this somehow?
	userInput = os.Stdin
	ll.keyring, err = keyring.New(ll.config.KeyringAccName, ll.config.Backend, ll.config.RootDir, userInput)
	return err
}

func (ll *LazyLedger) Start() (err error) {
	ll.rpcClient, err = grpc.Dial(ll.config.RPCAddress, grpc.WithInsecure())
	return
}

func (ll *LazyLedger) Stop() error {
	return ll.rpcClient.Close()
}

// SubmitBlock submits the passed in block to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (ll *LazyLedger) SubmitBlock(block *types.Block) da.ResultSubmitBlock {
	msg, err := ll.preparePayForMessage(block)
	if err != nil {
		return da.ResultSubmitBlock{Code: da.StatusError, Message: err.Error()}
	}

	err = ll.callRPC(msg)
	if err != nil {
		return da.ResultSubmitBlock{Code: da.StatusError, Message: err.Error()}
	}

	return da.ResultSubmitBlock{Code: da.StatusSuccess}
}

func (ll *LazyLedger) callRPC(msg *apptypes.MsgWirePayForMessage) error {
	txReq, err := ll.sign(msg)
	if err != nil {
		return err
	}

	txClient := tx.NewServiceClient(ll.rpcClient)

	resp, err := txClient.BroadcastTx(context.Background(), txReq)
	if err != nil {
		return err
	}

	fmt.Println(resp.String())

	return nil
}

func (ll *LazyLedger) preparePayForMessage(block *types.Block) (*apptypes.MsgWirePayForMessage, error) {
	// TODO(tzdybal): serialize block
	var message []byte
	message, err := block.Serialize()
	if err != nil {
		return nil, err
	}

	// create PayForMessage message
	msg, err := apptypes.NewMsgWirePayForMessage(
		ll.config.NamespaceID,
		message,
		ll.config.PubKey,
		&apptypes.TransactionFee{
			BaseRateMax: ll.config.BaseRateMax,
			TipRateMax:  ll.config.TipRateMax,
		},
		apptypes.SquareSize,
	)
	if err != nil {
		return nil, err
	}

	// sign the PayForMessage's ShareCommitments
	err = msg.SignShareCommitments(ll.config.KeyringAccName, ll.keyring)
	if err != nil {
		return nil, err
	}

	// run message checks
	err = msg.ValidateBasic()
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func (ll *LazyLedger) sign(msg *apptypes.MsgWirePayForMessage) (*tx.BroadcastTxRequest, error) {
	encCfg := params.MakeEncodingConfig()

	// Create a new TxBuilder.
	txBuilder := encCfg.TxConfig.NewTxBuilder()

	txBuilder = ll.setConfigs(txBuilder)

	err := txBuilder.SetMsgs(msg)
	if err != nil {
		return nil, err
	}

	info, err := ll.keyring.Key(ll.config.KeyringAccName)
	if err != nil {
		return nil, err
	}

	// we must first set an empty signature
	sigV2 := signing.SignatureV2{
		PubKey: info.GetPubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: nil,
		},
		Sequence: 0,
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	accNum, seq, err := ll.queryAccount()
	if err != nil {
		return nil, err
	}

	signerData := authsigning.SignerData{
		ChainID:       ll.config.ChainID,
		AccountNumber: accNum,
		Sequence:      seq,
	}

	// Generate the bytes to be signed.
	bytesToSign, err := encCfg.TxConfig.SignModeHandler().GetSignBytes(
		signing.SignMode_SIGN_MODE_DIRECT,
		signerData,
		txBuilder.GetTx(),
	)
	if err != nil {
		return nil, err
	}

	// Sign those bytes
	sigBytes, _, err := ll.keyring.Sign(ll.config.From, bytesToSign)
	if err != nil {
		return nil, err
	}

	// Construct the SignatureV2 struct
	sig := signing.SignatureV2{
		PubKey: info.GetPubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: sigBytes,
		},
		Sequence: seq,
	}

	err = txBuilder.SetSignatures(sig)
	if err != nil {
		return nil, err
	}

	// Generated Protobuf-encoded bytes.
	txBytes, err := encCfg.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, err
	}

	return &tx.BroadcastTxRequest{
		// probably need to change this
		Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
		TxBytes: txBytes,
	}, nil
}

func (ll *LazyLedger) setConfigs(builder client.TxBuilder) client.TxBuilder {
	coin := sdk.Coin{
		Denom:  "token",
		Amount: sdk.NewInt(int64(ll.config.feeAmount)),
	}
	// todo(evan): don't hardcode the gas limit
	builder.SetGasLimit(ll.config.gasLimit)
	builder.SetFeeAmount(sdk.NewCoins(coin))
	return builder
}

// queryAccount fetches the account number and sequence number from the lazyledger-app node
func (ll *LazyLedger) queryAccount() (accNum uint64, seqNum uint64, err error) {
	qclient := authtypes.NewQueryClient(ll.rpcClient)
	resp, err := qclient.Account(
		context.TODO(),
		&authtypes.QueryAccountRequest{Address: ll.config.From},
	)
	if err != nil {
		return accNum, seqNum, err
	}

	fmt.Println(resp.String())

	// acc := resp.Account.Value

	// accNum, seqNum = acc.GetAccountNumber(), acc.GetSequence()
	return
}
