// Package panelcerttest is a minimal in-process ACME CA (RFC 8555) for the
// tests of the panel certificates: enough of the protocol for
// golang.org/x/crypto/acme and autocert to register, order, prove control
// of a name and download a certificate.
//
// What it exercises for real: the JWS envelope (nonces, the url header, the
// account key's ES256 or RS256 signature, kid and jwk forms), the order and
// authorization life cycle, and both challenges against the listeners under
// test: TLS-ALPN-01 dials the TLS listener with ALPN acme-tls/1 and checks
// the acmeIdentifier extension against the key authorization; HTTP-01
// fetches /.well-known/acme-challenge/<token> over plain http with the
// name as Host. Certificates are signed by a root and an intermediate the
// test can trust (Roots).
//
// What it does not: DNS (the test maps every name to the listener
// addresses), CAA, rate limits, revocation, external account binding, key
// rollover, wildcard names, and the retries and timing of a real CA's
// validation. A real CA (the public one's staging directory, or Pebble in
// CI) covers those.
package panelcerttest

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/acme"
)

// idPeACMEIdentifier is the TLS-ALPN-01 certificate extension (RFC 8737).
var idPeACMEIdentifier = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 31}

// Challenge types.
const (
	TLSALPN01 = "tls-alpn-01"
	HTTP01    = "http-01"
)

// CA is the test CA. Set the validation targets before an issuance.
type CA struct {
	// DirectoryURL is the ACME directory to hand the client.
	DirectoryURL string
	// Roots trusts the certificates the CA issues.
	Roots *x509.CertPool

	server           *httptest.Server
	root, issuer     *x509.Certificate
	issuerKey        *ecdsa.PrivateKey
	mu               sync.Mutex
	tlsALPNTarget    func(name string) string
	httpTarget       func(name string) string
	lifetime         time.Duration
	offered          []string
	nextID           int
	nonces           map[string]bool
	accounts         map[string]*account
	accountByJWK     map[string]string
	orders           map[string]*order
	authorizations   map[string]*authorization
	challenges       map[string]*challenge
	certificates     map[string][]byte
	validations      []string
	issued           []*x509.Certificate
	directoryOffline bool
	// asyncFinalize answers a finalize with the order still processing and
	// no Location header, as Pebble does; the next read of the order shows
	// it valid.
	asyncFinalize bool
}

type account struct {
	id         string
	key        crypto.PublicKey
	thumbprint string
}

type order struct {
	id, account    string
	names          []string
	status         string
	authorizations []string
	certificate    string
	problem        *problem
	// processing: issued, shown valid from the next read on.
	processing bool
}

type authorization struct {
	id, order, name string
	status          string
	challenges      []string
}

type challenge struct {
	id, authorization, kind, token string
	status                         string
	problem                        *problem
}

type problem struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
	Status int    `json:"status,omitempty"`
}

// New starts a CA that issues certificates valid for lifetime (0: 90 days)
// and offers both challenges. It stops with the test.
func New(t testing.TB, lifetime time.Duration) *CA {
	t.Helper()
	if lifetime <= 0 {
		lifetime = 90 * 24 * time.Hour
	}
	ca := &CA{
		lifetime: lifetime, offered: []string{TLSALPN01, HTTP01},
		nonces: map[string]bool{}, accounts: map[string]*account{}, accountByJWK: map[string]string{},
		orders: map[string]*order{}, authorizations: map[string]*authorization{}, challenges: map[string]*challenge{},
		certificates: map[string][]byte{},
	}
	if err := ca.makeHierarchy(); err != nil {
		t.Fatal(err)
	}
	ca.server = httptest.NewServer(http.HandlerFunc(ca.serve))
	t.Cleanup(ca.server.Close)
	ca.DirectoryURL = ca.server.URL + "/directory"
	return ca
}

// SetTargets sets where each challenge is validated for a name: a
// host:port, or "" to fail that challenge with a connection error (a
// closed or filtered port).
func (ca *CA) SetTargets(tlsALPN, http01 func(name string) string) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.tlsALPNTarget, ca.httpTarget = tlsALPN, http01
}

// SetLifetime changes the lifetime of the next certificates.
func (ca *CA) SetLifetime(lifetime time.Duration) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.lifetime = lifetime
}

