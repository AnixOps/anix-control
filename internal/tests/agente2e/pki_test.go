package agente2e

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testPKI is the material the suite generates for one run: the TLS
// certificate of Control's gRPC listener (the Agent trusts its CA through
// SSL_CERT_FILE), the key that seals Control's built-in agent CA, and the
// Ed25519 key that signs the test plugin packages (Control and the Agent
// are both configured with its public half as the "official" key).
type testPKI struct {
	ServerCAFile   string
	ServerCertFile string
	ServerKeyFile  string
	CAKEK          string

	SigningPrivateKeyPath string
	SigningPublicKeyPath  string
	SigningPublicKey      string
	SigningKey            ed25519.PrivateKey
}

func newTestPKI(t *testing.T, dir string) testPKI {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	out := testPKI{
		ServerCAFile:   filepath.Join(dir, "server-ca.pem"),
		ServerCertFile: filepath.Join(dir, "server.pem"),
		ServerKeyFile:  filepath.Join(dir, "server-key.pem"),
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "AnixOps agent E2E server CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	writePEM(t, out.ServerCAFile, "CERTIFICATE", caDER, 0o644)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)
	writePEM(t, out.ServerCertFile, "CERTIFICATE", serverDER, 0o644)
	serverKeyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	require.NoError(t, err)
	writePEM(t, out.ServerKeyFile, "PRIVATE KEY", serverKeyDER, 0o600)

	kek := make([]byte, 32)
	_, err = rand.Read(kek)
	require.NoError(t, err)
	out.CAKEK = hex.EncodeToString(kek)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	out.SigningKey = privateKey
	out.SigningPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	out.SigningPrivateKeyPath = filepath.Join(dir, "official-ed25519.pem")
	out.SigningPublicKeyPath = filepath.Join(dir, "official-ed25519.raw")
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	writePEM(t, out.SigningPrivateKeyPath, "PRIVATE KEY", privateDER, 0o600)
	require.NoError(t, os.WriteFile(out.SigningPublicKeyPath, []byte(out.SigningPublicKey+"\n"), 0o644))
	return out
}

func writePEM(t *testing.T, path, blockType string, der []byte, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), mode))
}
