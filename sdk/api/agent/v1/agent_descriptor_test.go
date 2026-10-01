package agentv1pb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	controlSDKGoPackage   = "github.com/AnixOps/anix-control/sdk/api/agent/v1;agentv1pb"
	agentSDKV110GoPackage = "github.com/AnixOps/anix-agent/sdk/api/grpc/agent/v1;agentv1pb"
	// agentSDKV110DescriptorSHA256 is the SHA-256 of the serialized file
	// descriptor embedded in agent.pb.go of github.com/AnixOps/anix-agent/sdk
	// v1.1.0, the contract Agents in the field run.
	agentSDKV110DescriptorSHA256 = "2717310982eb0074cf0e7817300cb1a1db54f455eca017c9db5b1cc9ead2f203"
	// externalDescriptorEnv names a file holding a serialized
	// FileDescriptorProto of anix.agent.v1 from another build of the contract.
	// The SDK Sync workflow points it at anix-agent's own SDK.
	externalDescriptorEnv = "ANIXOPS_AGENT_SDK_DESCRIPTOR"
)

func TestAgentControlServiceDescriptor(t *testing.T) {
	if got := string(File_api_grpc_agent_v1_agent_proto.Package()); got != "anix.agent.v1" {
		t.Fatalf("package = %q, want anix.agent.v1", got)
	}

	service := File_api_grpc_agent_v1_agent_proto.Services().ByName("AgentControlService")
	if service == nil || service.Methods().ByName("ControlStream") == nil {
		t.Fatal("AgentControlService.ControlStream descriptor is missing")
	}
}

// TestDescriptorExtendsAgentSDKV110 proves Agents in the field keep working:
// the file descriptor is a superset of the one github.com/AnixOps/anix-agent/sdk
// v1.1.0 compiles in. Every v1.1.0 message, field, enum value and service
// method is still there unchanged; the contract only adds to it.
func TestDescriptorExtendsAgentSDKV110(t *testing.T) {
	v110 := agentSDKV110Descriptor(t)
	if got := v110.GetOptions().GetGoPackage(); got != agentSDKV110GoPackage {
		t.Fatalf("v1.1.0 go_package = %q, want %q", got, agentSDKV110GoPackage)
	}

	current := currentDescriptor()
	if got := current.GetOptions().GetGoPackage(); got != controlSDKGoPackage {
		t.Fatalf("go_package = %q, want %q", got, controlSDKGoPackage)
	}
	requireSuperset(t, v110, current)
}

