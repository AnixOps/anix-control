package agentv1pb

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestAgentEnrollmentDescriptor pins where AgentEnrollment lives: a file of
// its own in package anix.agent.v1, so agent.proto keeps the v1.1.0
// descriptor (TestDescriptorMatchesAgentSDKV110).
func TestAgentEnrollmentDescriptor(t *testing.T) {
	file := File_api_grpc_agent_v1_agent_enrollment_proto
	if got := file.Path(); got != "api/grpc/agent/v1/agent_enrollment.proto" {
		t.Fatalf("path = %q", got)
	}
	if got := string(file.Package()); got != "anix.agent.v1" {
		t.Fatalf("package = %q, want anix.agent.v1", got)
	}
	if file.Imports().Len() != 0 {
		t.Fatalf("agent_enrollment.proto imports %d files; it must stand alone", file.Imports().Len())
	}
	service := file.Services().ByName("AgentEnrollment")
	if service == nil {
		t.Fatal("AgentEnrollment service is missing")
	}
	for _, method := range []string{"Enroll", "Renew", "GetTrustBundle"} {
		descriptor := service.Methods().ByName(protoreflect.Name(method))
		if descriptor == nil {
			t.Fatalf("AgentEnrollment.%s is missing", method)
		}
		if descriptor.IsStreamingClient() || descriptor.IsStreamingServer() {
			t.Fatalf("AgentEnrollment.%s must be unary", method)
		}
	}
	if File_api_grpc_agent_v1_agent_proto.Services().ByName("AgentEnrollment") != nil {
		t.Fatal("AgentEnrollment must not be declared in agent.proto")
	}
}
