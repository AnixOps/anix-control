package modulepki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	modulepkiv1 "github.com/AnixOps/anix-control/v4/api/modulepki/v1"
	"github.com/AnixOps/anix-control/v4/pkg/moduletls"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// serveModulePKI runs ModulePKI behind the kernel's TLS configuration: client
// certificates are optional (enrollment) but verified when present.
func serveModulePKI(t *testing.T, authority *Authority) (string, moduletls.Source) {
	t.Helper()
	kernel, err := authority.KernelTLS(context.Background(), time.Minute)
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(kernel.ServerConfig(moduletls.AcceptModules(authority.Cluster()), true))))
	modulepkiv1.RegisterModulePKIServer(server, &Server{Authority: authority})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String(), kernel
}

func dialModulePKI(t *testing.T, address string, source moduletls.Source, cluster string) modulepkiv1.ModulePKIClient {
	t.Helper()
	kernelID, err := moduletls.Kernel(cluster)
	require.NoError(t, err)
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(source.ClientConfig(moduletls.AcceptExactly(kernelID)))))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	return modulepkiv1.NewModulePKIClient(connection)
}

func poolFromDER(t *testing.T, bundle [][]byte) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	for _, der := range bundle {
		certificate, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		pool.AddCert(certificate)
	}
	return pool
}

func TestModulePKIOverMutualTLS(t *testing.T) {
	db := openTestDB(t)
	authority := newTestAuthority(t, db, &testClock{now: time.Now()})
	authority.now = time.Now
	ctx := context.Background()
	address, kernel := serveModulePKI(t, authority)
	credential, _, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: time.Hour})
	require.NoError(t, err)

	// A new module knows only the trust bundle (distributed with its
	// deployment) and its credential.
	anonymous := dialModulePKI(t, address, moduletls.Source{Roots: kernel.Roots}, "prod")
	key, csr := newCSR(t)
	enrolled, err := anonymous.Enroll(ctx, &modulepkiv1.EnrollRequest{
		EnrollmentCredential: credential, PackageId: "identity-platform", Cluster: "prod", CsrDer: csr,
	})
	require.NoError(t, err)
	require.Equal(t, "spiffe://anixops/prod/module/identity-platform", enrolled.GetCertificate().GetSpiffeId())
	_, err = anonymous.Renew(ctx, &modulepkiv1.RenewRequest{CsrDer: csr})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	_, err = anonymous.GetTrustBundle(ctx, &modulepkiv1.GetTrustBundleRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	_, err = anonymous.Enroll(ctx, &modulepkiv1.EnrollRequest{
		EnrollmentCredential: credential, PackageId: "identity-platform", Cluster: "prod", CsrDer: csr,
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	moduleCertificate := &tls.Certificate{Certificate: [][]byte{enrolled.GetCertificate().GetCertificateDer()}, PrivateKey: key}
	roots := poolFromDER(t, enrolled.GetCertificate().GetTrustBundleDer())
	module := dialModulePKI(t, address, moduletls.Source{
		Certificate: func() (*tls.Certificate, error) { return moduleCertificate, nil },
		Roots:       func() *x509.CertPool { return roots },
	}, "prod")
	renewed, err := module.Renew(ctx, &modulepkiv1.RenewRequest{CsrDer: csr})
	require.NoError(t, err)
	require.NotEqual(t, enrolled.GetCertificate().GetCertificateDer(), renewed.GetCertificate().GetCertificateDer())
	bundle, err := module.GetTrustBundle(ctx, &modulepkiv1.GetTrustBundleRequest{})
	require.NoError(t, err)
	require.Len(t, bundle.GetTrustBundleDer(), 1)

	// A module certificate from another CA cannot even complete the
	// handshake.
	foreign := newTestAuthority(t, openTestDB(t), &testClock{now: time.Now()})
	foreign.now = time.Now
	foreignCredential, _, err := foreign.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: time.Hour})
	require.NoError(t, err)
	foreignKey, foreignCSR := newCSR(t)
	foreignIssued, err := foreign.Enroll(ctx, foreignCredential, "identity-platform", "prod", foreignCSR)
	require.NoError(t, err)
	impostor := dialModulePKI(t, address, moduletls.Source{
		Certificate: func() (*tls.Certificate, error) {
			return &tls.Certificate{Certificate: [][]byte{foreignIssued.CertificateDER}, PrivateKey: foreignKey}, nil
		},
		Roots: func() *x509.CertPool { return roots },
	}, "prod")
	_, err = impostor.GetTrustBundle(ctx, &modulepkiv1.GetTrustBundleRequest{})
	require.Equal(t, codes.Unavailable, status.Code(err), "the kernel rejects the handshake")

	// A client that expects another cluster's kernel refuses this one.
	_, err = dialModulePKI(t, address, moduletls.Source{Roots: kernel.Roots}, "staging").
		GetTrustBundle(ctx, &modulepkiv1.GetTrustBundleRequest{})
	require.Equal(t, codes.Unavailable, status.Code(err))
}

func TestExternalSourceLoadsKernelIdentity(t *testing.T) {
	db := openTestDB(t)
	authority := newTestAuthority(t, db, &testClock{now: time.Now()})
	authority.now = time.Now
	kernel, err := authority.KernelTLS(context.Background(), time.Minute)
	require.NoError(t, err)
	certificate, err := kernel.Certificate()
	require.NoError(t, err)
	bundle, err := authority.TrustBundle(context.Background())
	require.NoError(t, err)

	directory := t.TempDir()
	write := func(name string, blocks ...*pem.Block) string {
		path := filepath.Join(directory, name)
		var content []byte
		for _, block := range blocks {
			content = append(content, pem.EncodeToMemory(block)...)
		}
		require.NoError(t, os.WriteFile(path, content, 0o600))
		return path
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	require.NoError(t, err)
	bundlePath := write("ca.pem", &pem.Block{Type: "CERTIFICATE", Bytes: bundle[0].Raw})
	certPath := write("tls.crt", &pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
	keyPath := write("tls.key", &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	source := &ExternalSource{TrustBundleFile: bundlePath, CertFile: certPath, KeyFile: keyPath, Cluster: "prod"}
	loaded, err := source.Source()
	require.NoError(t, err)
	got, err := loaded.Certificate()
	require.NoError(t, err)
	require.Equal(t, certificate.Certificate[0], got.Certificate[0])
	require.NotNil(t, loaded.Roots())

	_, err = (&ExternalSource{TrustBundleFile: bundlePath, CertFile: certPath, KeyFile: keyPath, Cluster: "staging"}).Source()
	require.ErrorContains(t, err, "want the kernel of cluster staging")

	_, err = (&ExternalSource{TrustBundleFile: write("empty.pem"), CertFile: certPath, KeyFile: keyPath, Cluster: "prod"}).Source()
	require.ErrorContains(t, err, "no certificates")
}
