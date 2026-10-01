package kernelnodeops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// canonical returns the operation as the ledger identifies and stores it: a
// copy in which every sealed handle is reduced to SealedPrefix. A handle is
// minted per request, so a retry of the same intent carries other handles;
// they must not change the digest. The canonical operation never holds a
// secret: handles are not secrets, and the kinds' checks refuse a secret in
// clear.
func canonical(spec *kernelnodeopsv1.OperationSpec) *kernelnodeopsv1.OperationSpec {
	copied := proto.Clone(spec).(*kernelnodeopsv1.OperationSpec)
	switch op := copied.GetOperation().(type) {
	case *kernelnodeopsv1.OperationSpec_IssueCredential:
		if value := op.IssueCredential.GetValue(); value != nil && strings.HasPrefix(value.GetHandle(), SealedPrefix) {
			value.Handle = SealedPrefix
		}
	case *kernelnodeopsv1.OperationSpec_PutSecretDocument:
		op.PutSecretDocument.DocumentJson = canonicalDocument(op.PutSecretDocument.GetDocumentJson())
	case *kernelnodeopsv1.OperationSpec_TestForwardBackend:
		if token := op.TestForwardBackend.GetToken(); token != nil && strings.HasPrefix(token.GetHandle(), SealedPrefix) {
			token.Handle = SealedPrefix
		}
	}
	return copied
}

// canonicalDocument re-encodes a JSON document with sorted keys and every
// sealed handle reduced to SealedPrefix. A document that is not JSON is
// returned as it is (the kinds' checks refuse it first).
func canonicalDocument(raw []byte) []byte {
	value, err := decodeJSON(raw)
	if err != nil {
		return raw
	}
	value = stripHandles(value)
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return raw
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
}

func stripHandles(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			typed[key] = stripHandles(item)
		}
	case []any:
		for index, item := range typed {
			typed[index] = stripHandles(item)
		}
	case string:
		if strings.HasPrefix(typed, SealedPrefix) {
			return SealedPrefix
		}
	}
	return value
}

// digest identifies an operation: the SHA-256 of its kind and its canonical
// form, deterministically encoded.
func digest(kindName string, spec *kernelnodeopsv1.OperationSpec) (string, error) {
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(spec)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write([]byte(kindName))
	hash.Write([]byte{0})
	hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hasUnknownFields reports whether a message, or one it holds, carries
// fields this kernel does not know: an option a newer contract added that
// this kernel would otherwise silently ignore.
func hasUnknownFields(message protoreflect.Message) bool {
	if len(message.GetUnknown()) > 0 {
		return true
	}
	unknown := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsList() && field.Message() != nil:
			list := value.List()
			for i := 0; i < list.Len() && !unknown; i++ {
				unknown = hasUnknownFields(list.Get(i).Message())
			}
		case field.IsMap():
		case field.Message() != nil:
			unknown = hasUnknownFields(value.Message())
		}
		return !unknown
	})
	return unknown
}

var (
	storeJSON = protojson.MarshalOptions{UseProtoNames: true}
	loadJSON  = protojson.UnmarshalOptions{DiscardUnknown: true}
)

func encodeOperation(spec *kernelnodeopsv1.OperationSpec) (string, error) {
	encoded, err := storeJSON.Marshal(spec)
	return string(encoded), err
}

func decodeOperation(stored string) (*kernelnodeopsv1.OperationSpec, error) {
	spec := &kernelnodeopsv1.OperationSpec{}
	if err := loadJSON.Unmarshal([]byte(stored), spec); err != nil {
		return nil, err
	}
	return spec, nil
}
