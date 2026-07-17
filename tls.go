// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Copied from https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configtls/configtls.go

package mkot

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/lesomnus/mkot/opaque"
)

// certReloader re-reads a cert/key pair from disk when the reload interval has
// elapsed, so a rotated client certificate is picked up without a restart. On a
// transient read/parse error it keeps serving the last good keypair.
type certReloader struct {
	certFile string
	keyFile  string
	interval time.Duration

	mu       sync.Mutex
	cert     *tls.Certificate
	loadedAt time.Time
}

func (r *certReloader) get() (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cert != nil && time.Since(r.loadedAt) < r.interval {
		return r.cert, nil
	}
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		if r.cert != nil {
			return r.cert, nil // keep the last good cert on a transient failure
		}
		return nil, err
	}
	r.cert = &cert
	r.loadedAt = time.Now()
	return r.cert, nil
}

// TLSConfig exposes the common client and server TLS configurations.
// Note: Since there isn't anything specific to a server connection. Components
// with server connections should use TLSConfig.
type TLSConfig struct {
	// Path to the CA cert. For a client this verifies the server certificate.
	// For a server this verifies client certificates. If empty uses system root CA.
	// (optional)
	CAFile string `yaml:"ca_file,omitempty"`

	// In memory PEM encoded cert. (optional)
	CAPem opaque.String `yaml:"ca_pem,omitempty"`

	// If true, load system CA certificates pool in addition to the certificates
	// configured in this struct.
	IncludeSystemCACertsPool bool `yaml:"include_system_ca_certs_pool,omitempty"`

	// Path to the TLS cert to use for TLS required connections. (optional)
	CertFile string `yaml:"cert_file,omitempty"`

	// In memory PEM encoded TLS cert to use for TLS required connections. (optional)
	CertPem opaque.String `yaml:"cert_pem,omitempty"`

	// Path to the TLS key to use for TLS required connections. (optional)
	KeyFile string `yaml:"key_file,omitempty"`

	// In memory PEM encoded TLS key to use for TLS required connections. (optional)
	KeyPem opaque.String `yaml:"key_pem,omitempty"`

	// MinVersion sets the minimum TLS version that is acceptable.
	// If not set, TLS 1.2 will be used. (optional)
	MinVersion string `yaml:"min_version,omitempty"`

	// MaxVersion sets the maximum TLS version that is acceptable.
	// If not set, refer to crypto/tls for defaults. (optional)
	MaxVersion string `yaml:"max_version,omitempty"`

	// CipherSuites is a list of TLS cipher suites that the TLS transport can use.
	// If left blank, a safe default list is used.
	// See https://go.dev/src/crypto/tls/cipher_suites.go for a list of supported cipher suites.
	CipherSuites []string `yaml:"cipher_suites,omitempty"`

	// ReloadInterval specifies the duration after which the certificate will be reloaded
	// If not set, it will never be reloaded (optional)
	ReloadInterval time.Duration `yaml:"reload_interval,omitempty"`

	// contains the elliptic curves that will be used in
	// an ECDHE handshake, in preference order
	// Defaults to empty list and "crypto/tls" defaults are used, internally.
	CurvePreferences []string `yaml:"curve_preferences,omitempty"`

	// Trusted platform module configuration
	// TPMConfig TPMConfig `yaml:"tpm,omitempty"`
}

// ClientTlsConfig contains TLS configurations that are specific to client
// connections in addition to the common configurations. This should be used by
// components configuring TLS client connections.
type ClientTlsConfig struct {
	TLSConfig `yaml:",inline"`

	// These are config options specific to client connections.

	// In gRPC and HTTP when set to true, this is used to disable the client transport security.
	// See https://godoc.org/google.golang.org/grpc#WithInsecure for gRPC.
	// Please refer to https://godoc.org/crypto/tls#Config for more information.
	// (optional, default false)
	Insecure bool `yaml:"insecure,omitempty"`
	// InsecureSkipVerify will enable TLS but not verify the certificate.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
	// ServerName requested by client for virtual hosting.
	// This sets the ServerName in the TLSConfig. Please refer to
	// https://godoc.org/crypto/tls#Config for more information. (optional)
	ServerName string `yaml:"server_name_override,omitempty"`
}

