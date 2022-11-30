// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: rollmint/state.proto

package rollmint

import (
	fmt "fmt"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	github_com_gogo_protobuf_types "github.com/gogo/protobuf/types"
	_ "github.com/tendermint/tendermint/abci/types"
	state "github.com/tendermint/tendermint/proto/tendermint/state"
	types "github.com/tendermint/tendermint/proto/tendermint/types"
	_ "google.golang.org/protobuf/types/known/timestamppb"
	io "io"
	math "math"
	math_bits "math/bits"
	time "time"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf
var _ = time.Kitchen

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

type State struct {
	Version                          *state.Version        `protobuf:"bytes,1,opt,name=version,proto3" json:"version,omitempty"`
	ChainId                          string                `protobuf:"bytes,2,opt,name=chain_id,json=chainId,proto3" json:"chain_id,omitempty"`
	InitialHeight                    int64                 `protobuf:"varint,3,opt,name=initial_height,json=initialHeight,proto3" json:"initial_height,omitempty"`
	LastBlockHeight                  int64                 `protobuf:"varint,4,opt,name=last_block_height,json=lastBlockHeight,proto3" json:"last_block_height,omitempty"`
	LastBlockID                      types.BlockID         `protobuf:"bytes,5,opt,name=last_block_id,json=lastBlockId,proto3" json:"last_block_id"`
	LastBlockTime                    time.Time             `protobuf:"bytes,6,opt,name=last_block_time,json=lastBlockTime,proto3,stdtime" json:"last_block_time"`
	DAHeight                         uint64                `protobuf:"varint,7,opt,name=da_height,json=daHeight,proto3" json:"da_height,omitempty"`
	NextValidators                   *types.ValidatorSet   `protobuf:"bytes,8,opt,name=next_validators,json=nextValidators,proto3" json:"next_validators,omitempty"`
	Validators                       *types.ValidatorSet   `protobuf:"bytes,9,opt,name=validators,proto3" json:"validators,omitempty"`
	LastValidators                   *types.ValidatorSet   `protobuf:"bytes,10,opt,name=last_validators,json=lastValidators,proto3" json:"last_validators,omitempty"`
	LastHeightValidatorsChanged      int64                 `protobuf:"varint,11,opt,name=last_height_validators_changed,json=lastHeightValidatorsChanged,proto3" json:"last_height_validators_changed,omitempty"`
	ConsensusParams                  types.ConsensusParams `protobuf:"bytes,12,opt,name=consensus_params,json=consensusParams,proto3" json:"consensus_params"`
	LastHeightConsensusParamsChanged int64                 `protobuf:"varint,13,opt,name=last_height_consensus_params_changed,json=lastHeightConsensusParamsChanged,proto3" json:"last_height_consensus_params_changed,omitempty"`
	LastResultsHash                  []byte                `protobuf:"bytes,14,opt,name=last_results_hash,json=lastResultsHash,proto3" json:"last_results_hash,omitempty"`
	AppHash                          []byte                `protobuf:"bytes,15,opt,name=app_hash,json=appHash,proto3" json:"app_hash,omitempty"`
}

func (m *State) Reset()         { *m = State{} }
func (m *State) String() string { return proto.CompactTextString(m) }
func (*State) ProtoMessage()    {}
func (*State) Descriptor() ([]byte, []int) {
	return fileDescriptor_86999017b1330537, []int{0}
}
func (m *State) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *State) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_State.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *State) XXX_Merge(src proto.Message) {
	xxx_messageInfo_State.Merge(m, src)
}
func (m *State) XXX_Size() int {
	return m.Size()
}
func (m *State) XXX_DiscardUnknown() {
	xxx_messageInfo_State.DiscardUnknown(m)
}

var xxx_messageInfo_State proto.InternalMessageInfo

func (m *State) GetVersion() *state.Version {
	if m != nil {
		return m.Version
	}
	return nil
}

func (m *State) GetChainId() string {
	if m != nil {
		return m.ChainId
	}
	return ""
}

func (m *State) GetInitialHeight() int64 {
	if m != nil {
		return m.InitialHeight
	}
	return 0
}

func (m *State) GetLastBlockHeight() int64 {
	if m != nil {
		return m.LastBlockHeight
	}
	return 0
}