// TestDescriptorExtendsExternalAgentSDK compares against the descriptor named
// by ANIXOPS_AGENT_SDK_DESCRIPTOR, for example the one anix-agent's own SDK
// compiles in while anix-agent still depends on it.
func TestDescriptorExtendsExternalAgentSDK(t *testing.T) {
	path := os.Getenv(externalDescriptorEnv)
	if path == "" {
		t.Skip(externalDescriptorEnv + " is not set")
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	external := &descriptorpb.FileDescriptorProto{}
	if err := proto.Unmarshal(raw, external); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	requireSuperset(t, external, currentDescriptor())
}

func TestSupersetCheckAllowsAdditions(t *testing.T) {
	v110 := agentSDKV110Descriptor(t)
	if problems := descriptorRegressions(v110, v110); len(problems) != 0 {
		t.Fatalf("v1.1.0 against itself: %v", problems)
	}

	current := proto.Clone(currentDescriptor()).(*descriptorpb.FileDescriptorProto)
	current.Options.GoPackage = proto.String("example.com/other;other")
	hello := messageNamed(t, current, "Hello")
	hello.Field = append(hello.Field, &descriptorpb.FieldDescriptorProto{
		Name: proto.String("added"), Number: proto.Int32(99), JsonName: proto.String("added"),
		Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
	})
	current.MessageType = append(current.MessageType, &descriptorpb.DescriptorProto{Name: proto.String("Added")})
	phase := current.EnumType[0]
	phase.Value = append(phase.Value, &descriptorpb.EnumValueDescriptorProto{Name: proto.String("OBSERVED_PHASE_ADDED"), Number: proto.Int32(99)})
	service := current.Service[0]
	service.Method = append(service.Method, &descriptorpb.MethodDescriptorProto{
		Name: proto.String("Added"), InputType: proto.String(".anix.agent.v1.Hello"), OutputType: proto.String(".anix.agent.v1.HelloAck"),
	})
	if problems := descriptorRegressions(v110, current); len(problems) != 0 {
		t.Fatalf("additions and a go_package change must pass: %v", problems)
	}
}

// TestSupersetCheckRejectsBreakingChanges edits the current descriptor the
// ways that break Agents built against v1.1.0, and requires each to fail.
func TestSupersetCheckRejectsBreakingChanges(t *testing.T) {
	tests := []struct {
		name string
		edit func(t *testing.T, file *descriptorpb.FileDescriptorProto)
		want string
	}{
		{
			name: "field removed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				hello := messageNamed(t, file, "Hello")
				hello.Field = slices.DeleteFunc(hello.Field, func(field *descriptorpb.FieldDescriptorProto) bool {
					return field.GetName() == "labels"
				})
			},
			want: "field anix.agent.v1.Hello.labels = 5 was removed or renamed",
		},
		{
			name: "field renumbered",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				fieldNamed(t, messageNamed(t, file, "Hello"), "protocol").Number = proto.Int32(99)
			},
			want: "field anix.agent.v1.Hello.protocol changed",
		},
		{
			name: "field renamed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				fieldNamed(t, messageNamed(t, file, "Hello"), "instance_id").Name = proto.String("instance")
			},
			want: "field anix.agent.v1.Hello.instance_id = 3 was removed or renamed",
		},
		{
			name: "field type changed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				fieldNamed(t, messageNamed(t, file, "Heartbeat"), "uptime_seconds").Type = descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum()
			},
			want: "field anix.agent.v1.Heartbeat.uptime_seconds changed",
		},
		{
			name: "field label changed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				fieldNamed(t, messageNamed(t, file, "Hello"), "capabilities").Label = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
			},
			want: "field anix.agent.v1.Hello.capabilities changed",
		},
		{
			name: "field message type changed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				fieldNamed(t, messageNamed(t, file, "AgentToControl"), "hello").TypeName = proto.String(".anix.agent.v1.Heartbeat")
			},
			want: "field anix.agent.v1.AgentToControl.hello changed",
		},
		{
			name: "field moved out of its oneof",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				fieldNamed(t, messageNamed(t, file, "ControlToAgent"), "hello_ack").OneofIndex = nil
			},
			want: `field anix.agent.v1.ControlToAgent.hello_ack moved from oneof "payload" to ""`,
		},
		{
			name: "map entry changed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				entry := messageNamed(t, file, "Hello").NestedType[0]
				fieldNamed(t, entry, "value").Type = descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum()
			},
			want: "field anix.agent.v1.Hello.LabelsEntry.value changed",
		},
		{
			name: "message removed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				file.MessageType = slices.DeleteFunc(file.MessageType, func(message *descriptorpb.DescriptorProto) bool {
					return message.GetName() == "PluginRuleCounter"
				})
			},
			want: "message anix.agent.v1.PluginRuleCounter was removed",
		},
		{
			name: "enum value removed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				file.EnumType[0].Value = file.EnumType[0].Value[:len(file.EnumType[0].Value)-1]
			},
			want: "enum value anix.agent.v1.ObservedPhase.OBSERVED_PHASE_SUPERSEDED was removed or renamed",
		},
		{
			name: "enum value renumbered",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				file.EnumType[0].Value[1].Number = proto.Int32(9)
			},
			want: "enum value anix.agent.v1.ObservedPhase.OBSERVED_PHASE_ACCEPTED changed",
		},
		{
			name: "method removed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				file.Service[0].Method = nil
			},
			want: "method anix.agent.v1.AgentControlService.ControlStream was removed",
		},
		{
			name: "method streaming changed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				file.Service[0].Method[0].ClientStreaming = proto.Bool(false)
			},
			want: "method anix.agent.v1.AgentControlService.ControlStream changed",
		},
		{
			name: "file renamed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				file.Name = proto.String("api/agent/v1/agent.proto")
			},
			want: `file name "api/grpc/agent/v1/agent.proto" changed`,
		},
		{
			name: "package renamed",
			edit: func(t *testing.T, file *descriptorpb.FileDescriptorProto) {
				file.Package = proto.String("anix.agent.v2")
			},
			want: `package "anix.agent.v1" changed`,
		},
	}
	v110 := agentSDKV110Descriptor(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := proto.Clone(currentDescriptor()).(*descriptorpb.FileDescriptorProto)
			test.edit(t, current)
			problems := descriptorRegressions(v110, current)
			if !slices.ContainsFunc(problems, func(problem string) bool { return strings.HasPrefix(problem, test.want) }) {
				t.Fatalf("problems = %q, want one starting with %q", problems, test.want)
			}
		})
	}
}

