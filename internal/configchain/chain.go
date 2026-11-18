// Package configchain ports the parts of config-chain 1.1.13 (and of proto-list 1.2.4 under it)
// that the js-beautify command line uses: cc(...sources).snapshot, which links configuration
// objects and JSON files into one prototype chain and flattens it, and cc.env(prefix).
package configchain

import (
	"os"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsvalue"
	"github.com/beautifier/js-beautify-go/internal/nodefmt"
	"github.com/beautifier/js-beautify-go/internal/nodepath"
)

// link is one object of the chain: an *Object, an array, or the {valueOf: v} object proto-list
// makes of a value that is not an object.
type link struct {
	obj      *jsvalue.Object
	arr      []any
	valueOf  any
	isObject bool
	isArray  bool
}

func (l link) ownKeys() []string {
	switch {
	case l.isObject:
		return l.obj.Keys()
	case l.isArray:
		keys := make([]string, len(l.arr))
		for i := range l.arr {
			keys[i] = jsvalue.ToString(float64(i))
		}
		return keys
	}
	return []string{"valueOf"}
}

func (l link) get(key string) (any, bool) {
	switch {
	case l.isObject:
		return l.obj.Get(key)
	case l.isArray:
		for i, v := range l.arr {
			if jsvalue.ToString(float64(i)) == key {
				return v, true
			}
		}
		return nil, false
	}
	if key == "valueOf" {
		return l.valueOf, true
	}
	return nil, false
}

// Chain is a ConfigChain: its sources, first one first, each inheriting from the next.
type Chain struct {
	links []link
}

// Source is an argument of cc(): a *jsvalue.Object, or a path (string) to a JSON file. Arguments
// that are falsy (nil, "") are skipped.
type Source = any

// New returns cc(sources...). A path source is read as UTF-8 and parsed as JSON; a file that
// cannot be read contributes undefined, and a file holding null makes the chain fail with the
// TypeError proto-list's Object.setPrototypeOf throws. Parse errors are the *jsvalue.SyntaxError
// of JSON.parse.
func New(cwd string, sources ...Source) (*Chain, error) {
	c := &Chain{}
	for _, s := range sources {
		if !jsvalue.Truthy(s) {
			continue
		}
		v := s
		if path, ok := s.(string); ok {
			loaded, err := loadJSON(nodepath.Resolve(cwd, nodepath.Join(path)))
			if err != nil {
				return nil, err
			}
			v = loaded
		}
		if err := c.push(v); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// loadJSON is config-chain's json(): the parsed file, or undefined when it cannot be read.
func loadJSON(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return jsvalue.Undefined, nil
	}
	return jsvalue.ParseJSON(nodefmt.DecodeUTF8(data))
}

// push is ProtoList.prototype.push.
func (c *Chain) push(v any) error {
	switch x := v.(type) {
	case nil:
		return nodefmt.NewError("TypeError", "Object.setPrototypeOf called on null or undefined")
	case *jsvalue.Object:
		c.links = append(c.links, link{obj: x, isObject: true})
	case []any:
		c.links = append(c.links, link{arr: x, isArray: true})
	default:
		c.links = append(c.links, link{valueOf: x})
	}
	return nil
}

// Snapshot returns the chain's snapshot: an object with every key a for-in loop over the first
// source finds, valued as the chain resolves it. A "__proto__" key is assigned the way
// JavaScript assigns it, making its object value the snapshot's prototype.
func (c *Chain) Snapshot() *jsvalue.Object {
	out := jsvalue.NewObject()
	seen := map[string]bool{}
	for _, l := range c.links {
		for _, k := range l.ownKeys() {
			if seen[k] {
				continue
			}
			seen[k] = true
			v := c.get(k)
			if k == "__proto__" {
				if proto, ok := v.(*jsvalue.Object); ok {
					out.SetPrototype(proto)
				}
				continue
			}
			out.Set(k, v)
		}
	}
	return out
}

// get resolves key along the chain.
func (c *Chain) get(key string) any {
	for _, l := range c.links {
		if v, ok := l.get(key); ok {
			return v
		}
	}
	return jsvalue.Undefined
}

// Env returns cc.env(prefix): the environment variables whose name starts with prefix, keyed by
// the rest of the name, in the order of environ ("NAME=value" entries).
func Env(prefix string, environ []string) *jsvalue.Object {
	obj := jsvalue.NewObject()
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		key := k[len(prefix):]
		if key == "__proto__" {
			// Assigning a string to __proto__ changes nothing.
			continue
		}
		obj.Set(key, v)
	}
	return obj
}

// Copy returns cc(obj).snapshot for a single object: pushing it into a new chain removes its
// prototype, then its own properties are copied.
func Copy(obj *jsvalue.Object) *jsvalue.Object {
	obj.SetPrototype(nil)
	out := jsvalue.NewObject()
	for _, k := range obj.Keys() {
		v, _ := obj.Get(k)
		out.Set(k, v)
	}
	return out
}