func (m *State) GetLastBlockID() types.BlockID {
	if m != nil {
		return m.LastBlockID
	}
	return types.BlockID{}
}

func (m *State) GetLastBlockTime() time.Time {
	if m != nil {
		return m.LastBlockTime
	}
	return time.Time{}
}

func (m *State) GetDAHeight() uint64 {
	if m != nil {
		return m.DAHeight
	}
	return 0
}

func (m *State) GetNextValidators() *types.ValidatorSet {
	if m != nil {
		return m.NextValidators
	}
	return nil
}

func (m *State) GetValidators() *types.ValidatorSet {
	if m != nil {
		return m.Validators
	}
	return nil
}

func (m *State) GetLastValidators() *types.ValidatorSet {
	if m != nil {
		return m.LastValidators
	}
	return nil
}

func (m *State) GetLastHeightValidatorsChanged() int64 {
	if m != nil {
		return m.LastHeightValidatorsChanged
	}
	return 0
}

func (m *State) GetConsensusParams() types.ConsensusParams {
	if m != nil {
		return m.ConsensusParams
	}
	return types.ConsensusParams{}
}

func (m *State) GetLastHeightConsensusParamsChanged() int64 {
	if m != nil {
		return m.LastHeightConsensusParamsChanged
	}
	return 0
}

func (m *State) GetLastResultsHash() []byte {
	if m != nil {
		return m.LastResultsHash
	}
	return nil
}

func (m *State) GetAppHash() []byte {
	if m != nil {
		return m.AppHash
	}
	return nil
}

func init() {
	proto.RegisterType((*State)(nil), "rollmint.State")
}

func init() { proto.RegisterFile("rollmint/state.proto", fileDescriptor_86999017b1330537) }

