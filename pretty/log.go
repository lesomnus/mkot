package pretty

import (
	"bytes"
	"context"
	"hash/fnv"
	"io"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

type LogExporter struct {
	Out io.Writer
}

type logKind int

const (
	logKindUnknown logKind = 0

	logKindHttp   logKind = 0b0100_0000
	logKindGrpc   logKind = 0b1000_0000
	logKindServer logKind = 0b0001_0000
	logKindClient logKind = 0b0010_0000
	logKindRecv   logKind = 0b0000_0010
	logKindEnd    logKind = 0b0000_0110

	// logKindClientStream logKind = 0b0000_0010
	// logKindServerStream logKind = 0b0000_0100

	logKindHttpIngress logKind = logKindHttp | logKindServer
	logKindHttpEgress  logKind = logKindHttp | logKindServer | logKindEnd
	logKindHttpReq     logKind = logKindHttp | logKindClient
	logKindHttpRes     logKind = logKindHttp | logKindClient | logKindRecv
	logKindHttpEnd     logKind = logKindHttp | logKindClient | logKindEnd

	logKindGrpcIngress logKind = logKindGrpc | logKindServer
	logKindGrpcEgress  logKind = logKindGrpc | logKindServer | logKindEnd
	logKindGrpcReq     logKind = logKindGrpc | logKindClient
	// logKindGrpcRes     logKind = logKindGrpc | logKindClient | logKindRecv
	logKindGrpcEnd logKind = logKindGrpc | logKindClient | logKindEnd
)

type kindHttpAttr struct {
	Method   string
	Url      string
	PeerAddr string
	Code     int64
	Elapsed  int64

	HasBody  bool
	BodySize int64
}
type kindGrpcAttr struct {
	Method   string
	Service  string
	PeerAddr string
	Code     int64
	Elapsed  int64

	NotClientStream bool

	// Unary or server streaming
	Size  int64 // Uncompressed size
	Bytes int64 // Compressed size

	// Client streaming or bidi streaming
	ReqBytes int64 // Compressed size
	ResBytes int64 // Compressed size
}

func (c *Config) LogExporter(ctx context.Context) (sdklog.Exporter, error) {
	return &LogExporter{Out: c.Output}, nil
}

func (e *LogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	for _, r := range records {
		title := ""
		body := r.Body().AsString()
		kind := logKindUnknown
		rest := make([]log.KeyValue, 0, r.AttributesLen())

		attr_http := kindHttpAttr{}
		attr_grpc := kindGrpcAttr{}

		r.WalkAttributes(func(kv log.KeyValue) bool {
			switch kv.Key {
			case "app.widget.name":
				title = kv.Value.AsString()

			case "trace_id":
			case "span_id":
				// skip

			case string(semconv.NetworkPeerAddressKey):
				kind |= logKindServer
				v := kv.Value.AsString()
				attr_http.PeerAddr = v
				attr_grpc.PeerAddr = v

			case string(semconv.HTTPRequestMethodKey):
				kind |= logKindHttp
				attr_http.Method = kv.Value.AsString()
			case string(semconv.URLPathKey):
				kind |= logKindHttp | logKindServer
				attr_http.Url = kv.Value.AsString()
			case string(semconv.URLOriginalKey):
				kind |= logKindHttp | logKindClient
				attr_http.Url = kv.Value.AsString()
			case string(semconv.HTTPResponseStatusCodeKey):
				kind |= logKindHttp | logKindRecv
				attr_http.Code = kv.Value.AsInt64()
			case "http.response.header.content-length":
				kind |= logKindHttp | logKindClient | logKindRecv
				v := kv.Value.AsString()
				if i, err := strconv.ParseInt(v, 10, 64); err == nil {
					attr_http.HasBody = true
					attr_http.BodySize = i
				}
			case string(semconv.HTTPResponseBodySizeKey):
				kind |= logKindHttp | logKindEnd
				attr_http.HasBody = true
				attr_http.BodySize = kv.Value.AsInt64()

			case string(semconv.RPCServiceKey):
				kind |= logKindGrpc
				attr_grpc.Service = kv.Value.AsString()
			case string(semconv.RPCMethodKey):
				kind |= logKindGrpc
				attr_grpc.Method = kv.Value.AsString()
			case string(semconv.RPCGRPCStatusCodeKey):
				kind |= logKindGrpc | logKindEnd
				attr_grpc.Code = kv.Value.AsInt64()
			case string(semconv.RPCMessageCompressedSizeKey):
				kind |= logKindGrpc
				attr_grpc.NotClientStream = true
				attr_grpc.Bytes = kv.Value.AsInt64()
			case string(semconv.RPCMessageUncompressedSizeKey):
				kind |= logKindGrpc
				attr_grpc.NotClientStream = true
				attr_grpc.Size = kv.Value.AsInt64()
			case "rpc.request.compressed_size":
				kind |= logKindGrpc | logKindEnd
				attr_grpc.ReqBytes = kv.Value.AsInt64()
			case "rpc.response.compressed_size":
				kind |= logKindGrpc | logKindEnd
				attr_grpc.ResBytes = kv.Value.AsInt64()

			case "server.elapsed_ns":
				kind |= logKindServer | logKindEnd
				v := kv.Value.AsInt64()
				attr_http.Elapsed = v
				attr_grpc.Elapsed = v
			case "client.elapsed_ns":
				kind |= logKindClient | logKindRecv
				v := kv.Value.AsInt64()
				attr_http.Elapsed = v
				attr_grpc.Elapsed = v

			default:
				rest = append(rest, kv)
			}
			return true
		})

		var sym string
		switch (r.Severity() - 1) / 4 {
		case 0: // Trace1~4
			sym = c_faint.Sprint(" • ")
		case 1: // Debug1~4
			sym = c_debug.Sprint(" ? ")
		case 2: // Info1~4
			sym = c_info.Sprint(" ○ ")
		case 3: // Warn1~4
			sym = c_warn.Sprint(" ! ")
		case 4: // Error1~4
			sym = c_error.Sprint(" x ")
		case 5: // Fatal1~5
			sym = c_error.Sprint("-x-")
		default:
			sym = c_error.Sprint(" • ")
		}

		b := bytes.NewBuffer(make([]byte, 0, 128))
		writeHeader(b, title, r.Timestamp(), sym, r.TraceID(), r.SpanID())
		switch kind {
		case logKindHttpIngress, logKindGrpcIngress:
			b.WriteString(c_req.Sprint("›» "))
		case logKindHttpEgress, logKindGrpcEgress:
			b.WriteString(c_res.Sprint("«‹ "))
		case logKindHttpReq, logKindGrpcReq:
			b.WriteString(c_req.Sprint(" ⇗ "))
		case logKindHttpRes:
			b.WriteString(c_res.Sprint("↙  "))
		case logKindHttpEnd, logKindGrpcEnd:
			b.WriteString(c_res.Sprint("⇙  "))
		}

		// |........| 02:12:51.612 ○ d58626 13882c ›» 127.0.0.1......... |GET| /foo:bar
		// |........| 02:12:51.612 ○ d58626 ccbeee  ⇗ registry:5000..... |GET| /v2/foo/manifests/bar
		// |........| 02:12:51.620 ○ d58626 ccbeee ↙  200 007.41ms 0588B |GET| /v2/foo/manifests/bar
		// |........| 02:12:51.620 ○ d58626 ccbeee ⇙  200 007.48ms 0588B |GET| /v2/foo/manifests/bar
		// |........| 02:12:51.620 ○ d58626 13882c «‹ 200 007.92ms 0012K |GET| /foo:bar
		// |........| 03:28:40.115 ○ ababab cdcdcd ›» peer_addr......... 0000B foo.SomeService/Method
		// |........| 03:28:40.116 ○ ababab cdcdcd «‹ .OK 653.86µs ..... 0000B foo.SomeService/Method
		// |........| 03:28:40.115 ○ ababab cdcdcd ›» peer_addr......... ..... foo.SomeService/Method
		// |........| 03:28:40.116 ○ ababab cdcdcd «‹ .OK 653.86µs 0000B 0000B foo.SomeService/Method
		switch kind {
		case logKindHttpIngress:
			e.writeHttpIngressLine(b, &attr_http)
		case logKindHttpEgress:
			e.writeHttpEgressLine(b, &attr_http)
		case logKindHttpReq:
			e.writeHttpReqLine(b, &attr_http)
		case logKindHttpRes, logKindHttpEnd:
			e.writeHttpResLine(b, &attr_http)
		case logKindGrpcIngress:
			e.writeGrpcIngressLine(b, &attr_grpc)
		case logKindGrpcEgress:
			e.writeGrpcEgressLine(b, &attr_grpc)
		default:
			e.writeLogLine(b, body, rest)
		}
		b.WriteByte('\n')

		e.Out.Write(b.Bytes())
	}
	return nil
}

func (e *LogExporter) Shutdown(ctx context.Context) error {
	return nil
}

func (e *LogExporter) ForceFlush(ctx context.Context) error {
	return nil
}

func (*LogExporter) writeLogLine(w *bytes.Buffer, msg string, attrs []log.KeyValue) {
	w.WriteString(msg)
	if len(attrs) == 0 {
		return
	}

	w.WriteString(c_faint.Sprint(" -"))
	for _, attr := range attrs {
		w.WriteString(" ")
		w.WriteString(attr.Key)
		w.WriteString("=")

		c := attr_colors[attr.Value.Kind()]
		switch attr.Value.Kind() {
		case log.KindBool,
			log.KindInt64,
			log.KindString:
			c.Fprint(w, attr.Value.String())
		case log.KindFloat64:
			v := attr.Value.AsFloat64()
			c.Fprintf(w, "%.[1]*g", 3, v)
		}
	}
}

func (*LogExporter) writeHttpIngressLine(w *bytes.Buffer, attr *kindHttpAttr) {
	method := httpMethodAbbr(attr.Method)
	peer_addr := attr.PeerAddr
	if l := len(peer_addr); l < 18 {
		peer_addr += c_faint.Sprint(".................."[:18-l])
	} else {
		peer_addr = peer_addr[:18]
	}

	w.WriteString(peer_addr)
	w.WriteByte(' ')
	w.WriteString(c_faint.Sprint("|"))
	w.WriteString(c_http_method.Sprint(method))
	w.WriteString(c_faint.Sprint("|"))
	w.WriteByte(' ')
	w.WriteString(attr.Url)
}

func (e *LogExporter) writeHttpEgressLine(w *bytes.Buffer, attr *kindHttpAttr) {
	method := httpMethodAbbr(attr.Method)
	dt := time.Duration(attr.Elapsed)

	code_s := ""
	if attr.Code >= 500 {
		code_s = c_error.Sprintf("%d ", attr.Code)
	} else if attr.Code >= 400 {
		code_s = c_warn.Sprintf("%d ", attr.Code)
	} else if attr.Code > 0 {
		code_s = c_info.Sprintf("%d ", attr.Code)
	} else {
		code_s = c_faint.Sprint("--- ")
	}

	w.WriteString(code_s)
	renderDuration(w, dt)
	w.WriteByte(' ')
	if attr.HasBody {
		humanizeBytes(w, attr.BodySize)
	} else {
		w.WriteString(c_faint.Sprint("----"))
		w.WriteByte('B')
	}
	w.WriteByte(' ')
	w.WriteString(c_faint.Sprint("|"))
	w.WriteString(c_http_method.Sprint(method))
	w.WriteString(c_faint.Sprint("|"))
	w.WriteByte(' ')
	w.WriteString(c_msg.Sprint(attr.Url))
}

func (e *LogExporter) writeHttpReqLine(w *bytes.Buffer, attr *kindHttpAttr) {
	method := httpMethodAbbr(attr.Method)
	domain := ""
	url := attr.Url
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	}
	if i := strings.Index(url, "/"); i >= 0 {
		domain = url[:i]
		url = url[i:]
	}
	if l := len(domain); l < 18 {
		domain += c_faint.Sprint(".................."[:18-l])
	} else {
		domain = domain[:18]
	}

	w.WriteString(domain)
	w.WriteByte(' ')
	w.WriteString(c_faint.Sprint("|"))
	w.WriteString(c_http_method.Sprint(method))
	w.WriteString(c_faint.Sprint("|"))
	w.WriteByte(' ')
	w.WriteString(c_msg.Sprint(url))
}

