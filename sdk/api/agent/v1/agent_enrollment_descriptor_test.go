package agentv1pb

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestAgentEnrollmentDescriptor pins where AgentEnrollment lives: a file of
// its own in package anix.agent.v1, so agent.proto keeps the v1.1.0
// descriptor (TestDescriptorExtendsAgentSDKV110).
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
	for _, method := range []string{"Enroll", "Join", "Renew", "GetTrustBundle", "IssueLinkCertificate", "GetLinkTrustBundle"} {
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

// TestAgentEnrollmentJoinDescriptor pins the Join RPC and its messages to the
// node onboarding design (docs/design/node-onboarding, "Join Protocol"),
// which the installer, the Agent's join subcommand and Control's Join server
// are built against.
func TestAgentEnrollmentJoinDescriptor(t *testing.T) {
	file := File_api_grpc_agent_v1_agent_enrollment_proto
	join := file.Services().ByName("AgentEnrollment").Methods().ByName("Join")
	if join == nil {
		t.Fatal("AgentEnrollment.Join is missing")
	}
	if got := join.Input().FullName(); got != "anix.agent.v1.JoinRequest" {
		t.Fatalf("Join input = %s, want anix.agent.v1.JoinRequest", got)
	}
	if got := join.Output().FullName(); got != "anix.agent.v1.JoinResponse" {
		t.Fatalf("Join output = %s, want anix.agent.v1.JoinResponse", got)
	}

	type field struct {
		number      protoreflect.FieldNumber
		kind        protoreflect.Kind
		cardinality protoreflect.Cardinality
	}
	single := func(number protoreflect.FieldNumber, kind protoreflect.Kind) field {
		return field{number: number, kind: kind, cardinality: protoreflect.Optional}
	}
	messages := map[protoreflect.Name]map[protoreflect.Name]field{
		"JoinRequest": {
			"csr_der":             single(1, protoreflect.BytesKind),
			"join_token":          single(2, protoreflect.StringKind),
			"machine_fingerprint": single(3, protoreflect.StringKind),
			"hostname":            single(4, protoreflect.StringKind),
			"requested_name":      single(5, protoreflect.StringKind),
			"connect_host":        single(6, protoreflect.StringKind),
			"public_addresses":    {number: 7, kind: protoreflect.StringKind, cardinality: protoreflect.Repeated},
			"arch":                single(8, protoreflect.StringKind),
			"os":                  single(9, protoreflect.StringKind),
			"agent_version":       single(10, protoreflect.StringKind),
			"instance_id":         single(11, protoreflect.StringKind),
			"reset":               single(12, protoreflect.BoolKind),
		},
		"JoinResponse": {
			"certificate": single(1, protoreflect.MessageKind),
			"node":        single(2, protoreflect.StringKind),
			"node_name":   single(3, protoreflect.StringKind),
			"group":       single(4, protoreflect.StringKind),
			"created":     single(5, protoreflect.BoolKind),
			"approval":    single(6, protoreflect.StringKind),
		},
	}
	for messageName, want := range messages {
		message := file.Messages().ByName(messageName)
		if message == nil {
			t.Fatalf("%s is missing", messageName)
		}
		if got := message.Fields().Len(); got != len(want) {
			t.Fatalf("%s has %d fields, want %d", messageName, got, len(want))
		}
		for fieldName, wantField := range want {
			descriptor := message.Fields().ByName(fieldName)
			if descriptor == nil {
				t.Fatalf("%s.%s is missing", messageName, fieldName)
			}
			got := field{number: descriptor.Number(), kind: descriptor.Kind(), cardinality: descriptor.Cardinality()}
			if got != wantField {
				t.Fatalf("%s.%s = %+v, want %+v", messageName, fieldName, got, wantField)
			}
			if descriptor.HasPresence() && descriptor.Kind() != protoreflect.MessageKind {
				t.Fatalf("%s.%s must not be an explicit-presence (optional) field", messageName, fieldName)
			}
		}
	}
	certificate := file.Messages().ByName("JoinResponse").Fields().ByName("certificate")
	if got := certificate.Message().FullName(); got != "anix.agent.v1.AgentCertificate" {
		t.Fatalf("JoinResponse.certificate = %s, want anix.agent.v1.AgentCertificate", got)
	}
}
