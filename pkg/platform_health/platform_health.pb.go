// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.1
// 	protoc        v5.28.3
// source: proto/platform_health.proto

package platform_health

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	anypb "google.golang.org/protobuf/types/known/anypb"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Status int32

const (
	Status_UNKNOWN       Status = 0
	Status_HEALTHY       Status = 1
	Status_UNHEALTHY     Status = 2
	Status_LOOP_DETECTED Status = 3
)

// Enum value maps for Status.
var (
	Status_name = map[int32]string{
		0: "UNKNOWN",
		1: "HEALTHY",
		2: "UNHEALTHY",
		3: "LOOP_DETECTED",
	}
	Status_value = map[string]int32{
		"UNKNOWN":       0,
		"HEALTHY":       1,
		"UNHEALTHY":     2,
		"LOOP_DETECTED": 3,
	}
)

func (x Status) Enum() *Status {
	p := new(Status)
	*p = x
	return p
}

func (x Status) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Status) Descriptor() protoreflect.EnumDescriptor {
	return file_proto_platform_health_proto_enumTypes[0].Descriptor()
}

func (Status) Type() protoreflect.EnumType {
	return &file_proto_platform_health_proto_enumTypes[0]
}

func (x Status) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Status.Descriptor instead.
func (Status) EnumDescriptor() ([]byte, []int) {
	return file_proto_platform_health_proto_rawDescGZIP(), []int{0}
}

type HealthCheckRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// allow specification of restricted subset of components to validate
	Component string   `protobuf:"bytes,1,opt,name=component,proto3" json:"component,omitempty"` // TODO: define syntax for specifying component
	Hops      []string `protobuf:"bytes,2,rep,name=hops,proto3" json:"hops,omitempty"`           // list of server IDs for loop detection
}

func (x *HealthCheckRequest) Reset() {
	*x = HealthCheckRequest{}
	mi := &file_proto_platform_health_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *HealthCheckRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HealthCheckRequest) ProtoMessage() {}

func (x *HealthCheckRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_platform_health_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HealthCheckRequest.ProtoReflect.Descriptor instead.
func (*HealthCheckRequest) Descriptor() ([]byte, []int) {
	return file_proto_platform_health_proto_rawDescGZIP(), []int{0}
}

func (x *HealthCheckRequest) GetComponent() string {
	if x != nil {
		return x.Component
	}
	return ""
}

func (x *HealthCheckRequest) GetHops() []string {
	if x != nil {
		return x.Hops
	}
	return nil
}

type HealthCheckResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type       string                 `protobuf:"bytes,1,opt,name=type,proto3" json:"type,omitempty"`               // e.g. "tcp", "rest", "grpc", "kafka", "s3"
	Name       string                 `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`               // instance name
	ServerId   *string                `protobuf:"bytes,3,opt,name=serverId,proto3,oneof" json:"serverId,omitempty"` // unique identifier for server/satellite instance
	Status     Status                 `protobuf:"varint,4,opt,name=status,proto3,enum=platform_health.v1.Status" json:"status,omitempty"`
	Message    string                 `protobuf:"bytes,5,opt,name=message,proto3" json:"message,omitempty"`
	Details    []*anypb.Any           `protobuf:"bytes,6,rep,name=details,proto3" json:"details,omitempty"`
	Components []*HealthCheckResponse `protobuf:"bytes,7,rep,name=components,proto3" json:"components,omitempty"`
	Duration   *durationpb.Duration   `protobuf:"bytes,8,opt,name=duration,proto3" json:"duration,omitempty"`
}

func (x *HealthCheckResponse) Reset() {
	*x = HealthCheckResponse{}
	mi := &file_proto_platform_health_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *HealthCheckResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HealthCheckResponse) ProtoMessage() {}

func (x *HealthCheckResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_platform_health_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HealthCheckResponse.ProtoReflect.Descriptor instead.
func (*HealthCheckResponse) Descriptor() ([]byte, []int) {
	return file_proto_platform_health_proto_rawDescGZIP(), []int{1}
}

func (x *HealthCheckResponse) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *HealthCheckResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *HealthCheckResponse) GetServerId() string {
	if x != nil && x.ServerId != nil {
		return *x.ServerId
	}
	return ""
}

func (x *HealthCheckResponse) GetStatus() Status {
	if x != nil {
		return x.Status
	}
	return Status_UNKNOWN
}

func (x *HealthCheckResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *HealthCheckResponse) GetDetails() []*anypb.Any {
	if x != nil {
		return x.Details
	}
	return nil
}

func (x *HealthCheckResponse) GetComponents() []*HealthCheckResponse {
	if x != nil {
		return x.Components
	}
	return nil
}

func (x *HealthCheckResponse) GetDuration() *durationpb.Duration {
	if x != nil {
		return x.Duration
	}
	return nil
}

var File_proto_platform_health_proto protoreflect.FileDescriptor

