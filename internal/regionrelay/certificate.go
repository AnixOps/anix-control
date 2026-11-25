package regionrelay

import (
	"crypto/tls"
	"os"
	"sync"
	"time"
)

// certificateReloader serves the cache certificate and reads the files
// again when they change, so a renewed certificate needs no restart.
type certificateReloader struct {
	certFile, keyFile string

	mu      sync.Mutex
	current *tls.Certificate
	certMod time.Time
	keyMod  time.Time
	checked time.Time
}

func newCertificateReloader(certFile, keyFile string) (*certificateReloader, error) {
	reloader := &certificateReloader{certFile: certFile, keyFile: keyFile}
	if err := reloader.load(); err != nil {
		return nil, err
	}
	return reloader, nil
}

func (r *certificateReloader) load() error {
	certInfo, err := os.Stat(r.certFile)
	if err != nil {
		return err
	}
	keyInfo, err := os.Stat(r.keyFile)
	if err != nil {
		return err
	}
	certificate, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return err
	}
	r.current = &certificate
	r.certMod, r.keyMod = certInfo.ModTime(), keyInfo.ModTime()
	return nil
}

// get serves the current certificate, checking the files at most once a
// minute. A pair that does not load keeps the previous certificate.
func (r *certificateReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.checked) >= time.Minute {
		r.checked = time.Now()
		certInfo, certErr := os.Stat(r.certFile)
		keyInfo, keyErr := os.Stat(r.keyFile)
		if certErr == nil && keyErr == nil && (!certInfo.ModTime().Equal(r.certMod) || !keyInfo.ModTime().Equal(r.keyMod)) {
			previous := r.current
			if err := r.load(); err != nil {
				r.current = previous
			}
		}
	}
	return r.current, nil
}