// TestAgentSDKV110ReadsTheGrownContract decodes messages of the current
// contract with the v1.1.0 descriptor, as an Agent in the field does: what it
// knew reads the same, and the additions are unknown fields it skips. A data
// payload is absent to it, which is why Control sends one only when the Agent
// advertises the capability.
func TestAgentSDKV110ReadsTheGrownContract(t *testing.T) {
	v110, err := protodesc.NewFile(agentSDKV110Descriptor(t), new(protoregistry.Files))
	if err != nil {
		t.Fatal(err)
	}
	decode := func(t *testing.T, message proto.Message) *dynamicpb.Message {
		t.Helper()
		raw, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		decoded := dynamicpb.NewMessage(v110.Messages().ByName(message.ProtoReflect().Descriptor().Name()))
		if err := proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}

	helloAck := decode(t, &ControlToAgent{RequestId: "hello", Payload: &ControlToAgent_HelloAck{HelloAck: &HelloAck{
		SessionId:          "session-1",
		ServerCapabilities: []*Capability{{Name: "reports", Version: "v1"}},
	}}})
	ack := helloAck.Get(helloAck.Descriptor().Fields().ByName("hello_ack")).Message()
	if got := ack.Get(ack.Descriptor().Fields().ByName("session_id")).String(); got != "session-1" {
		t.Fatalf("v1.1.0 HelloAck.session_id = %q, want session-1", got)
	}
	if len(ack.GetUnknown()) == 0 {
		t.Fatal("v1.1.0 must see server_capabilities as an unknown field")
	}

	hello := decode(t, &Hello{Protocol: "anix.agent.v1", ConfigRevision: 7, UsersCursor: 9})
	if got := hello.Get(hello.Descriptor().Fields().ByName("protocol")).String(); got != "anix.agent.v1" {
		t.Fatalf("v1.1.0 Hello.protocol = %q", got)
	}

	snapshot := decode(t, &ControlToAgent{RequestId: "config", Payload: &ControlToAgent_Config{Config: &ConfigSnapshot{ConfigRevision: 1}}})
	if which := snapshot.WhichOneof(snapshot.Descriptor().Oneofs().ByName("payload")); which != nil {
		t.Fatalf("v1.1.0 decoded the snapshot as %s", which.Name())
	}
	if got := snapshot.Get(snapshot.Descriptor().Fields().ByName("request_id")).String(); got != "config" {
		t.Fatalf("v1.1.0 ControlToAgent.request_id = %q", got)
	}
}

