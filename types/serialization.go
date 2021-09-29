package types

import (
	"errors"

	pb "github.com/celestiaorg/optimint/types/pb/optimint"
	abci "github.com/tendermint/tendermint/abci/types"
)

// MarshalBinary encodes Block into binary form and returns it.
func (b *Block) MarshalBinary() ([]byte, error) {
	return b.ToProto().Marshal()
}

// UnmarshalBinary decodes binary form of Block into object.
func (b *Block) UnmarshalBinary(data []byte) error {
	var pBlock pb.Block
	err := pBlock.Unmarshal(data)
	if err != nil {
		return err
	}
	err = b.FromProto(&pBlock)
	return err
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

// MarshalBinary encodes Commit into binary form and returns it.
func (c *Commit) MarshalBinary() ([]byte, error) {
	return c.ToProto().Marshal()
}

// UnmarshalBinary decodes binary form of Commit into object.
func (c *Commit) UnmarshalBinary(data []byte) error {
	var pCommit pb.Commit
	err := pCommit.Unmarshal(data)
	if err != nil {
		return err
	}
	err = c.FromProto(&pCommit)
	return err
}

// ToProto converts Header into protobuf representation and returns it.
func (h *Header) ToProto() *pb.Header {
	return &pb.Header{
		Version: &pb.Version{
			Block: h.Version.Block,
			App:   h.Version.App,
		},
		NamespaceId:     h.NamespaceID[:],
		Height:          h.Height,
		Time:            h.Time,
		LastHeaderHash:  h.LastHeaderHash[:],
		LastCommitHash:  h.LastCommitHash[:],
		DataHash:        h.DataHash[:],
		ConsensusHash:   h.ConsensusHash[:],
		AppHash:         h.AppHash[:],
		LastResultsHash: h.LastResultsHash[:],
		ProposerAddress: h.ProposerAddress[:],
	}
}

// FromProto fills Header with data from its protobuf representation.
func (h *Header) FromProto(other *pb.Header) error {
	h.Version.Block = other.Version.Block
	h.Version.App = other.Version.App
	if !safeCopy(h.NamespaceID[:], other.NamespaceId) {
		return errors.New("invalid length of 'NamespaceId'")
	}
	h.Height = other.Height
	h.Time = other.Time
	if !safeCopy(h.LastHeaderHash[:], other.LastHeaderHash) {
		return errors.New("invalid length of 'InvalidLastHeaderHash'")
	}
	if !safeCopy(h.LastCommitHash[:], other.LastCommitHash) {
		return errors.New("invalid length of 'InvalidLastCommitHash'")
	}
	if !safeCopy(h.DataHash[:], other.DataHash) {
		return errors.New("invalid length of 'InvalidDataHash'")
	}
	if !safeCopy(h.ConsensusHash[:], other.ConsensusHash) {
		return errors.New("invalid length of 'InvalidConsensusHash'")
	}
	if !safeCopy(h.AppHash[:], other.AppHash) {
		return errors.New("invalid length of 'InvalidAppHash'")
	}
	if !safeCopy(h.LastResultsHash[:], other.LastResultsHash) {
		return errors.New("invalid length of 'InvalidLastResultsHash'")
	}
	if len(other.ProposerAddress) > 0 {
		h.ProposerAddress = make([]byte, len(other.ProposerAddress))
		copy(h.ProposerAddress, other.ProposerAddress)
	}

	return nil
}

// safeCopy copies bytes from src slice into dst slice if both have same size.
// It returns true if sizes of src and dst are the same.
func safeCopy(dst, src []byte) bool {
	if len(src) != len(dst) {
		return false
	}
	_ = copy(dst, src)
	return true
}

// ToProto converts Block into protobuf representation and returns it.
func (b *Block) ToProto() *pb.Block {
	return &pb.Block{
		Header:     b.Header.ToProto(),
		Data:       b.Data.ToProto(),
		LastCommit: b.LastCommit.ToProto(),
	}
}

// ToProto converts Data into protobuf representation and returns it.
func (d *Data) ToProto() *pb.Data {
	return &pb.Data{
		Txs:                    txsToByteSlices(d.Txs),
		IntermediateStateRoots: d.IntermediateStateRoots.RawRootsList,
		Evidence:               evidenceToProto(d.Evidence),
	}
}

// FromProto fills Block with data from its protobuf representation.
func (b *Block) FromProto(other *pb.Block) error {
	err := b.Header.FromProto(other.Header)
	if err != nil {
		return err
	}
	b.Data.Txs = byteSlicesToTxs(other.Data.Txs)
	b.Data.IntermediateStateRoots.RawRootsList = other.Data.IntermediateStateRoots
	b.Data.Evidence = evidenceFromProto(other.Data.Evidence)
	if other.LastCommit != nil {
		err := b.LastCommit.FromProto(other.LastCommit)
		if err != nil {
			return err
		}
	}

	return nil
}

// ToProto converts Commit into protobuf representation and returns it.
func (c *Commit) ToProto() *pb.Commit {
	return &pb.Commit{
		Height:     c.Height,
		HeaderHash: c.HeaderHash[:],
		Signatures: signaturesToByteSlices(c.Signatures),
	}
}

// FromProto fills Commit with data from its protobuf representation.
func (c *Commit) FromProto(other *pb.Commit) error {
	c.Height = other.Height
	if !safeCopy(c.HeaderHash[:], other.HeaderHash) {
		return errors.New("invalid length of HeaderHash")
	}
	c.Signatures = byteSlicesToSignatures(other.Signatures)

	return nil
}

func txsToByteSlices(txs Txs) [][]byte {
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

func evidenceToProto(evidence EvidenceData) []*abci.Evidence {
	var ret []*abci.Evidence
	for _, e := range evidence.Evidence {
		for _, ae := range e.ABCI() {
			ret = append(ret, &ae)
		}
	}
	return ret
}

func evidenceFromProto(evidence []*abci.Evidence) EvidenceData {
	var ret EvidenceData
	// TODO(tzdybal): right now Evidence is just an interface without implementations
	return ret
}

func signaturesToByteSlices(sigs []Signature) [][]byte {
	if sigs == nil {
		return nil
	}
	bytes := make([][]byte, len(sigs))
	for i := range sigs {
		bytes[i] = sigs[i]
	}
	return bytes
}

func byteSlicesToSignatures(bytes [][]byte) []Signature {
	if bytes == nil {
		return nil
	}
	sigs := make([]Signature, len(bytes))
	for i := range bytes {
		sigs[i] = bytes[i]
	}
	return sigs
}
