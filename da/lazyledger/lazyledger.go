package lazyledger

import (
	"context"
	"os"
	"time"

	"github.com/pelletier/go-toml"
	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	appclient "github.com/lazyledger/lazyledger-app/x/lazyledgerapp/client"
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

	// TODO(tzdybal): this means interactive reading from stdin - shouldn't we replace this somehow?
	userInput := os.Stdin
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

	ctx, cancel := context.WithTimeout(context.TODO(), ll.config.Timeout)
	defer cancel()

	err = ll.callRPC(ctx, msg)
	if err != nil {
		return da.ResultSubmitBlock{Code: da.StatusError, Message: err.Error()}
	}

	return da.ResultSubmitBlock{Code: da.StatusSuccess}
}

func (ll *LazyLedger) callRPC(ctx context.Context, msg *apptypes.MsgWirePayForMessage) error {
	info, err := ll.keyring.Key(ll.config.KeyringAccName)
	if err != nil {
		return err
	}

	b := appclient.NewBuilder(info.GetAddress(), ll.config.ChainID)

	err = b.UpdateAccountNumber(ctx, ll.rpcClient)
	if err != nil {
		return err
	}

	coin := sdk.Coin{
		Denom:  "token",
		Amount: sdk.NewInt(10),
	}
	b.SetFeeAmount(sdk.NewCoins(coin))

	b.SetGasLimit(ll.config.GasLimit)

	signedTx, err := b.BuildSignedTx(msg, ll.keyring)
	if err != nil {
		return err
	}

	rawTx, err := b.EncodeTx(signedTx)
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

	return msg, msg.ValidateBasic()
}
