// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package otel

import (
	"fmt"

	"github.com/russoj88/gio-ide/internal/gopls/xtools/event/keys"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event/label"
)

// labelToAttribute converts a label to an OTLP attribute.
// It returns false if the label has no value.
func labelToAttribute(l label.Label) (otlpAttribute, bool) {
	key := l.Key()
	var v otlpAttributeValue

	switch key := key.(type) {
	case *keys.Int:
		v = otlpAttributeValue{IntValue: new(intAsString(key.From(l)))}
	case *keys.Uint:
		v = otlpAttributeValue{IntValue: new(intAsString(key.From(l)))}
	case *keys.Float:
		v = otlpAttributeValue{DoubleValue: new(key.From(l))}
	case *keys.String:
		v = otlpAttributeValue{StringValue: new(key.From(l))}
	case *keys.Error:
		if err := key.From(l); err != nil {
			v = otlpAttributeValue{StringValue: new(err.Error())}
		}
	case *keys.Value:
		v = otlpAttributeValue{StringValue: new(fmt.Sprint(key.From(l)))}
	default:
		if l.Valid() {
			v = otlpAttributeValue{StringValue: new(fmt.Sprint(l))}
		}
	}

	hasValue := v.StringValue != nil || v.IntValue != nil || v.DoubleValue != nil
	return otlpAttribute{Key: key.Name(), Value: v}, hasValue
}

// TODO(adonovan): use go1.26 new(expr)
//
//go:fix inline
func varOf[T any](x T) *T { return new(x) }
