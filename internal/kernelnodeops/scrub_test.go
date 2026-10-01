package kernelnodeops

import (
	"context"
	"fmt"
	"strings"
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
)

// secretKeys are the keys IsNodeSecretKey marks, as nodes, NodeX and Ansible
// write them: every name its documentation and tests list, in the spellings
// configurations use.
var secretKeys = []string{
	"private_key", "privateKey", "PrivateKey", "private-key", "server_private_key", "serverPrivateKey", "preshared_key",
	"presharedKey", "server_key", "key", "keys", "api_key", "apiKey", "x-api-key", "X-Api-Key", "secret_key",
	"password", "Password", "obfs-password", "obfs_password", "passwd", "pass", "auth", "auth_str", "authStr",
	"psk", "seed", "token", "api_token", "apiToken", "access_token", "Token", "secret", "client_secret",
	"shared_secret", "credential", "credentials", "registration_key", "node_token",
}

// textForms embed key and value the ways free text carries them.
var textForms = []func(key, value string) string{
	func(key, value string) string { return fmt.Sprintf(`{"%s":"%s"}`, key, value) },
	func(key, value string) string { return fmt.Sprintf(`{"outer":{"%s":"%s","port":443}}`, key, value) },
	func(key, value string) string { return fmt.Sprintf(`[{"%s":["%s"]}]`, key, value) },
	func(key, value string) string {
		return fmt.Sprintf(`nodex answered 401: {"%s": "%s", "ok": false}`, key, value)
	},
	func(key, value string) string { return fmt.Sprintf(`{"message":"bad %s=%s"}`, key, value) },
	func(key, value string) string { return fmt.Sprintf("GET /api?%s=%s&page=1 failed", key, value) },
	func(key, value string) string { return fmt.Sprintf("%s: %s", key, value) },
	func(key, value string) string { return fmt.Sprintf("ansible: %s = '%s' rejected", key, value) },
	func(key, value string) string { return fmt.Sprintf(`yaml error near %s: "%s"`, key, value) },
}

// resultCases are a value of every OperationResult case, so the walk below
// reaches every result type the contract has.
func resultCases(t *testing.T) []*kernelnodeopsv1.OperationResult {
	t.Helper()
	oneof := (&kernelnodeopsv1.OperationResult{}).ProtoReflect().Descriptor().Oneofs().ByName("result")
	var cases []*kernelnodeopsv1.OperationResult
	for i := 0; i < oneof.Fields().Len(); i++ {
		result := &kernelnodeopsv1.OperationResult{}
		field := oneof.Fields().Get(i)
		result.ProtoReflect().Set(field, result.ProtoReflect().NewField(field))
		cases = append(cases, result)
	}
	require.Len(t, cases, 12, "a result type was added: it is covered by this walk")
	return cases
}

// textField is one string or bytes field reached from a result.
type textField struct {
	path string
	set  func(text string)
}

// textFields sets every string and bytes field of message (and of the
// messages it holds, one level of each list) and returns setters for them.
func textFields(message protoreflect.Message, path string) []textField {
	var fields []textField
	descriptor := message.Descriptor()
	for i := 0; i < descriptor.Fields().Len(); i++ {
		field := descriptor.Fields().Get(i)
		name := path + "." + string(field.Name())
		switch {
		case field.Kind() == protoreflect.MessageKind && field.Message().FullName() == "anixops.kernelnodeops.v1.SecretHandle":
			// Handles are opaque and expanded by the gateway only.
		case field.IsList() && field.Kind() == protoreflect.MessageKind:
			list := message.Mutable(field).List()
			element := list.NewElement()
			list.Append(element)
			fields = append(fields, textFields(element.Message(), name+"[]")...)
		case field.Kind() == protoreflect.MessageKind:
			fields = append(fields, textFields(message.Mutable(field).Message(), name)...)
		case field.IsList() && (field.Kind() == protoreflect.StringKind || field.Kind() == protoreflect.BytesKind):
			fields = append(fields, textField{path: name + "[]", set: func(text string) {
				list := message.Mutable(field).List()
				if field.Kind() == protoreflect.StringKind {
					list.Append(protoreflect.ValueOfString(text))
				} else {
					list.Append(protoreflect.ValueOfBytes([]byte(text)))
				}
			}})
		case field.Kind() == protoreflect.StringKind:
			fields = append(fields, textField{path: name, set: func(text string) { message.Set(field, protoreflect.ValueOfString(text)) }})
		case field.Kind() == protoreflect.BytesKind:
			fields = append(fields, textField{path: name, set: func(text string) { message.Set(field, protoreflect.ValueOfBytes([]byte(text))) }})
		}
	}
	return fields
}

// resultFields returns the text fields of a result's case.
func resultFields(result *kernelnodeopsv1.OperationResult) []textField {
	message := result.ProtoReflect()
	field := message.WhichOneof(message.Descriptor().Oneofs().ByName("result"))
	return textFields(message.Mutable(field).Message(), string(field.Name()))
}

