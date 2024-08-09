package types

import (
	cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/celestiaorg/go-header"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/crypto/secp256k1"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	"github.com/cometbft/cometbft/p2p"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// TestChainID is a constant used for testing purposes. It represents a mock chain ID.
const TestChainID = "test"

// DefaultSigningKeyType is the key type used by the sequencer signing key
const DefaultSigningKeyType = "ed25519"

var (
	errNilKey             = errors.New("key can't be nil")
	errUnsupportedKeyType = errors.New("unsupported key type")
)

// ValidatorConfig carries all necessary state for generating a Validator
type ValidatorConfig struct {
	PrivKey     cmcrypto.PrivKey
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
	PrivKey      cmcrypto.PrivKey // Input and Output option
	ProposerAddr []byte           // Input option
}

// GetRandomBlock creates a block with a given height and number of transactions, intended for testing.
// It's tailored for simplicity, primarily used in test setups where additional outputs are not needed.
func GetRandomBlock(height uint64, nTxs int) (*SignedHeader, *Data) {
	config := BlockConfig{
		Height: height,
		NTxs:   nTxs,
	}
	// Assuming GenerateBlock modifies the context directly with the generated block and other needed data.
	header, data, _ := GenerateRandomBlockCustom(&config)

	return header, data
}

// GenerateRandomBlockCustom returns a block with random data and the given height, transactions, privateKey and proposer address.
func GenerateRandomBlockCustom(config *BlockConfig) (*SignedHeader, *Data, cmcrypto.PrivKey) {
	data := getBlockDataWith(config.NTxs)
	dataHash := data.Hash()

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

	if config.ProposerAddr != nil {
		signedHeader.ProposerAddress = config.ProposerAddr
	}

	data.Metadata = &Metadata{
		ChainID:      TestChainID,
		Height:       signedHeader.Height(),
		LastDataHash: header.Hash{},
		Time:         uint64(signedHeader.Time().UnixNano()),
	}

	return signedHeader, data, config.PrivKey
}

// GetRandomNextBlock returns a block with random data and height of +1 from the provided block
func GetRandomNextBlock(header *SignedHeader, data *Data, privKey cmcrypto.PrivKey, appHash header.Hash, nTxs int) (*SignedHeader, *Data) {
	nextData := getBlockDataWith(nTxs)
	dataHash := nextData.Hash()

	valSet := header.Validators
	newSignedHeader := &SignedHeader{
		Header:     GetRandomNextHeader(header.Header),
		Validators: valSet,
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
	return newSignedHeader, nextData
}

// HeaderConfig carries all necessary state for header generation
type HeaderConfig struct {
	Height      uint64
	DataHash    header.Hash
	PrivKey     cmcrypto.PrivKey
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
func GetRandomSignedHeader() (*SignedHeader, cmcrypto.PrivKey, error) {
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

	signature, err := GetSignature(signedHeader.Header, config.PrivKey)
	if err != nil {
		return nil, err
	}
	signedHeader.Signature = *signature
	return signedHeader, nil
}

// GetRandomNextSignedHeader returns a signed header with random data and height of +1 from
// the provided signed header
func GetRandomNextSignedHeader(signedHeader *SignedHeader, privKey cmcrypto.PrivKey) (*SignedHeader, error) {
	valSet := signedHeader.Validators
	newSignedHeader := &SignedHeader{
		Header:     GetRandomNextHeader(signedHeader.Header),
		Validators: valSet,
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
	case "secp256k1":
		privKey, err := crypto.UnmarshalSecp256k1PrivateKey(nodeKey.PrivKey.Bytes())
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
	signature, err := GetSignature(header, privkey)
	if err != nil {
		return nil, err
	}
	signedHeader.Signature = *signature
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
func GetGenesisWithPrivkey(sigingKeyType string) (*cmtypes.GenesisDoc, cmcrypto.PrivKey) {
	var genesisValidatorKey cmcrypto.PrivKey
	switch sigingKeyType {
	case "secp256k1":
		genesisValidatorKey = secp256k1.GenPrivKey()
	default:
		genesisValidatorKey = ed25519.GenPrivKey()
	}
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
func PrivKeyToSigningKey(privKey cmcrypto.PrivKey) (crypto.PrivKey, error) {
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

// GetSignature returns a signature from the given private key over the given header
func GetSignature(header Header, privKey cmcrypto.PrivKey) (*Signature, error) {
	consensusVote := header.MakeCometBFTVote()
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
