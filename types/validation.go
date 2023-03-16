package types

import (
	"errors"

	"github.com/tendermint/tendermint/crypto/ed25519"
)

// ValidateBasic performs basic validation of a block.
func (b *Block) ValidateBasic() error {
	err := b.SignedHeader.Header.ValidateBasic()
	if err != nil {
		return err
	}

	err = b.Data.ValidateBasic()
	if err != nil {
		return err
	}

	err = b.SignedHeader.Commit.ValidateBasic()
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

// ValidateBasic performs basic validation of a validator set
func (vSet *ValidatorSet) ValidateBasic() error {
	if len(vSet.Validators) == 0 {
		return errors.New("no validators")
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

	err = h.Validators.ValidateBasic()
	if err != nil {
		return err
	}

	// Make sure there are as many signatures as validators
	if len(h.Commit.Signatures) != len(h.Validators.Validators) {
		errors.New("number of signatures and keys don't match")
	}

	for i, val := range h.Validators.Validators {
		sig := h.Commit.Signatures[i]
		var pubKey ed25519.PubKey
		pubKey = val.PublicKey
		if !pubKey.VerifySignature(h.Hash(), sig) {
			errors.New("signature verification failed")
		}
	}

	return nil
}
