package types

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/version"
)

// ValidateBasic performs basic validation of a block.
func (b *Block) ValidateBasic() error {
	err := b.Header.ValidateBasic()
	if err != nil {
		return err
	}

	err = b.Data.ValidateBasic()
	if err != nil {
		return err
	}

	err = b.LastCommit.ValidateBasic()
	if err != nil {
		return err
	}

	return nil
}

// ValidateBasic performs basic validation of a header.
func (h *Header) ValidateBasic() error {
	if h.Version.Block != version.BlockProtocol {
		return fmt.Errorf("block protocol is incorrect: got: %d, want: %d ", h.Version.Block, version.BlockProtocol)
	}

	if len(h.ProposerAddress) != crypto.AddressSize {
		return fmt.Errorf(
			"invalid ProposerAddress length; got: %d, expected: %d",
			len(h.ProposerAddress), crypto.AddressSize,
		)
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
	if c.Height > 0 {
		if len(c.Signatures) == 0 {
			return errors.New("no signatures")
		}
	}
	return nil
}

// ValidateBasic performs basic validation of a signed header.
func (h *SignedHeader) ValidateBasic() error {
	err := h.Commit.ValidateBasic()
	if err != nil {
		return err
	}
	err = h.Header.ValidateBasic()
	if err != nil {
		return err
	}

	if h.Commit.Height != h.Header.Height {
		return fmt.Errorf("height missmatch - header height: %d, commit height: %d", h.Header.Height, h.Commit.Height)
	}
	if h.Commit.HeaderHash != h.Header.Hash() {
		hash := h.Header.Hash()
		return fmt.Errorf("hash missmatch - header hash: %s, commit hash: %s", hex.EncodeToString(hash[:]), hex.EncodeToString(h.Commit.HeaderHash[:]))
	}
	return nil
}