func agentSDKV110Descriptor(t *testing.T) *descriptorpb.FileDescriptorProto {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "agent-sdk-v1.1.0.descriptor.txtpb"))
	if err != nil {
		t.Fatal(err)
	}
	v110 := &descriptorpb.FileDescriptorProto{}
	if err := prototext.Unmarshal(raw, v110); err != nil {
		t.Fatalf("parse v1.1.0 descriptor: %v", err)
	}
	sum := sha256.Sum256(marshalDescriptor(t, v110))
	if got := hex.EncodeToString(sum[:]); got != agentSDKV110DescriptorSHA256 {
		t.Fatalf("testdata descriptor SHA-256 = %s, want %s: it is no longer the v1.1.0 descriptor", got, agentSDKV110DescriptorSHA256)
	}
	return v110
}

func currentDescriptor() *descriptorpb.FileDescriptorProto {
	return protodesc.ToFileDescriptorProto(File_api_grpc_agent_v1_agent_proto)
}

func requireSuperset(t *testing.T, base, current *descriptorpb.FileDescriptorProto) {
	t.Helper()
	if problems := descriptorRegressions(base, current); len(problems) != 0 {
		t.Fatalf("anix.agent.v1 descriptor is no longer a superset of the base contract:\n%s", strings.Join(problems, "\n"))
	}
}

// descriptorRegressions lists what current removed or changed from base. Only
// additions and a go_package change are allowed: every base message, field
// (name, number, label, type, type name, JSON name, options, oneof), enum
// value, service and method must be in current unchanged.
func descriptorRegressions(base, current *descriptorpb.FileDescriptorProto) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if base.GetName() != current.GetName() {
		report("file name %q changed to %q", base.GetName(), current.GetName())
	}
	if base.GetPackage() != current.GetPackage() {
		report("package %q changed to %q", base.GetPackage(), current.GetPackage())
	}
	if base.GetSyntax() != current.GetSyntax() {
		report("syntax %q changed to %q", base.GetSyntax(), current.GetSyntax())
	}
	if !proto.Equal(fileOptionsWithoutGoPackage(base), fileOptionsWithoutGoPackage(current)) {
		report("file options changed beyond go_package")
	}
	for _, dependency := range base.GetDependency() {
		if !slices.Contains(current.GetDependency(), dependency) {
			report("import %q was removed", dependency)
		}
	}
	scope := base.GetPackage()
	messageRegressions(report, scope, base.GetMessageType(), current.GetMessageType())
	enumRegressions(report, scope, base.GetEnumType(), current.GetEnumType())
	for _, service := range base.GetService() {
		name := scope + "." + service.GetName()
		got := named(current.GetService(), service.GetName())
		if got == nil {
			report("service %s was removed", name)
			continue
		}
		if !proto.Equal(service.GetOptions(), got.GetOptions()) {
			report("service %s options changed", name)
		}
		for _, method := range service.GetMethod() {
			gotMethod := named(got.GetMethod(), method.GetName())
			switch {
			case gotMethod == nil:
				report("method %s.%s was removed", name, method.GetName())
			case !proto.Equal(method, gotMethod):
				report("method %s.%s changed: %s -> %s", name, method.GetName(), compact(method), compact(gotMethod))
			}
		}
	}
	return problems
}

func messageRegressions(report func(string, ...any), scope string, base, current []*descriptorpb.DescriptorProto) {
	for _, message := range base {
		name := scope + "." + message.GetName()
		got := named(current, message.GetName())
		if got == nil {
			report("message %s was removed", name)
			continue
		}
		if !proto.Equal(message.GetOptions(), got.GetOptions()) {
			report("message %s options changed", name)
		}
		for _, field := range message.GetField() {
			gotField := named(got.GetField(), field.GetName())
			if gotField == nil {
				report("field %s.%s = %d was removed or renamed", name, field.GetName(), field.GetNumber())
				continue
			}
			if want, have := withoutOneofIndex(field), withoutOneofIndex(gotField); !proto.Equal(want, have) {
				report("field %s.%s changed: %s -> %s", name, field.GetName(), compact(want), compact(have))
			}
			if want, have := oneofName(message, field), oneofName(got, gotField); want != have {
				report("field %s.%s moved from oneof %q to %q", name, field.GetName(), want, have)
			}
		}
		for _, reserved := range message.GetReservedRange() {
			if !slices.ContainsFunc(got.GetReservedRange(), func(other *descriptorpb.DescriptorProto_ReservedRange) bool {
				return proto.Equal(reserved, other)
			}) {
				report("message %s no longer reserves %d to %d", name, reserved.GetStart(), reserved.GetEnd())
			}
		}
		for _, reserved := range message.GetReservedName() {
			if !slices.Contains(got.GetReservedName(), reserved) {
				report("message %s no longer reserves %q", name, reserved)
			}
		}
		messageRegressions(report, name, message.GetNestedType(), got.GetNestedType())
		enumRegressions(report, name, message.GetEnumType(), got.GetEnumType())
	}
}

