package types

import (
	"crypto/sha256"
	"errors"

	"github.com/lazyledger/smt"
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

// ValidateBasic performs basic validation of fraud proof.
// Actually it's a placeholder, because nothing is checked.
func (fp *FraudProof) ValidateMerkleProofs() error {
	var root []byte
	for i := range fp.StateWitnesses {
		sparseMerkleProof := smt.SparseMerkleProof(fp.StateWitnesses[i].Proof)
		if !smt.VerifyProof(sparseMerkleProof, root, fp.StateWitnesses[i].Key, fp.StateWitnesses[i].Value, sha256.New()) {
			return errors.New("invalid merkle proofs in state witnesses of fraud proof")
		}
	}
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
