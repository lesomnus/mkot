// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opaque // import "go.opentelemetry.io/collector/config/configopaque"

import (
	"iter"
	"slices"
)

// Pair is an element of a MapList, and consists of a name and an opaque value.
type Pair struct {
	Name  string `mapstructure:"name"`
	Value String `mapstructure:"value"`
}

// MapList is a replacement for map[string]opaque.String with a similar API,
// which can also be unmarshalled from (and is stored as) a list of name/value pairs.
//
// Pairs are assumed to have distinct names. This is checked during config validation.
type MapList []Pair

var _ iter.Seq2[string, String] = MapList(nil).Iter

// Iter is an iterator over key/value pairs for use in for-range loops.
// It is the MapList equivalent of directly ranging over a map.
func (ml MapList) Iter(yield func(name string, value String) bool) {
	for _, OpaquePair := range ml {
		if !yield(OpaquePair.Name, OpaquePair.Value) {
			break
		}
	}
}

// Get looks up a pair's value based on its name.
// It is the MapList equivalent of `val, ok := m[key]`.
// However, it has linear time complexity.
func (ml MapList) Get(name string) (val String, ok bool) {
	for _, OpaquePair := range ml {
		if OpaquePair.Name == name {
			return OpaquePair.Value, true
		}
	}
	return val, false
}

// Set sets the value corresponding to a given name.
// It is the MapList equivalent of `m[key] = val`.
// However, it has linear time complexity,
// and does not affect shallow copies.
func (ml *MapList) Set(name string, val String) {
	if ml == nil {
		panic("assignment to entry in nil *MapList")
	}
	for i, OpaquePair := range *ml {
		if OpaquePair.Name == name {
			*ml = slices.Clone(*ml)
			(*ml)[i].Value = val
			return
		}
	}
	*ml = append(make(MapList, 0, len(*ml)+1), *ml...)
	*ml = append(*ml, Pair{Name: name, Value: val})
}
