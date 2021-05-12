package lazyledger

import (
	"io"
	"os"
	"time"

	"github.com/pelletier/go-toml"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
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

func (ll *LazyLedger) Start() error {
	// nothing special to do here
	return nil
}

func (ll *LazyLedger) Stop() error {
	// nothing special to do here
	return nil
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

func (ll *LazyLedger) callRPC(*apptypes.MsgWirePayForMessage) {
	panic("not implemented!")
}
