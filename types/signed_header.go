package types

import (
	"bytes"
	"fmt"

	"github.com/celestiaorg/go-header"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	tmtypes "github.com/tendermint/tendermint/types"
)

func (sH *SignedHeader) New() header.Header {
	return new(SignedHeader)
}

func (sH *SignedHeader) IsZero() bool {
	return sH == nil
}

func (sH *SignedHeader) Verify(untrst header.Header) error {
	// Explicit type checks are required due to embedded Header which also does the explicit type check
	untrstH, ok := untrst.(*SignedHeader)
	if !ok {
		// if the header type is wrong, something very bad is going on
		// and is a programmer bug
		panic(fmt.Errorf("%T is not of type %T", untrst, untrstH))
	}
	err := untrstH.ValidateBasic()
	if err != nil {
		return &header.VerifyError{
			Reason: err,
		}
	}
	sHHash := sH.Header.Hash()
	if !bytes.Equal(untrstH.LastHeaderHash[:], sHHash) {
		return &header.VerifyError{
			Reason: fmt.Errorf("Last header hash %v does not match hash of previous header %v", untrstH.LastHeaderHash[:], sHHash),
		}
	}
	sHLastCommitHash := sH.getLastCommitHash()
	if !bytes.Equal(untrstH.LastCommitHash[:], sHLastCommitHash) {
		return &header.VerifyError{
			Reason: fmt.Errorf("Last commit hash %v does not match hash of previous header %v", untrstH.LastCommitHash[:], sHHash),
		}
	}
	return sH.Header.Verify(&untrstH.Header)
}

var _ header.Header = &SignedHeader{}

func (sH *SignedHeader) getLastCommitHash() []byte {
	lastABCICommit := tmtypes.Commit{
		Height: sH.Height(),
		Round:  0,
		BlockID: tmtypes.BlockID{
			Hash:          tmbytes.HexBytes(sH.Hash()),
			PartSetHeader: tmtypes.PartSetHeader{},
		},
	}
	for _, sig := range sH.Commit.Signatures {
		commitSig := tmtypes.CommitSig{
			BlockIDFlag: tmtypes.BlockIDFlagCommit,
			Signature:   sig,
		}
		lastABCICommit.Signatures = append(lastABCICommit.Signatures, commitSig)
	}

	if len(sH.Commit.Signatures) == 1 {
		lastABCICommit.Signatures[0].ValidatorAddress = sH.ProposerAddress
		lastABCICommit.Signatures[0].Timestamp = sH.Time()
	}
	return lastABCICommit.Hash()
}
