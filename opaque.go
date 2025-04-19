// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
// original source:

package mkot

import "fmt"

// OpaqueString alias that is marshaled and printed in an opaque way.
// To recover the original value, cast it to a string.
type OpaqueString string

const maskedString = "[REDACTED]"

// MarshalText marshals the string as `[REDACTED]`.
func (s OpaqueString) MarshalText() ([]byte, error) {
	return []byte(maskedString), nil
}

// String formats the string as `[REDACTED]`.
// This is used for the %s and %q verbs.
func (s OpaqueString) String() string {
	return maskedString
}

// GoString formats the string as `[REDACTED]`.
// This is used for the %#v verb.
func (s OpaqueString) GoString() string {
	return fmt.Sprintf("%#v", maskedString)
}

// MarshalBinary marshals the string `[REDACTED]` as []byte.
func (s OpaqueString) MarshalBinary() (text []byte, err error) {
	return []byte(maskedString), nil
}
