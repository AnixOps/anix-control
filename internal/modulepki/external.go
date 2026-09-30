package modulepki

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/pkg/moduletls"
)

// ExternalSource serves certificates issued outside the kernel, for example
// by cert-manager: a trust bundle file and the kernel's certificate and key.
// Files are re-read when they change, at most every refresh, so rotated
// secrets apply without a restart.
type ExternalSource struct {
	TrustBundleFile string
	CertFile        string
	KeyFile         string
	Cluster         string
	Refresh         time.Duration

	mu          sync.Mutex
	checkedAt   time.Time
	modTimes    [3]time.Time
	roots       *x509.CertPool
	certificate *tls.Certificate
}

// Source returns the kernel's moduletls.Source after checking that the files
// load and that the certificate carries the cluster's kernel identity.
func (s *ExternalSource) Source() (moduletls.Source, error) {
	if err := s.reload(true); err != nil {
		return moduletls.Source{}, err
	}
	return moduletls.Source{
		Certificate: func() (*tls.Certificate, error) {
			if err := s.reload(false); err != nil {
				return nil, err
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.certificate, nil
		},
		Roots: func() *x509.CertPool {
			_ = s.reload(false)
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.roots
		},
	}, nil
}

func (s *ExternalSource) reload(force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	refresh := s.Refresh
	if refresh <= 0 {
		refresh = 30 * time.Second
	}
	if !force && time.Since(s.checkedAt) < refresh {
		return nil
	}
	s.checkedAt = time.Now()
	var modTimes [3]time.Time
	for index, path := range []string{s.TrustBundleFile, s.CertFile, s.KeyFile} {
		info, err := os.Stat(path)
		if err != nil {
			if s.certificate != nil {
				return nil
			}
			return err
		}
		modTimes[index] = info.ModTime()
	}
	if !force && modTimes == s.modTimes && s.certificate != nil {
		return nil
	}
	roots, err := loadTrustBundle(s.TrustBundleFile)
	if err != nil {
		return keepOrFail(s.certificate != nil, err)
	}
	certificate, err := tls.LoadX509KeyPair(s.CertFile, s.KeyFile)
	if err != nil {
		return keepOrFail(s.certificate != nil, err)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return keepOrFail(s.certificate != nil, err)
	}
	identity, err := moduletls.FromCertificate(leaf)
	if err != nil {
		return keepOrFail(s.certificate != nil, err)
	}
	if identity.Kind != moduletls.KindKernel || identity.Cluster != s.Cluster {
		return keepOrFail(s.certificate != nil, fmt.Errorf("kernel certificate identity is %s, want the kernel of cluster %s", identity, s.Cluster))
	}
	certificate.Leaf = leaf
	s.roots, s.certificate, s.modTimes = roots, &certificate, modTimes
	return nil
}

// keepOrFail keeps serving the previous material when a rotated file is
// broken, and fails only when nothing was loaded yet.
func keepOrFail(havePrevious bool, err error) error {
	if havePrevious {
		return nil
	}
	return err
}

func loadTrustBundle(path string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-configured trust bundle path.
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	count := 0
	for {
		var block *pem.Block
		block, raw = pem.Decode(raw)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		pool.AddCert(certificate)
		count++
	}
	if count == 0 {
		return nil, errors.New("module trust bundle contains no certificates")
	}
	return pool, nil
}
