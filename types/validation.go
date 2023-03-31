package types

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"
)

// ValidateBasic performs basic validation of a block.
func (b *Block) ValidateBasic() error {
	err := b.SignedHeader.ValidateBasic()
	if err != nil {
		return err
	}

	err = b.Data.ValidateBasic()
	if err != nil {
		return err
	}

	return nil
}

// ValidateBasic performs basic validation of a header.
func (h *Header) ValidateBasic() error {
	if len(h.ProposerAddress) == 0 {
		return errors.New("no proposer address")
	}

	return nil
}

// ValidateBasic performs basic validation of block data.
// Actually it's a placeholder, because nothing is checked.
func (d *Data) ValidateBasic() error {
	return nil
}

// ValidateBasic performs basic validation of a commit.
func (c *Commit) ValidateBasic() error {
	if len(c.Signatures) == 0 {
		return errors.New("no signatures")
	}
	return nil
}

// ValidateBasic performs basic validation of a signed header.
func (h *SignedHeader) ValidateBasic() error {
	err := h.Header.ValidateBasic()
	if err != nil {
		return err
	}

	err = h.Commit.ValidateBasic()
	if err != nil {
		return err
	}

	err = ValidatorsValidateBasic(h.Validators)
	if err != nil {
		return err
	}

	if !bytes.Equal(h.Validators.Hash(), h.AggregatorsHash[:]) {
		return errors.New("aggregator set hash in signed header and hash of validator set do not match")
	}

	// Make sure there is exactly one signature
	if len(h.Commit.Signatures) != 1 {
		return errors.New("expected exactly one signature")
	}

	signature := h.Commit.Signatures[0]
	proposer := h.Validators.GetProposer()
	if proposer == nil {
		return nil
	}
	var pubKey ed25519.PubKey = proposer.PubKey.Bytes()
	msg, err := h.Header.MarshalBinary()
	if err != nil {
		return errors.New("signature verification failed, unable to marshal header")
	}
	if !pubKey.VerifySignature(msg, signature) {
		return errors.New("signature verification failed")
	}

	return nil
}

func ValidatorsValidateBasic(vals *tmtypes.ValidatorSet) error {
	for idx, val := range vals.Validators {
		if err := val.ValidateBasic(); err != nil {
			return fmt.Errorf("invalid validator #%d: %w", idx, err)
		}
	}

	if len(vals.Validators) > 0 && vals.Proposer != nil {
		if err := vals.Proposer.ValidateBasic(); err != nil {
			return fmt.Errorf("proposer failed validate basic, error: %w", err)
		}
	}

	return nil
}