// SetAsyncFinalize makes finalize answer like Pebble: the order still
// processing, without a Location header.
func (ca *CA) SetAsyncFinalize(async bool) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.asyncFinalize = async
}

// SetOffline makes every request answer 503 (true) or serves again.
func (ca *CA) SetOffline(offline bool) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.directoryOffline = offline
}

// Validations lists "<type> <name> ok|failed" in order.
func (ca *CA) Validations() []string {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	return append([]string(nil), ca.validations...)
}

// Issued lists the issued leaf certificates in order.
func (ca *CA) Issued() []*x509.Certificate {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	return append([]*x509.Certificate(nil), ca.issued...)
}

func (ca *CA) makeHierarchy() error {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "panelcerttest root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(10 * 365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return err
	}
	ca.root, _ = x509.ParseCertificate(rootDER)
	ca.issuerKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	issuerTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "panelcerttest issuer"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(5 * 365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	issuerDER, err := x509.CreateCertificate(rand.Reader, issuerTemplate, ca.root, &ca.issuerKey.PublicKey, rootKey)
	if err != nil {
		return err
	}
	ca.issuer, _ = x509.ParseCertificate(issuerDER)
	ca.Roots = x509.NewCertPool()
	ca.Roots.AddCert(ca.root)
	return nil
}

func (ca *CA) url(path string) string { return ca.server.URL + path }

func (ca *CA) newID() string {
	ca.nextID++
	return strconv.Itoa(ca.nextID)
}

func (ca *CA) newNonce() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	nonce := base64.RawURLEncoding.EncodeToString(raw)
	ca.nonces[nonce] = true
	return nonce
}

func (ca *CA) serve(w http.ResponseWriter, r *http.Request) {
	ca.mu.Lock()
	offline := ca.directoryOffline
	w.Header().Set("Replay-Nonce", ca.newNonce())
	ca.mu.Unlock()
	if offline {
		writeProblem(w, http.StatusServiceUnavailable, "serverInternal", "the CA is down for maintenance")
		return
	}
	switch {
	case r.URL.Path == "/directory":
		writeJSON(w, http.StatusOK, map[string]any{
			"newNonce": ca.url("/new-nonce"), "newAccount": ca.url("/new-account"), "newOrder": ca.url("/new-order"),
			"revokeCert": ca.url("/revoke"), "keyChange": ca.url("/key-change"),
			"meta": map[string]any{"termsOfService": ca.url("/terms")},
		})
	case r.URL.Path == "/new-nonce":
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	case r.Method != http.MethodPost:
		writeProblem(w, http.StatusMethodNotAllowed, "malformed", "POST only")
	default:
		ca.servePost(w, r)
	}
}

// jws is a parsed and verified request.
type jws struct {
	account *account
	jwk     crypto.PublicKey
	payload []byte
}

func (ca *CA) parseJWS(r *http.Request) (*jws, *problem) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, &problem{Type: "malformed", Detail: err.Error()}
	}
	var envelope struct {
		Protected, Payload, Signature string
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, &problem{Type: "malformed", Detail: "not a JWS"}
	}
	protectedRaw, err := base64.RawURLEncoding.DecodeString(envelope.Protected)
	if err != nil {
		return nil, &problem{Type: "malformed", Detail: "protected header"}
	}
	var header struct {
		Alg   string          `json:"alg"`
		KID   string          `json:"kid"`
		JWK   json.RawMessage `json:"jwk"`
		Nonce string          `json:"nonce"`
		URL   string          `json:"url"`
	}
	if err := json.Unmarshal(protectedRaw, &header); err != nil {
		return nil, &problem{Type: "malformed", Detail: "protected header"}
	}
	if !ca.nonces[header.Nonce] {
		return nil, &problem{Type: "badNonce", Detail: "unknown nonce"}
	}
	delete(ca.nonces, header.Nonce)
	if header.URL != ca.url(r.URL.Path) {
		return nil, &problem{Type: "unauthorized", Detail: "the url header does not match"}
	}
	request := &jws{}
	var key crypto.PublicKey
	switch {
	case header.KID != "" && len(header.JWK) == 0:
		found, ok := ca.accounts[header.KID]
		if !ok {
			return nil, &problem{Type: "accountDoesNotExist", Detail: "unknown kid"}
		}
		request.account, key = found, found.key
	case header.KID == "" && len(header.JWK) > 0:
		parsed, err := parseJWK(header.JWK)
		if err != nil {
			return nil, &problem{Type: "badPublicKey", Detail: err.Error()}
		}
		request.jwk, key = parsed, parsed
	default:
		return nil, &problem{Type: "malformed", Detail: "exactly one of kid and jwk"}
	}
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return nil, &problem{Type: "malformed", Detail: "signature"}
	}
	if err := verifyJWS(header.Alg, key, []byte(envelope.Protected+"."+envelope.Payload), signature); err != nil {
		return nil, &problem{Type: "malformed", Detail: "bad signature: " + err.Error()}
	}
	request.payload, err = base64.RawURLEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return nil, &problem{Type: "malformed", Detail: "payload"}
	}
	return request, nil
}