func (e *LogExporter) writeHttpResLine(w *bytes.Buffer, attr *kindHttpAttr) {
	method := httpMethodAbbr(attr.Method)
	dt := time.Duration(attr.Elapsed)

	code_s := ""
	if attr.Code >= 500 {
		code_s = c_error.Sprintf("%d ", attr.Code)
	} else if attr.Code >= 400 {
		code_s = c_warn.Sprintf("%d ", attr.Code)
	} else if attr.Code > 0 {
		code_s = c_info.Sprintf("%d ", attr.Code)
	} else {
		code_s = c_faint.Sprint("--- ")
	}

	url := attr.Url
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	}
	if i := strings.Index(url, "/"); i >= 0 {
		url = url[i:]
	}

	w.WriteString(code_s)
	renderDuration(w, dt)
	w.WriteByte(' ')
	if attr.HasBody {
		humanizeBytes(w, attr.BodySize)
	} else {
		w.WriteString(c_faint.Sprint("----"))
		w.WriteByte('B')
	}
	w.WriteByte(' ')
	w.WriteString(c_faint.Sprint("|"))
	w.WriteString(c_http_method.Sprint(method))
	w.WriteString(c_faint.Sprint("|"))
	w.WriteByte(' ')
	w.WriteString(c_msg.Sprint(url))
}

