package lazyledger

import (
	"context"
	"os"
	"time"

	"github.com/pelletier/go-toml"
	"google.golang.org/grpc"

	appclient "github.com/celestiaorg/celestia-app/x/payment/client"
	apptypes "github.com/celestiaorg/celestia-app/x/payment/types"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"

	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

// Config holds all configuration required by LazyLedger DA layer client.
type Config struct {
	// PayForMessage related params
	NamespaceID []byte
	PubKey      []byte
	BaseRateMax uint64 // currently not used
	TipRateMax  uint64 // currently not used

	// temporary fee fields
	GasLimit  uint64
	FeeAmount uint64

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

// LazyLedger implements DataAvailabilityLayerClient interface.
// It use lazyledger-app via gRPC.
type LazyLedger struct {
	config  Config
	kvStore store.KVStore
	logger  log.Logger

	keyring keyring.Keyring

	rpcClient *grpc.ClientConn
}

var _ da.DataAvailabilityLayerClient = &LazyLedger{}

// Init is called once to allow DA client to read configuration and initialize resources.
func (ll *LazyLedger) Init(config []byte, kvStore store.KVStore, logger log.Logger) error {
	ll.logger = logger
	ll.kvStore = kvStore
	err := toml.Unmarshal(config, &ll.config)
	if err != nil {
		return err
	}

	// TODO(tzdybal): this means interactive reading from stdin - shouldn't we replace this somehow?
	userInput := os.Stdin
	ll.keyring, err = keyring.New(ll.config.KeyringAccName, ll.config.Backend, ll.config.RootDir, userInput)
	return err
}

// Start establishes gRPC connection to lazyledger app.
func (ll *LazyLedger) Start() (err error) {
	ll.rpcClient, err = grpc.Dial(ll.config.RPCAddress, grpc.WithInsecure())
	return
}

// Stop closes gRPC connection.
func (ll *LazyLedger) Stop() error {
	return ll.rpcClient.Close()
}

// SubmitBlock submits the passed in block to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (ll *LazyLedger) SubmitBlock(block *types.Block) da.ResultSubmitBlock {
	msg, err := ll.preparePayForMessage(block)
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), ll.config.Timeout)
	defer cancel()

	err = ll.callRPC(ctx, msg)
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusSuccess}}
}

// CheckBlockAvailability queries DA layer to check data availability of block corresponding to given header.
func (l *LazyLedger) CheckBlockAvailability(header *types.Header) da.ResultCheckBlock {
	panic("not implemented") // TODO: Implement
}

func (ll *LazyLedger) callRPC(ctx context.Context, msg *apptypes.MsgWirePayForMessage) error {
	signer := appclient.NewKeyringSigner(ll.keyring, ll.config.KeyringAccName, ll.config.ChainID)

	err := signer.QueryAccountNumber(ctx, ll.rpcClient)
	if err != nil {
		return err
	}

	coin := sdk.Coin{
		Denom:  "token",
		Amount: sdk.NewInt(10), // TODO(tzdybal): un-hardcode
	}

	builder := signer.NewTxBuilder()
	builder.SetFeeAmount(sdk.NewCoins(coin))
	builder.SetGasLimit(ll.config.GasLimit)

	signedTx, err := signer.BuildSignedTx(builder, msg)
	if err != nil {
		return err
	}

	rawTx, err := signer.EncodeTx(signedTx)
	if err != nil {
		return err
	}

	// wait for the transaction to be confirmed in a block
	_, err = appclient.BroadcastTx(ctx, ll.rpcClient, tx.BroadcastMode_BROADCAST_MODE_BLOCK, rawTx)
	if err != nil {
		return err
	}

	return nil
}

func (ll *LazyLedger) preparePayForMessage(block *types.Block) (*apptypes.MsgWirePayForMessage, error) {
	// TODO(tzdybal): serialize block
	var message []byte
	message, err := block.MarshalBinary()
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

	return msg, msg.ValidateBasic()
}
