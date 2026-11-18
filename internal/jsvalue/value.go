// Package jsvalue models the JavaScript values that the js-beautify command line moves around:
// option values parsed from the command line, the environment and JSON configuration files, and
// the objects built from them.
//
// A value is one of: Undefined, nil (null), bool, float64, string, []any (an array) or *Object.
// Strings are Go (UTF-8) strings.
package jsvalue

import (
	"math"
	"strconv"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

type undefined struct{}

// Undefined is JavaScript's undefined value.
var Undefined any = undefined{}

// IsUndefined reports whether v is Undefined.
func IsUndefined(v any) bool {
	_, ok := v.(undefined)
	return ok
}

// Object is a plain JavaScript object. Its keys enumerate in JavaScript order: array-index keys
// ("0", "1", ...) in ascending numeric order, then the other keys in insertion order.
type Object struct {
	order []string
	vals  map[string]any
	proto *Object
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{vals: map[string]any{}}
}

// Get returns the value of own property key and whether it exists.
func (o *Object) Get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Has reports whether key is an own property of o.
func (o *Object) Has(key string) bool {
	_, ok := o.vals[key]
	return ok
}

// Set assigns o[key] = v, keeping the position of an existing key.
func (o *Object) Set(key string, v any) {
	if _, ok := o.vals[key]; !ok {
		o.order = append(o.order, key)
	}
	o.vals[key] = v
}

// Delete removes own property key (delete o[key]).
func (o *Object) Delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.order {
		if k == key {
			o.order = append(o.order[:i:i], o.order[i+1:]...)
			break
		}
	}
}

// SetPrototype makes p the object o inherits from; nil leaves o without inherited properties
// (Object.prototype contributes no enumerable ones).
func (o *Object) SetPrototype(p *Object) {
	o.proto = p
}

// Lookup returns o[key]: the own property, or the one inherited through the prototype chain.
func (o *Object) Lookup(key string) (any, bool) {
	for x := o; x != nil; x = x.proto {
		if v, ok := x.vals[key]; ok {
			return v, true
		}
	}
	return nil, false
}

// ForInKeys returns the keys a for-in loop over o visits: the own keys, then those of each
// prototype that were not met before.
func (o *Object) ForInKeys() []string {
	if o.proto == nil {
		return o.Keys()
	}
	seen := map[string]bool{}
	var out []string
	for x := o; x != nil; x = x.proto {
		for _, k := range x.Keys() {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// Len returns the number of own properties.
func (o *Object) Len() int {
	return len(o.order)
}

// Keys returns the own property keys in enumeration order (Object.keys).
func (o *Object) Keys() []string {
	var index []string
	var named []string
	for _, k := range o.order {
		if isArrayIndex(k) {
			index = append(index, k)
		} else {
			named = append(named, k)
		}
	}
	if len(index) == 0 {
		return named
	}
	sortIndexKeys(index)
	return append(index, named...)
}

// isArrayIndex reports whether k is the canonical form of an integer in [0, 2^32-2].
func isArrayIndex(k string) bool {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return false
	}
	n, err := strconv.ParseUint(k, 10, 64)
	return err == nil && n < 1<<32-1
}

func sortIndexKeys(keys []string) {
	value := func(k string) uint64 {
		n, _ := strconv.ParseUint(k, 10, 64)
		return n
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && value(keys[j]) < value(keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

// ToString returns String(v). It extends jsrt.ToString to undefined and objects, which may be
// nested in arrays.
func ToString(v any) string {
	switch x := v.(type) {
	case undefined:
		return "undefined"
	case *Object:
		return "[object Object]"
	case []any:
		s := ""
		for i, e := range x {
			if i > 0 {
				s += ","
			}
			if e != nil && !IsUndefined(e) {
				s += ToString(e)
			}
		}
		return s
	}
	return jsrt.ToString(v).UTF8()
}

// ToNumber returns Number(v) (the unary + operator).
func ToNumber(v any) float64 {
	switch x := v.(type) {
	case undefined:
		return math.NaN()
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case float64:
		return x
	case string:
		return jsrt.StringToNumber(jsrt.FromUTF8(x))
	case []any:
		return ToNumber(ToString(x))
	}
	return math.NaN()
}

// IsNaN returns the global isNaN(v).
func IsNaN(v any) bool {
	return math.IsNaN(ToNumber(v))
}

// Truthy returns !!v: jsrt.Truthy, with undefined false.
func Truthy(v any) bool {
	return !IsUndefined(v) && jsrt.Truthy(v)
}

// ToJSON converts v into the JSON-typed form the beautifiers take: *Object becomes
// map[string]any holding the properties a for-in loop sees (undefined ones left out), and
// undefined array elements become nil.
func ToJSON(v any) any {
	switch x := v.(type) {
	case undefined:
		return nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = ToJSON(e)
		}
		return out
	case *Object:
		out := make(map[string]any, x.Len())
		for _, k := range x.ForInKeys() {
			e, _ := x.Lookup(k)
			if !IsUndefined(e) {
				out[k] = ToJSON(e)
			}
		}
		return out
	}
	return v
}