func parseJWK(raw json.RawMessage) (crypto.PublicKey, error) {
	var jwk struct {
		Kty, Crv, X, Y, N, E string
	}
	if err := json.Unmarshal(raw, &jwk); err != nil {
		return nil, err
	}
	decode := func(value string) *big.Int {
		data, _ := base64.RawURLEncoding.DecodeString(value)
		return new(big.Int).SetBytes(data)
	}
	switch jwk.Kty {
	case "EC":
		if jwk.Crv != "P-256" {
			return nil, errors.New("only P-256")
		}
		x, errX := base64.RawURLEncoding.DecodeString(jwk.X)
		y, errY := base64.RawURLEncoding.DecodeString(jwk.Y)
		if errX != nil || errY != nil || len(x) != 32 || len(y) != 32 {
			return nil, errors.New("bad P-256 coordinates")
		}
		return ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
	case "RSA":
		return &rsa.PublicKey{N: decode(jwk.N), E: int(decode(jwk.E).Int64())}, nil
	}
	return nil, fmt.Errorf("unsupported key type %q", jwk.Kty)
}

func verifyJWS(alg string, key crypto.PublicKey, signed, signature []byte) error {
	digest := sha256.Sum256(signed)
	switch k := key.(type) {
	case *ecdsa.PublicKey:
		if alg != "ES256" || len(signature) != 64 {
			return errors.New("want ES256")
		}
		r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])
		if !ecdsa.Verify(k, digest[:], r, s) {
			return errors.New("ES256 does not verify")
		}
		return nil
	case *rsa.PublicKey:
		if alg != "RS256" {
			return errors.New("want RS256")
		}
		return rsa.VerifyPKCS1v15(k, crypto.SHA256, digest[:], signature)
	}
	return errors.New("unsupported key")
}

func (ca *CA) servePost(w http.ResponseWriter, r *http.Request) {
	ca.mu.Lock()
	request, failure := ca.parseJWS(r)
	if failure != nil {
		ca.mu.Unlock()
		writeProblem(w, http.StatusBadRequest, failure.Type, failure.Detail)
		return
	}
	path := r.URL.Path
	if path == "/new-account" {
		defer ca.mu.Unlock()
		ca.newAccount(w, request)
		return
	}
	if request.account == nil {
		ca.mu.Unlock()
		writeProblem(w, http.StatusBadRequest, "malformed", "kid required")
		return
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	id := ""
	if len(segments) == 2 {
		id = segments[1]
	}
	switch segments[0] {
	case "new-order":
		defer ca.mu.Unlock()
		ca.newOrder(w, request)
	case "order":
		defer ca.mu.Unlock()
		if o := ca.orders[id]; o != nil && o.processing {
			o.processing, o.status = false, acme.StatusValid
		}
		ca.writeOrder(w, http.StatusOK, ca.orders[id])
	case "authz":
		defer ca.mu.Unlock()
		ca.serveAuthorization(w, request, ca.authorizations[id])
	case "chall":
		// Validation dials the listeners under test, which may call back
		// into nothing here, but must not hold the lock while waiting.
		ca.serveChallenge(w, request, id)
	case "finalize":
		defer ca.mu.Unlock()
		ca.finalize(w, request, ca.orders[id])
	case "cert":
		defer ca.mu.Unlock()
		chain, ok := ca.certificates[id]
		if !ok {
			writeProblem(w, http.StatusNotFound, "malformed", "no such certificate")
			return
		}
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		_, _ = w.Write(chain)
	default:
		ca.mu.Unlock()
		writeProblem(w, http.StatusNotFound, "malformed", "no such resource")
	}
}

func (ca *CA) newAccount(w http.ResponseWriter, request *jws) {
	if request.jwk == nil {
		writeProblem(w, http.StatusBadRequest, "malformed", "newAccount needs jwk")
		return
	}
	thumbprint, err := acme.JWKThumbprint(request.jwk)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "badPublicKey", err.Error())
		return
	}
	var payload struct {
		OnlyReturnExisting bool `json:"onlyReturnExisting"`
		TermsAgreed        bool `json:"termsOfServiceAgreed"`
	}
	_ = json.Unmarshal(request.payload, &payload)
	if kid, ok := ca.accountByJWK[thumbprint]; ok {
		w.Header().Set("Location", kid)
		writeJSON(w, http.StatusOK, map[string]any{"status": "valid"})
		return
	}
	if payload.OnlyReturnExisting {
		writeProblem(w, http.StatusBadRequest, "accountDoesNotExist", "no account for this key")
		return
	}
	if !payload.TermsAgreed {
		writeProblem(w, http.StatusBadRequest, "userActionRequired", "agree to the terms")
		return
	}
	kid := ca.url("/account/" + ca.newID())
	ca.accounts[kid] = &account{id: kid, key: request.jwk, thumbprint: thumbprint}
	ca.accountByJWK[thumbprint] = kid
	w.Header().Set("Location", kid)
	writeJSON(w, http.StatusCreated, map[string]any{"status": "valid"})
}

