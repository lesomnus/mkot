// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mkot

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// typeAndNameSeparator is the separator that is used between type and name in type/name composite keys.
const typeAndNameSeparator = "/"

var (
	// typeRegexp is used to validate the type of component.
	// A type must start with an ASCII alphabetic character and
	// can only contain ASCII alphanumeric characters and '_'.
	// This must be kept in sync with the regex in cmd/mdatagen/validate.go.
	typeRegexp = regexp.MustCompile(`^[a-zA-Z][0-9a-zA-Z_]{0,62}$`)

	// nameRegexp is used to validate the name of a component. A name can consist of
	// 1 to 1024 Unicode characters excluding whitespace, control characters, and
	// symbols.
	nameRegexp = regexp.MustCompile(`^[^\pZ\pC\pS]+$`)
)

type Id string

func (id Id) String() string {
	return string(id)
}

func (id Id) split() (string, string) {
	typeStr, nameStr, _ := strings.Cut(string(id), typeAndNameSeparator)
	return typeStr, nameStr
}

func (id Id) Type() string {
	v, _ := id.split()
	return v
}

func (id Id) Name() string {
	_, v := id.split()
	return v
}

func (id Id) WithName(name string) Id {
	v, _ := id.split()
	if name == "" {
		return Id(v)
	}

	return Id(v + "/" + name)
}

func (id Id) MarshalText() (text []byte, err error) {
	return []byte(id), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (id *Id) UnmarshalText(text []byte) error {
	idStr := string(text)
	typeStr, nameStr, hasName := strings.Cut(idStr, typeAndNameSeparator)
	typeStr = strings.TrimSpace(typeStr)

	if typeStr == "" {
		if hasName {
			return fmt.Errorf("in %q id: the part before %s should not be empty", idStr, typeAndNameSeparator)
		}
		return errors.New("id must not be empty")
	}
	if !typeRegexp.MatchString(typeStr) {
		return fmt.Errorf("invalid character(s) in type %q", typeStr)
	}

	if hasName {
		// "name" part is present.
		nameStr = strings.TrimSpace(nameStr)
		if nameStr == "" {
			return fmt.Errorf("in %q id: the part after %s should not be empty", idStr, typeAndNameSeparator)
		}
		if !nameRegexp.MatchString(nameStr) {
			return fmt.Errorf("invalid character(s) in name %q", nameStr)
		}
	}

	v := typeStr
	if hasName {
		v += "/" + nameStr
	}

	*id = Id(v)
	return nil
}
