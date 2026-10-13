package panelcert

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// errTransportClosed is what a closed issuance transport answers: the
// autocert.Manager of a finished issuance never reaches the CA again.
var errTransportClosed = errors.New("panel certificate: this issuance has ended")

// caProblem is a failure the CA reported: an RFC 7807 problem document, or
// the error of an invalid challenge.
type caProblem struct {
	Identifier string
	Challenge  string
	Type       string
	Detail     string
}

func (p caProblem) String() string {
	label := p.Challenge
	if label == "" {
		label = shortProblemType(p.Type)
	}
	if label == "" {
		return p.Detail
	}
	return label + ": " + p.Detail
}

// shortProblemType is the last part of an ACME problem type
// ("urn:ietf:params:acme:error:dns" -> "dns").
func shortProblemType(value string) string {
	if index := strings.LastIndex(value, ":"); index >= 0 {
		return value[index+1:]
	}
	return value
}

// recordingTransport carries one issuance's ACME requests and keeps the
// problems the CA answered with: autocert tries TLS-ALPN-01 and then
// HTTP-01 and reports only that no challenge type worked, while the CA's
// documents say why (a timeout on port 80, an NXDOMAIN, a wrong answer).
// Closing it cuts the issuance's manager off the CA.
type recordingTransport struct {
	base   http.RoundTripper
	closed atomic.Bool

	mu       sync.Mutex
	problems []caProblem
}

func newRecordingTransport(base http.RoundTripper) *recordingTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &recordingTransport{base: base}
}

func (t *recordingTransport) close() { t.closed.Store(true) }

func (t *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.closed.Load() {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, errTransportClosed
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "json") {
		return response, nil
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	_ = response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	if readErr == nil {
		t.inspect(contentType, body)
	}
	return response, nil
}

// acmeDocument is the part of an ACME answer the transport reads: a
// problem, or an authorization or order with its challenges.
type acmeDocument struct {
	Type        string `json:"type"`
	Detail      string `json:"detail"`
	Status      string `json:"status"`
	Subproblems []struct {
		Type       string `json:"type"`
		Detail     string `json:"detail"`
		Identifier struct {
			Value string `json:"value"`
		} `json:"identifier"`
	} `json:"subproblems"`
	Identifier struct {
		Value string `json:"value"`
	} `json:"identifier"`
	Challenges []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
		Error  *struct {
			Type   string `json:"type"`
			Detail string `json:"detail"`
		} `json:"error"`
	} `json:"challenges"`
	Error *struct {
		Type   string `json:"type"`
		Detail string `json:"detail"`
	} `json:"error"`
}

func (t *recordingTransport) inspect(contentType string, body []byte) {
	var document acmeDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return
	}
	var found []caProblem
	if strings.Contains(contentType, "problem+json") && document.Detail != "" {
		if shortProblemType(document.Type) == "badNonce" {
			return // the client retries these by itself
		}
		found = append(found, caProblem{Type: document.Type, Detail: document.Detail})
		for _, sub := range document.Subproblems {
			found = append(found, caProblem{Identifier: sub.Identifier.Value, Type: sub.Type, Detail: sub.Detail})
		}
	}
	if document.Status == "invalid" {
		for _, challenge := range document.Challenges {
			if challenge.Error != nil && challenge.Error.Detail != "" {
				found = append(found, caProblem{Identifier: document.Identifier.Value, Challenge: challenge.Type, Type: challenge.Error.Type, Detail: challenge.Error.Detail})
			}
		}
		if document.Error != nil && document.Error.Detail != "" {
			found = append(found, caProblem{Identifier: document.Identifier.Value, Type: document.Error.Type, Detail: document.Error.Detail})
		}
	}
	if len(found) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, problem := range found {
		duplicate := false
		for _, known := range t.problems {
			if known == problem {
				duplicate = true
				break
			}
		}
		if !duplicate {
			t.problems = append(t.problems, problem)
		}
	}
}

func (t *recordingTransport) recorded() []caProblem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]caProblem(nil), t.problems...)
}

// issuanceError is a failed issuance: autocert's error and what the CA
// said.
type issuanceError struct {
	err      error
	problems []caProblem
}

func (e *issuanceError) Error() string {
	if len(e.problems) == 0 {
		return e.err.Error()
	}
	parts := make([]string, 0, len(e.problems))
	for _, problem := range e.problems {
		parts = append(parts, problem.String())
	}
	return fmt.Sprintf("the CA refused the certificate: %s", strings.Join(parts, "; "))
}

func (e *issuanceError) Unwrap() error { return e.err }

// classify names the class of an issuance failure from the CA's problems
// and the error itself.
func classify(err error, problems []caProblem) string {
	for _, problem := range problems {
		switch shortProblemType(problem.Type) {
		case "rateLimited":
			return ProblemRateLimited
		}
	}
	for _, problem := range problems {
		switch shortProblemType(problem.Type) {
		case "dns":
			return ProblemDNSUnresolved
		case "connection", "tls":
			return ProblemUnreachable
		case "unauthorized", "incorrectResponse", "caa":
			return ProblemChallenge
		}
	}
	if len(problems) > 0 {
		return ProblemCAError
	}
	if err != nil {
		text := err.Error()
		for _, marker := range []string{"dial tcp", "no such host", "connection refused", "i/o timeout", "TLS handshake", "certificate signed by unknown authority", "context deadline exceeded"} {
			if strings.Contains(text, marker) {
				return ProblemCAUnreachable
			}
		}
	}
	return ProblemCAError
}
