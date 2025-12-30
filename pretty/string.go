package pretty

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func writeHeader(w *bytes.Buffer, title string, t time.Time, sym string, trace_id trace.TraceID, span_id trace.SpanID) {
	c_title := c_faint
	if len(title) > 8 {
		title = title[:8]
	}
	if title != "" {
		sum := sum(title)
		c_title = pastel_colors[sum%len(pastel_colors)]
	}

	tid := trace_id.String()[:6]
	c_trace_id := c_faint
	if trace_id.IsValid() {
		sum := sum(tid)
		c_trace_id = dimmed_colors[sum%len(dimmed_colors)]
	}

	sid := span_id.String()[:6]
	c_span_id := c_faint
	if span_id.IsValid() {
		sum := sum(sid)
		c_span_id = dimmed_colors[sum%len(dimmed_colors)]
	}

	w.WriteString(c_title.Sprintf("|%s%s| ", strings.Repeat(".", 8-len(title)), title))
	w.WriteString(c_time.Sprint(t.Format("15:04:05.000")))
	w.WriteString(sym)
	w.WriteString(c_trace_id.Sprint(tid, " "))
	w.WriteString(c_span_id.Sprint(sid, " "))
}

func httpMethodAbbr(v string) string {
	switch v {
	case http.MethodGet:
		v = "GET"
	case http.MethodHead:
		v = "HED"
	case http.MethodPost:
		v = "PST"
	case http.MethodPut:
		v = "PUT"
	case http.MethodPatch:
		v = "PAT"
	case http.MethodDelete:
		v = "DEL"
	case http.MethodConnect:
		v = "CON"
	case http.MethodOptions:
		v = "OPT"
	case http.MethodTrace:
		v = "TRC"
	default:
		if len(v) > 3 {
			v = v[:3]
		}
	}

	return v
}

func grpcCodeAbbr(code int) string {
	switch code {
	case 0:
		return ".OK"
	case 1: // Canceled
		return ".CX"
	case 2: // Unknown
		return "UNK"
	case 3: // InvalidArgument
		return "ARG"
	case 4: // DeadlineExceeded
		return "EXP" // Expired
	case 5: // NotFound
		return "404" // HTTP Not Found
	case 6: // AlreadyExists
		return "DUP" // Duplicated
	case 7: // PermissionDenied
		return "PDN"
	case 8: // ResourceExhausted
		return "OOM" // Out Of Memory
	case 9: // FailedPrecondition
		return "RJT" // Rejected
	case 10: // Aborted
		return "ABT"
	case 11: // OutOfRange
		return "OOR"
	case 12: // Unimplemented
		return "IMP"
	case 13: // Internal
		return "INT"
	case 14: // Unavailable
		return "OFF" // Offline
	case 15: // DataLoss
		return "DLS"
	case 16: // Unauthenticated
		return "ATH"
	default:
		return "???"
	}
}

// 000.00µs
// 000.00ms
// 00m00.0s
// 00d00:00
// 0000d00h
func renderDuration(b *bytes.Buffer, d time.Duration) {
	// 000.00µs
	if d < time.Millisecond {
		us := int((d % time.Millisecond) / time.Microsecond)
		if us < 100 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if us < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if us == 0 {
			b.WriteString(c_faint.Sprint("0"))
		} else {
			b.WriteString(c_msg.Sprintf("%d", us))
		}
		b.WriteString(".")

		ns := int((d % time.Microsecond) / time.Nanosecond)
		if ns < 100 {
			if us == 0 || ns == 0 {
				b.WriteString(c_faint.Sprint("0"))
			} else {
				b.WriteString("0")
			}
		}
		if ns < 10 {
			b.WriteString(c_faint.Sprint("0"))
		} else {
			b.WriteString(c_msg.Sprintf("%d", ns/10))
		}
		b.WriteString("µs")
		return
	}

	// 000.00ms
	if d < time.Second {
		ms := (d % time.Second) / time.Millisecond
		if ms < 100 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if ms < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if ms == 0 {
			b.WriteString(c_faint.Sprint("0"))
		} else {
			b.WriteString(c_msg.Sprintf("%d", ms))
		}
		b.WriteString(".")

		us := (d % time.Millisecond) / time.Microsecond
		if us < 100 {
			b.WriteString("0")
		}
		if us < 10 {
			b.WriteString(c_faint.Sprint("0"))
		} else {
			b.WriteString(c_msg.Sprintf("%d", us/10))
		}
		b.WriteString("ms")
		return
	}

	// 00m00.0s
	if d < time.Hour {
		min := (d % time.Hour) / time.Minute
		if min < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if min > 0 {
			b.WriteString(c_msg.Sprintf("%d", min))
		} else {
			b.WriteString("0")
		}
		b.WriteString("m")

		sec := (d % time.Minute) / time.Second
		if sec < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if sec == 0 {
			b.WriteString(c_faint.Sprint("0"))
		} else {
			b.WriteString(c_msg.Sprintf("%d", sec))
		}
		b.WriteString(".")

		ms := (d % time.Second) / time.Millisecond
		if ms >= 100 {
			b.WriteString(c_msg.Sprintf("%d", ms/100))
		} else {
			b.WriteString(c_faint.Sprint("0"))
		}
		b.WriteString("s")
		return
	}

	// 00d00:00
	if d < time.Hour*24*100 {
		day := int(d / (time.Hour * 24))
		if day < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		b.WriteString(c_msg.Sprintf("%d", day))
		b.WriteString("d")

		h := (d % (time.Hour * 24)) / time.Hour
		if h < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if h > 0 {
			b.WriteString(c_msg.Sprintf("%d", h))
		} else {
			b.WriteString("0")
		}
		b.WriteString(":")

		min := (d % time.Hour) / time.Minute
		if min < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if min > 0 {
			b.WriteString(c_msg.Sprintf("%d", min))
		} else {
			b.WriteString("0")
		}
		return
	}

	// 0000d00h
	if d < time.Hour*24*10_000 {
		day := d / (time.Hour * 24)
		if day < 1000 {
			b.WriteString(c_faint.Sprint("0"))
		}
		b.WriteString(c_msg.Sprintf("%d", day))
		b.WriteString("d")
		d = d % (time.Hour * 24)

		h := (d % (time.Hour * 24)) / time.Hour
		if h < 10 {
			b.WriteString(c_faint.Sprint("0"))
		}
		if h > 0 {
			b.WriteString(c_msg.Sprintf("%d", h))
		} else {
			b.WriteString("0")
		}
		b.WriteString("h")
		return
	}

	b.WriteString("TOO_LONG")
}

// 0000K
func humanizeBytes(b *bytes.Buffer, n int64) {
	const K = 1024

	i := 0
	for n > 9999 {
		n /= K
		i++
	}

	if n < 1000 {
		b.WriteString(c_faint.Sprint("0"))
	}
	if n < 100 {
		b.WriteString(c_faint.Sprint("0"))
	}
	if n < 10 {
		b.WriteString(c_faint.Sprint("0"))
	}
	if n == 0 {
		b.WriteString(c_faint.Sprint("0"))
	} else {
		b.WriteString(c_msg.Sprintf("%d", n))
	}
	b.WriteByte("BKMGTPE"[i])
}
