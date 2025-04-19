package mkot

import "time"

type TlsConfig struct {
	// Path to the CA cert. For a client this verifies the server certificate.
	// For a server this verifies client certificates. If empty uses system root CA.
	// (optional)
	CaFile string `yaml:"ca_file,omitempty"`

	// In memory PEM encoded cert. (optional)
	CaPem OpaqueString `yaml:"ca_pem,omitempty"`

	// If true, load system CA certificates pool in addition to the certificates
	// configured in this struct.
	IncludeSystemCaCertsPool bool `yaml:"include_system_ca_certs_pool,omitempty"`

	// Path to the TLS cert to use for TLS required connections. (optional)
	CertFile string `yaml:"cert_file,omitempty"`

	// In memory PEM encoded TLS cert to use for TLS required connections. (optional)
	CertPem OpaqueString `yaml:"cert_pem,omitempty"`

	// Path to the TLS key to use for TLS required connections. (optional)
	KeyFile string `yaml:"key_file,omitempty"`

	// In memory PEM encoded TLS key to use for TLS required connections. (optional)
	KeyPem OpaqueString `yaml:"key_pem,omitempty"`

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
	CurvePreferences []string `mapstructure:"curve_preferences,omitempty"`

	// In gRPC and HTTP when set to true, this is used to disable the client transport security.
	// See https://godoc.org/google.golang.org/grpc#WithInsecure for gRPC.
	// Please refer to https://godoc.org/crypto/tls#Config for more information.
	// (optional, default false)
	Insecure bool `yaml:"insecure"`
	// InsecureSkipVerify will enable TLS but not verify the certificate.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}