func (c ClientTlsConfig) Build() (*tls.Config, error) {
	var ca_pem []byte
	if c.CAPem != "" {
		ca_pem = []byte(c.CAPem)
	} else if c.CAFile != "" {
		var err error
		ca_pem, err = os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
	}

	var pool *x509.CertPool
	if ca_pem == nil || c.IncludeSystemCACertsPool {
		var err error
		pool, err = x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("get system cert pool: %w", err)
		}
	} else {
		pool = x509.NewCertPool()
	}
	if ca_pem != nil {
		if ok := pool.AppendCertsFromPEM(ca_pem); !ok {
			return nil, fmt.Errorf("failed to append CA cert from PEM")
		}
	}

	var certs []tls.Certificate
	var get_client_cert func(*tls.CertificateRequestInfo) (*tls.Certificate, error)
	if c.ReloadInterval > 0 {
		// Reload only works for on-disk cert/key: an in-memory PEM cannot rotate.
		// Serve a callback that re-reads the keypair once the interval elapses so
		// a rotated client cert (cert-manager, SPIFFE) is picked up without a
		// restart.
		if c.CertFile == "" || c.KeyFile == "" {
			return nil, fmt.Errorf("reload_interval requires cert_file and key_file")
		}
		r := &certReloader{certFile: c.CertFile, keyFile: c.KeyFile, interval: c.ReloadInterval}
		if _, err := r.get(); err != nil { // fail fast on a bad initial keypair
			return nil, fmt.Errorf("load TLS cert: %w", err)
		}
		get_client_cert = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return r.get() }
	} else {
		var cert_pem, key_pem []byte
		if c.CertPem != "" {
			cert_pem = []byte(c.CertPem)
		} else if c.CertFile != "" {
			var err error
			cert_pem, err = os.ReadFile(c.CertFile)
			if err != nil {
				return nil, fmt.Errorf("read cert file: %w", err)
			}
		}
		if c.KeyPem != "" {
			key_pem = []byte(c.KeyPem)
		} else if c.KeyFile != "" {
			var err error
			key_pem, err = os.ReadFile(c.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("read key file: %w", err)
			}
		}
		if cert_pem != nil && key_pem != nil {
			cert, err := tls.X509KeyPair(cert_pem, key_pem)
			if err != nil {
				return nil, fmt.Errorf("load TLS cert from PEM: %w", err)
			}
			certs = []tls.Certificate{cert}
		} else if cert_pem != nil || key_pem != nil {
			return nil, fmt.Errorf("both cert and key must be provided together")
		}
	}

	var min_version, max_version uint16
	if c.MinVersion != "" {
		var err error
		if min_version, err = parseTLSVersion(c.MinVersion); err != nil {
			return nil, fmt.Errorf("min_version: %w", err)
		}
	}
	if c.MaxVersion != "" {
		var err error
		if max_version, err = parseTLSVersion(c.MaxVersion); err != nil {
			return nil, fmt.Errorf("max_version: %w", err)
		}
	}
	cipher_suites, err := parseCipherSuites(c.CipherSuites)
	if err != nil {
		return nil, err
	}
	curve_preferences, err := parseCurvePreferences(c.CurvePreferences)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates:         certs,
		GetClientCertificate: get_client_cert,
		// This is a CLIENT config: the peer (server) certificate is verified
		// against RootCAs. ClientCAs is a server-side field and would leave a
		// configured CA silently unused.
		RootCAs: pool,
		// MinVersion is left at 0 (crypto/tls defaults a client to TLS 1.2)
		// unless the config pins a floor.
		MinVersion: min_version,
		MaxVersion: max_version,
		// CipherSuites only governs TLS 1.2 handshakes; crypto/tls ignores it for
		// TLS 1.3 (whose suites are not configurable in Go).
		CipherSuites:       cipher_suites,
		CurvePreferences:   curve_preferences,
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
	}, nil
}

// parseTLSVersion maps a collector-style version string ("1.0".."1.3") to the
// crypto/tls constant.
func parseTLSVersion(s string) (uint16, error) {
	switch s {
	case "1.0":
		return tls.VersionTLS10, nil
	case "1.1":
		return tls.VersionTLS11, nil
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unsupported version %q (want 1.0, 1.1, 1.2, or 1.3)", s)
	}
}

// parseCipherSuites resolves crypto/tls cipher suite names (e.g.
// "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256") to their IDs, erroring on any
// unknown name rather than silently dropping the restriction.
func parseCipherSuites(names []string) ([]uint16, error) {
	if len(names) == 0 {
		return nil, nil
	}
	by_name := map[string]uint16{}
	for _, cs := range tls.CipherSuites() {
		by_name[cs.Name] = cs.ID
	}
	for _, cs := range tls.InsecureCipherSuites() {
		by_name[cs.Name] = cs.ID
	}
	ids := make([]uint16, 0, len(names))
	for _, n := range names {
		id, ok := by_name[n]
		if !ok {
			return nil, fmt.Errorf("unsupported cipher suite %q", n)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// parseCurvePreferences maps curve names to crypto/tls CurveIDs in the given
// preference order.
func parseCurvePreferences(names []string) ([]tls.CurveID, error) {
	if len(names) == 0 {
		return nil, nil
	}
	by_name := map[string]tls.CurveID{
		"X25519": tls.X25519,
		"P256":   tls.CurveP256,
		"P384":   tls.CurveP384,
		"P521":   tls.CurveP521,
	}
	ids := make([]tls.CurveID, 0, len(names))
	for _, n := range names {
		id, ok := by_name[n]
		if !ok {
			return nil, fmt.Errorf("unsupported curve %q (want X25519, P256, P384, or P521)", n)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
