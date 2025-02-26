package types

import (
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/types"
	"google.golang.org/protobuf/proto"

	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// MarshalBinary encodes Metadata into binary form and returns it.
func (m *Metadata) MarshalBinary() ([]byte, error) {
	return proto.Marshal(m.ToProto())
}

// UnmarshalBinary decodes binary form of Metadata into object.
func (m *Metadata) UnmarshalBinary(metadata []byte) error {
	var pMetadata pb.Metadata
	err := proto.Unmarshal(metadata, &pMetadata)
	if err != nil {
		return err
	}
	m.FromProto(&pMetadata)
	return nil
}

// MarshalBinary encodes Header into binary form and returns it.
func (h *Header) MarshalBinary() ([]byte, error) {
	return proto.Marshal(h.ToProto())
}

// UnmarshalBinary decodes binary form of Header into object.
func (h *Header) UnmarshalBinary(data []byte) error {
	var pHeader pb.Header
	err := proto.Unmarshal(data, &pHeader)
	if err != nil {
		return err
	}
	err = h.FromProto(&pHeader)
	return err
}

// MarshalBinary encodes Data into binary form and returns it.
func (d *Data) MarshalBinary() ([]byte, error) {
	return proto.Marshal(d.ToProto())
}

// UnmarshalBinary decodes binary form of Data into object.
func (d *Data) UnmarshalBinary(data []byte) error {
	var pData pb.Data
	err := proto.Unmarshal(data, &pData)
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
	shp := &pb.SignedHeader{}
	shp.SetHeader(sh.Header.ToProto())
	shp.SetSignature(sh.Signature)
	shp.SetValidators(vSet)
	return shp, nil
}

// FromProto fills SignedHeader with data from protobuf representation.
func (sh *SignedHeader) FromProto(other *pb.SignedHeader) error {
	err := sh.Header.FromProto(other.GetHeader())
	if err != nil {
		return err
	}
	sh.Signature = other.GetSignature()

	if other.GetValidators() != nil && other.GetValidators().GetProposer() != nil {
		validators, err := ValidatorSetFromProto(other.GetValidators())
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
	return proto.Marshal(hp)
}

// UnmarshalBinary decodes binary form of SignedHeader into object.
func (sh *SignedHeader) UnmarshalBinary(data []byte) error {
	var pHeader pb.SignedHeader
	err := proto.Unmarshal(data, &pHeader)
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
	v := &pb.Version{}
	v.SetBlock(h.Version.Block)
	v.SetApp(h.Version.App)

	hp := &pb.Header{}
	hp.SetVersion(v)
	hp.SetHeight(h.BaseHeader.Height)
	hp.SetTime(h.BaseHeader.Time)
	hp.SetLastHeaderHash(h.LastHeaderHash[:])
	hp.SetLastCommitHash(h.LastCommitHash[:])
	hp.SetDataHash(h.DataHash[:])
	hp.SetConsensusHash(h.ConsensusHash[:])
	hp.SetAppHash(h.AppHash[:])
	hp.SetLastResultsHash(h.LastResultsHash[:])
	hp.SetProposerAddress(h.ProposerAddress[:])
	hp.SetChainId(h.BaseHeader.ChainID)
	hp.SetValidatorHash(h.ValidatorHash)

	return hp
}

// FromProto fills Header with data from its protobuf representation.
func (h *Header) FromProto(other *pb.Header) error {
	h.Version.Block = other.GetVersion().GetBlock()
	h.Version.App = other.GetVersion().GetApp()
	h.BaseHeader.ChainID = other.GetChainId()
	h.BaseHeader.Height = other.GetHeight()
	h.BaseHeader.Time = other.GetTime()
	h.LastHeaderHash = other.GetLastHeaderHash()
	h.LastCommitHash = other.GetLastCommitHash()
	h.DataHash = other.GetDataHash()
	h.ConsensusHash = other.GetConsensusHash()
	h.AppHash = other.GetAppHash()
	h.LastResultsHash = other.GetLastResultsHash()
	h.ValidatorHash = other.GetValidatorHash()
	if len(other.GetProposerAddress()) > 0 {
		h.ProposerAddress = make([]byte, len(other.GetProposerAddress()))
		copy(h.ProposerAddress, other.GetProposerAddress())
	}

	return nil
}

// ToProto ...
func (m *Metadata) ToProto() *pb.Metadata {
	mp := &pb.Metadata{}
	mp.SetChainId(m.ChainID)
	mp.SetHeight(m.Height)
	mp.SetTime(m.Time)
	mp.SetLastDataHash(m.LastDataHash[:])
	return mp
}

// FromProto ...
func (m *Metadata) FromProto(other *pb.Metadata) {
	m.ChainID = other.GetChainId()
	m.Height = other.GetHeight()
	m.Time = other.GetTime()
	m.LastDataHash = other.GetLastDataHash()
}

// ToProto converts Data into protobuf representation and returns it.
func (d *Data) ToProto() *pb.Data {
	var mProto *pb.Metadata
	if d.Metadata != nil {
		mProto = d.Metadata.ToProto()
	}
	dp := &pb.Data{}
	dp.SetMetadata(mProto)
	dp.SetTxs(txsToByteSlices(d.Txs))
	// dp.SetIntermediateStateRoots(d.IntermediateStateRoots.RawRootsList)
	// Note: Temporarily remove Evidence #896
	// dp.SetEvidence(evidenceToProto(d.Evidence))
	return dp
}

// FromProto fills the Data with data from its protobuf representation
func (d *Data) FromProto(other *pb.Data) error {
	if other.GetMetadata() != nil {
		if d.Metadata == nil {
			d.Metadata = &Metadata{}
		}
		d.Metadata.FromProto(other.GetMetadata())
	}
	d.Txs = byteSlicesToTxs(other.GetTxs())
	// d.IntermediateStateRoots.RawRootsList = other.IntermediateStateRoots
	// Note: Temporarily remove Evidence #896
	// d.Evidence = evidenceFromProto(other.Evidence)

	return nil
}

// ToProto converts State into protobuf representation and returns it.
func (s *State) ToProto() (*pb.State, error) {
	sp := &pb.State{}
	sp.SetChainId(s.ChainID)
	sp.SetInitialHeight(s.InitialHeight)
	sp.SetLastBlockHeight(s.LastBlockHeight)
	sp.SetDaHeight(s.DAHeight)
	return sp, nil
}

// FromProto fills State with data from its protobuf representation.
func (s *State) FromProto(other *pb.State) error {
	s.ChainID = other.GetChainId()
	s.InitialHeight = other.GetInitialHeight()
	s.LastBlockHeight = other.GetLastBlockHeight()
	s.DAHeight = other.GetDaHeight()

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

// Note: Temporarily remove Evidence #896

// func evidenceToProto(evidence EvidenceData) []*abci.Evidence {
// 	var ret []*abci.Evidence
// 	for _, e := range evidence.Evidence {
// 		for i := range e.ABCI() {
// 			ae := e.ABCI()[i]
// 			ret = append(ret, &ae)
// 		}
// 	}
// 	return ret
// }

// func evidenceFromProto(evidence []*abci.Evidence) EvidenceData {
// 	var ret EvidenceData
// 	// TODO(tzdybal): right now Evidence is just an interface without implementations
// 	return ret
// }

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
