package types

import "errors"

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

func (h *Header) ValidateBasic() error {
	if len(h.ProposerAddress) == 0 {
		return errors.New("no proposer address")
	}

	return nil
}

func (d *Data) ValidateBasic() error {
	return nil
}

func (c *Commit) ValidateBasic() error {
	if c.Height > 0 {
		if len(c.Signatures) == 0 {
			return errors.New("no signatures")
		}
	}
	return nil
}
