package lazyledger

import (
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
}

type LazyLedger struct {
	config Config
	logger log.Logger
}

var _ da.DataAvailabilityLayerClient = &LazyLedger{}

// Init is called once to allow DA client to read configuration and initialize resources.
func (ll *LazyLedger) Init(config []byte, logger log.Logger) error {
	ll.logger = logger
	return toml.Unmarshal(config, &ll.config)
}

func (ll *LazyLedger) Start() error {
	panic("not implemented") // TODO: Implement
}

func (ll *LazyLedger) Stop() error {
	panic("not implemented") // TODO: Implement
}

// SubmitBlock submits the passed in block to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (ll *LazyLedger) SubmitBlock(block *types.Block) da.ResultSubmitBlock {
	msg, err := ll.preparePayForMessage(block)
	if err != nil {
		return da.ResultSubmitBlock{
			Code: da.StatusError,
		}
	}

	ll.callRPC(msg)

	return da.ResultSubmitBlock{Code: da.StatusSuccess}
}

func (ll *LazyLedger) preparePayForMessage(block *types.Block) (*apptypes.MsgWirePayForMessage, error) {
	// TODO(tzdybal): serialize block
	var message []byte

	// TODO(tzdybal): add configuration
	var keyring keyring.Keyring

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
	err = msg.SignShareCommitments(ll.config.From, keyring)
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
