package modulesdk

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	modulepkiv1 "github.com/AnixOps/anix-control/sdk/api/modulepki/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// Files in the certificate directory.
const (
	keyFileName    = "key.pem"
	certFileName   = "cert.pem"
	bundleFileName = "bundle.pem"
)

// certificates holds the module's current certificate and trust bundle and
// keeps them fresh: built-in PKI certificates are renewed at their renew
// time, external ones are re-read when their files change.
type certificates struct {
	settings settings
	identity moduletls.Identity
	kernel   moduletls.Identity

	mu          sync.RWMutex
	certificate *tls.Certificate
	roots       *x509.CertPool
	renewAfter  time.Time
	modTimes    [3]time.Time
}

func newCertificates(ctx context.Context, s settings, logf func(string, ...any)) (*certificates, error) {
	identity, err := moduletls.Module(s.cluster, s.packageID)
	if err != nil {
		return nil, err
	}
	kernel, err := moduletls.Kernel(s.cluster)
	if err != nil {
		return nil, err
	}
	c := &certificates{settings: s, identity: identity, kernel: kernel}
	if s.externalPKI() {
		if err := c.reloadExternal(true); err != nil {
			return nil, err
		}
		return c, nil
	}
	if err := c.loadStored(); err == nil && c.usable() {
		return c, nil
	}
	// The kernel may be starting or restarting (a rolling update creates
	// module pods while Control restarts): wait for it instead of exiting.
	// A refused credential still fails at once.
	delay := minEnrollRetryDelay
	for {
		err := c.enroll(ctx)
		if err == nil {
			return c, nil
		}
		if !errors.Is(err, errKernelUnreachable) || ctx.Err() != nil {
			return nil, err
		}
		logf("module enrollment: %v; retrying in %s", err, delay)
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(delay):
		}
		delay = min(2*delay, maxEnrollRetryDelay)
	}
}

// Enrollment retry bounds while the kernel is unreachable.
const (
	minEnrollRetryDelay = time.Second
	maxEnrollRetryDelay = 30 * time.Second
)

// errKernelUnreachable marks enrollment failures worth retrying.
var errKernelUnreachable = errors.New("the kernel is unreachable")

// source is the module's moduletls.Source.
func (c *certificates) source() moduletls.Source {
	return moduletls.Source{
		Certificate: func() (*tls.Certificate, error) {
			if c.settings.externalPKI() {
				_ = c.reloadExternal(false)
			}
			c.mu.RLock()
			defer c.mu.RUnlock()
			if c.certificate == nil {
				return nil, errors.New("module certificate is not loaded")
			}
			return c.certificate, nil
		},
		Roots: func() *x509.CertPool {
			c.mu.RLock()
			defer c.mu.RUnlock()
			return c.roots
		},
	}
}

// usable reports whether the stored certificate is this module's and still
// valid for a while.
func (c *certificates) usable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.certificate == nil || c.certificate.Leaf == nil {
		return false
	}
	identity, err := moduletls.FromCertificate(c.certificate.Leaf)
	return err == nil && identity == c.identity && time.Now().Add(time.Minute).Before(c.certificate.Leaf.NotAfter)
}

func (c *certificates) loadStored() error {
	directory := c.settings.certDir
	certificate, err := tls.LoadX509KeyPair(filepath.Join(directory, certFileName), filepath.Join(directory, keyFileName))
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return err
	}
	certificate.Leaf = leaf
	roots, err := readPool(filepath.Join(directory, bundleFileName))
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.certificate, c.roots = &certificate, roots
	c.renewAfter = renewTime(leaf)
	return nil
}

// enroll obtains the first certificate with the enrollment credential.
func (c *certificates) enroll(ctx context.Context) error {
	if c.settings.enrollCredentialFile == "" || c.settings.trustBundleFile == "" {
		return errors.New("module enrollment needs ANIX_MODULE_ENROLL_CREDENTIAL_FILE and ANIX_MODULE_TRUST_BUNDLE_FILE")
	}
	rawCredential, err := os.ReadFile(c.settings.enrollCredentialFile)
	if err != nil {
		return fmt.Errorf("read enrollment credential: %w", err)
	}
	roots, err := readPool(c.settings.trustBundleFile)
	if err != nil {
		return fmt.Errorf("read trust bundle: %w", err)
	}
	key, csr, err := newKeyAndRequest()
	if err != nil {
		return err
	}
	client, closeClient, err := c.pkiClient(moduletls.Source{Roots: func() *x509.CertPool { return roots }})
	if err != nil {
		return err
	}
	defer closeClient()
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := client.Enroll(callCtx, &modulepkiv1.EnrollRequest{
		EnrollmentCredential: strings.TrimSpace(string(rawCredential)),
		PackageId:            c.settings.packageID, Cluster: c.settings.cluster, CsrDer: csr,
	})
	if err != nil {
		if code := status.Code(err); code == codes.Unavailable || code == codes.DeadlineExceeded {
			return fmt.Errorf("%w: enroll: %w", errKernelUnreachable, err)
		}
		return fmt.Errorf("enroll with the kernel: %w", err)
	}
	return c.store(key, response.GetCertificate())
}