func (ca *CA) newOrder(w http.ResponseWriter, request *jws) {
	var payload struct {
		Identifiers []struct{ Type, Value string } `json:"identifiers"`
	}
	if err := json.Unmarshal(request.payload, &payload); err != nil || len(payload.Identifiers) == 0 {
		writeProblem(w, http.StatusBadRequest, "malformed", "identifiers")
		return
	}
	o := &order{id: ca.newID(), account: request.account.id, status: acme.StatusPending}
	for _, identifier := range payload.Identifiers {
		if identifier.Type != "dns" {
			writeProblem(w, http.StatusBadRequest, "rejectedIdentifier", "dns only")
			return
		}
		o.names = append(o.names, identifier.Value)
		z := &authorization{id: ca.newID(), order: o.id, name: identifier.Value, status: acme.StatusPending}
		for _, kind := range ca.offered {
			token := make([]byte, 16)
			_, _ = rand.Read(token)
			c := &challenge{id: ca.newID(), authorization: z.id, kind: kind, token: base64.RawURLEncoding.EncodeToString(token), status: acme.StatusPending}
			ca.challenges[c.id] = c
			z.challenges = append(z.challenges, c.id)
		}
		ca.authorizations[z.id] = z
		o.authorizations = append(o.authorizations, z.id)
	}
	ca.orders[o.id] = o
	ca.writeOrder(w, http.StatusCreated, o)
}

func (ca *CA) writeOrder(w http.ResponseWriter, status int, o *order) {
	ca.writeOrderBody(w, status, o, true)
}

