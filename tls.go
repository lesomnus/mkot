// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Copied from https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configtls/configtls.go

package mkot

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/lesomnus/mkot/opaque"
)

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

	return &tls.Config{
		Certificates: certs,
		ClientCAs:    pool,
		// MinVersion:         minVersion,
		// MaxVersion:         maxVersion,
		// CipherSuites:       cipherSuites,
		// CurvePreferences:   curvePreferences,
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
	}, nil
}
