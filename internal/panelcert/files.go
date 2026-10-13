package panelcert

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"
)

// fileStamp is what tells a changed certificate file: modification time
// and size of both files.
type fileStamp struct {
	certModified, keyModified time.Time
	certSize, keySize         int64
	err                       string
}

func statFiles(certFile, keyFile string) fileStamp {
	var stamp fileStamp
	certInfo, err := os.Stat(certFile)
	if err != nil {
		stamp.err = err.Error()
		return stamp
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		stamp.err = err.Error()
		return stamp
	}
	stamp.certModified, stamp.certSize = certInfo.ModTime(), certInfo.Size()
	stamp.keyModified, stamp.keySize = keyInfo.ModTime(), keyInfo.Size()
	return stamp
}

// loadFiles reads s's certificate files when they changed since the last
// look (or force). A file that cannot be read or does not hold a key pair
// keeps the certificate served so far, and the status says why.
func (m *Manager) loadFiles(s *slot, force bool) {
	stamp := statFiles(s.spec.CertFile, s.spec.KeyFile)
	s.mu.Lock()
	unchanged := s.loadedAny && stamp == s.stamp
	s.mu.Unlock()
	if unchanged && !force {
		return
	}
	now := m.now()
	certificate, err := loadKeyPair(s.spec.CertFile, s.spec.KeyFile)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stamp = stamp
	s.lastAttempt = now
	if err != nil {
		if s.lastError != err.Error() {
			m.opts.Logf("Panel certificate %s: %v; the certificate served so far stays", s.spec.Name, err)
		}
		s.attemptFailed = true
		s.lastError, s.problem = err.Error(), ProblemFileUnreadable
		return
	}
	served := m.servedFrom(certificate, ServingFile, s.spec.Name, now)
	previous := s.served.Swap(served)
	s.loadedAny, s.attemptFailed = true, false
	s.lastSuccess, s.lastError, s.problem = now, "", ""
	if previous == nil || previous.certificate.Leaf.SerialNumber.Cmp(certificate.Leaf.SerialNumber) != 0 {
		m.opts.Logf("Panel certificate %s: loaded %s (issued by %s, valid until %s)", s.spec.Name, s.spec.CertFile, issuerName(certificate.Leaf), certificate.Leaf.NotAfter.UTC().Format(time.RFC3339))
	}
}

// loadKeyPair reads a PEM certificate chain and its key.
func loadKeyPair(certFile, keyFile string) (*tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("cannot load %s and %s: %w", certFile, keyFile, err)
	}
	if certificate.Leaf == nil {
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("cannot parse %s: %w", certFile, err)
		}
		certificate.Leaf = leaf
	}
	return &certificate, nil
}
