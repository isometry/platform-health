// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.33.0
// 	protoc        v4.25.3
// source: proto/detail_tls.proto

package details

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Detail_TLS struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	CommonName         string                 `protobuf:"bytes,1,opt,name=commonName,proto3" json:"commonName,omitempty"`
	SubjectAltNames    []string               `protobuf:"bytes,2,rep,name=subjectAltNames,proto3" json:"subjectAltNames,omitempty"`
	Chain              []string               `protobuf:"bytes,3,rep,name=chain,proto3" json:"chain,omitempty"`
	ValidUntil         *timestamppb.Timestamp `protobuf:"bytes,4,opt,name=validUntil,proto3" json:"validUntil,omitempty"`
	SignatureAlgorithm string                 `protobuf:"bytes,5,opt,name=signatureAlgorithm,proto3" json:"signatureAlgorithm,omitempty"`
	PublicKeyAlgorithm string                 `protobuf:"bytes,6,opt,name=publicKeyAlgorithm,proto3" json:"publicKeyAlgorithm,omitempty"`
	Version            string                 `protobuf:"bytes,7,opt,name=version,proto3" json:"version,omitempty"`
	CipherSuite        string                 `protobuf:"bytes,8,opt,name=cipherSuite,proto3" json:"cipherSuite,omitempty"`
	Protocol           string                 `protobuf:"bytes,9,opt,name=protocol,proto3" json:"protocol,omitempty"`
}

func (x *Detail_TLS) Reset() {
	*x = Detail_TLS{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_detail_tls_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Detail_TLS) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Detail_TLS) ProtoMessage() {}