var file_proto_platform_health_proto_rawDesc = []byte{
	0x0a, 0x1b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d,
	0x5f, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x12, 0x70,
	0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x5f, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x2e, 0x76,
	0x31, 0x1a, 0x19, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2f, 0x61, 0x6e, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x75,
	0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x46, 0x0a, 0x12,
	0x48, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74,
	0x12, 0x12, 0x0a, 0x04, 0x68, 0x6f, 0x70, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x04,
	0x68, 0x6f, 0x70, 0x73, 0x22, 0xe9, 0x02, 0x0a, 0x13, 0x48, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x43,
	0x68, 0x65, 0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x12, 0x0a, 0x04,
	0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1f, 0x0a, 0x08, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x49, 0x64,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x48, 0x00, 0x52, 0x08, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x49, 0x64, 0x88, 0x01, 0x01, 0x12, 0x32, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x1a, 0x2e, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d,
	0x5f, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x12, 0x2e, 0x0a, 0x07, 0x64, 0x65, 0x74, 0x61, 0x69, 0x6c, 0x73, 0x18, 0x06,
	0x20, 0x03, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x41, 0x6e, 0x79, 0x52, 0x07, 0x64, 0x65, 0x74, 0x61,
	0x69, 0x6c, 0x73, 0x12, 0x47, 0x0a, 0x0a, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74,
	0x73, 0x18, 0x07, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x27, 0x2e, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f,
	0x72, 0x6d, 0x5f, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x2e, 0x76, 0x31, 0x2e, 0x48, 0x65, 0x61,
	0x6c, 0x74, 0x68, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x52, 0x0a, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74, 0x73, 0x12, 0x35, 0x0a, 0x08,
	0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x08, 0x64, 0x75, 0x72, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x42, 0x0b, 0x0a, 0x09, 0x5f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x49, 0x64,
	0x2a, 0x44, 0x0a, 0x06, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x0b, 0x0a, 0x07, 0x55, 0x4e,
	0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x10, 0x00, 0x12, 0x0b, 0x0a, 0x07, 0x48, 0x45, 0x41, 0x4c, 0x54,
	0x48, 0x59, 0x10, 0x01, 0x12, 0x0d, 0x0a, 0x09, 0x55, 0x4e, 0x48, 0x45, 0x41, 0x4c, 0x54, 0x48,
	0x59, 0x10, 0x02, 0x12, 0x11, 0x0a, 0x0d, 0x4c, 0x4f, 0x4f, 0x50, 0x5f, 0x44, 0x45, 0x54, 0x45,
	0x43, 0x54, 0x45, 0x44, 0x10, 0x03, 0x32, 0x64, 0x0a, 0x06, 0x48, 0x65, 0x61, 0x6c, 0x74, 0x68,
	0x12, 0x5a, 0x0a, 0x05, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x12, 0x26, 0x2e, 0x70, 0x6c, 0x61, 0x74,
	0x66, 0x6f, 0x72, 0x6d, 0x5f, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x2e, 0x76, 0x31, 0x2e, 0x48,
	0x65, 0x61, 0x6c, 0x74, 0x68, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x27, 0x2e, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x5f, 0x68, 0x65, 0x61,
	0x6c, 0x74, 0x68, 0x2e, 0x76, 0x31, 0x2e, 0x48, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x43, 0x68, 0x65,
	0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x39, 0x5a, 0x37,
	0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x69, 0x73, 0x6f, 0x6d, 0x65,
	0x74, 0x72, 0x79, 0x2f, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2d, 0x68, 0x65, 0x61,
	0x6c, 0x74, 0x68, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d,
	0x5f, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_proto_platform_health_proto_rawDescOnce sync.Once
	file_proto_platform_health_proto_rawDescData = file_proto_platform_health_proto_rawDesc
)

func file_proto_platform_health_proto_rawDescGZIP() []byte {
	file_proto_platform_health_proto_rawDescOnce.Do(func() {
		file_proto_platform_health_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_platform_health_proto_rawDescData)
	})
	return file_proto_platform_health_proto_rawDescData
}

var file_proto_platform_health_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_proto_platform_health_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_proto_platform_health_proto_goTypes = []any{
	(Status)(0),                 // 0: platform_health.v1.Status
	(*HealthCheckRequest)(nil),  // 1: platform_health.v1.HealthCheckRequest
	(*HealthCheckResponse)(nil), // 2: platform_health.v1.HealthCheckResponse
	(*anypb.Any)(nil),           // 3: google.protobuf.Any
	(*durationpb.Duration)(nil), // 4: google.protobuf.Duration
}
var file_proto_platform_health_proto_depIdxs = []int32{
	0, // 0: platform_health.v1.HealthCheckResponse.status:type_name -> platform_health.v1.Status
	3, // 1: platform_health.v1.HealthCheckResponse.details:type_name -> google.protobuf.Any
	2, // 2: platform_health.v1.HealthCheckResponse.components:type_name -> platform_health.v1.HealthCheckResponse
	4, // 3: platform_health.v1.HealthCheckResponse.duration:type_name -> google.protobuf.Duration
	1, // 4: platform_health.v1.Health.Check:input_type -> platform_health.v1.HealthCheckRequest
	2, // 5: platform_health.v1.Health.Check:output_type -> platform_health.v1.HealthCheckResponse
	5, // [5:6] is the sub-list for method output_type
	4, // [4:5] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_proto_platform_health_proto_init() }
func file_proto_platform_health_proto_init() {
	if File_proto_platform_health_proto != nil {
		return
	}
	file_proto_platform_health_proto_msgTypes[1].OneofWrappers = []any{}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_proto_platform_health_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_platform_health_proto_goTypes,
		DependencyIndexes: file_proto_platform_health_proto_depIdxs,
		EnumInfos:         file_proto_platform_health_proto_enumTypes,
		MessageInfos:      file_proto_platform_health_proto_msgTypes,
	}.Build()
	File_proto_platform_health_proto = out.File
	file_proto_platform_health_proto_rawDesc = nil
	file_proto_platform_health_proto_goTypes = nil
	file_proto_platform_health_proto_depIdxs = nil
}
