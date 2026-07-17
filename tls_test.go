package mkot_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"github.com/lesomnus/mkot/opaque"
)

// issueKeypairPEM returns a fresh self-signed cert/key pair in PEM, tagged with
// the given common name so a reload can be observed.
func issueKeypairPEM(t *testing.T, cn string) (cert_pem, key_pem []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert_pem = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	key_der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	key_pem = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key_der})
	return cert_pem, key_pem
}

// A rotated on-disk client cert must be picked up via GetClientCertificate once
// the reload interval elapses, and reload requires files (not in-memory PEM).
func TestClientTlsConfigReload(t *testing.T) {
	_, x := x.New(t)

	if _, err := (mkot.ClientTlsConfig{TLSConfig: mkot.TLSConfig{
		ReloadInterval: time.Second, CertPem: "x", KeyPem: "y",
	}}).Build(); err == nil {
		t.Fatal("reload_interval without cert_file/key_file must error")
	}

	dir := t.TempDir()
	cert_path := filepath.Join(dir, "cert.pem")
	key_path := filepath.Join(dir, "key.pem")
	write := func(cn string) {
		cp, kp := issueKeypairPEM(t, cn)
		x.NoError(os.WriteFile(cert_path, cp, 0o600))
		x.NoError(os.WriteFile(key_path, kp, 0o600))
	}
	leaf_cn := func(cert *tls.Certificate) string {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		x.NoError(err)
		return leaf.Subject.CommonName
	}

	write("first")
	conf, err := mkot.ClientTlsConfig{TLSConfig: mkot.TLSConfig{
		CertFile: cert_path, KeyFile: key_path, ReloadInterval: time.Nanosecond,
	}}.Build()
	x.NoError(err)
	if conf.GetClientCertificate == nil {
		t.Fatal("expected a reloading GetClientCertificate callback")
	}
	c1, err := conf.GetClientCertificate(&tls.CertificateRequestInfo{})
	x.NoError(err)
	x.Eq("first", leaf_cn(c1))

	write("second")
	time.Sleep(time.Millisecond) // let the nanosecond reload interval elapse
	c2, err := conf.GetClientCertificate(&tls.CertificateRequestInfo{})
	x.NoError(err)
	x.Eq("second", leaf_cn(c2))
}

// selfSignedCert issues a CA-capable self-signed cert for 127.0.0.1, returned
// as a server keypair plus its PEM (usable as the client's trust anchor).
func selfSignedCert(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "mkot-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert_pem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	key_der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	key_pem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key_der})
	pair, err := tls.X509KeyPair(cert_pem, key_pem)
	if err != nil {
		t.Fatal(err)
	}
	return pair, cert_pem
}

// min/max version, cipher suites, and curve preferences must reach the built
// *tls.Config instead of being silently dropped.
func TestClientTlsConfigVersionsSuitesCurves(t *testing.T) {
	_, x := x.New(t)

	conf, err := mkot.ClientTlsConfig{TLSConfig: mkot.TLSConfig{
		MinVersion:       "1.2",
		MaxVersion:       "1.3",
		CipherSuites:     []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
		CurvePreferences: []string{"X25519", "P256"},
	}}.Build()
	x.NoError(err)
	x.Eq(uint16(tls.VersionTLS12), conf.MinVersion)
	x.Eq(uint16(tls.VersionTLS13), conf.MaxVersion)
	x.Eq(1, len(conf.CipherSuites))
	x.Eq(tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, conf.CipherSuites[0])
	x.Eq(2, len(conf.CurvePreferences))
	x.Eq(tls.X25519, conf.CurvePreferences[0])
	x.Eq(tls.CurveP256, conf.CurvePreferences[1])

	// Unknown values are rejected, not silently dropped.
	for _, c := range []mkot.ClientTlsConfig{
		{TLSConfig: mkot.TLSConfig{MinVersion: "1.4"}},
		{TLSConfig: mkot.TLSConfig{MaxVersion: "nope"}},
		{TLSConfig: mkot.TLSConfig{CipherSuites: []string{"TLS_NOT_A_SUITE"}}},
		{TLSConfig: mkot.TLSConfig{CurvePreferences: []string{"P999"}}},
	} {
		if _, err := c.Build(); err == nil {
			t.Fatalf("expected an error for %+v", c.TLSConfig)
		}
	}
}

// A configured CA must become the CLIENT trust anchor (RootCAs): the handshake
// against a server signed by that CA succeeds, and fails without it.
func TestClientTlsConfigBuildCustomCA(t *testing.T) {
	_, x := x.New(t)
	pair, ca_pem := selfSignedCert(t)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{pair}})
	x.NoError(err)
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				conn.(*tls.Conn).Handshake()
				conn.Close()
			}()
		}
	}()

	t.Run("trusted with the configured CA", func(t *testing.T) {
		conf, err := mkot.ClientTlsConfig{TLSConfig: mkot.TLSConfig{CAPem: opaque.String(ca_pem)}}.Build()
		x.NoError(err)
		conn, err := tls.Dial("tcp", ln.Addr().String(), conf)
		x.NoError(err)
		conn.Close()
	})
	t.Run("untrusted without it", func(t *testing.T) {
		conf, err := mkot.ClientTlsConfig{}.Build()
		x.NoError(err)
		if conn, err := tls.Dial("tcp", ln.Addr().String(), conf); err == nil {
			conn.Close()
			t.Fatal("handshake against a self-signed server should fail with the system pool")
		}
	})
}
