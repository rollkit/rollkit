package types

import (
	"errors"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/evstack/ev-node/types/pb/rollkit/v1"
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
	return m.FromProto(&pMetadata)
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
	if sh.Signer.PubKey == nil {
		return &pb.SignedHeader{
			Header:    sh.Header.ToProto(),
			Signature: sh.Signature[:],
			Signer:    &pb.Signer{},
		}, nil
	}

	pubKey, err := crypto.MarshalPublicKey(sh.Signer.PubKey)
	if err != nil {
		return nil, err
	}
	return &pb.SignedHeader{
		Header:    sh.Header.ToProto(),
		Signature: sh.Signature[:],
		Signer: &pb.Signer{
			Address: sh.Signer.Address,
			PubKey:  pubKey,
		},
	}, nil
}

// FromProto fills SignedHeader with data from protobuf representation. The contained
// Signer can only be used to verify signatures, not to sign messages.
func (sh *SignedHeader) FromProto(other *pb.SignedHeader) error {
	if other == nil {
		return errors.New("signed header is nil")
	}
	if other.Header == nil {
		return errors.New("signed header's Header is nil")
	}
	if err := sh.Header.FromProto(other.Header); err != nil {
		return err
	}
	if other.Signature != nil {
		sh.Signature = make([]byte, len(other.Signature))
		copy(sh.Signature, other.Signature)
	} else {
		sh.Signature = nil
	}
	if other.Signer != nil && len(other.Signer.PubKey) > 0 {
		pubKey, err := crypto.UnmarshalPublicKey(other.Signer.PubKey)
		if err != nil {
			return err
		}
		sh.Signer = Signer{
			Address: append([]byte(nil), other.Signer.Address...),
			PubKey:  pubKey,
		}
	} else {
		sh.Signer = Signer{}
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
	if other == nil {
		return errors.New("header is nil")
	}
	if other.Version != nil {
		h.Version.Block = other.Version.Block
		h.Version.App = other.Version.App
	} else {
		h.Version = Version{}
	}
	h.BaseHeader.ChainID = other.GetChainId()
	h.BaseHeader.Height = other.GetHeight()
	h.BaseHeader.Time = other.GetTime()
	if other.LastHeaderHash != nil {
		h.LastHeaderHash = append([]byte(nil), other.LastHeaderHash...)
	} else {
		h.LastHeaderHash = nil
	}
	if other.LastCommitHash != nil {
		h.LastCommitHash = append([]byte(nil), other.LastCommitHash...)
	} else {
		h.LastCommitHash = nil
	}
	if other.DataHash != nil {
		h.DataHash = append([]byte(nil), other.DataHash...)
	} else {
		h.DataHash = nil
	}
	if other.ConsensusHash != nil {
		h.ConsensusHash = append([]byte(nil), other.ConsensusHash...)
	} else {
		h.ConsensusHash = nil
	}
	if other.AppHash != nil {
		h.AppHash = append([]byte(nil), other.AppHash...)
	} else {
		h.AppHash = nil
	}
	if other.LastResultsHash != nil {
		h.LastResultsHash = append([]byte(nil), other.LastResultsHash...)
	} else {
		h.LastResultsHash = nil
	}
	if other.ProposerAddress != nil {
		h.ProposerAddress = append([]byte(nil), other.ProposerAddress...)
	} else {
		h.ProposerAddress = nil
	}
	if other.ValidatorHash != nil {
		h.ValidatorHash = append([]byte(nil), other.ValidatorHash...)
	} else {
		h.ValidatorHash = nil
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
func (m *Metadata) FromProto(other *pb.Metadata) error {
	if other == nil {
		return errors.New("metadata is nil")
	}
	m.ChainID = other.GetChainId()
	m.Height = other.GetHeight()
	m.Time = other.GetTime()
	if other.LastDataHash != nil {
		m.LastDataHash = append([]byte(nil), other.LastDataHash...)
	} else {
		m.LastDataHash = nil
	}
	return nil
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
	}
}

// FromProto fills the Data with data from its protobuf representation
func (d *Data) FromProto(other *pb.Data) error {
	if other == nil {
		return errors.New("data is nil")
	}
	if other.Metadata != nil {
		if d.Metadata == nil {
			d.Metadata = &Metadata{}
		}
		if err := d.Metadata.FromProto(other.Metadata); err != nil {
			return err
		}
	} else {
		d.Metadata = nil
	}
	d.Txs = byteSlicesToTxs(other.GetTxs())
	return nil
}

// ToProto converts State into protobuf representation and returns it.
func (s *State) ToProto() (*pb.State, error) {

	return &pb.State{
		Version: &pb.Version{
			Block: s.Version.Block,
			App:   s.Version.App,
		},
		ChainId:         s.ChainID,
		InitialHeight:   s.InitialHeight,
		LastBlockHeight: s.LastBlockHeight,
		LastBlockTime:   timestamppb.New(s.LastBlockTime),
		DaHeight:        s.DAHeight,
		LastResultsHash: s.LastResultsHash[:],
		AppHash:         s.AppHash[:],
	}, nil
}

// FromProto fills State with data from its protobuf representation.
func (s *State) FromProto(other *pb.State) error {
	if other == nil {
		return errors.New("state is nil")
	}
	if other.Version != nil {
		s.Version = Version{
			Block: other.Version.Block,
			App:   other.Version.App,
		}
	} else {
		s.Version = Version{}
	}
	s.ChainID = other.GetChainId()
	s.InitialHeight = other.GetInitialHeight()
	s.LastBlockHeight = other.GetLastBlockHeight()
	if other.LastBlockTime != nil {
		s.LastBlockTime = other.LastBlockTime.AsTime()
	} else {
		s.LastBlockTime = time.Time{}
	}
	if other.LastResultsHash != nil {
		s.LastResultsHash = append([]byte(nil), other.LastResultsHash...)
	} else {
		s.LastResultsHash = nil
	}
	if other.AppHash != nil {
		s.AppHash = append([]byte(nil), other.AppHash...)
	} else {
		s.AppHash = nil
	}
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
		return Txs{}
	}
	txs := make(Txs, len(bytes))
	for i := range txs {
		txs[i] = bytes[i]
	}
	return txs
}

// ToProto converts SignedData into protobuf representation and returns it.
func (sd *SignedData) ToProto() (*pb.SignedData, error) {
	dataProto := sd.Data.ToProto()

	var signerProto *pb.Signer
	if sd.Signer.PubKey != nil {
		pubKey, err := crypto.MarshalPublicKey(sd.Signer.PubKey)
		if err != nil {
			return nil, err
		}
		signerProto = &pb.Signer{
			Address: sd.Signer.Address,
			PubKey:  pubKey,
		}
	} else {
		signerProto = &pb.Signer{}
	}

	return &pb.SignedData{
		Data:      dataProto,
		Signature: sd.Signature[:],
		Signer:    signerProto,
	}, nil
}

// FromProto fills SignedData with data from protobuf representation.
func (sd *SignedData) FromProto(other *pb.SignedData) error {
	if other == nil {
		return errors.New("signed data is nil")
	}
	if other.Data != nil {
		if err := sd.Data.FromProto(other.Data); err != nil {
			return err
		}
	}
	if other.Signature != nil {
		sd.Signature = make([]byte, len(other.Signature))
		copy(sd.Signature, other.Signature)
	} else {
		sd.Signature = nil
	}
	if other.Signer != nil && len(other.Signer.PubKey) > 0 {
		pubKey, err := crypto.UnmarshalPublicKey(other.Signer.PubKey)
		if err != nil {
			return err
		}
		sd.Signer = Signer{
			Address: append([]byte(nil), other.Signer.Address...),
			PubKey:  pubKey,
		}
	} else {
		sd.Signer = Signer{}
	}
	return nil
}

// MarshalBinary encodes SignedData into binary form and returns it.
func (sd *SignedData) MarshalBinary() ([]byte, error) {
	p, err := sd.ToProto()
	if err != nil {
		return nil, err
	}
	return proto.Marshal(p)
}

// UnmarshalBinary decodes binary form of SignedData into object.
func (sd *SignedData) UnmarshalBinary(data []byte) error {
	var pData pb.SignedData
	err := proto.Unmarshal(data, &pData)
	if err != nil {
		return err
	}
	return sd.FromProto(&pData)
}
