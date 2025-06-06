package types

import (
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/celestiaorg/go-header"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/pkg/signer/noop"
)

// DefaultSigningKeyType is the key type used by the sequencer signing key
const DefaultSigningKeyType = "ed25519"

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

// GenerateRandomBlockCustomWithAppHash returns a block with random data and the given height, transactions, privateKey, proposer address, and custom appHash.
func GenerateRandomBlockCustomWithAppHash(config *BlockConfig, chainID string, appHash []byte) (*SignedHeader, *Data, crypto.PrivKey) {
	data := getBlockDataWith(config.NTxs)
	dataHash := data.DACommitment()

	if config.PrivKey == nil {
		pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
		if err != nil {
			panic(err)
		}
		config.PrivKey = pk
	}

	noopSigner, err := noop.NewNoopSigner(config.PrivKey)
	if err != nil {
		panic(err)
	}

	headerConfig := HeaderConfig{
		Height:   config.Height,
		DataHash: dataHash,
		AppHash:  appHash,
		Signer:   noopSigner,
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

// GenerateRandomBlockCustom returns a block with random data and the given height, transactions, privateKey and proposer address.
func GenerateRandomBlockCustom(config *BlockConfig, chainID string) (*SignedHeader, *Data, crypto.PrivKey) {
	// Use random bytes for appHash
	appHash := GetRandomBytes(32)
	return GenerateRandomBlockCustomWithAppHash(config, chainID, appHash)
}

// HeaderConfig carries all necessary state for header generation
type HeaderConfig struct {
	Height   uint64
	DataHash header.Hash
	AppHash  header.Hash
	Signer   signer.Signer
}

// GetRandomHeader returns a header with random fields and current time
func GetRandomHeader(chainID string, appHash []byte) Header {
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
		AppHash:         appHash,
		LastResultsHash: GetRandomBytes(32),
		ProposerAddress: GetRandomBytes(32),
		ValidatorHash:   GetRandomBytes(32),
	}
}

// GetRandomNextHeader returns a header with random data and height of +1 from
// the provided Header
func GetRandomNextHeader(header Header, chainID string) Header {
	nextHeader := GetRandomHeader(chainID, GetRandomBytes(32))
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

	noopSigner, err := noop.NewNoopSigner(pk)
	if err != nil {
		return nil, nil, err
	}
	config := HeaderConfig{
		Height:   uint64(rand.Int63()), //nolint:gosec
		DataHash: GetRandomBytes(32),
		AppHash:  GetRandomBytes(32),
		Signer:   noopSigner,
	}

	signedHeader, err := GetRandomSignedHeaderCustom(&config, chainID)
	if err != nil {
		return nil, nil, err
	}
	return signedHeader, pk, nil
}

// GetRandomSignedHeaderCustom creates a signed header based on the provided HeaderConfig.
func GetRandomSignedHeaderCustom(config *HeaderConfig, chainID string) (*SignedHeader, error) {
	pk, err := config.Signer.GetPublic()
	if err != nil {
		return nil, err
	}
	signer, err := NewSigner(pk)
	if err != nil {
		return nil, err
	}

	signedHeader := &SignedHeader{
		Header: GetRandomHeader(chainID, config.AppHash),
		Signer: signer,
	}
	signedHeader.BaseHeader.Height = config.Height
	signedHeader.DataHash = config.DataHash
	signedHeader.ProposerAddress = signer.Address
	signedHeader.ValidatorHash = signer.Address
	signedHeader.BaseHeader.Time = uint64(time.Now().UnixNano()) + (config.Height)*10

	b, err := signedHeader.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	signature, err := config.Signer.Sign(b)
	if err != nil {
		return nil, err
	}
	signedHeader.Signature = signature
	return signedHeader, nil
}

// GetRandomNextSignedHeader returns a signed header with random data and height of +1 from
// the provided signed header
func GetRandomNextSignedHeader(signedHeader *SignedHeader, signer signer.Signer, chainID string) (*SignedHeader, error) {
	newSignedHeader := &SignedHeader{
		Header: GetRandomNextHeader(signedHeader.Header, chainID),
		Signer: signedHeader.Signer,
	}

	signature, err := GetSignature(signedHeader.Header, signer)
	if err != nil {
		return nil, err
	}
	newSignedHeader.Signature = signature
	return newSignedHeader, nil
}

// GetFirstSignedHeader creates a 1st signed header for a chain, given a valset and signing key.
func GetFirstSignedHeader(signer signer.Signer, chainID string) (*SignedHeader, error) {
	pk, err := signer.GetPublic()
	if err != nil {
		return nil, err
	}
	sig, err := NewSigner(pk)
	if err != nil {
		return nil, err
	}

	addr, err := signer.GetAddress()
	if err != nil {
		return nil, err
	}
	header := Header{
		BaseHeader: BaseHeader{
			Height:  1,
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
		AppHash:         make([]byte, 32),
		ProposerAddress: addr,
	}
	signedHeader := SignedHeader{
		Header: header,
		Signer: sig,
	}

	signature, err := GetSignature(header, signer)
	if err != nil {
		return nil, err
	}

	signedHeader.Signature = signature
	return &signedHeader, nil
}

// GetGenesisWithPrivkey returns a genesis state and a private key
func GetGenesisWithPrivkey(chainID string) (genesis.Genesis, crypto.PrivKey, crypto.PubKey) {
	privKey, pubKey, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		panic(err)
	}

	signer, err := NewSigner(privKey.GetPublic())
	if err != nil {
		panic(err)
	}

	return genesis.NewGenesis(
		chainID,
		1,
		time.Now().UTC(),
		signer.Address,
	), privKey, pubKey
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
func GetSignature(header Header, signer signer.Signer) (Signature, error) {
	b, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return signer.Sign(b)
}

func getBlockDataWith(nTxs int) *Data {
	data := &Data{
		Txs: make(Txs, nTxs),
	}

	for i := range nTxs {
		data.Txs[i] = GetRandomTx()
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		data.Txs = nil
	}
	return data
}

// CreateDefaultSignaturePayloadProvider creates a basic signature payload provider
// that returns a hash of the header for signing
func CreateDefaultSignaturePayloadProvider() SignaturePayloadProvider {
	return func(header *Header, data *Data) ([]byte, error) {
		if header == nil {
			return nil, errors.New("header cannot be nil")
		}

		// Create a simple payload by hashing the header bytes
		headerBytes, err := header.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal header: %w", err)
		}

		hash := sha256.Sum256(headerBytes)
		return hash[:], nil
	}
}

// createDefaultHeaderHasher creates a basic header hasher that returns a hash of the header
func createDefaultHeaderHasher() HeaderHasher {
	return func(header *Header) (Hash, error) {
		if header == nil {
			return nil, errors.New("header cannot be nil")
		}

		// Create a hash of the header bytes
		headerBytes, err := header.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal header: %w", err)
		}

		hash := sha256.Sum256(headerBytes)
		return hash[:], nil
	}
}

// createDefaultCommitHashProvider creates a basic commit hash provider
func CreateDefaultCommitHashProvider() CommitHashProvider {
	return func(signature *Signature, header *Header, proposerAddress []byte) (Hash, error) {
		if header == nil {
			return nil, errors.New("header cannot be nil")
		}

		// Create a simple commit hash from header and signature
		headerBytes, err := header.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal header: %w", err)
		}

		// Combine header bytes with signature if available
		var commitData []byte
		commitData = append(commitData, headerBytes...)
		if signature != nil {
			commitData = append(commitData, *signature...)
		}

		hash := sha256.Sum256(commitData)
		return hash[:], nil
	}
}

func CreateDefaultValidatorHasher() ValidatorHasher {
	return func(proposerAddress []byte, pubKey crypto.PubKey) (Hash, error) {
		hash := sha256.Sum256(proposerAddress)
		return hash[:], nil
	}
}
