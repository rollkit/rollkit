package types

import (
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
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
	var vSet *cmproto.ValidatorSet
	var err error
	if sh.Validators != nil {
		vSet, err = sh.Validators.ToProto()
		if err != nil {
			return nil, err
		}
	}

	var sig []byte
	if sh.Signature != nil {
		sig = sh.Signature[:]
	}

	return &pb.SignedHeader{
		Header:     sh.Header.ToProto(),
		Signature:  sig,
		Validators: vSet,
	}, nil
}

// FromProto fills SignedHeader with data from protobuf representation.
func (sh *SignedHeader) FromProto(other *pb.SignedHeader) error {
	err := sh.Header.FromProto(other.Header)
	if err != nil {
		return err
	}

	if len(other.Signature) > 0 {
		sh.Signature = make(Signature, len(other.Signature))
		copy(sh.Signature, other.Signature)
	}

	if other.Validators != nil {
		vSet, err := types.ValidatorSetFromProto(other.Validators)
		if err != nil {
			return err
		}
		sh.Validators = vSet
	}
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

	if len(other.LastHeaderHash) > 0 {
		h.LastHeaderHash = make(Hash, 32)
		copy(h.LastHeaderHash[:], other.LastHeaderHash)
	}
	if len(other.LastCommitHash) > 0 {
		h.LastCommitHash = make(Hash, 32)
		copy(h.LastCommitHash[:], other.LastCommitHash)
	}
	if len(other.DataHash) > 0 {
		h.DataHash = make(Hash, 32)
		copy(h.DataHash[:], other.DataHash)
	}
	if len(other.ConsensusHash) > 0 {
		h.ConsensusHash = make(Hash, 32)
		copy(h.ConsensusHash[:], other.ConsensusHash)
	}
	if len(other.AppHash) > 0 {
		h.AppHash = make(Hash, 32)
		copy(h.AppHash[:], other.AppHash)
	}
	if len(other.LastResultsHash) > 0 {
		h.LastResultsHash = make(Hash, 32)
		copy(h.LastResultsHash[:], other.LastResultsHash)
	}
	if len(other.ValidatorHash) > 0 {
		h.ValidatorHash = make(Hash, 32)
		copy(h.ValidatorHash[:], other.ValidatorHash)
	}

	h.ProposerAddress = other.ProposerAddress
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
	if len(other.Txs) > 0 {
		txs := make(Txs, len(other.Txs))
		for i, tx := range other.Txs {
			txs[i] = tx
		}
		d.Txs = txs
	} else {
		d.Txs = nil
	}
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
		Evidence:   types.EvidenceData{}, // Create empty evidence data directly
		LastCommit: abciCommit,
	}

	return tmblock, nil
}

// ToTendermintHeader converts rollkit Header to tendermint Header.
func ToTendermintHeader(header *Header) (types.Header, error) {
	// Create a tendermint header directly
	return types.Header{
		Version: cmversion.Consensus{
			Block: 0, // Use default values as we're hitting type conversion issues
			App:   0, // Use default values as we're hitting type conversion issues
		},
		ChainID: header.ChainID(),
		Height:  int64(header.Height()),
		Time:    header.Time(),
		LastBlockID: types.BlockID{
			Hash: cmbytes.HexBytes(header.LastHeaderHash[:]),
			PartSetHeader: types.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     cmbytes.HexBytes(header.LastCommitHash[:]),
		DataHash:           cmbytes.HexBytes(header.DataHash[:]),
		ValidatorsHash:     cmbytes.HexBytes(header.ValidatorHash[:]),
		NextValidatorsHash: cmbytes.HexBytes(header.ValidatorHash[:]),
		ConsensusHash:      cmbytes.HexBytes(header.ConsensusHash[:]),
		AppHash:            cmbytes.HexBytes(header.AppHash[:]),
		LastResultsHash:    cmbytes.HexBytes(header.LastResultsHash[:]),
		EvidenceHash:       cmbytes.HexBytes{},
		ProposerAddress:    header.ProposerAddress,
	}, nil
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

// txsToByteSlices converts Txs to a slice of byte slices
func txsToByteSlices(txs Txs) [][]byte {
	if txs == nil {
		return nil
	}
	byteSlices := make([][]byte, len(txs))
	for i, tx := range txs {
		byteSlices[i] = tx
	}
	return byteSlices
}

// byteSlicesToTxs converts a slice of byte slices to Txs
func byteSlicesToTxs(byteSlices [][]byte) Txs {
	if byteSlices == nil {
		return nil
	}
	txs := make(Txs, len(byteSlices))
	for i, byteSlice := range byteSlices {
		txs[i] = byteSlice
	}
	return txs
}

// ConsensusParamsFromProto converts protobuf consensus parameters to native type
func ConsensusParamsFromProto(pbParams cmproto.ConsensusParams) types.ConsensusParams {
	params := types.ConsensusParams{}

	// Handle nil fields safely
	if pbParams.Block != nil {
		params.Block = types.BlockParams{
			MaxBytes: pbParams.Block.MaxBytes,
			MaxGas:   pbParams.Block.MaxGas,
		}
	}

	if pbParams.Evidence != nil {
		params.Evidence = types.EvidenceParams{
			MaxAgeNumBlocks: pbParams.Evidence.MaxAgeNumBlocks,
			MaxAgeDuration:  pbParams.Evidence.MaxAgeDuration,
			MaxBytes:        pbParams.Evidence.MaxBytes,
		}
	}

	if pbParams.Validator != nil {
		params.Validator = types.ValidatorParams{
			PubKeyTypes: pbParams.Validator.PubKeyTypes,
		}
	}

	if pbParams.Version != nil {
		params.Version = types.VersionParams{
			App: pbParams.Version.App,
		}
	}

	return params
}

// MarshalBinary encodes SignedHeader into binary form and returns it.
func (sh *SignedHeader) MarshalBinary() ([]byte, error) {
	protoSH, err := sh.ToProto()
	if err != nil {
		return nil, err
	}
	return protoSH.Marshal()
}

// UnmarshalBinary decodes binary form of SignedHeader into object.
func (sh *SignedHeader) UnmarshalBinary(data []byte) error {
	var pSignedHeader pb.SignedHeader
	err := pSignedHeader.Unmarshal(data)
	if err != nil {
		return err
	}
	err = sh.FromProto(&pSignedHeader)
	return err
}
