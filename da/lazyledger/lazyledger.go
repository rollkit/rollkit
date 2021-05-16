package lazyledger

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pelletier/go-toml"
	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types/tx"
	apptypes "github.com/lazyledger/lazyledger-app/x/lazyledgerapp/types"

	"github.com/lazyledger/optimint/da"
	"github.com/lazyledger/optimint/log"
	"github.com/lazyledger/optimint/types"
)

type Config struct {
	// PayForMessage related params
	NamespaceID []byte
	PubKey      []byte
	BaseRateMax uint64
	TipRateMax  uint64
	From        string

	// RPC related params
	Address string
	Timeout time.Duration

	// keyring related params
	AppName string
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
	ll.keyring, err = keyring.New(ll.config.AppName, ll.config.Backend, ll.config.RootDir, userInput)
	return err
}

func (ll *LazyLedger) Start() (err error) {
	ll.rpcClient, err = grpc.Dial(ll.config.Address, grpc.WithInsecure())
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

func (ll *LazyLedger) preparePayForMessage(block *types.Block) (*apptypes.MsgWirePayForMessage, error) {
	// TODO(tzdybal): serialize block
	var message []byte
	message, err := types.SerializeBlock(block)
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
	err = msg.SignShareCommitments(ll.config.From, ll.keyring)
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

func (ll *LazyLedger) callRPC(msg *apptypes.MsgWirePayForMessage) error {
	txClient := tx.NewServiceClient(ll.rpcClient)

	txBytes, err := msg.Marshal()
	if err != nil {
		return err
	}

	resp, err := txClient.BroadcastTx(context.Background(),
		&tx.BroadcastTxRequest{
			Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
			TxBytes: txBytes,
		},
	)
	fmt.Println("tzdybal:", resp)
	if err != nil {
		return err
	}

	return nil
}
