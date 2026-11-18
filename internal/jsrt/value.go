package jsrt

import (
	"errors"
	"math"
	"strings"
)

// Values handled here are what encoding/json decodes into (nil, bool, float64, string, []any,
// map[string]any), as JavaScript sees the same JSON. Go integer and float32 values are accepted
// as numbers too.

// AsNumber returns v as a JavaScript number when v holds a Go number.
func AsNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}

// Truthy returns !!v.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case String:
		return x != ""
	}
	if n, ok := AsNumber(v); ok {
		return n != 0 && !math.IsNaN(n)
	}
	return true
}

// ToString returns String(v): "null" for nil, "true"/"false", NumberToString for numbers, the
// elements joined with "," for arrays (nil elements as ""), "[object Object]" for objects.
func ToString(v any) String {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return FromUTF8(x)
	case String:
		return x
	case []any:
		parts := make([]String, len(x))
		for i, e := range x {
			if e != nil {
				parts[i] = ToString(e)
			}
		}
		return Join(parts, ",")
	}
	if n, ok := AsNumber(v); ok {
		return NumberToString(n)
	}
	return "[object Object]"
}

// objectPrototypeProperties are the properties every plain JavaScript object inherits.
var objectPrototypeProperties = map[String]bool{
	"constructor": true, "__defineGetter__": true, "__defineSetter__": true, "hasOwnProperty": true,
	"__lookupGetter__": true, "__lookupSetter__": true, "isPrototypeOf": true,
	"propertyIsEnumerable": true, "toString": true, "valueOf": true, "__proto__": true,
	"toLocaleString": true,
}

// ObjectPrototypeHas reports whether key names a property inherited from Object.prototype, which
// the `in` operator finds on every object literal: ("toString" in {}) is true.
func ObjectPrototypeHas(key String) bool {
	return objectPrototypeProperties[key]
}

// MaxStringLength is V8's maximum string length on 64-bit platforms.
const MaxStringLength = 1<<29 - 24

// MaxArrayLength is the largest valid JavaScript array length.
const MaxArrayLength = 1<<32 - 1

// ErrInvalidArrayLength is V8's RangeError for new Array(n) with an invalid n.
var ErrInvalidArrayLength = errors.New("Invalid array length")

// ErrInvalidStringLength is V8's RangeError for a string longer than MaxStringLength.
var ErrInvalidStringLength = errors.New("Invalid string length")

// ArrayJoinRepeat returns new Array(n).join(s): s repeated n-1 times. It fails as V8 does when n
// is not a valid array length or the result would exceed MaxStringLength.
func ArrayJoinRepeat(n int, s String) (String, error) {
	if n < 0 || n > MaxArrayLength {
		return "", ErrInvalidArrayLength
	}
	if n <= 1 || s == "" {
		return "", nil
	}
	if (n - 1) > MaxStringLength/s.Len() {
		return "", ErrInvalidStringLength
	}
	return String(strings.Repeat(string(s), n-1)), nil
}
