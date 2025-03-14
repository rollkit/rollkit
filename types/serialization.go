package types

import (
	"time"

	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/types"

	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// MarshalBinary encodes Metadata into binary form and returns it.
func (m *Metadata) MarshalBinary() ([]byte, error) {
	return m.ToProto().Marshal()
}

// UnmarshalBinary decodes binary form of Metadata into object.
func (m *Metadata) UnmarshalBinary(metadata []byte) error {
	var pMetadata pb.Metadata
	err := pMetadata.Unmarshal(metadata)
	if err != nil {
		return err
	}
	m.FromProto(&pMetadata)
	return nil
}

// MarshalBinary encodes Header into binary form and returns it.
func (h *Header) MarshalBinary() ([]byte, error) {
	return h.ToProto().Marshal()
}

// UnmarshalBinary decodes binary form of Header into object.
func (h *Header) UnmarshalBinary(data []byte) error {
	var pHeader pb.Header
	err := pHeader.Unmarshal(data)
	if err != nil {
		return err
	}
	err = h.FromProto(&pHeader)
	return err
}

// MarshalBinary encodes Data into binary form and returns it.
func (d *Data) MarshalBinary() ([]byte, error) {
	return d.ToProto().Marshal()
}

// UnmarshalBinary decodes binary form of Data into object.
func (d *Data) UnmarshalBinary(data []byte) error {
	var pData pb.Data
	err := pData.Unmarshal(data)
	if err != nil {
		return err
	}
	err = d.FromProto(&pData)
	return err
}

// ToProto converts SignedHeader into protobuf representation and returns it.
func (sh *SignedHeader) ToProto() (*pb.SignedHeader, error) {
	vSet, err := sh.Validators.ToProto()
	if err != nil {
		return nil, err
	}
	return &pb.SignedHeader{
		Header:     sh.Header.ToProto(),
		Signature:  sh.Signature[:],
		Validators: vSet,
	}, nil
}

// FromProto fills SignedHeader with data from protobuf representation.
func (sh *SignedHeader) FromProto(other *pb.SignedHeader) error {
	err := sh.Header.FromProto(other.Header)
	if err != nil {
		return err
	}
	copy(sh.Signature[:], other.Signature)
	vSet, err := types.ValidatorSetFromProto(other.Validators)
	if err != nil {
		return err
	}
	sh.Validators = vSet
	return nil
}

// ToProto converts Metadata into protobuf representation and returns it.
func (m *Metadata) ToProto() *pb.Metadata {
	return &pb.Metadata{
		ChainId:      m.ChainID,
		Height:       m.Height,
		Time:         m.Time,
		LastDataHash: m.LastDataHash[:],
	}
}

// FromProto fills Metadata with data from its protobuf representation.
func (m *Metadata) FromProto(other *pb.Metadata) {
	m.ChainID = other.ChainId
	m.Height = other.Height
	m.Time = other.Time
	copy(m.LastDataHash[:], other.LastDataHash)
}

// ToProto converts Header into protobuf representation and returns it.
func (h *Header) ToProto() *pb.Header {
	blockVersion := uint64(0)
	appVersion := uint64(0)
	if h.Version.Block != 0 {
		blockVersion = h.Version.Block
	}
	if h.Version.App != 0 {
		appVersion = h.Version.App
	}
	version := &pb.Version{
		Block: blockVersion,
		App:   appVersion,
	}
	return &pb.Header{
		Version:         version,
		Height:          h.Height(),
		Time:            h.BaseHeader.Time,
		LastHeaderHash:  h.LastHeaderHash[:],
		LastCommitHash:  h.LastCommitHash[:],
		DataHash:        h.DataHash[:],
		ConsensusHash:   h.ConsensusHash[:],
		AppHash:         h.AppHash[:],
		LastResultsHash: h.LastResultsHash[:],
		ProposerAddress: h.ProposerAddress,
		ValidatorHash:   h.ValidatorHash[:],
		ChainId:         h.ChainID(),
	}
}

// FromProto fills Header with data from protobuf representation.
func (h *Header) FromProto(other *pb.Header) error {
	v := Version{}
	if other.Version != nil {
		v.Block = other.Version.Block
		v.App = other.Version.App
	}
	h.Version = v
	h.BaseHeader.Height = other.Height
	h.BaseHeader.Time = other.Time
	h.BaseHeader.ChainID = other.ChainId
	copy(h.LastHeaderHash[:], other.LastHeaderHash)
	copy(h.LastCommitHash[:], other.LastCommitHash)
	copy(h.DataHash[:], other.DataHash)
	copy(h.ConsensusHash[:], other.ConsensusHash)
	copy(h.AppHash[:], other.AppHash)
	copy(h.LastResultsHash[:], other.LastResultsHash)
	h.ProposerAddress = other.ProposerAddress
	copy(h.ValidatorHash[:], other.ValidatorHash)
	return nil
}

// ToProto converts Data into protobuf representation and returns it.
func (d *Data) ToProto() *pb.Data {
	txs := make([][]byte, len(d.Txs))
	for i, tx := range d.Txs {
		txs[i] = tx
	}

	md := &pb.Metadata{}
	if d.Metadata != nil {
		md = &pb.Metadata{
			ChainId:      d.Metadata.ChainID,
			Height:       d.Metadata.Height,
			Time:         d.Metadata.Time,
			LastDataHash: d.Metadata.LastDataHash[:],
		}
	}
	return &pb.Data{
		Metadata: md,
		Txs:      txs,
	}
}

// FromProto fills Data with data from protobuf representation.
func (d *Data) FromProto(other *pb.Data) error {
	if other.Metadata != nil {
		if d.Metadata == nil {
			d.Metadata = &Metadata{}
		}
		d.Metadata.ChainID = other.Metadata.ChainId
		d.Metadata.Height = other.Metadata.Height
		d.Metadata.Time = other.Metadata.Time
		copy(d.Metadata.LastDataHash[:], other.Metadata.LastDataHash)
	}
	txs := make(Txs, len(other.Txs))
	for i, tx := range other.Txs {
		txs[i] = tx
	}
	d.Txs = txs
	return nil
}

// NewABCICommit returns a new commit in ABCI format.
func NewABCICommit(height uint64, round int32, blockID types.BlockID, commitSigs []types.CommitSig) *types.Commit {
	return &types.Commit{
		Height:     int64(height), // nolint: gosec
		Round:      round,
		BlockID:    blockID,
		Signatures: commitSigs,
	}
}

// GetCometBFTBlock converts *Types.SignedHeader and *Types.Data to *cmtTypes.Block.
func GetCometBFTBlock(blockHeader *SignedHeader, blockData *Data) (*types.Block, error) {
	// Create a tendermint TmBlock
	abciHeader, err := ToTendermintHeader(&blockHeader.Header)
	if err != nil {
		return nil, err
	}

	val := blockHeader.Validators.Validators[0].Address
	abciCommit := GetABCICommit(blockHeader.Height(), blockHeader.Hash(), val, blockHeader.Time(), blockHeader.Signature)

	if len(abciCommit.Signatures) == 1 {
		abciCommit.Signatures[0].ValidatorAddress = blockHeader.ProposerAddress
	}

	// put the validatorhash from rollkit to tendermint
	abciHeader.ValidatorsHash = blockHeader.Header.ValidatorHash
	abciHeader.NextValidatorsHash = blockHeader.Header.ValidatorHash

	// add txs to the block
	var txs = make([]types.Tx, 0)
	for _, tx := range blockData.Txs {
		txs = append(txs, types.Tx(tx))
	}
	tmblock := &types.Block{
		Header: abciHeader,
		Data: types.Data{
			Txs: txs,
		},
		Evidence:   *types.NewEvidenceData(),
		LastCommit: abciCommit,
	}

	return tmblock, nil
}

// ToTendermintHeader converts rollkit Header to tendermint Header.
func ToTendermintHeader(header *Header) (types.Header, error) {
	pHeader := header.ToProto()
	var tmHeader cmproto.Header
	tmHeader.Version.Block = int64(pHeader.Version.Block)
	tmHeader.Version.App = int64(pHeader.Version.App)

	tmHeader.ChainID = pHeader.ChainId
	tmHeader.Height = int64(pHeader.Height)

	t := time.Unix(0, int64(pHeader.Time))
	tmHeader.Time = t

	tmHeader.LastBlockId.Hash = pHeader.LastHeaderHash
	tmHeader.LastBlockId.PartSetHeader.Total = 0
	tmHeader.LastBlockId.PartSetHeader.Hash = nil

	tmHeader.LastCommitHash = pHeader.LastCommitHash
	tmHeader.DataHash = pHeader.DataHash
	tmHeader.ValidatorsHash = pHeader.ValidatorHash
	tmHeader.NextValidatorsHash = pHeader.ValidatorHash
	tmHeader.ConsensusHash = pHeader.ConsensusHash
	tmHeader.AppHash = pHeader.AppHash
	tmHeader.LastResultsHash = pHeader.LastResultsHash
	tmHeader.EvidenceHash = []byte{}
	tmHeader.ProposerAddress = pHeader.ProposerAddress

	return types.HeaderFromProto(tmHeader)
}

// ToProto converts State into protobuf representation and returns it.
func (s *State) ToProto() (*pb.State, error) {
	// nextValidators, err := s.NextValidators.ToProto()
	// if err != nil {
	// 	return nil, err
	// }
	// validators, err := s.Validators.ToProto()
	// if err != nil {
	// 	return nil, err
	// }
	// lastValidators, err := s.LastValidators.ToProto()
	// if err != nil {
	// 	return nil, err
	// }
	return &pb.State{
		Version:         &s.Version,
		ChainId:         s.ChainID,
		InitialHeight:   s.InitialHeight,
		LastBlockHeight: s.LastBlockHeight,
		LastBlockID:     s.LastBlockID.ToProto(),
		LastBlockTime:   s.LastBlockTime,
		DAHeight:        s.DAHeight,
		ConsensusParams: s.ConsensusParams,
		//LastHeightConsensusParamsChanged: s.LastHeightConsensusParamsChanged,
		LastResultsHash: s.LastResultsHash[:],
		AppHash:         s.AppHash[:],
		//NextValidators:                   nextValidators,
		//Validators:                       validators,
		//LastValidators:                   lastValidators,
		//LastHeightValidatorsChanged:      s.LastHeightValidatorsChanged,
	}, nil
}

// FromProto fills State with data from its protobuf representation.
func (s *State) FromProto(other *pb.State) error {
	var err error
	s.Version = *other.Version
	s.ChainID = other.ChainId
	s.InitialHeight = other.InitialHeight
	s.LastBlockHeight = other.LastBlockHeight

	lastBlockID, err := types.BlockIDFromProto(&other.LastBlockID)
	if err != nil {
		return err
	}
	s.LastBlockID = *lastBlockID
	s.LastBlockTime = other.LastBlockTime
	s.DAHeight = other.DAHeight

	// Unmarshal validator sets
	// if other.NextValidators != nil {
	// 	s.NextValidators, err = types.ValidatorSetFromProto(other.NextValidators)
	// 	if err != nil {
	// 		return err
	// 	}
	// }
	// if other.Validators != nil {
	// 	s.Validators, err = types.ValidatorSetFromProto(other.Validators)
	// 	if err != nil {
	// 		return err
	// 	}
	// }
	// if other.LastValidators != nil {
	// 	s.LastValidators, err = types.ValidatorSetFromProto(other.LastValidators)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	// s.LastHeightValidatorsChanged = other.LastHeightValidatorsChanged
	s.ConsensusParams = other.ConsensusParams
	// s.LastHeightConsensusParamsChanged = other.LastHeightConsensusParamsChanged
	s.LastResultsHash = other.LastResultsHash
	s.AppHash = other.AppHash

	return nil
}