func enumRegressions(report func(string, ...any), scope string, base, current []*descriptorpb.EnumDescriptorProto) {
	for _, enum := range base {
		name := scope + "." + enum.GetName()
		got := named(current, enum.GetName())
		if got == nil {
			report("enum %s was removed", name)
			continue
		}
		if !proto.Equal(enum.GetOptions(), got.GetOptions()) {
			report("enum %s options changed", name)
		}
		for _, value := range enum.GetValue() {
			gotValue := named(got.GetValue(), value.GetName())
			switch {
			case gotValue == nil:
				report("enum value %s.%s was removed or renamed", name, value.GetName())
			case !proto.Equal(value, gotValue):
				report("enum value %s.%s changed: %s -> %s", name, value.GetName(), compact(value), compact(gotValue))
			}
		}
	}
}

func named[T interface{ GetName() string }](elements []T, name string) T {
	for _, element := range elements {
		if element.GetName() == name {
			return element
		}
	}
	var zero T
	return zero
}

// withoutOneofIndex drops the oneof's position, which additions may shift;
// oneofName compares the oneof itself.
func withoutOneofIndex(field *descriptorpb.FieldDescriptorProto) *descriptorpb.FieldDescriptorProto {
	clone := proto.Clone(field).(*descriptorpb.FieldDescriptorProto)
	clone.OneofIndex = nil
	return clone
}

func oneofName(message *descriptorpb.DescriptorProto, field *descriptorpb.FieldDescriptorProto) string {
	if field.OneofIndex == nil {
		return ""
	}
	index := int(field.GetOneofIndex())
	if index < 0 || index >= len(message.GetOneofDecl()) {
		return fmt.Sprintf("#%d", index)
	}
	return message.GetOneofDecl()[index].GetName()
}

// fileOptionsWithoutGoPackage returns the file options with go_package
// cleared. Options left empty compare equal to no options.
func fileOptionsWithoutGoPackage(file *descriptorpb.FileDescriptorProto) *descriptorpb.FileOptions {
	if file.GetOptions() == nil {
		return &descriptorpb.FileOptions{}
	}
	options := proto.Clone(file.GetOptions()).(*descriptorpb.FileOptions)
	options.GoPackage = nil
	return options
}

func compact(message proto.Message) string {
	return prototext.MarshalOptions{}.Format(message)
}

func messageNamed(t *testing.T, file *descriptorpb.FileDescriptorProto, name string) *descriptorpb.DescriptorProto {
	t.Helper()
	message := named(file.GetMessageType(), name)
	if message == nil {
		t.Fatalf("message %s is missing", name)
	}
	return message
}

func fieldNamed(t *testing.T, message *descriptorpb.DescriptorProto, name string) *descriptorpb.FieldDescriptorProto {
	t.Helper()
	field := named(message.GetField(), name)
	if field == nil {
		t.Fatalf("field %s.%s is missing", message.GetName(), name)
	}
	return field
}

func marshalDescriptor(t *testing.T, file *descriptorpb.FileDescriptorProto) []byte {
	t.Helper()
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