var fileDescriptor_86999017b1330537 = []byte{
	// 590 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x94, 0x4f, 0x6f, 0xd3, 0x30,
	0x18, 0xc6, 0x1b, 0xf6, 0xa7, 0x99, 0xfb, 0x0f, 0xc2, 0x0e, 0x59, 0x07, 0x69, 0x40, 0x20, 0x15,
	0x0e, 0x89, 0x60, 0x77, 0x24, 0xd2, 0x4a, 0xac, 0x62, 0x42, 0x28, 0x43, 0x3b, 0x70, 0x89, 0x9c,
	0xc4, 0x24, 0x16, 0x69, 0x1c, 0xc5, 0xee, 0x04, 0xdf, 0x62, 0x1f, 0x6b, 0xc7, 0x1d, 0x39, 0x15,
	0xd4, 0x7e, 0x0a, 0x6e, 0xc8, 0x76, 0x9c, 0x66, 0x2b, 0x87, 0xdd, 0x9a, 0xe7, 0x7d, 0xde, 0x5f,
	0x9f, 0xd7, 0xaf, 0x13, 0x70, 0x58, 0x92, 0x2c, 0x9b, 0xe3, 0x9c, 0xb9, 0x94, 0x41, 0x86, 0x9c,
	0xa2, 0x24, 0x8c, 0x18, 0xba, 0x52, 0x87, 0x87, 0x09, 0x49, 0x88, 0x10, 0x5d, 0xfe, 0x4b, 0xd6,
	0x87, 0xa3, 0x84, 0x90, 0x24, 0x43, 0xae, 0x78, 0x0a, 0x17, 0xdf, 0x5c, 0x86, 0xe7, 0x88, 0x32,
	0x38, 0x2f, 0x2a, 0xc3, 0x31, 0x43, 0x79, 0x8c, 0x4a, 0x01, 0x86, 0x61, 0x84, 0x5d, 0xf6, 0xb3,
	0x40, 0xb4, 0x2a, 0x3e, 0x69, 0x14, 0x85, 0x7e, 0xab, 0x6a, 0x6f, 0x55, 0x2f, 0x61, 0x86, 0x63,
	0xc8, 0x48, 0x59, 0x39, 0x9e, 0x6e, 0x39, 0x0a, 0x58, 0xc2, 0xf9, 0xff, 0xf0, 0x62, 0xa8, 0x26,
	0xfe, 0xf9, 0xdf, 0x7d, 0xb0, 0x77, 0xce, 0x55, 0xe3, 0x04, 0xb4, 0x2f, 0x51, 0x49, 0x31, 0xc9,
	0x4d, 0xcd, 0xd6, 0xc6, 0x9d, 0xb7, 0x47, 0xce, 0xa6, 0xd3, 0x91, 0xc7, 0x71, 0x21, 0x0d, 0xbe,
	0x72, 0x1a, 0x47, 0x40, 0x8f, 0x52, 0x88, 0xf3, 0x00, 0xc7, 0xe6, 0x03, 0x5b, 0x1b, 0x1f, 0xf8,
	0x6d, 0xf1, 0x3c, 0x8b, 0x8d, 0x97, 0xa0, 0x8f, 0x73, 0xcc, 0x30, 0xcc, 0x82, 0x14, 0xe1, 0x24,
	0x65, 0xe6, 0x8e, 0xad, 0x8d, 0x77, 0xfc, 0x5e, 0xa5, 0x9e, 0x0a, 0xd1, 0x78, 0x0d, 0x1e, 0x65,
	0x90, 0xb2, 0x20, 0xcc, 0x48, 0xf4, 0x5d, 0x39, 0x77, 0x85, 0x73, 0xc0, 0x0b, 0x1e, 0xd7, 0x2b,
	0xaf, 0x0f, 0x7a, 0x0d, 0x2f, 0x8e, 0xcd, 0xbd, 0xed, 0xa0, 0x72, 0x38, 0xd1, 0x35, 0x9b, 0x7a,
	0x8f, 0xaf, 0x97, 0xa3, 0xd6, 0x6a, 0x39, 0xea, 0x9c, 0x29, 0xd4, 0x6c, 0xea, 0x77, 0x6a, 0xee,
	0x2c, 0x36, 0xce, 0xc0, 0xa0, 0xc1, 0xe4, 0x8b, 0x33, 0xf7, 0x05, 0x75, 0xe8, 0xc8, 0xad, 0x3a,
	0x6a, 0xab, 0xce, 0x17, 0xb5, 0x55, 0x4f, 0xe7, 0xd8, 0xab, 0xdf, 0x23, 0xcd, 0xef, 0xd5, 0x2c,
	0x5e, 0x35, 0x5e, 0x81, 0x83, 0x18, 0xaa, 0x29, 0xda, 0xb6, 0x36, 0xde, 0xf5, 0xba, 0xab, 0xe5,
	0x48, 0x9f, 0xbe, 0x97, 0x23, 0xf8, 0x7a, 0x0c, 0xab, 0x61, 0x3e, 0x80, 0x41, 0x8e, 0x7e, 0xb0,
	0xa0, 0x5e, 0x27, 0x35, 0x75, 0xf1, 0xc7, 0xd6, 0xf6, 0x38, 0x17, 0xca, 0x73, 0x8e, 0x98, 0xdf,
	0xe7, 0x6d, 0xb5, 0x42, 0x8d, 0x77, 0x00, 0x34, 0x18, 0x07, 0xf7, 0x62, 0x34, 0x3a, 0x78, 0x10,
	0x71, 0x02, 0x0d, 0x08, 0xb8, 0x5f, 0x10, 0xde, 0xd6, 0x08, 0x32, 0x01, 0x96, 0x00, 0xc9, 0xf1,
	0x1b, 0xbc, 0x20, 0x4a, 0x61, 0x9e, 0xa0, 0xd8, 0xec, 0x88, 0xbd, 0x1e, 0x73, 0x97, 0x3c, 0x85,
	0x4d, 0xf7, 0x44, 0x5a, 0x0c, 0x1f, 0x3c, 0x8c, 0x48, 0x4e, 0x51, 0x4e, 0x17, 0x34, 0x90, 0x17,
	0xd9, 0xec, 0x8a, 0x38, 0xcf, 0xb6, 0xe3, 0x4c, 0x94, 0xf3, 0xb3, 0x30, 0x7a, 0xbb, 0x7c, 0x2f,
	0xfe, 0x20, 0xba, 0x2d, 0x1b, 0x9f, 0xc0, 0x8b, 0x66, 0xb0, 0xbb, 0xfc, 0x3a, 0x5e, 0x4f, 0xc4,
	0xb3, 0x37, 0xf1, 0xee, 0xf0, 0x55, 0x46, 0x75, 0x67, 0x4b, 0x44, 0x17, 0x19, 0xa3, 0x41, 0x0a,
	0x69, 0x6a, 0xf6, 0x6d, 0x6d, 0xdc, 0x95, 0x77, 0xd6, 0x97, 0xfa, 0x29, 0xa4, 0x29, 0x7f, 0x43,
	0x60, 0x51, 0x48, 0xcb, 0x40, 0x58, 0xda, 0xb0, 0x28, 0x78, 0xc9, 0xfb, 0x78, 0xbd, 0xb2, 0xb4,
	0x9b, 0x95, 0xa5, 0xfd, 0x59, 0x59, 0xda, 0xd5, 0xda, 0x6a, 0xdd, 0xac, 0xad, 0xd6, 0xaf, 0xb5,
	0xd5, 0xfa, 0xfa, 0x26, 0xc1, 0x2c, 0x5d, 0x84, 0x4e, 0x44, 0xe6, 0x6e, 0x84, 0x32, 0x44, 0x19,
	0x86, 0xa4, 0x4c, 0xdc, 0xfa, 0xeb, 0x54, 0xbd, 0xe7, 0x61, 0xad, 0x84, 0xfb, 0xe2, 0x9a, 0x9e,
	0xfc, 0x0b, 0x00, 0x00, 0xff, 0xff, 0x44, 0xdb, 0xc0, 0x7e, 0xc2, 0x04, 0x00, 0x00,
}

