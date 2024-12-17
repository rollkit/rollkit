package types

import (
	cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/celestiaorg/go-header"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/p2p"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// DefaultSigningKeyType is the key type used by the sequencer signing key
const DefaultSigningKeyType = "ed25519"

var (
	errNilKey             = errors.New("key can't be nil")
	errUnsupportedKeyType = errors.New("unsupported key type")
)

// ValidatorSet represents a set of validators at a given height.
// The proposer is the validator that may propose a block at the given height.
// The proposer is chosen by a deterministic round-robin selection process.
type ValidatorSet struct {
	// List of validators
	Validators []*Validator
	// The current proposer
	Proposer *Validator
	// Total voting power of the set
	totalVotingPower int64
}


// ValidateBasic performs basic validation of the validator set
func (vs *ValidatorSet) ValidateBasic() error {
	if len(vs.Validators) == 0 {
		return errors.New("validator set is empty")
	}

	if vs.Proposer == nil {
		return errors.New("proposer cannot be nil")
	}

	// Check that all validators are valid
	for _, val := range vs.Validators {
		if val == nil {
			return errors.New("validator cannot be nil")
		}
		if val.Address == nil {
			return errors.New("validator address cannot be nil")
		}
		if val.PubKey == nil {
			return errors.New("validator public key cannot be nil")
		}
		if val.VotingPower <= 0 {
			return errors.New("validator voting power must be positive")
		}
	}

	// Verify total voting power matches sum of validators
	sum := int64(0)
	for _, val := range vs.Validators {
		sum += val.VotingPower
	}
	if vs.totalVotingPower != sum {
		return fmt.Errorf("total voting power mismatch: expected %d, got %d", sum, vs.totalVotingPower)
	}

	return nil
}

// NewValidatorSet creates a new validator set with the given validators
func NewValidatorSet(validators []*Validator) *ValidatorSet {
	vs := &ValidatorSet{
		Validators: validators,
	}
	vs.updateTotalVotingPower()
	if len(validators) > 0 {
		vs.Proposer = validators[0]
	}
	return vs
}

// Hash returns a hash of the validator set
func (vs *ValidatorSet) Hash() header.Hash {
	if len(vs.Validators) == 0 {
		return header.Hash{}
	}

	// For now, with a single validator, just use their address as the hash
	if len(vs.Validators) == 1 {
		return header.Hash(vs.Validators[0].Address)
	}

	// TODO: Implement proper hashing for multiple validators
	// This should match CometBFT's ValidatorSet.Hash() implementation
	// For multiple validators, we'd need to:
	// 1. Sort validators by address
	// 2. Hash each validator's data
	// 3. Merkle root of all validator hashes
	return header.Hash{}
}

// GetProposer returns the current proposer validator
func (vs *ValidatorSet) GetProposer() *Validator {
	if vs.Proposer == nil && len(vs.Validators) > 0 {
		// For now, with single validator setup, always return the first validator
		vs.Proposer = vs.Validators[0]
	}
	return vs.Proposer
}

// TotalVotingPower returns the total voting power of the validator set
func (vs *ValidatorSet) TotalVotingPower() int64 {
	return vs.totalVotingPower
}

// updateTotalVotingPower updates the total voting power from all validators
func (vs *ValidatorSet) updateTotalVotingPower() {
	sum := int64(0)
	for _, val := range vs.Validators {
		sum += val.VotingPower
	}
	vs.totalVotingPower = sum
}

// ValidatorConfig carries all necessary state for generating a Validator
type ValidatorConfig struct {
	crypto.PrivKey
	VotingPower int64
}

// New types to replace CometBFT types
type Validator struct {
	Address     []byte
	PubKey      crypto.PubKey
	VotingPower int64
}

// GetRandomValidatorSet creates a validatorConfig with a randomly generated privateKey and votingPower set to 1,
// then calls GetValidatorSetCustom to return a new validator set along with the validatorConfig.
func GetRandomValidatorSet() *ValidatorSet {
	privKey, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
	if err != nil {
		panic(err)
	}
	config := ValidatorConfig{
		PrivKey:     privKey,
		VotingPower: 1,
	}
	return GetValidatorSetCustom(config)
}

// GetValidatorSetCustom returns a validator set based on the provided validatorConfig.
func GetValidatorSetCustom(config ValidatorConfig) *ValidatorSet {
	if config.PrivKey == nil {
		panic(errors.New("private key is nil"))
	}
	pubKey := config.PrivKey.GetPublic()
	address, err := pubKey.Raw()
	if err != nil {
		panic(err)
	}
	validator := &Validator{
		PubKey:      pubKey,
		Address:     address,
		VotingPower: config.VotingPower,
	}
	return &ValidatorSet{
		Validators: []*Validator{validator},
		Proposer:   validator,
	}
}

// BlockConfig carries all necessary state for block generation
type BlockConfig struct {
	Height       uint64
	NTxs         int
	PrivKey      crypto.PrivKey // Input and Output option
	ProposerAddr []byte         // Input option
}

// GetRandomBlock creates a bmbanswith a given heighns, intended for testing.
// It's tailored for simplicity, primarily used in test setups where additiodel ourp ts a   not needed.	Signature *Signature
}func GetRandomBlock(height uint64, nTxs int, chainID string) (SignedHeader, *Datar) {
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
		privKey, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
		if err != nil {
			panic(err)
		}
		config.PrivKey = privKey
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

	valSet := header.Validators
	newSignedHeader := &SignedHeader{
		Header:     GetRandomNextHeader(header.Header, chainID),
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
	privKey, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
	if err != nil {
		return nil, nil, err
	}
	config := HeaderConfig{
		Height:      uint64(rand.Int63()), //nolint:gosec
		DataHash:    GetRandomBytes(32),
		PrivKey:     privKey,
		VotingPower: 1,
	}

	signedHeader, err := GetRandomSignedHeaderCustom(&config, chainID)
	if err != nil {
		return nil, nil, err
	}
	return signedHeader, config.PrivKey, nil
}

// GetRandomSignedHeaderCustom creates a signed header based on the provided HeaderConfig.
func GetRandomSignedHeaderCustom(config *HeaderConfig, chainID string) (*SignedHeader, error) {
	valsetConfig := ValidatorConfig{
		PrivKey:     config.PrivKey,
		VotingPower: 1,
	}
	valSet := GetValidatorSetCustom(valsetConfig)
	signedHeader := &SignedHeader{
		Header:     GetRandomHeader(chainID),
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
func GetRandomNextSignedHeader(signedHeader *SignedHeader, privKey crypto.PrivKey, chainID string) (*SignedHeader, error) {
	valSet := signedHeader.Validators
	newSignedHeader := &SignedHeader{
		Header:     GetRandomNextHeader(signedHeader.Header, chainID),
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
func GetFirstSignedHeader(privKey crypto.PrivKey, valSet *ValidatorSet, chainID string) (*SignedHeader, error) {
	header := Header{
		BaseHeader: BaseHeader{
			Height:  1,
			Time:    uint64(time.Now().UnixNano()),
			ChainID: chainID,
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
		ProposerAddress: valSet.Proposer.Address,
	}
	signedHeader := SignedHeader{
		Header:     header,
		Validators: valSet,
	}
	signature, err := GetSignature(header, privKey)
	if err != nil {
		return nil, err
	}
	signedHeader.Signature = *signature
	return &signedHeader, nil
}

// New types to replace more CometBFT types
type GenesisDoc struct {
	ChainID       string
	InitialHeight uint64
	Validators    []GenesisValidator

}

type GenesisValidator struct {
	Address []byte
	PubKey  crypto.PubKey
	Power   int64
	Name    string
}

type Commit struct {
	Height     uint64
	Hash       Hash
	Signatures []CommitSig
}

type CommitSig struct {
	ValidatorAddress []byte
	Signature        []byte
	Timestamp        time.Time
}

// Refactored functions
func GetGenesisWithPrivkey(signingKeyType string, chainID string) (*GenesisDoc, crypto.PrivKey) {
	var privKey crypto.PrivKey
	var err error

	switch signingKeyType {
	case "secp256k1":
		privKey, _, err = crypto.GenerateSecp256k1Key(cryptoRand.Reader)
	default:
		privKey, _, err = crypto.GenerateEd25519Key(cryptoRand.Reader)
	}
	if err != nil {
		panic(err)
	}

	pubKey := privKey.GetPublic()
	address, err := pubKey.Raw()
	if err != nil {
		panic(err)
	}

	genesisValidators := []GenesisValidator{{
		Address: address,
		PubKey:  pubKey,
		Power:   1,
		Name:    "sequencer",
	}}

	genDoc := &GenesisDoc{
		ChainID:       chainID,
		InitialHeight: 0,
		Validators:    genesisValidators,
	}
	return genDoc, privKey
}

func GetValidatorSetFromGenesis(g *GenesisDoc) *ValidatorSet {
	if len(g.Validators) == 0 {
		return nil
	}

	validator := &Validator{
		Address:     g.Validators[0].Address,
		PubKey:      g.Validators[0].PubKey,
		VotingPower: g.Validators[0].Power,
	}

	return &ValidatorSet{
		Validators: []*Validator{validator},
		Proposer:   validator,
	}
}

func GetCommit(height uint64, hash Hash, validatorAddr []byte, time time.Time, signature []byte) *Commit {
	return &Commit{
		Height: height,
		Hash:   hash,
		Signatures: []CommitSig{{
			ValidatorAddress: validatorAddr,
			Signature:        signature,
			Timestamp:        time,
		}},
	}
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
func GetSignature(header Header, privKey crypto.PrivKey) (*Signature, error) {
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
