package types

import (
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
	sh.Signature = other.Signature

	if other.Validators != nil && other.Validators.GetProposer() != nil {
		validators, err := types.ValidatorSetFromProto(other.Validators)
		if err != nil {
			return err
		}

		sh.Validators = validators
	}
	return nil
}

// MarshalBinary encodes SignedHeader into binary form and returns it.
func (sh *SignedHeader) MarshalBinary() ([]byte, error) {
	hp, err := sh.ToProto()
	if err != nil {
		return nil, err
	}
	return hp.Marshal()
}

// UnmarshalBinary decodes binary form of SignedHeader into object.
func (sh *SignedHeader) UnmarshalBinary(data []byte) error {
	var pHeader pb.SignedHeader
	err := pHeader.Unmarshal(data)
	if err != nil {
		return err
	}
	err = sh.FromProto(&pHeader)
	if err != nil {
		return err
	}
	return nil
}

// ToProto converts Header into protobuf representation and returns it.
func (h *Header) ToProto() *pb.Header {
	return &pb.Header{
		Version: &pb.Version{
			Block: h.Version.Block,
			App:   h.Version.App,
		},
		Height:          h.BaseHeader.Height,
		Time:            h.BaseHeader.Time,
		LastHeaderHash:  h.LastHeaderHash[:],
		LastCommitHash:  h.LastCommitHash[:],
		DataHash:        h.DataHash[:],
		ConsensusHash:   h.ConsensusHash[:],
		AppHash:         h.AppHash[:],
		LastResultsHash: h.LastResultsHash[:],
		ProposerAddress: h.ProposerAddress[:],
		ChainId:         h.BaseHeader.ChainID,
		ValidatorHash:   h.ValidatorHash,
	}
}

// FromProto fills Header with data from its protobuf representation.
func (h *Header) FromProto(other *pb.Header) error {
	h.Version.Block = other.Version.Block
	h.Version.App = other.Version.App
	h.BaseHeader.ChainID = other.ChainId
	h.BaseHeader.Height = other.Height
	h.BaseHeader.Time = other.Time
	h.LastHeaderHash = other.LastHeaderHash
	h.LastCommitHash = other.LastCommitHash
	h.DataHash = other.DataHash
	h.ConsensusHash = other.ConsensusHash
	h.AppHash = other.AppHash
	h.LastResultsHash = other.LastResultsHash
	h.ValidatorHash = other.ValidatorHash
	if len(other.ProposerAddress) > 0 {
		h.ProposerAddress = make([]byte, len(other.ProposerAddress))
		copy(h.ProposerAddress, other.ProposerAddress)
	}

	return nil
}

// ToProto ...
func (m *Metadata) ToProto() *pb.Metadata {
	return &pb.Metadata{
		ChainId:      m.ChainID,
		Height:       m.Height,
		Time:         m.Time,
		LastDataHash: m.LastDataHash[:],
	}
}

// FromProto ...
func (m *Metadata) FromProto(other *pb.Metadata) {
	m.ChainID = other.ChainId
	m.Height = other.Height
	m.Time = other.Time
	m.LastDataHash = other.LastDataHash
}

// ToProto converts Data into protobuf representation and returns it.
func (d *Data) ToProto() *pb.Data {
	var mProto *pb.Metadata
	if d.Metadata != nil {
		mProto = d.Metadata.ToProto()
	}
	return &pb.Data{
		Metadata: mProto,
		Txs:      txsToByteSlices(d.Txs),
		// IntermediateStateRoots: d.IntermediateStateRoots.RawRootsList,
		// Note: Temporarily remove Evidence #896
		// Evidence:               evidenceToProto(d.Evidence),
	}
}

// FromProto fills the Data with data from its protobuf representation
func (d *Data) FromProto(other *pb.Data) error {
	if other.Metadata != nil {
		if d.Metadata == nil {
			d.Metadata = &Metadata{}
		}
		d.Metadata.FromProto(other.Metadata)
	}
	d.Txs = byteSlicesToTxs(other.Txs)
	// d.IntermediateStateRoots.RawRootsList = other.IntermediateStateRoots
	// Note: Temporarily remove Evidence #896
	// d.Evidence = evidenceFromProto(other.Evidence)

	return nil
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

func txsToByteSlices(txs Txs) [][]byte {
	if txs == nil {
		return nil
	}
	bytes := make([][]byte, len(txs))
	for i := range txs {
		bytes[i] = txs[i]
	}
	return bytes
}

func byteSlicesToTxs(bytes [][]byte) Txs {
	if len(bytes) == 0 {
		return nil
	}
	txs := make(Txs, len(bytes))
	for i := range txs {
		txs[i] = bytes[i]
	}
	return txs
}

// ConsensusParamsFromProto converts protobuf consensus parameters to consensus parameters
func ConsensusParamsFromProto(pbParams cmproto.ConsensusParams) types.ConsensusParams {
	c := types.ConsensusParams{
		Block: types.BlockParams{
			MaxBytes: pbParams.Block.MaxBytes,
			MaxGas:   pbParams.Block.MaxGas,
		},
		Validator: types.ValidatorParams{
			PubKeyTypes: pbParams.Validator.PubKeyTypes,
		},
		Version: types.VersionParams{
			App: pbParams.Version.App,
		},
	}
	if pbParams.Abci != nil {
		c.ABCI.VoteExtensionsEnableHeight = pbParams.Abci.GetVoteExtensionsEnableHeight()
	}
	return c
}
