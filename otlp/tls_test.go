package otlp

import (
	"context"
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
	"go.opentelemetry.io/otel/sdk/metric"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
		Subject:               pkix.Name{CommonName: "mkot-otlp-test"},
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

// A configured ca_pem must be used as the client's trust anchor.
func TestTLSCustomCA(t *testing.T) {
	ctx, x := x.New(t)
	pair, ca_pem := selfSignedCert(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	x.NoError(err)
	sink := &metricSink{}
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}})))
	collectormetricspb.RegisterMetricsServiceServer(srv, sink)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	e := ExporterConfig{
		Endpoint: lis.Addr().String(),
		TLS:      &mkot.ClientTlsConfig{TLSConfig: mkot.TLSConfig{CAPem: opaque.String(ca_pem)}},
		Interval: time.Hour,
	}
	_, opts, err := e.MetricReader(ctx)
	x.NoError(err)
	mp := metric.NewMeterProvider(opts...)
	t.Cleanup(func() { mp.Shutdown(context.Background()) })
	ctr, err := mp.Meter("test").Int64Counter("mkot.test.tls")
	x.NoError(err)
	ctr.Add(ctx, 1)
	err = mp.ForceFlush(ctx)
	x.NoError(err)
	x.Eq(true, sink.seen("mkot.test.tls"))
}
