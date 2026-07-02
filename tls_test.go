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
	"testing"
	"time"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"github.com/lesomnus/mkot/opaque"
)

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
