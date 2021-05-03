package da

import (
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	apptypes "github.com/lazyledger/lazyledger-app/x/lazyledgerapp/types"

	"github.com/lazyledger/optimint/types"
)

type LazyLedger struct {
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
func (ll *LazyLedger) SubmitBlock(block *types.Block) ResultSubmitBlock {
	msg, err := ll.preparePayForMessage(block)
	if err != nil {
		return ResultSubmitBlock{
			Code: StatusError,
		}
	}

	ll.callRPC(msg)

	return ResultSubmitBlock{Code: StatusSuccess}
}

func (ll *LazyLedger) preparePayForMessage(block *types.Block) (*apptypes.MsgWirePayForMessage, error) {
	// TODO(tzdybal): serialize block
	var message []byte

	// TODO(tzdybal): add configuration
	var namespaceID []byte
	var pubKey []byte
	var baseRateMax uint64
	var tipRateMax uint64
	var account string // AKA from
	var keyring keyring.Keyring

	// create PayForMessage message
	msg, err := apptypes.NewMsgWirePayForMessage(
		namespaceID,
		message,
		pubKey,
		&apptypes.TransactionFee{
			BaseRateMax: baseRateMax,
			TipRateMax:  tipRateMax,
		},
		apptypes.SquareSize,
	)
	if err != nil {
		return nil, err
	}

	// sign the PayForMessage's ShareCommitments
	err = msg.SignShareCommitments(account, keyring)
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