func (x *Detail_TLS) ProtoReflect() protoreflect.Message {
	mi := &file_proto_detail_tls_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Detail_TLS.ProtoReflect.Descriptor instead.
func (*Detail_TLS) Descriptor() ([]byte, []int) {
	return file_proto_detail_tls_proto_rawDescGZIP(), []int{0}
}

func (x *Detail_TLS) GetCommonName() string {
	if x != nil {
		return x.CommonName
	}
	return ""
}

func (x *Detail_TLS) GetSubjectAltNames() []string {
	if x != nil {
		return x.SubjectAltNames
	}
	return nil
}

func (x *Detail_TLS) GetChain() []string {
	if x != nil {
		return x.Chain
	}
	return nil
}

func (x *Detail_TLS) GetValidUntil() *timestamppb.Timestamp {
	if x != nil {
		return x.ValidUntil
	}
	return nil
}

func (x *Detail_TLS) GetSignatureAlgorithm() string {
	if x != nil {
		return x.SignatureAlgorithm
	}
	return ""
}

func (x *Detail_TLS) GetPublicKeyAlgorithm() string {
	if x != nil {
		return x.PublicKeyAlgorithm
	}
	return ""
}

func (x *Detail_TLS) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

func (x *Detail_TLS) GetCipherSuite() string {
	if x != nil {
		return x.CipherSuite
	}
	return ""
}

func (x *Detail_TLS) GetProtocol() string {
	if x != nil {
		return x.Protocol
	}
	return ""
}

var File_proto_detail_tls_proto protoreflect.FileDescriptor

var file_proto_detail_tls_proto_rawDesc = []byte{
	0x0a, 0x16, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x64, 0x65, 0x74, 0x61, 0x69, 0x6c, 0x5f, 0x74,
	0x6c, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x19, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f,
	0x72, 0x6d, 0x5f, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x2e, 0x64, 0x65, 0x74, 0x61, 0x69, 0x6c,
	0x2e, 0x76, 0x31, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0xe0, 0x02, 0x0a, 0x0a, 0x44, 0x65, 0x74, 0x61, 0x69, 0x6c, 0x5f,
	0x54, 0x4c, 0x53, 0x12, 0x1e, 0x0a, 0x0a, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x4e, 0x61, 0x6d,
	0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x4e,
	0x61, 0x6d, 0x65, 0x12, 0x28, 0x0a, 0x0f, 0x73, 0x75, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x41, 0x6c,
	0x74, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0f, 0x73, 0x75,
	0x62, 0x6a, 0x65, 0x63, 0x74, 0x41, 0x6c, 0x74, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x12, 0x14, 0x0a,
	0x05, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x18, 0x03, 0x20, 0x03, 0x28, 0x09, 0x52, 0x05, 0x63, 0x68,
	0x61, 0x69, 0x6e, 0x12, 0x3a, 0x0a, 0x0a, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x55, 0x6e, 0x74, 0x69,
	0x6c, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x52, 0x0a, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x55, 0x6e, 0x74, 0x69, 0x6c, 0x12,
	0x2e, 0x0a, 0x12, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x41, 0x6c, 0x67, 0x6f,
	0x72, 0x69, 0x74, 0x68, 0x6d, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x12, 0x73, 0x69, 0x67,
	0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x41, 0x6c, 0x67, 0x6f, 0x72, 0x69, 0x74, 0x68, 0x6d, 0x12,
	0x2e, 0x0a, 0x12, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x41, 0x6c, 0x67, 0x6f,
	0x72, 0x69, 0x74, 0x68, 0x6d, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x12, 0x70, 0x75, 0x62,
	0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x41, 0x6c, 0x67, 0x6f, 0x72, 0x69, 0x74, 0x68, 0x6d, 0x12,
	0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x20, 0x0a, 0x0b, 0x63, 0x69, 0x70,
	0x68, 0x65, 0x72, 0x53, 0x75, 0x69, 0x74, 0x65, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b,
	0x63, 0x69, 0x70, 0x68, 0x65, 0x72, 0x53, 0x75, 0x69, 0x74, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x42, 0x41, 0x5a, 0x3f, 0x67, 0x69, 0x74, 0x68, 0x75,
	0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x69, 0x73, 0x6f, 0x6d, 0x65, 0x74, 0x72, 0x79, 0x2f, 0x70,
	0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2d, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x2f, 0x70,
	0x6b, 0x67, 0x2f, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x5f, 0x68, 0x65, 0x61, 0x6c,
	0x74, 0x68, 0x2f, 0x64, 0x65, 0x74, 0x61, 0x69, 0x6c, 0x73, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_proto_detail_tls_proto_rawDescOnce sync.Once
	file_proto_detail_tls_proto_rawDescData = file_proto_detail_tls_proto_rawDesc
)

func file_proto_detail_tls_proto_rawDescGZIP() []byte {
	file_proto_detail_tls_proto_rawDescOnce.Do(func() {
		file_proto_detail_tls_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_detail_tls_proto_rawDescData)
	})
	return file_proto_detail_tls_proto_rawDescData
}

var file_proto_detail_tls_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_proto_detail_tls_proto_goTypes = []interface{}{
	(*Detail_TLS)(nil),            // 0: platform_health.detail.v1.Detail_TLS
	(*timestamppb.Timestamp)(nil), // 1: google.protobuf.Timestamp
}
var file_proto_detail_tls_proto_depIdxs = []int32{
	1, // 0: platform_health.detail.v1.Detail_TLS.validUntil:type_name -> google.protobuf.Timestamp
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_proto_detail_tls_proto_init() }
func file_proto_detail_tls_proto_init() {
	if File_proto_detail_tls_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_proto_detail_tls_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Detail_TLS); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_proto_detail_tls_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_proto_detail_tls_proto_goTypes,
		DependencyIndexes: file_proto_detail_tls_proto_depIdxs,
		MessageInfos:      file_proto_detail_tls_proto_msgTypes,
	}.Build()
	File_proto_detail_tls_proto = out.File
	file_proto_detail_tls_proto_rawDesc = nil
	file_proto_detail_tls_proto_goTypes = nil
	file_proto_detail_tls_proto_depIdxs = nil
}
