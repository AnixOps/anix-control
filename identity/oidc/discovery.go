// Package oidc renders the OpenID Connect discovery document of the identity
// issuer, so that any service can find its signing keys (jwks_uri) from the
// iss claim alone. Only token verification metadata is published: the
// identity module does not run OAuth flows.
package oidc

import (
	"errors"
	"net/url"
	"strings"

	"github.com/AnixOps/anix-control/sdk/identitytoken"
)

// WellKnownPath is where the discovery document is served, relative to the
// issuer URL.
const WellKnownPath = "/.well-known/openid-configuration"

// Discovery is the subset of OpenID Provider Metadata the identity issuer
// publishes.
type Discovery struct {
	Issuer                           string   `json:"issuer"`
	JWKSURI                          string   `json:"jwks_uri"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}

// NewDiscovery returns the discovery document for an issuer URL and the URL
// of its JWKS.
func NewDiscovery(issuerURL, jwksURL string) (Discovery, error) {
	for _, value := range []string{issuerURL, jwksURL} {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Host == "" {
			return Discovery{}, errors.New("issuer and JWKS must be absolute http(s) URLs")
		}
	}
	return Discovery{
		Issuer:                           strings.TrimSuffix(issuerURL, "/"),
		JWKSURI:                          jwksURL,
		IDTokenSigningAlgValuesSupported: []string{identitytoken.Algorithm},
		SubjectTypesSupported:            []string{"public"},
		ResponseTypesSupported:           []string{"id_token"},
		ClaimsSupported:                  []string{"iss", "sub", "aud", "exp", "iat", "jti", "user_id", "email", "is_admin", "sid", "tv"},
	}, nil
}
