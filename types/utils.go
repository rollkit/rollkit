package types

import (
	cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/celestiaorg/go-header"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/p2p"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// TestChainID is a constant used for testing purposes. It represents a mock chain ID.
const TestChainID = "test"

var (
	errNilKey             = errors.New("key can't be nil")
	errUnsupportedKeyType = errors.New("unsupported key type")
)

// ValidatorConfig carries all necessary state for generating a Validator
type ValidatorConfig struct {
	PrivKey     ed25519.PrivKey
	VotingPower int64
}

// GetRandomValidatorSet creates a validatorConfig with a randomly generated privateKey and votingPower set to 1,
// then calls GetValidatorSetCustom to return a new validator set along with the validatorConfig.
func GetRandomValidatorSet() *cmtypes.ValidatorSet {
	config := ValidatorConfig{
		PrivKey:     ed25519.GenPrivKey(),
		VotingPower: 1,
	}
	valset := GetValidatorSetCustom(config)
	return valset
}

// GetValidatorSetCustom returns a validator set based on the provided validatorConfig.
func GetValidatorSetCustom(config ValidatorConfig) *cmtypes.ValidatorSet {
	if config.PrivKey == nil {
		panic(errors.New("private key is nil"))
	}
	pubKey := config.PrivKey.PubKey()
	valset := cmtypes.NewValidatorSet(
		[]*cmtypes.Validator{
			{PubKey: pubKey, Address: pubKey.Address(), VotingPower: config.VotingPower},
		},
	)
	return valset
}

// BlockConfig carries all necessary state for block generation
type BlockConfig struct {
	Height       uint64
	NTxs         int
	PrivKey      ed25519.PrivKey // Input and Output option
	ProposerAddr []byte          // Input option
}

// GetRandomBlock creates a block with a given height and number of transactions, intended for testing.
// It's tailored for simplicity, primarily used in test setups where additional outputs are not needed.
func GetRandomBlock(height uint64, nTxs int) *Block {
	config := BlockConfig{
		Height: height,
		NTxs:   nTxs,
	}
	// Assuming GenerateBlock modifies the context directly with the generated block and other needed data.
	block, _ := GenerateRandomBlockCustom(&config)

	return block
}

// GenerateRandomBlockCustom returns a block with random data and the given height, transactions, privateKey and proposer address.
func GenerateRandomBlockCustom(config *BlockConfig) (*Block, ed25519.PrivKey) {
	block := getBlockDataWith(config.NTxs)
	dataHash, err := block.Data.Hash()
	if err != nil {
		panic(err)
	}

	if config.PrivKey == nil {
		config.PrivKey = ed25519.GenPrivKey()
	}

	headerConfig := HeaderConfig{
		Height:   config.Height,
		DataHash: dataHash,
		PrivKey:  config.PrivKey,
	}

	signedHeader, err := GetRandomSignedHeaderCustom(&headerConfig)
	if err != nil {
		panic(err)
	}

	block.SignedHeader = *signedHeader
	if config.ProposerAddr != nil {
		block.SignedHeader.ProposerAddress = config.ProposerAddr
	}

	return block, config.PrivKey
}

// GetRandomNextBlock returns a block with random data and height of +1 from the provided block
func GetRandomNextBlock(block *Block, privKey ed25519.PrivKey, appHash header.Hash, nTxs int) *Block {
	nextBlock := getBlockDataWith(nTxs)
	dataHash, err := nextBlock.Data.Hash()
	if err != nil {
		panic(err)
	}
	nextBlock.SignedHeader.Header.ProposerAddress = block.SignedHeader.Header.ProposerAddress
	nextBlock.SignedHeader.Header.AppHash = appHash

	valSet := block.SignedHeader.Validators
	newSignedHeader := &SignedHeader{
		Header:     GetRandomNextHeader(block.SignedHeader.Header),
		Validators: valSet,
	}
	newSignedHeader.LastResultsHash = nil
	newSignedHeader.Header.DataHash = dataHash
	newSignedHeader.AppHash = appHash
	newSignedHeader.LastCommitHash = block.SignedHeader.Commit.GetCommitHash(
		&newSignedHeader.Header, block.SignedHeader.ProposerAddress,
	)
	commit, err := GetCommit(newSignedHeader.Header, privKey)
	if err != nil {
		panic(err)
	}
	newSignedHeader.Commit = *commit
	nextBlock.SignedHeader = *newSignedHeader
	return nextBlock
}

// HeaderConfig carries all necessary state for header generation
type HeaderConfig struct {
	Height      uint64
	DataHash    header.Hash
	PrivKey     ed25519.PrivKey
	VotingPower int64
}

// GetRandomHeader returns a header with random fields and current time
func GetRandomHeader() Header {
	return Header{
		BaseHeader: BaseHeader{
			Height:  uint64(rand.Int63()), //nolint:gosec,
			Time:    uint64(time.Now().UnixNano()),
			ChainID: TestChainID,
		},
		Version: Version{
			Block: InitStateVersion.Consensus.Block,
			App:   InitStateVersion.Consensus.App,
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
func GetRandomNextHeader(header Header) Header {
	nextHeader := GetRandomHeader()
	nextHeader.BaseHeader.Height = header.Height() + 1
	nextHeader.BaseHeader.Time = uint64(time.Now().Add(1 * time.Second).UnixNano())
	nextHeader.LastHeaderHash = header.Hash()
	nextHeader.ProposerAddress = header.ProposerAddress
	nextHeader.ValidatorHash = header.ValidatorHash
	return nextHeader
}

// GetRandomSignedHeader generates a signed header with random data and returns it.
func GetRandomSignedHeader() (*SignedHeader, ed25519.PrivKey, error) {
	config := HeaderConfig{
		Height:      uint64(rand.Int63()), //nolint:gosec
		DataHash:    GetRandomBytes(32),
		PrivKey:     ed25519.GenPrivKey(),
		VotingPower: 1,
	}

	signedHeader, err := GetRandomSignedHeaderCustom(&config)
	if err != nil {
		return nil, ed25519.PrivKey{}, err
	}
	return signedHeader, config.PrivKey, nil
}

// GetRandomSignedHeaderCustom creates a signed header based on the provided HeaderConfig.
func GetRandomSignedHeaderCustom(config *HeaderConfig) (*SignedHeader, error) {
	valsetConfig := ValidatorConfig{
		PrivKey:     config.PrivKey,
		VotingPower: 1,
	}
	valSet := GetValidatorSetCustom(valsetConfig)
	signedHeader := &SignedHeader{
		Header:     GetRandomHeader(),
		Validators: valSet,
	}
	signedHeader.Header.BaseHeader.Height = config.Height
	signedHeader.Header.DataHash = config.DataHash
	signedHeader.Header.ProposerAddress = valSet.Proposer.Address
	signedHeader.Header.ValidatorHash = valSet.Hash()
	signedHeader.Header.BaseHeader.Time = uint64(time.Now().UnixNano()) + (config.Height)*10

	commit, err := GetCommit(signedHeader.Header, config.PrivKey)
	if err != nil {
		return nil, err
	}
	signedHeader.Commit = *commit
	return signedHeader, nil
}

// GetRandomNextSignedHeader returns a signed header with random data and height of +1 from
// the provided signed header
func GetRandomNextSignedHeader(signedHeader *SignedHeader, privKey ed25519.PrivKey) (*SignedHeader, error) {
	valSet := signedHeader.Validators
	newSignedHeader := &SignedHeader{
		Header:     GetRandomNextHeader(signedHeader.Header),
		Validators: valSet,
	}
	newSignedHeader.LastCommitHash = signedHeader.Commit.GetCommitHash(
		&newSignedHeader.Header, signedHeader.ProposerAddress,
	)
	commit, err := GetCommit(newSignedHeader.Header, privKey)
	if err != nil {
		return nil, err
	}
	newSignedHeader.Commit = *commit
	return newSignedHeader, nil
}

// GetNodeKey creates libp2p private key from Tendermints NodeKey.
func GetNodeKey(nodeKey *p2p.NodeKey) (crypto.PrivKey, error) {
	if nodeKey == nil || nodeKey.PrivKey == nil {
		return nil, errNilKey
	}
	switch nodeKey.PrivKey.Type() {
	case "ed25519":
		privKey, err := crypto.UnmarshalEd25519PrivateKey(nodeKey.PrivKey.Bytes())
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling node private key: %w", err)
		}
		return privKey, nil
	default:
		return nil, errUnsupportedKeyType
	}
}

// GetFirstSignedHeader creates a 1st signed header for a chain, given a valset and signing key.
func GetFirstSignedHeader(privkey ed25519.PrivKey, valSet *cmtypes.ValidatorSet) (*SignedHeader, error) {
	header := Header{
		BaseHeader: BaseHeader{
			Height:  1, //nolint:gosec,
			Time:    uint64(time.Now().UnixNano()),
			ChainID: TestChainID,
		},
		Version: Version{
			Block: InitStateVersion.Consensus.Block,
			App:   InitStateVersion.Consensus.App,
		},
		LastHeaderHash:  GetRandomBytes(32),
		LastCommitHash:  GetRandomBytes(32),
		DataHash:        GetRandomBytes(32),
		ConsensusHash:   GetRandomBytes(32),
		AppHash:         make([]byte, 32),
		LastResultsHash: GetRandomBytes(32),
		ValidatorHash:   valSet.Hash(),
		ProposerAddress: valSet.Proposer.Address.Bytes(),
	}
	signedHeader := SignedHeader{
		Header:     header,
		Validators: valSet,
	}
	commit, err := GetCommit(header, privkey)
	signedHeader.Commit = *commit
	if err != nil {
		return nil, err
	}
	return &signedHeader, nil
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
func GetGenesisWithPrivkey() (*cmtypes.GenesisDoc, ed25519.PrivKey) {
	genesisValidatorKey := ed25519.GenPrivKey()
	pubKey := genesisValidatorKey.PubKey()

	genesisValidators := []cmtypes.GenesisValidator{{
		Address: pubKey.Address(),
		PubKey:  pubKey,
		Power:   int64(1),
		Name:    "sequencer",
	}}
	genDoc := &cmtypes.GenesisDoc{
		ChainID:       TestChainID,
		InitialHeight: 0,
		Validators:    genesisValidators,
	}
	return genDoc, genesisValidatorKey
}

// PrivKeyToSigningKey converts a privKey to a signing key
func PrivKeyToSigningKey(privKey ed25519.PrivKey) (crypto.PrivKey, error) {
	nodeKey := &p2p.NodeKey{
		PrivKey: privKey,
	}
	signingKey, err := GetNodeKey(nodeKey)
	return signingKey, err
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

// GetCommit returns a commit with a signature from the given private key over the given header
func GetCommit(header Header, privKey ed25519.PrivKey) (*Commit, error) {
	consensusVote := header.MakeCometBFTVote()
	sign, err := privKey.Sign(consensusVote)
	if err != nil {
		return nil, err
	}
	return &Commit{
		Signatures: []Signature{sign},
	}, nil
}

func getBlockDataWith(nTxs int) *Block {
	block := &Block{
		Data: Data{
			Txs: make(Txs, nTxs),
			// IntermediateStateRoots: IntermediateStateRoots{
			// 	RawRootsList: make([][]byte, nTxs),
			// },
		},
	}

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = GetRandomTx()
		// block.Data.IntermediateStateRoots.RawRootsList[i] = GetRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		// block.Data.IntermediateStateRoots.RawRootsList = nil
	}
	return block
}
