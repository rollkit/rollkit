package types

import (
	cryptoRand "crypto/rand"
	"errors"
	"math/rand"
	"time"

	"github.com/celestiaorg/go-header"

	"github.com/cometbft/cometbft/crypto/ed25519"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// DefaultSigningKeyType is the key type used by the sequencer signing key
const DefaultSigningKeyType = "ed25519"

// ValidatorConfig carries all necessary state for generating a Validator
type ValidatorConfig struct {
	PrivKey     crypto.PrivKey
	VotingPower int64
}

// GetValidatorSetCustom returns a validator set based on the provided validatorConfig.
func GetValidatorSetCustom(config ValidatorConfig) (Signer, error) {
	if config.PrivKey == nil {
		panic(errors.New("private key is nil"))
	}
	pubKey := config.PrivKey.GetPublic()

	signer, err := NewSigner(pubKey)
	if err != nil {
		return Signer{}, err
	}
	return signer, nil
}

// BlockConfig carries all necessary state for block generation
type BlockConfig struct {
	Height       uint64
	NTxs         int
	PrivKey      crypto.PrivKey // Input and Output option
	ProposerAddr []byte         // Input option
}

// GetRandomBlock creates a block with a given height and number of transactions, intended for testing.
// It's tailored for simplicity, primarily used in test setups where additional outputs are not needed.
func GetRandomBlock(height uint64, nTxs int, chainID string) (*SignedHeader, *Data) {
	config := BlockConfig{
		Height: height,
		NTxs:   nTxs,
	}
	// Assuming GenerateBlock modifies the context directly with the generated block and other needed data.
	header, data, _ := GenerateRandomBlockCustom(&config, chainID)

	return header, data
}

// GenerateRandomBlockCustom returns a block with random data and the given height, transactions, privateKey and proposer address.
func GenerateRandomBlockCustom(config *BlockConfig, chainID string) (*SignedHeader, *Data, crypto.PrivKey) {
	data := getBlockDataWith(config.NTxs)
	dataHash := data.Hash()

	if config.PrivKey == nil {
		pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
		if err != nil {
			panic(err)
		}
		config.PrivKey = pk
	}

	headerConfig := HeaderConfig{
		Height:   config.Height,
		DataHash: dataHash,
		PrivKey:  config.PrivKey,
	}

	signedHeader, err := GetRandomSignedHeaderCustom(&headerConfig, chainID)
	if err != nil {
		panic(err)
	}

	if config.ProposerAddr != nil {
		signedHeader.ProposerAddress = config.ProposerAddr
	}

	data.Metadata = &Metadata{
		ChainID:      chainID,
		Height:       signedHeader.Height(),
		LastDataHash: nil,
		Time:         uint64(signedHeader.Time().UnixNano()),
	}

	return signedHeader, data, config.PrivKey
}

// GetRandomNextBlock returns a block with random data and height of +1 from the provided block
func GetRandomNextBlock(header *SignedHeader, data *Data, privKey crypto.PrivKey, appHash header.Hash, nTxs int, chainID string) (*SignedHeader, *Data) {
	nextData := getBlockDataWith(nTxs)
	dataHash := nextData.Hash()

	newSignedHeader := &SignedHeader{
		Header: GetRandomNextHeader(header.Header, chainID),
		Signer: header.Signer,
	}
	newSignedHeader.ProposerAddress = header.Header.ProposerAddress
	newSignedHeader.LastResultsHash = nil
	newSignedHeader.Header.DataHash = dataHash
	newSignedHeader.AppHash = appHash
	newSignedHeader.LastCommitHash = header.Signature.GetCommitHash(
		&newSignedHeader.Header, header.ProposerAddress,
	)
	signature, err := GetSignature(newSignedHeader.Header, privKey)
	if err != nil {
		panic(err)
	}
	newSignedHeader.Signature = *signature
	nextData.Metadata = &Metadata{
		ChainID:      chainID,
		Height:       newSignedHeader.Height(),
		LastDataHash: nil,
		Time:         uint64(newSignedHeader.Time().UnixNano()),
	}
	return newSignedHeader, nextData
}

// HeaderConfig carries all necessary state for header generation
type HeaderConfig struct {
	Height      uint64
	DataHash    header.Hash
	PrivKey     crypto.PrivKey
	VotingPower int64
}

// GetRandomHeader returns a header with random fields and current time
func GetRandomHeader(chainID string) Header {
	return Header{
		BaseHeader: BaseHeader{
			Height:  uint64(rand.Int63()), //nolint:gosec
			Time:    uint64(time.Now().UnixNano()),
			ChainID: chainID,
		},
		Version: Version{
			Block: InitStateVersion.Block,
			App:   InitStateVersion.App,
		},
		LastHeaderHash:  GetRandomBytes(32),
		LastCommitHash:  GetRandomBytes(32),
		DataHash:        GetRandomBytes(32),
		ConsensusHash:   GetRandomBytes(32),
		AppHash:         GetRandomBytes(32),
		LastResultsHash: GetRandomBytes(32),
		ProposerAddress: GetRandomBytes(32),
		ValidatorHash:   GetRandomBytes(32),
	}
}

// GetRandomNextHeader returns a header with random data and height of +1 from
// the provided Header
func GetRandomNextHeader(header Header, chainID string) Header {
	nextHeader := GetRandomHeader(chainID)
	nextHeader.BaseHeader.Height = header.Height() + 1
	nextHeader.BaseHeader.Time = uint64(time.Now().Add(1 * time.Second).UnixNano())
	nextHeader.LastHeaderHash = header.Hash()
	nextHeader.ProposerAddress = header.ProposerAddress
	nextHeader.ValidatorHash = header.ValidatorHash
	return nextHeader
}

// GetRandomSignedHeader generates a signed header with random data and returns it.
func GetRandomSignedHeader(chainID string) (*SignedHeader, crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
	if err != nil {
		return nil, nil, err
	}
	config := HeaderConfig{
		Height:      uint64(rand.Int63()), //nolint:gosec
		DataHash:    GetRandomBytes(32),
		PrivKey:     pk,
		VotingPower: 1,
	}

	signedHeader, err := GetRandomSignedHeaderCustom(&config, chainID)
	if err != nil {
		return nil, nil, err
	}
	return signedHeader, pk, nil
}

// GetRandomSignedHeaderCustom creates a signed header based on the provided HeaderConfig.
func GetRandomSignedHeaderCustom(config *HeaderConfig, chainID string) (*SignedHeader, error) {
	signer, err := NewSigner(config.PrivKey.GetPublic())
	if err != nil {
		return nil, err
	}

	signedHeader := &SignedHeader{
		Header: GetRandomHeader(chainID),
		Signer: signer,
	}
	signedHeader.Header.BaseHeader.Height = config.Height
	signedHeader.Header.DataHash = config.DataHash
	signedHeader.Header.ProposerAddress = signer.Address
	signedHeader.Header.ValidatorHash = signer.Address
	signedHeader.Header.BaseHeader.Time = uint64(time.Now().UnixNano()) + (config.Height)*10

	signature, err := GetSignature(signedHeader.Header, config.PrivKey)
	if err != nil {
		return nil, err
	}
	signedHeader.Signature = *signature
	return signedHeader, nil
}

// GetRandomNextSignedHeader returns a signed header with random data and height of +1 from
// the provided signed header
func GetRandomNextSignedHeader(signedHeader *SignedHeader, privKey crypto.PrivKey, chainID string) (*SignedHeader, error) {
	newSignedHeader := &SignedHeader{
		Header: GetRandomNextHeader(signedHeader.Header, chainID),
		Signer: signedHeader.Signer,
	}
	newSignedHeader.LastCommitHash = signedHeader.Signature.GetCommitHash(
		&newSignedHeader.Header, signedHeader.ProposerAddress,
	)
	signature, err := GetSignature(newSignedHeader.Header, privKey)
	if err != nil {
		return nil, err
	}
	newSignedHeader.Signature = *signature
	return newSignedHeader, nil
}

// GetValidatorSetFromGenesis returns a ValidatorSet from a GenesisDoc, for usage with the centralized sequencer scheme.
func GetValidatorSetFromGenesis(g *cmtypes.GenesisDoc) cmtypes.ValidatorSet {
	vals := []*cmtypes.Validator{
		{
			Address:          g.Validators[0].Address,
			PubKey:           g.Validators[0].PubKey,
			VotingPower:      int64(1),
			ProposerPriority: int64(1),
		},
	}
	return cmtypes.ValidatorSet{
		Validators: vals,
		Proposer:   vals[0],
	}
}

// GetGenesisWithPrivkey returns a genesis doc with a single validator and a signing key
func GetGenesisWithPrivkey(chainID string) (*cmtypes.GenesisDoc, crypto.PrivKey) {
	genesisValidatorKey := ed25519.GenPrivKey()
	pubKey := genesisValidatorKey.PubKey()

	genesisValidators := []cmtypes.GenesisValidator{{
		Address: pubKey.Address(),
		PubKey:  pubKey,
		Power:   int64(1),
		Name:    "sequencer",
	}}
	genDoc := &cmtypes.GenesisDoc{
		ChainID:       chainID,
		InitialHeight: 1,
		Validators:    genesisValidators,
	}
	privKey, err := crypto.UnmarshalEd25519PrivateKey(genesisValidatorKey.Bytes())
	if err != nil {
		panic(err)
	}
	return genDoc, privKey
}

// GetRandomTx returns a tx with random data
func GetRandomTx() Tx {
	size := rand.Int()%100 + 100 //nolint:gosec
	return Tx(GetRandomBytes(uint(size)))
}

// GetRandomBytes returns a byte slice of random bytes of length n.
// It uses crypto/rand for cryptographically secure random number generation.
func GetRandomBytes(n uint) []byte {
	data := make([]byte, n)
	if _, err := cryptoRand.Read(data); err != nil {
		panic(err)
	}
	return data
}

// GetSignature returns a signature from the given private key over the given header
func GetSignature(header Header, privKey crypto.PrivKey) (*Signature, error) {
	consensusVote, err := header.Vote()
	if err != nil {
		return nil, err
	}
	sign, err := privKey.Sign(consensusVote)
	if err != nil {
		return nil, err
	}
	signature := Signature(sign)
	return &signature, nil
}

func getBlockDataWith(nTxs int) *Data {
	data := &Data{
		Txs: make(Txs, nTxs),
		// IntermediateStateRoots: IntermediateStateRoots{
		// 	RawRootsList: make([][]byte, nTxs),
		// },
	}

	for i := 0; i < nTxs; i++ {
		data.Txs[i] = GetRandomTx()
		// block.Data.IntermediateStateRoots.RawRootsList[i] = GetRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		data.Txs = nil
		// block.Data.IntermediateStateRoots.RawRootsList = nil
	}
	return data
}

// GetABCICommit returns a commit format defined by ABCI.
// Other fields (especially ValidatorAddress and Timestamp of Signature) have to be filled by caller.
func GetABCICommit(height uint64, hash Hash, val cmtypes.Address, time time.Time, signature Signature) *cmtypes.Commit {
	tmCommit := cmtypes.Commit{
		Height: int64(height), //nolint:gosec
		Round:  0,
		BlockID: cmtypes.BlockID{
			Hash:          cmbytes.HexBytes(hash),
			PartSetHeader: cmtypes.PartSetHeader{},
		},
		Signatures: make([]cmtypes.CommitSig, 1),
	}
	commitSig := cmtypes.CommitSig{
		BlockIDFlag:      cmtypes.BlockIDFlagCommit,
		Signature:        signature,
		ValidatorAddress: val,
		Timestamp:        time,
	}
	tmCommit.Signatures[0] = commitSig

	return &tmCommit
}
