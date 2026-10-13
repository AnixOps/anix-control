package panelcert

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"slices"
	"time"
)

// The self-signed fallback: served while no certificate from ACME or the
// files can be, so that the panel always opens (with a browser warning) and
// a handshake never waits for an issuance. Agents refuse it, which is what
// keeps them from sending credentials to an unverified Control.
const (
	selfSignedLifetime = 30 * 24 * time.Hour
	// selfSignedRenewBefore: a fallback this close to its end is replaced.
	selfSignedRenewBefore = 7 * 24 * time.Hour
	selfSignedOrg         = "AnixOps Control (self-signed fallback)"
)

// newSelfSigned makes a self-signed ECDSA P-256 certificate for name and
// addresses, valid for selfSignedLifetime from now.
func newSelfSigned(name string, addresses []net.IP, now time.Time) (*tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name, Organization: []string{selfSignedOrg}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(selfSignedLifetime),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{name},
		IPAddresses:           addresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	var encoded bytes.Buffer
	if err := pem.Encode(&encoded, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return nil, nil, err
	}
	if err := pem.Encode(&encoded, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return nil, nil, err
	}
	certificate, err := parseKeyAndChain(encoded.Bytes())
	if err != nil {
		return nil, nil, err
	}
	return certificate, encoded.Bytes(), nil
}

// parseKeyAndChain reads the PEM layout autocert caches: the private key,
// then the chain, leaf first.
func parseKeyAndChain(data []byte) (*tls.Certificate, error) {
	var keyBlock *pem.Block
	var chain [][]byte
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			chain = append(chain, block.Bytes)
		default:
			if keyBlock == nil {
				keyBlock = block
			}
		}
	}
	if keyBlock == nil || len(chain) == 0 {
		return nil, errors.New("no private key or certificate")
	}
	keyPEM := pem.EncodeToMemory(keyBlock)
	var chainPEM bytes.Buffer
	for _, der := range chain {
		_ = pem.Encode(&chainPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	certificate, err := tls.X509KeyPair(chainPEM.Bytes(), keyPEM)
	if err != nil {
		return nil, err
	}
	if certificate.Leaf == nil {
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return nil, err
		}
		certificate.Leaf = leaf
	}
	return &certificate, nil
}

// loadSelfSigned returns the stored fallback of name when it still covers
// name and addresses and is not close to its end; otherwise it makes and
// stores a new one.
func loadSelfSigned(ctx context.Context, store *Store, name string, addresses []net.IP, now time.Time) (*tls.Certificate, error) {
	key := selfSignedKeyStart + name
	if data, err := store.Get(ctx, key); err == nil {
		if certificate, err := parseKeyAndChain(data); err == nil && fallbackUsable(certificate.Leaf, name, addresses, now) {
			return certificate, nil
		}
	}
	certificate, encoded, err := newSelfSigned(name, addresses, now)
	if err != nil {
		return nil, err
	}
	if err := store.Put(ctx, key, encoded); err != nil {
		return certificate, err
	}
	return certificate, nil
}

// fallbackUsable tells whether a stored fallback can be served on: it names
// name and every address and lasts more than selfSignedRenewBefore.
func fallbackUsable(leaf *x509.Certificate, name string, addresses []net.IP, now time.Time) bool {
	if leaf == nil || leaf.NotAfter.Sub(now) < selfSignedRenewBefore || leaf.VerifyHostname(name) != nil {
		return false
	}
	for _, address := range addresses {
		if !slices.ContainsFunc(leaf.IPAddresses, address.Equal) {
			return false
		}
	}
	return true
}

// LocalAddresses are this machine's global unicast interface addresses:
// the fallback names them, so that https://<address>/ opens before the DNS
// record exists, and the DNS check compares a name's records with them.
// No network call.
func LocalAddresses() []net.IP {
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, address := range interfaces {
		network, ok := address.(*net.IPNet)
		if !ok || !network.IP.IsGlobalUnicast() {
			continue
		}
		out = append(out, network.IP)
	}
	return out
}