// writeOrderBody writes an order, with its URL as Location when location.
func (ca *CA) writeOrderBody(w http.ResponseWriter, status int, o *order, location bool) {
	if o == nil {
		writeProblem(w, http.StatusNotFound, "malformed", "no such order")
		return
	}
	authorizations := make([]string, 0, len(o.authorizations))
	identifiers := make([]map[string]string, 0, len(o.names))
	for _, id := range o.authorizations {
		authorizations = append(authorizations, ca.url("/authz/"+id))
	}
	for _, name := range o.names {
		identifiers = append(identifiers, map[string]string{"type": "dns", "value": name})
	}
	body := map[string]any{
		"status": o.status, "identifiers": identifiers, "authorizations": authorizations,
		"finalize": ca.url("/finalize/" + o.id), "expires": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	if o.certificate != "" && o.status == acme.StatusValid {
		body["certificate"] = ca.url("/cert/" + o.certificate)
	}
	if o.problem != nil {
		body["error"] = o.problem
	}
	if location {
		w.Header().Set("Location", ca.url("/order/"+o.id))
	}
	writeJSON(w, status, body)
}

func (ca *CA) authorizationBody(z *authorization) map[string]any {
	challenges := make([]map[string]any, 0, len(z.challenges))
	for _, id := range z.challenges {
		c := ca.challenges[id]
		entry := map[string]any{"type": c.kind, "url": ca.url("/chall/" + c.id), "token": c.token, "status": c.status}
		if c.problem != nil {
			entry["error"] = c.problem
		}
		challenges = append(challenges, entry)
	}
	return map[string]any{
		"identifier": map[string]string{"type": "dns", "value": z.name}, "status": z.status,
		"expires": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "challenges": challenges,
	}
}

func (ca *CA) serveAuthorization(w http.ResponseWriter, request *jws, z *authorization) {
	if z == nil {
		writeProblem(w, http.StatusNotFound, "malformed", "no such authorization")
		return
	}
	var payload struct {
		Status string `json:"status"`
	}
	if len(request.payload) > 0 {
		_ = json.Unmarshal(request.payload, &payload)
		if payload.Status == "deactivated" {
			z.status = "deactivated"
		}
	}
	writeJSON(w, http.StatusOK, ca.authorizationBody(z))
}

// serveChallenge validates a challenge. It is entered with ca.mu held and
// releases it while it dials the listener under test.
func (ca *CA) serveChallenge(w http.ResponseWriter, request *jws, id string) {
	c := ca.challenges[id]
	if c == nil {
		ca.mu.Unlock()
		writeProblem(w, http.StatusNotFound, "malformed", "no such challenge")
		return
	}
	z := ca.authorizations[c.authorization]
	if len(request.payload) == 0 || c.status != acme.StatusPending || z.status != acme.StatusPending {
		body := map[string]any{"type": c.kind, "url": ca.url("/chall/" + c.id), "token": c.token, "status": c.status}
		ca.mu.Unlock()
		writeJSON(w, http.StatusOK, body)
		return
	}
	c.status = acme.StatusProcessing
	keyAuthorization := c.token + "." + request.account.thumbprint
	target := ""
	switch c.kind {
	case TLSALPN01:
		if ca.tlsALPNTarget != nil {
			target = ca.tlsALPNTarget(z.name)
		}
	case HTTP01:
		if ca.httpTarget != nil {
			target = ca.httpTarget(z.name)
		}
	}
	ca.mu.Unlock()

	var failure *problem
	if target == "" {
		failure = &problem{Type: "urn:ietf:params:acme:error:connection", Detail: fmt.Sprintf("%s: Timeout during connect (likely firewall problem)", z.name)}
	} else if c.kind == TLSALPN01 {
		failure = validateTLSALPN(target, z.name, keyAuthorization)
	} else {
		failure = validateHTTP(target, z.name, c.token, keyAuthorization)
	}

	ca.mu.Lock()
	defer ca.mu.Unlock()
	outcome := "ok"
	if failure != nil {
		outcome = "failed"
		c.status, c.problem = acme.StatusInvalid, failure
		z.status = acme.StatusInvalid
		if o := ca.orders[z.order]; o != nil {
			o.status = acme.StatusInvalid
			o.problem = failure
		}
	} else {
		c.status = acme.StatusValid
		z.status = acme.StatusValid
		if o := ca.orders[z.order]; o != nil {
			ready := true
			for _, other := range o.authorizations {
				if ca.authorizations[other].status != acme.StatusValid {
					ready = false
				}
			}
			if ready {
				o.status = acme.StatusReady
			}
		}
	}
	ca.validations = append(ca.validations, c.kind+" "+z.name+" "+outcome)
	body := map[string]any{"type": c.kind, "url": ca.url("/chall/" + c.id), "token": c.token, "status": c.status}
	writeJSON(w, http.StatusOK, body)
}

func validateTLSALPN(target, name, keyAuthorization string) *problem {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", target, &tls.Config{
		ServerName: name, NextProtos: []string{acme.ALPNProto}, InsecureSkipVerify: true, // #nosec G402 -- the challenge certificate is self-signed by design (RFC 8737).
	})
	if err != nil {
		return &problem{Type: "urn:ietf:params:acme:error:connection", Detail: fmt.Sprintf("%s: Error getting validation data: %v", name, err)}
	}
	defer func() { _ = conn.Close() }()
	state := conn.ConnectionState()
	if state.NegotiatedProtocol != acme.ALPNProto || len(state.PeerCertificates) == 0 {
		return &problem{Type: "urn:ietf:params:acme:error:unauthorized", Detail: name + ": the server did not negotiate acme-tls/1"}
	}
	leaf := state.PeerCertificates[0]
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != name {
		return &problem{Type: "urn:ietf:params:acme:error:unauthorized", Detail: name + ": the challenge certificate names another domain"}
	}
	want := sha256.Sum256([]byte(keyAuthorization))
	for _, extension := range leaf.Extensions {
		if !extension.Id.Equal(idPeACMEIdentifier) {
			continue
		}
		var got []byte
		if _, err := asn1.Unmarshal(extension.Value, &got); err != nil || !extension.Critical || !bytes.Equal(got, want[:]) {
			return &problem{Type: "urn:ietf:params:acme:error:unauthorized", Detail: name + ": incorrect acmeIdentifier"}
		}
		return nil
	}
	return &problem{Type: "urn:ietf:params:acme:error:unauthorized", Detail: name + ": no acmeIdentifier extension"}
}

