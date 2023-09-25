package types

import (
	"math/rand"
	"time"

	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtypes "github.com/cometbft/cometbft/types"
)

// TODO: accept argument for number of validators / proposer index
func GetRandomValidatorSet() *cmtypes.ValidatorSet {
	valSet, _ := GetRandomValidatorSetWithPrivKey()
	return valSet
}

func GetRandomValidatorSetWithPrivKey() (*cmtypes.ValidatorSet, ed25519.PrivKey) {
	privKey := ed25519.GenPrivKey()
	pubKey := privKey.PubKey()
	return &cmtypes.ValidatorSet{
		Proposer: &cmtypes.Validator{PubKey: pubKey, Address: pubKey.Address()},
		Validators: []*cmtypes.Validator{
			{PubKey: pubKey, Address: pubKey.Address()},
		},
	}, privKey
}

func GetRandomSignedHeader() (*SignedHeader, ed25519.PrivKey, error) {
	valSet, privKey := GetRandomValidatorSetWithPrivKey()
	signedHeader := &SignedHeader{
		Header: Header{
			Version: Version{
				Block: InitStateVersion.Consensus.Block,
				App:   InitStateVersion.Consensus.App,
			},

			BaseHeader: BaseHeader{
				ChainID: "test",
				Height:  uint64(rand.Int63()), //nolint:gosec,
				Time:    uint64(time.Now().UnixNano()),
			},
			LastHeaderHash:  GetRandomBytes(32),
			LastCommitHash:  GetRandomBytes(32),
			DataHash:        GetRandomBytes(32),
			ConsensusHash:   GetRandomBytes(32),
			AppHash:         GetRandomBytes(32),
			LastResultsHash: GetRandomBytes(32),
			ProposerAddress: valSet.Proposer.Address,
			Signatures:      [][]byte{valSet.Hash()},
		},
		Validators: valSet,
	}
	commit, err := getCommit(signedHeader.Header, privKey)
	if err != nil {
		return nil, nil, err
	}
	signedHeader.Commit = *commit
	return signedHeader, privKey, nil
}

func GetNextRandomHeader(signedHeader *SignedHeader, privKey ed25519.PrivKey) (*SignedHeader, error) {
	valSet := signedHeader.Validators
	newSignedHeader := &SignedHeader{
		Header: Header{
			Version: Version{
				Block: InitStateVersion.Consensus.Block,
				App:   InitStateVersion.Consensus.App,
			},

			BaseHeader: BaseHeader{
				ChainID: "test",
				Height:  signedHeader.Height() + 1,
				Time:    uint64(time.Now().UnixNano()),
			},
			LastHeaderHash:  signedHeader.Hash(),
			DataHash:        GetRandomBytes(32),
			ConsensusHash:   GetRandomBytes(32),
			AppHash:         GetRandomBytes(32),
			LastResultsHash: GetRandomBytes(32),
			ProposerAddress: valSet.Proposer.Address,
			Signatures:      [][]byte{valSet.Hash()},
		},
		Validators: valSet,
	}
	newSignedHeader.LastCommitHash = signedHeader.Commit.GetCommitHash(
		&newSignedHeader.Header, signedHeader.ProposerAddress,
	)
	commit, err := getCommit(newSignedHeader.Header, privKey)
	if err != nil {
		return nil, err
	}
	newSignedHeader.Commit = *commit
	return newSignedHeader, nil
}

func GetRandomTx() Tx {
	size := rand.Int()%100 + 100 //nolint:gosec
	return Tx(GetRandomBytes(size))
}

func GetRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data) //nolint:gosec,staticcheck
	return data
}

func getCommit(header Header, privKey ed25519.PrivKey) (*Commit, error) {
	headerBytes, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	sign, err := privKey.Sign(headerBytes)
	if err != nil {
		return nil, err
	}
	return &Commit{
		Signatures: []Signature{sign},
	}, nil
}