// Results never carry a secret: every key IsNodeSecretKey matches, in every
// form free text carries it, in every string and bytes field of every result
// type, is scrubbed; so is every credential the operation used, wherever it
// appears.
func TestResultsNeverCarrySecrets(t *testing.T) {
	for _, key := range secretKeys {
		require.True(t, service.IsNodeSecretKey(key), "%s is a secret key", key)
	}
	checked, withText := 0, 0
	for _, template := range resultCases(t) {
		// Some types hold no text at all (RetireResult, NodeStatsResult,
		// CredentialResult outside its handles): nothing to scrub there.
		fields := resultFields(proto.Clone(template).(*kernelnodeopsv1.OperationResult))
		if len(fields) > 0 {
			withText++
		}
		for fieldIndex := range fields {
			for keyIndex, key := range secretKeys {
				for formIndex, form := range textForms {
					value := fmt.Sprintf("s3cr3t-%d-%d-%d", fieldIndex, keyIndex, formIndex)
					result := proto.Clone(template).(*kernelnodeopsv1.OperationResult)
					resultFields(result)[fieldIndex].set(form(key, value))
					scrubbed := scrubResult(result, nil)
					encoded, err := protojson.Marshal(scrubbed)
					require.NoError(t, err)
					wire, err := proto.Marshal(scrubbed)
					require.NoError(t, err)
					require.NotContains(t, string(encoded), value, "%s, key %q, form %d", fields[fieldIndex].path, key, formIndex)
					require.NotContains(t, string(wire), value, "%s, key %q, form %d", fields[fieldIndex].path, key, formIndex)
					checked++
				}
			}
			// A credential the operation used is scrubbed by value, as
			// written and encoded, whatever surrounds it.
			const used = "tok/en+val ue=9"
			for _, text := range []string{"failed with " + used, `{"detail":"` + used + `"}`, "url?x=tok%2Fen%2Bval+ue%3D9", "GET /nodes/tok%2Fen+val%20ue=9/rules"} {
				result := proto.Clone(template).(*kernelnodeopsv1.OperationResult)
				resultFields(result)[fieldIndex].set(text)
				encoded, err := protojson.Marshal(scrubResult(result, []string{used}))
				require.NoError(t, err)
				for _, form := range []string{used, "tok%2Fen%2Bval+ue%3D9", "tok%2Fen+val%20ue=9"} {
					require.NotContains(t, string(encoded), form, fields[fieldIndex].path)
				}
			}
		}
	}
	require.Equal(t, 9, withText)
	require.Greater(t, checked, 5000)
}

// Scrubbing keeps what is not a secret: public keys, short ids, counters,
// messages and the placeholder itself.
func TestScrubbingKeepsWhatIsNotASecret(t *testing.T) {
	for _, text := range []string{
		`{"public_key":"pub","short_id":"ab12","port":443}`,
		"forward 40 applied on node relay (198.51.100.10:8080)",
		"token: ********", "password=", `{"password":""}`, "authority=example", "key_file=/etc/key.pem",
	} {
		require.Equal(t, text, scrubText(text, nil), text)
	}
	require.Equal(t, `{"private_key":"********","public_key":"pub"}`, scrubText(`{"private_key":"k","public_key":"pub"}`, nil))
	require.Equal(t, "Authorization: Bearer ********", scrubText("Authorization: Bearer abcdef123456", nil))
	require.Equal(t, "dial https://admin:********@198.51.100.1/api", scrubText("dial https://admin:pa55word@198.51.100.1/api", nil))
	require.Equal(t, "short abc", scrubText("short abc", []string{"abc"}), "values under 4 bytes are not scrubbed by value")
}

// The engine scrubs a result, its error and the evidence before it stores
// them, so GetOperation and the ledger never hold the credential the
// executor used.
func TestTheEngineStoresScrubbedOutcomes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		const token = "forward-node-token-77"
		h.serve(KindDiagnoseForward, ExecutorFunc(func(_ context.Context, run *Run) Outcome {
			run.UseSecret(token)
			return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Diagnosis{Diagnosis: &kernelnodeopsv1.DiagnosisResult{
				Outcomes: []*kernelnodeopsv1.DiagnosisOutcome{{
					Description: "gost api", Message: "401 for token " + token, NodeName: `{"api_token":"other-secret"}`,
				}},
			}}})
		}))
		h.serve(KindDiagnoseTunnel, ExecutorFunc(func(_ context.Context, run *Run) Outcome {
			run.UseSecret(token)
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, "nodex: "+token+" refused; password=hunter22", true)
		}))
		h.start(t)
		client := h.client(forwardHost, allFamilies())
		diagnosis := submit(t, client, "diag:forward", sampleOperation(KindDiagnoseForward)).GetOperation()
		tunnel := submit(t, client, "diag:tunnel", sampleOperation(KindDiagnoseTunnel)).GetOperation()
		ended := eventually(t, get(t, client, diagnosis.GetOperationId()), inState(succeeded))
		outcome := ended.GetResult().GetDiagnosis().GetOutcomes()[0]
		require.Equal(t, "401 for token ********", outcome.GetMessage())
		require.Equal(t, `{"api_token":"********"}`, outcome.GetNodeName())
		failedTunnel := eventually(t, get(t, client, tunnel.GetOperationId()), inState(failed))
		require.Equal(t, "nodex: ******** refused; password=********", failedTunnel.GetError().GetMessage())
		for _, row := range h.rows(t) {
			for _, text := range []string{row.Result, row.ErrorMessage, row.Evidence, row.Operation} {
				require.False(t, strings.Contains(text, token) || strings.Contains(text, "other-secret") || strings.Contains(text, "hunter22"))
			}
		}
	})
}
