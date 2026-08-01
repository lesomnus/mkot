package mkot

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

// envRef matches an environment reference: ${env:NAME} or ${env:NAME:-default}.
// NAME is a POSIX-ish identifier. The optional ":-default" supplies a fallback
// when the variable is unset, as in the collector and in shell parameter
// expansion.
var envRef = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// ExpandEnv resolves ${env:NAME} / ${env:NAME:-default} references in the raw
// config against the process environment, mirroring the OpenTelemetry
// Collector's env substitution so a config lifted from the collector resolves
// the same way. A literal dollar sign is written "$$".
//
// An unset variable with no default is an error rather than an empty string, so
// a required endpoint or token never silently resolves to "".
func ExpandEnv(data []byte) ([]byte, error) {
	// Protect "$$" (an escaped literal "$") from expansion with a sentinel that
	// cannot appear in the input, then restore it afterwards.
	const sentinel = "\x00mkot-dollar\x00"
	s := strings.ReplaceAll(string(data), "$$", sentinel)

	var missing []string
	s = envRef.ReplaceAllStringFunc(s, func(m string) string {
		sub := envRef.FindStringSubmatch(m)
		name := sub[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		if sub[2] != "" { // ":-default" present (possibly empty)
			return sub[3]
		}
		missing = append(missing, name)
		return m
	})
	if len(missing) > 0 {
		return nil, fmt.Errorf("unresolved environment variable(s): %s", strings.Join(missing, ", "))
	}

	s = strings.ReplaceAll(s, sentinel, "$")
	return []byte(s), nil
}

// Load expands environment references (see [ExpandEnv]) and unmarshals the
// result into a Config. It is the entry point for configs that use the
// collector's ${env:...} idiom; plain [yaml.Unmarshal] into a Config still works
// but performs no substitution.
func Load(data []byte) (*Config, error) {
	expanded, err := ExpandEnv(data)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := yaml.Unmarshal(expanded, c); err != nil {
		return nil, err
	}
	return c, nil
}
