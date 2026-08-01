package otlp

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/lesomnus/mkot/opaque"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/grpc/credentials"
)

// AuthConfig applies a credential to every export, beyond the static headers.
// Exactly one of bearer/basic/oauth2 must be set.
//
// Unlike the collector — which references a configured auth *extension* by name
// (auth: {authenticator: bearertokenauth}) — mkot has no extensions, so the
// credential is configured inline here. The effect is the same: an Authorization
// header (gRPC per-RPC metadata / an HTTP request header) on every export.
type AuthConfig struct {
	Bearer *BearerAuth `yaml:"bearer,omitempty"`
	Basic  *BasicAuth  `yaml:"basic,omitempty"`
	OAuth2 *OAuth2Auth `yaml:"oauth2,omitempty"`

	// ts memoizes the oauth2 client-credentials source so its cached/refreshed
	// token is reused across exports.
	ts oauth2.TokenSource `yaml:"-"`
}

// BearerAuth sends "Authorization: <scheme> <token>". A token_file is re-read on
// each export so a rotated token is picked up without a restart.
type BearerAuth struct {
	Token     opaque.String `yaml:"token,omitempty"`
	TokenFile string        `yaml:"token_file,omitempty"`
	// Scheme is the Authorization scheme; defaults to "Bearer".
	Scheme string `yaml:"scheme,omitempty"`
}

// BasicAuth sends "Authorization: Basic base64(username:password)".
type BasicAuth struct {
	Username string        `yaml:"username,omitempty"`
	Password opaque.String `yaml:"password,omitempty"`
}

// OAuth2Auth performs the client-credentials flow and sends the resulting
// bearer token, refreshing it as it expires.
type OAuth2Auth struct {
	ClientID     string        `yaml:"client_id,omitempty"`
	ClientSecret opaque.String `yaml:"client_secret,omitempty"`
	TokenURL     string        `yaml:"token_url,omitempty"`
	Scopes       []string      `yaml:"scopes,omitempty"`
}

func (c *AuthConfig) validate() error {
	n := 0
	if c.Bearer != nil {
		n++
		if c.Bearer.Token == "" && c.Bearer.TokenFile == "" {
			return fmt.Errorf("auth.bearer: one of token or token_file is required")
		}
		if c.Bearer.Token != "" && c.Bearer.TokenFile != "" {
			return fmt.Errorf("auth.bearer: token and token_file are mutually exclusive")
		}
	}
	if c.Basic != nil {
		n++
		if c.Basic.Username == "" {
			return fmt.Errorf("auth.basic: username is required")
		}
	}
	if c.OAuth2 != nil {
		n++
		if c.OAuth2.ClientID == "" || c.OAuth2.TokenURL == "" {
			return fmt.Errorf("auth.oauth2: client_id and token_url are required")
		}
	}
	switch n {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("auth: one of bearer, basic, or oauth2 is required")
	default:
		return fmt.Errorf("auth: bearer, basic, and oauth2 are mutually exclusive")
	}
}

// header returns the Authorization header value for one request. It is the
// single source of truth shared by the gRPC and HTTP paths.
func (c *AuthConfig) header(ctx context.Context) (string, error) {
	switch {
	case c.Bearer != nil:
		scheme := c.Bearer.Scheme
		if scheme == "" {
			scheme = "Bearer"
		}
		tok := string(c.Bearer.Token)
		if c.Bearer.TokenFile != "" {
			b, err := os.ReadFile(c.Bearer.TokenFile)
			if err != nil {
				return "", fmt.Errorf("auth.bearer: read token_file: %w", err)
			}
			tok = strings.TrimSpace(string(b))
		}
		return scheme + " " + tok, nil
	case c.Basic != nil:
		raw := c.Basic.Username + ":" + string(c.Basic.Password)
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw)), nil
	case c.OAuth2 != nil:
		tok, err := c.tokenSource().Token()
		if err != nil {
			return "", fmt.Errorf("auth.oauth2: fetch token: %w", err)
		}
		return tok.Type() + " " + tok.AccessToken, nil
	default:
		return "", fmt.Errorf("auth: no credential configured")
	}
}

// tokenSource is a lazily-built, caching client-credentials token source. It is
// memoized so the cached/refreshed token is reused across exports.
func (c *AuthConfig) tokenSource() oauth2.TokenSource {
	if c.ts == nil {
		cc := &clientcredentials.Config{
			ClientID:     c.OAuth2.ClientID,
			ClientSecret: string(c.OAuth2.ClientSecret),
			TokenURL:     c.OAuth2.TokenURL,
			Scopes:       c.OAuth2.Scopes,
		}
		c.ts = cc.TokenSource(context.Background())
	}
	return c.ts
}

// perRPCCredentials adapts the credential to gRPC's per-RPC metadata. It permits
// insecure transports (RequireTransportSecurity=false) so a token still works
// against a plaintext local collector, matching the collector's behavior; use
// TLS to protect the credential on the wire.
func (c *AuthConfig) perRPCCredentials() credentials.PerRPCCredentials {
	return perRPC{c}
}

type perRPC struct{ c *AuthConfig }

func (p perRPC) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	v, err := p.c.header(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{"authorization": v}, nil
}

func (perRPC) RequireTransportSecurity() bool { return false }

// roundTripper wraps base so every HTTP export carries the Authorization header.
func (c *AuthConfig) roundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &authRoundTripper{c: c, base: base}
}

type authRoundTripper struct {
	c    *AuthConfig
	base http.RoundTripper
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	v, err := t.c.header(req.Context())
	if err != nil {
		return nil, err
	}
	// Clone: RoundTrippers must not mutate the caller's request.
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", v)
	return t.base.RoundTrip(r)
}