func validateHTTP(target, name, token, keyAuthorization string) *problem {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+target+"/.well-known/acme-challenge/"+token, nil)
	if err != nil {
		return &problem{Type: "urn:ietf:params:acme:error:malformed", Detail: err.Error()}
	}
	request.Host = name
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return &problem{Type: "urn:ietf:params:acme:error:connection", Detail: fmt.Sprintf("%s: Fetching http://%s/.well-known/acme-challenge/%s: %v", name, name, token, err)}
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != keyAuthorization {
		return &problem{Type: "urn:ietf:params:acme:error:unauthorized", Detail: fmt.Sprintf("%s: Invalid response from http://%s/.well-known/acme-challenge/%s: %d", name, name, token, response.StatusCode)}
	}
	return nil
}

func (ca *CA) finalize(w http.ResponseWriter, request *jws, o *order) {
	if o == nil {
		writeProblem(w, http.StatusNotFound, "malformed", "no such order")
		return
	}
	if o.status != acme.StatusReady {
		writeProblem(w, http.StatusForbidden, "orderNotReady", "the order is "+o.status)
		return
	}
	var payload struct {
		CSR string `json:"csr"`
	}
	if err := json.Unmarshal(request.payload, &payload); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", "csr")
		return
	}
	der, err := base64.RawURLEncoding.DecodeString(payload.CSR)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "badCSR", "csr encoding")
		return
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil || csr.CheckSignature() != nil {
		writeProblem(w, http.StatusBadRequest, "badCSR", "csr signature")
		return
	}
	if len(csr.DNSNames) != len(o.names) {
		writeProblem(w, http.StatusBadRequest, "badCSR", "the csr names other identifiers")
		return
	}
	for i, name := range o.names {
		if csr.DNSNames[i] != name {
			writeProblem(w, http.StatusBadRequest, "badCSR", "the csr names other identifiers")
			return
		}
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	now := time.Now().Truncate(time.Second)
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: o.names[0]}, DNSNames: o.names,
		NotBefore: now, NotAfter: now.Add(ca.lifetime),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, template, ca.issuer, csr.PublicKey, ca.issuerKey)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "serverInternal", err.Error())
		return
	}
	leaf, _ := x509.ParseCertificate(leafDER)
	ca.issued = append(ca.issued, leaf)
	var chain bytes.Buffer
	_ = pem.Encode(&chain, &pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	_ = pem.Encode(&chain, &pem.Block{Type: "CERTIFICATE", Bytes: ca.issuer.Raw})
	id := ca.newID()
	ca.certificates[id] = chain.Bytes()
	o.certificate, o.status = id, acme.StatusValid
	if ca.asyncFinalize {
		o.status, o.processing = acme.StatusProcessing, true
		ca.writeOrderBody(w, http.StatusOK, o, false)
		return
	}
	ca.writeOrder(w, http.StatusOK, o)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeProblem(w http.ResponseWriter, status int, kind, detail string) {
	if !strings.HasPrefix(kind, "urn:") {
		kind = "urn:ietf:params:acme:error:" + kind
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Type: kind, Detail: detail, Status: status})
}