// renew replaces the certificate using the current one.
func (c *certificates) renew(ctx context.Context) error {
	key, csr, err := newKeyAndRequest()
	if err != nil {
		return err
	}
	client, closeClient, err := c.pkiClient(c.source())
	if err != nil {
		return err
	}
	defer closeClient()
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := client.Renew(callCtx, &modulepkiv1.RenewRequest{CsrDer: csr})
	if err != nil {
		return fmt.Errorf("renew with the kernel: %w", err)
	}
	return c.store(key, response.GetCertificate())
}

// maintain renews built-in certificates until ctx ends. When renewal keeps
// failing past expiry it enrolls again, which works with a reusable
// credential.
func (c *certificates) maintain(ctx context.Context, logf func(string, ...any)) {
	if c.settings.externalPKI() {
		return
	}
	for {
		c.mu.RLock()
		wait := time.Until(c.renewAfter)
		c.mu.RUnlock()
		if wait < 0 {
			wait = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		err := c.renew(ctx)
		if err != nil && !c.usable() {
			err = errors.Join(err, c.enroll(ctx))
		}
		if err != nil {
			if ctx.Err() == nil {
				logf("module certificate: %v; retrying in 1m", err)
			}
			c.mu.Lock()
			c.renewAfter = time.Now().Add(time.Minute)
			c.mu.Unlock()
		}
	}
}

func (c *certificates) pkiClient(source moduletls.Source) (modulepkiv1.ModulePKIClient, func(), error) {
	connection, err := grpc.NewClient(c.settings.kernelAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(source.ClientConfig(moduletls.AcceptExactly(c.kernel)))))
	if err != nil {
		return nil, nil, err
	}
	return modulepkiv1.NewModulePKIClient(connection), func() { _ = connection.Close() }, nil
}

// store keeps an issued certificate in memory and on disk.
func (c *certificates) store(key *ecdsa.PrivateKey, issued *modulepkiv1.IssuedCertificate) error {
	if issued == nil || len(issued.GetCertificateDer()) == 0 {
		return errors.New("the kernel returned no certificate")
	}
	leaf, err := x509.ParseCertificate(issued.GetCertificateDer())
	if err != nil {
		return err
	}
	identity, err := moduletls.FromCertificate(leaf)
	if err != nil || identity != c.identity {
		return fmt.Errorf("the kernel issued a certificate for %v, want %s", identity, c.identity)
	}
	roots := x509.NewCertPool()
	var bundlePEM []byte
	for _, der := range issued.GetTrustBundleDer() {
		certificate, err := x509.ParseCertificate(der)
		if err != nil {
			return err
		}
		roots.AddCert(certificate)
		bundlePEM = append(bundlePEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.settings.certDir, 0o700); err != nil {
		return err
	}
	for name, content := range map[string][]byte{
		keyFileName:    pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		certFileName:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw}),
		bundleFileName: bundlePEM,
	} {
		if err := writeFileAtomic(filepath.Join(c.settings.certDir, name), content); err != nil {
			return err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.certificate = &tls.Certificate{Certificate: [][]byte{leaf.Raw}, PrivateKey: key, Leaf: leaf}
	c.roots = roots
	c.renewAfter = time.Unix(issued.GetRenewAfterUnix(), 0)
	if issued.GetRenewAfterUnix() == 0 {
		c.renewAfter = renewTime(leaf)
	}
	return nil
}

// reloadExternal reads externally issued files when they changed.
func (c *certificates) reloadExternal(force bool) error {
	paths := [3]string{c.settings.certFile, c.settings.keyFile, c.settings.trustBundleFile}
	var modTimes [3]time.Time
	for index, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if !force {
				return nil
			}
			return err
		}
		modTimes[index] = info.ModTime()
	}
	c.mu.RLock()
	unchanged := !force && modTimes == c.modTimes
	c.mu.RUnlock()
	if unchanged {
		return nil
	}
	certificate, err := tls.LoadX509KeyPair(c.settings.certFile, c.settings.keyFile)
	if err == nil {
		certificate.Leaf, err = x509.ParseCertificate(certificate.Certificate[0])
	}
	var roots *x509.CertPool
	if err == nil {
		roots, err = readPool(c.settings.trustBundleFile)
	}
	if err == nil {
		var identity moduletls.Identity
		identity, err = moduletls.FromCertificate(certificate.Leaf)
		if err == nil && identity != c.identity {
			err = fmt.Errorf("module certificate identity is %s, want %s", identity, c.identity)
		}
	}
	if err != nil {
		if force {
			return err
		}
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.certificate, c.roots, c.modTimes = &certificate, roots, modTimes
	return nil
}

func newKeyAndRequest() (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	return key, csr, err
}

func renewTime(leaf *x509.Certificate) time.Time {
	return leaf.NotBefore.Add(leaf.NotAfter.Sub(leaf.NotBefore) * 2 / 3)
}

func readPool(path string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-configured trust bundle path.
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("%s contains no certificates", path)
	}
	return pool, nil
}

func writeFileAtomic(path string, content []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
