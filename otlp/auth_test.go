package otlp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func TestAuthValidate(t *testing.T) {
	for _, c := range []*AuthConfig{
		{},                      // none
		{Bearer: &BearerAuth{}}, // bearer needs token/file
		{Bearer: &BearerAuth{Token: "a", TokenFile: "/f"}},                  // both
		{Basic: &BasicAuth{}},                                               // basic needs username
		{OAuth2: &OAuth2Auth{ClientID: "id"}},                               // oauth2 needs token_url
		{Bearer: &BearerAuth{Token: "a"}, Basic: &BasicAuth{Username: "u"}}, // two
	} {
		if err := c.validate(); err == nil {
			t.Fatalf("%+v must fail validation", c)
		}
	}
	// Each single, complete form validates.
	for _, c := range []*AuthConfig{
		{Bearer: &BearerAuth{Token: "t"}},
		{Bearer: &BearerAuth{TokenFile: "/some/path"}},
		{Basic: &BasicAuth{Username: "u", Password: "p"}},
		{OAuth2: &OAuth2Auth{ClientID: "id", TokenURL: "https://idp/token"}},
	} {
		if err := c.validate(); err != nil {
			t.Fatalf("%+v should validate: %v", c, err)
		}
	}
}

func TestAuthHeader(t *testing.T) {
	ctx, x := x.New(t)

	v, err := (&AuthConfig{Bearer: &BearerAuth{Token: "abc"}}).header(ctx)
	x.NoError(err)
	x.Eq("Bearer abc", v)

	v, err = (&AuthConfig{Bearer: &BearerAuth{Token: "abc", Scheme: "Token"}}).header(ctx)
	x.NoError(err)
	x.Eq("Token abc", v)

	v, err = (&AuthConfig{Basic: &BasicAuth{Username: "user", Password: "pass"}}).header(ctx)
	x.NoError(err)
	x.Eq("Basic dXNlcjpwYXNz", v) // base64("user:pass")

	// token_file is re-read each call, so a rotated token is picked up.
	dir := t.TempDir()
	p := filepath.Join(dir, "token")
	x.NoError(os.WriteFile(p, []byte("first\n"), 0o600))
	a := &AuthConfig{Bearer: &BearerAuth{TokenFile: p}}
	v, err = a.header(ctx)
	x.NoError(err)
	x.Eq("Bearer first", v) // trailing newline trimmed
	x.NoError(os.WriteFile(p, []byte("second"), 0o600))
	v, err = a.header(ctx)
	x.NoError(err)
	x.Eq("Bearer second", v)
}

// authGRPCSink records the authorization metadata of incoming exports.
type authGRPCSink struct {
	collectortracepb.UnimplementedTraceServiceServer
	mu   sync.Mutex
	auth string
}

func (s *authGRPCSink) Export(ctx context.Context, _ *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("authorization"); len(v) > 0 {
			s.mu.Lock()
			s.auth = v[0]
			s.mu.Unlock()
		}
	}
	return &collectortracepb.ExportTraceServiceResponse{}, nil
}

func (s *authGRPCSink) seen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.auth
}

// gRPC auth is carried as per-RPC "authorization" metadata on every export.
func TestAuthGRPC(t *testing.T) {
	ctx, x := x.New(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	x.NoError(err)
	sink := &authGRPCSink{}
	srv := grpc.NewServer()
	collectortracepb.RegisterTraceServiceServer(srv, sink)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	src := `
exporters:
  otlp:
    endpoint: "` + lis.Addr().String() + `"
    tls: { insecure: true }
    auth:
      bearer:
        token: "sekret"
providers:
  tracer:
    exporters: [otlp]
`
	var c mkot.Config
	x.NoError(yaml.Unmarshal([]byte(src), &c))
	r := mkot.Make(ctx, &c)
	tp, err := r.Tracer(ctx, "")
	x.NoError(err)
	x.NoError(r.Start(ctx))
	_, span := tp.Tracer("t").Start(ctx, "s")
	span.End()
	x.NoError(r.Shutdown(context.Background()))
	x.Eq("Bearer sekret", sink.seen())
}

// HTTP auth is carried as the Authorization request header, alongside the
// SDK-managed request (a custom client owns TLS/proxy/timeout).
func TestAuthHTTP(t *testing.T) {
	ctx, x := x.New(t)
	got := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/x-protobuf")
		resp, _ := proto.Marshal(&collectortracepb.ExportTraceServiceResponse{})
		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)

	src := `
exporters:
  otlp:
    protocol: http
    endpoint: "` + srv.URL + `"
    auth:
      basic:
        username: user
        password: pass
providers:
  tracer:
    exporters: [otlp]
`
	var c mkot.Config
	x.NoError(yaml.Unmarshal([]byte(src), &c))
	r := mkot.Make(ctx, &c)
	tp, err := r.Tracer(ctx, "")
	x.NoError(err)
	x.NoError(r.Start(ctx))
	_, span := tp.Tracer("t").Start(ctx, "s")
	span.End()
	x.NoError(r.Shutdown(context.Background()))

	select {
	case v := <-got:
		x.Eq("Basic dXNlcjpwYXNz", v)
	case <-time.After(3 * time.Second):
		t.Fatal("no export received")
	}
}
