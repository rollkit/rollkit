// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        (unknown)
// source: evnode/v1/signer.proto

package v1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// The SignRequest holds the bytes we want to sign.
type SignRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Message       []byte                 `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SignRequest) Reset() {
	*x = SignRequest{}
	mi := &file_evnode_v1_signer_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SignRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignRequest) ProtoMessage() {}

func (x *SignRequest) ProtoReflect() protoreflect.Message {
	mi := &file_evnode_v1_signer_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignRequest.ProtoReflect.Descriptor instead.
func (*SignRequest) Descriptor() ([]byte, []int) {
	return file_evnode_v1_signer_proto_rawDescGZIP(), []int{0}
}

func (x *SignRequest) GetMessage() []byte {
	if x != nil {
		return x.Message
	}
	return nil
}

// The SignResponse returns the signature bytes.
type SignResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Signature     []byte                 `protobuf:"bytes,1,opt,name=signature,proto3" json:"signature,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SignResponse) Reset() {
	*x = SignResponse{}
	mi := &file_evnode_v1_signer_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SignResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignResponse) ProtoMessage() {}

func (x *SignResponse) ProtoReflect() protoreflect.Message {
	mi := &file_evnode_v1_signer_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignResponse.ProtoReflect.Descriptor instead.
func (*SignResponse) Descriptor() ([]byte, []int) {
	return file_evnode_v1_signer_proto_rawDescGZIP(), []int{1}
}

func (x *SignResponse) GetSignature() []byte {
	if x != nil {
		return x.Signature
	}
	return nil
}

// The GetPublicRequest is an empty request.
type GetPublicRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetPublicRequest) Reset() {
	*x = GetPublicRequest{}
	mi := &file_evnode_v1_signer_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetPublicRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetPublicRequest) ProtoMessage() {}

func (x *GetPublicRequest) ProtoReflect() protoreflect.Message {
	mi := &file_evnode_v1_signer_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetPublicRequest.ProtoReflect.Descriptor instead.
func (*GetPublicRequest) Descriptor() ([]byte, []int) {
	return file_evnode_v1_signer_proto_rawDescGZIP(), []int{2}
}

// The GetPublicResponse returns the public key.
type GetPublicResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	PublicKey     []byte                 `protobuf:"bytes,1,opt,name=public_key,json=publicKey,proto3" json:"public_key,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetPublicResponse) Reset() {
	*x = GetPublicResponse{}
	mi := &file_evnode_v1_signer_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetPublicResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetPublicResponse) ProtoMessage() {}

func (x *GetPublicResponse) ProtoReflect() protoreflect.Message {
	mi := &file_evnode_v1_signer_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetPublicResponse.ProtoReflect.Descriptor instead.
func (*GetPublicResponse) Descriptor() ([]byte, []int) {
	return file_evnode_v1_signer_proto_rawDescGZIP(), []int{3}
}

func (x *GetPublicResponse) GetPublicKey() []byte {
	if x != nil {
		return x.PublicKey
	}
	return nil
}

var File_evnode_v1_signer_proto protoreflect.FileDescriptor

const file_evnode_v1_signer_proto_rawDesc = "" +
	"\n" +
	"\x16evnode/v1/signer.proto\x12\tevnode.v1\"'\n" +
	"\vSignRequest\x12\x18\n" +
	"\amessage\x18\x01 \x01(\fR\amessage\",\n" +
	"\fSignResponse\x12\x1c\n" +
	"\tsignature\x18\x01 \x01(\fR\tsignature\"\x12\n" +
	"\x10GetPublicRequest\"2\n" +
	"\x11GetPublicResponse\x12\x1d\n" +
	"\n" +
	"public_key\x18\x01 \x01(\fR\tpublicKey2\x90\x01\n" +
	"\rSignerService\x127\n" +
	"\x04Sign\x12\x16.evnode.v1.SignRequest\x1a\x17.evnode.v1.SignResponse\x12F\n" +
	"\tGetPublic\x12\x1b.evnode.v1.GetPublicRequest\x1a\x1c.evnode.v1.GetPublicResponseB/Z-github.com/evstack/ev-node/types/pb/evnode/v1b\x06proto3"

var (
	file_evnode_v1_signer_proto_rawDescOnce sync.Once
	file_evnode_v1_signer_proto_rawDescData []byte
)

func file_evnode_v1_signer_proto_rawDescGZIP() []byte {
	file_evnode_v1_signer_proto_rawDescOnce.Do(func() {
		file_evnode_v1_signer_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_evnode_v1_signer_proto_rawDesc), len(file_evnode_v1_signer_proto_rawDesc)))
	})
	return file_evnode_v1_signer_proto_rawDescData
}

var file_evnode_v1_signer_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_evnode_v1_signer_proto_goTypes = []any{
	(*SignRequest)(nil),       // 0: evnode.v1.SignRequest
	(*SignResponse)(nil),      // 1: evnode.v1.SignResponse
	(*GetPublicRequest)(nil),  // 2: evnode.v1.GetPublicRequest
	(*GetPublicResponse)(nil), // 3: evnode.v1.GetPublicResponse
}
var file_evnode_v1_signer_proto_depIdxs = []int32{
	0, // 0: evnode.v1.SignerService.Sign:input_type -> evnode.v1.SignRequest
	2, // 1: evnode.v1.SignerService.GetPublic:input_type -> evnode.v1.GetPublicRequest
	1, // 2: evnode.v1.SignerService.Sign:output_type -> evnode.v1.SignResponse
	3, // 3: evnode.v1.SignerService.GetPublic:output_type -> evnode.v1.GetPublicResponse
	2, // [2:4] is the sub-list for method output_type
	0, // [0:2] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_evnode_v1_signer_proto_init() }
func file_evnode_v1_signer_proto_init() {
	if File_evnode_v1_signer_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_evnode_v1_signer_proto_rawDesc), len(file_evnode_v1_signer_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_evnode_v1_signer_proto_goTypes,
		DependencyIndexes: file_evnode_v1_signer_proto_depIdxs,
		MessageInfos:      file_evnode_v1_signer_proto_msgTypes,
	}.Build()
	File_evnode_v1_signer_proto = out.File
	file_evnode_v1_signer_proto_goTypes = nil
	file_evnode_v1_signer_proto_depIdxs = nil
}