func (*LogExporter) writeGrpcIngressLine(w *bytes.Buffer, attr *kindGrpcAttr) {
	peer_addr := attr.PeerAddr
	if l := len(peer_addr); l < 18 {
		peer_addr += c_faint.Sprint(".................."[:18-l])
	} else {
		peer_addr = peer_addr[:18]
	}

	w.WriteString(peer_addr)
	w.WriteByte(' ')
	if attr.NotClientStream {
		humanizeBytes(w, attr.Bytes)
		w.WriteByte(' ')
	} else {
		w.WriteString(c_faint.Sprint("..... "))
	}
	w.WriteString(attr.Service)
	w.WriteByte('/')
	w.WriteString(c_http_method.Sprint(attr.Method))
}
func (e *LogExporter) writeGrpcEgressLine(w *bytes.Buffer, attr *kindGrpcAttr) {
	dt := time.Duration(attr.Elapsed)

	code_s := grpcCodeAbbr(int(attr.Code))
	switch attr.Code {
	case 0: // OK
		code_s = c_info.Sprint(code_s)
	case 13: // INTERNAL
		code_s = c_error.Sprint(code_s)
	default:
		code_s = c_warn.Sprint(code_s)
	}

	w.WriteString(code_s)
	w.WriteByte(' ')
	renderDuration(w, dt)
	w.WriteByte(' ')
	if attr.NotClientStream {
		w.WriteString("..... ")
		humanizeBytes(w, attr.Bytes)
	} else {
		humanizeBytes(w, attr.ReqBytes)
		w.WriteByte(' ')
		humanizeBytes(w, attr.ResBytes)
	}
	w.WriteByte(' ')
	w.WriteString(attr.Service)
	w.WriteByte('/')
	w.WriteString(c_http_method.Sprint(attr.Method))
}

func sum(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32())
}