func (m *State) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *State) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *State) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.AppHash) > 0 {
		i -= len(m.AppHash)
		copy(dAtA[i:], m.AppHash)
		i = encodeVarintState(dAtA, i, uint64(len(m.AppHash)))
		i--
		dAtA[i] = 0x7a
	}
	if len(m.LastResultsHash) > 0 {
		i -= len(m.LastResultsHash)
		copy(dAtA[i:], m.LastResultsHash)
		i = encodeVarintState(dAtA, i, uint64(len(m.LastResultsHash)))
		i--
		dAtA[i] = 0x72
	}
	if m.LastHeightConsensusParamsChanged != 0 {
		i = encodeVarintState(dAtA, i, uint64(m.LastHeightConsensusParamsChanged))
		i--
		dAtA[i] = 0x68
	}
	{
		size, err := m.ConsensusParams.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintState(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x62
	if m.LastHeightValidatorsChanged != 0 {
		i = encodeVarintState(dAtA, i, uint64(m.LastHeightValidatorsChanged))
		i--
		dAtA[i] = 0x58
	}
	if m.LastValidators != nil {
		{
			size, err := m.LastValidators.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintState(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x52
	}
	if m.Validators != nil {
		{
			size, err := m.Validators.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintState(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x4a
	}
	if m.NextValidators != nil {
		{
			size, err := m.NextValidators.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintState(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x42
	}
	if m.DAHeight != 0 {
		i = encodeVarintState(dAtA, i, uint64(m.DAHeight))
		i--
		dAtA[i] = 0x38
	}
	n5, err5 := github_com_gogo_protobuf_types.StdTimeMarshalTo(m.LastBlockTime, dAtA[i-github_com_gogo_protobuf_types.SizeOfStdTime(m.LastBlockTime):])
	if err5 != nil {
		return 0, err5
	}
	i -= n5
	i = encodeVarintState(dAtA, i, uint64(n5))
	i--
	dAtA[i] = 0x32
	{
		size, err := m.LastBlockID.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintState(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x2a
	if m.LastBlockHeight != 0 {
		i = encodeVarintState(dAtA, i, uint64(m.LastBlockHeight))
		i--
		dAtA[i] = 0x20
	}
	if m.InitialHeight != 0 {
		i = encodeVarintState(dAtA, i, uint64(m.InitialHeight))
		i--
		dAtA[i] = 0x18
	}
	if len(m.ChainId) > 0 {
		i -= len(m.ChainId)
		copy(dAtA[i:], m.ChainId)
		i = encodeVarintState(dAtA, i, uint64(len(m.ChainId)))
		i--
		dAtA[i] = 0x12
	}
	if m.Version != nil {
		{
			size, err := m.Version.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintState(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func encodeVarintState(dAtA []byte, offset int, v uint64) int {
	offset -= sovState(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *State) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Version != nil {
		l = m.Version.Size()
		n += 1 + l + sovState(uint64(l))
	}
	l = len(m.ChainId)
	if l > 0 {
		n += 1 + l + sovState(uint64(l))
	}
	if m.InitialHeight != 0 {
		n += 1 + sovState(uint64(m.InitialHeight))
	}
	if m.LastBlockHeight != 0 {
		n += 1 + sovState(uint64(m.LastBlockHeight))
	}
	l = m.LastBlockID.Size()
	n += 1 + l + sovState(uint64(l))
	l = github_com_gogo_protobuf_types.SizeOfStdTime(m.LastBlockTime)
	n += 1 + l + sovState(uint64(l))
	if m.DAHeight != 0 {
		n += 1 + sovState(uint64(m.DAHeight))
	}
	if m.NextValidators != nil {
		l = m.NextValidators.Size()
		n += 1 + l + sovState(uint64(l))
	}
	if m.Validators != nil {
		l = m.Validators.Size()
		n += 1 + l + sovState(uint64(l))
	}
	if m.LastValidators != nil {
		l = m.LastValidators.Size()
		n += 1 + l + sovState(uint64(l))
	}
	if m.LastHeightValidatorsChanged != 0 {
		n += 1 + sovState(uint64(m.LastHeightValidatorsChanged))
	}
	l = m.ConsensusParams.Size()
	n += 1 + l + sovState(uint64(l))
	if m.LastHeightConsensusParamsChanged != 0 {
		n += 1 + sovState(uint64(m.LastHeightConsensusParamsChanged))
	}
	l = len(m.LastResultsHash)
	if l > 0 {
		n += 1 + l + sovState(uint64(l))
	}
	l = len(m.AppHash)
	if l > 0 {
		n += 1 + l + sovState(uint64(l))
	}
	return n
}

func sovState(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozState(x uint64) (n int) {
	return sovState(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *State) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowState
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: State: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: State: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Version", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Version == nil {
				m.Version = &state.Version{}
			}
			if err := m.Version.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field ChainId", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.ChainId = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field InitialHeight", wireType)
			}
			m.InitialHeight = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.InitialHeight |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastBlockHeight", wireType)
			}
			m.LastBlockHeight = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.LastBlockHeight |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastBlockID", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.LastBlockID.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastBlockTime", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := github_com_gogo_protobuf_types.StdTimeUnmarshal(&m.LastBlockTime, dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 7:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field DAHeight", wireType)
			}
			m.DAHeight = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.DAHeight |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field NextValidators", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.NextValidators == nil {
				m.NextValidators = &types.ValidatorSet{}
			}
			if err := m.NextValidators.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 9:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Validators", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Validators == nil {
				m.Validators = &types.ValidatorSet{}
			}
			if err := m.Validators.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 10:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastValidators", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.LastValidators == nil {
				m.LastValidators = &types.ValidatorSet{}
			}
			if err := m.LastValidators.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 11:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastHeightValidatorsChanged", wireType)
			}
			m.LastHeightValidatorsChanged = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.LastHeightValidatorsChanged |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 12:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field ConsensusParams", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.ConsensusParams.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 13:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastHeightConsensusParamsChanged", wireType)
			}
			m.LastHeightConsensusParamsChanged = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.LastHeightConsensusParamsChanged |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 14:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastResultsHash", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.LastResultsHash = append(m.LastResultsHash[:0], dAtA[iNdEx:postIndex]...)
			if m.LastResultsHash == nil {
				m.LastResultsHash = []byte{}
			}
			iNdEx = postIndex
		case 15:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field AppHash", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowState
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthState
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthState
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.AppHash = append(m.AppHash[:0], dAtA[iNdEx:postIndex]...)
			if m.AppHash == nil {
				m.AppHash = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipState(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthState
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipState(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowState
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowState
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowState
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthState
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupState
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthState
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthState        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowState          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupState = fmt.Errorf("proto: unexpected end of group")
)
