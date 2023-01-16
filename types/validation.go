package types

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
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

	if h.Commit.Height != uint64(h.Header.Height()) {
		return fmt.Errorf("height missmatch - header height: %d, commit height: %d", h.Header.Height(), h.Commit.Height)
	}
	if !bytes.Equal(h.Commit.HeaderHash[:], h.Header.Hash()) {
		return fmt.Errorf("hash missmatch - header hash: %s, commit hash: %s", hex.EncodeToString(h.Header.Hash()), hex.EncodeToString(h.Commit.HeaderHash[:]))
	}
	return nil
}
