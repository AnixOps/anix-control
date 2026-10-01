package agentv1pb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
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

// TestDescriptorMatchesAgentSDKV110 proves the contract moved here without a
// wire change: apart from go_package, the file descriptor is identical to the
// one github.com/AnixOps/anix-agent/sdk v1.1.0 compiles in.
func TestDescriptorMatchesAgentSDKV110(t *testing.T) {
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
	if got := v110.GetOptions().GetGoPackage(); got != agentSDKV110GoPackage {
		t.Fatalf("v1.1.0 go_package = %q, want %q", got, agentSDKV110GoPackage)
	}

	current := currentDescriptor()
	if got := current.GetOptions().GetGoPackage(); got != controlSDKGoPackage {
		t.Fatalf("go_package = %q, want %q", got, controlSDKGoPackage)
	}
	requireSameExceptGoPackage(t, v110, current)
}

// TestDescriptorMatchesExternalAgentSDK compares against the descriptor named
// by ANIXOPS_AGENT_SDK_DESCRIPTOR, for example the one anix-agent's own SDK
// compiles in while anix-agent still depends on it.
func TestDescriptorMatchesExternalAgentSDK(t *testing.T) {
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
	requireSameExceptGoPackage(t, external, currentDescriptor())
}

func TestWithoutGoPackageKeepsEverythingElse(t *testing.T) {
	base := currentDescriptor()
	renamedPackage := proto.Clone(base).(*descriptorpb.FileDescriptorProto)
	renamedPackage.Options.GoPackage = proto.String("example.com/other;other")
	if !bytes.Equal(withoutGoPackage(t, base), withoutGoPackage(t, renamedPackage)) {
		t.Fatal("a go_package change must be ignored")
	}

	renumbered := proto.Clone(base).(*descriptorpb.FileDescriptorProto)
	renumbered.MessageType[0].Field[0].Number = proto.Int32(99)
	if bytes.Equal(withoutGoPackage(t, base), withoutGoPackage(t, renumbered)) {
		t.Fatal("a field number change must be detected")
	}

	renamedFile := proto.Clone(base).(*descriptorpb.FileDescriptorProto)
	renamedFile.Name = proto.String("api/agent/v1/agent.proto")
	if bytes.Equal(withoutGoPackage(t, base), withoutGoPackage(t, renamedFile)) {
		t.Fatal("a file name change must be detected")
	}
}

func currentDescriptor() *descriptorpb.FileDescriptorProto {
	return protodesc.ToFileDescriptorProto(File_api_grpc_agent_v1_agent_proto)
}

func requireSameExceptGoPackage(t *testing.T, want, got *descriptorpb.FileDescriptorProto) {
	t.Helper()
	if !bytes.Equal(withoutGoPackage(t, want), withoutGoPackage(t, got)) {
		t.Fatalf("anix.agent.v1 descriptor changed beyond go_package\nwant:\n%s\ngot:\n%s",
			prototext.Format(want), prototext.Format(got))
	}
}

// withoutGoPackage serializes a copy of the descriptor with go_package cleared.
// An options message left empty is dropped, so "no options" and "only
// go_package" compare equal.
func withoutGoPackage(t *testing.T, file *descriptorpb.FileDescriptorProto) []byte {
	t.Helper()
	clone := proto.Clone(file).(*descriptorpb.FileDescriptorProto)
	if clone.Options != nil {
		clone.Options.GoPackage = nil
		if proto.Size(clone.Options) == 0 {
			clone.Options = nil
		}
	}
	return marshalDescriptor(t, clone)
}

func marshalDescriptor(t *testing.T, file *descriptorpb.FileDescriptorProto) []byte {
	t.Helper()
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
