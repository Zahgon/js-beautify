// Package nopt ports nopt 10.0.1, the option parser of the js-beautify command line: parsing an
// argument list against a table of option types and shorthands (Parse), and validating and
// coercing option values (Clean), with the same results and the same errors as the original.
//
// Parsed values are jsvalue values. An option whose name, after abbreviation, is a property of
// Object.prototype (such as "constructor") behaves as nopt makes it behave on a plain object.
package nopt

import (
	"regexp"
	"sort"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
	"github.com/beautifier/js-beautify-go/internal/jsvalue"
	"github.com/beautifier/js-beautify-go/internal/nodefmt"
	"github.com/beautifier/js-beautify-go/internal/nodepath"
)

type typeKind int

const (
	kindFunc    typeKind = iota // a constructor matched by name: String, Number, Boolean, Array
	kindPath                    // the path module
	kindLiteral                 // a string compared with the value, and with custom type defs
)

// Type is one entry of an option's type: a constructor, the path module or a string.
type Type struct {
	kind typeKind
	name string
}

// The types the js-beautify command line uses.
var (
	String  = Type{kindFunc, "String"}
	Number  = Type{kindFunc, "Number"}
	Boolean = Type{kindFunc, "Boolean"}
	Array   = Type{kindFunc, "Array"}
	Path    = Type{kind: kindPath}
)

// Literal returns the string type s: a value equal to s is valid, and a TypeDef registered for
// Literal(s) validates other values.
func Literal(s string) Type {
	return Type{kindLiteral, s}
}

// TypeSpec is an option's entry in the types table: a single type, or a list of types any of
// which the value may satisfy.
type TypeSpec struct {
	types []Type
	list  bool
}

// One returns the type spec holding the single type t.
func One(t Type) TypeSpec {
	return TypeSpec{types: []Type{t}}
}

// List returns the type spec for the list of types ts (a JavaScript array in the table).
func List(ts ...Type) TypeSpec {
	return TypeSpec{types: ts, list: true}
}

func (s TypeSpec) has(t Type) bool {
	for _, x := range s.types {
		if x == t {
			return true
		}
	}
	return false
}

func (s TypeSpec) hasLiteral(v string) bool {
	return s.has(Literal(v))
}

// TypeDef validates values of one type, as an entry of nopt.typeDefs does. Validate returns the
// coerced value and whether it is valid; an error is an exception thrown by the validator.
type TypeDef struct {
	Type     Type
	Validate func(v any) (any, bool, error)
}

// Parser holds the tables nopt is called with and the environment its validators depend on.
type Parser struct {
	// Types maps option names to their types.
	Types map[string]TypeSpec
	// Shorthands maps a shorthand to the arguments it expands to.
	Shorthands map[string][]string
	// TypeDefs are the validators, in the order of nopt.typeDefs; String, Boolean, Number and
	// path are provided by StandardTypeDefs.
	TypeDefs []TypeDef
}

// Invalid is the exception the js-beautify command line's invalidHandler throws for a value that
// does not validate.
func Invalid(key string, v any) *nodefmt.Error {
	return nodefmt.NewError("Error", key+` was invalid with value "`+jsvalue.ToString(v)+`"`)
}

// StandardTypeDefs returns nopt's validators for String, Boolean, Number and path, in nopt's
// order; cwd and home are what path.resolve and os.homedir() give.
func StandardTypeDefs(cwd, home string) []TypeDef {
	return []TypeDef{
		{String, func(v any) (any, bool, error) { return jsvalue.ToString(v), true, nil }},
		{Boolean, validateBoolean},
		{Number, func(v any) (any, bool, error) {
			if jsvalue.IsNaN(v) {
				return nil, false, nil
			}
			return jsvalue.ToNumber(v), true, nil
		}},
		{Path, func(v any) (any, bool, error) { return ValidatePath(v, cwd, home) }},
	}
}

func validateBoolean(v any) (any, bool, error) {
	if s, ok := v.(string); ok {
		switch {
		case !jsvalue.IsNaN(s):
			return jsvalue.Truthy(jsvalue.ToNumber(s)), true, nil
		case s == "null" || s == "false":
			return false, true, nil
		}
		return true, true, nil
	}
	return jsvalue.Truthy(v), true, nil
}

// ValidatePath is nopt's path validator: true is invalid, null is accepted as undefined, "~/"
// is expanded to home, and anything else is resolved against cwd.
func ValidatePath(v any, cwd, home string) (any, bool, error) {
	switch v {
	case true:
		return nil, false, nil
	case nil:
		return jsvalue.Undefined, true, nil
	}
	s := jsvalue.ToString(v)
	if home != "" && strings.HasPrefix(s, "~/") {
		return nodepath.Resolve(cwd, home, s[2:]), true, nil
	}
	return nodepath.Resolve(cwd, s), true, nil
}

// objectPrototypeName returns, for a name that a plain object inherits from Object.prototype,
// String() of the inherited value, which is what nopt ends up using as the option's name.
func objectPrototypeName(name string) (string, bool) {
	if !jsrt.ObjectPrototypeHas(jsrt.String(name)) {
		return "", false
	}
	switch name {
	case "__proto__":
		return "[object Object]", true
	case "constructor":
		return "function Object() { [native code] }", true
	}
	return "function " + name + "() { [native code] }", true
}

var (
	allDashes      = regexp.MustCompile(`^-{2,}$`)
	leadingDashes  = regexp.MustCompile(`^-+`)
	doubleDashWord = regexp.MustCompile(`^-{2,}[^-]`)
	singleDashWord = regexp.MustCompile(`^-[^-]`)
	dashedWord     = regexp.MustCompile(`^-{1,2}[^-]+`)
)

// Parse returns nopt(types, shorthands, args, 0): the parsed and cleaned options, with an "argv"
// property holding the remaining, cooked and original arguments.
func (p *Parser) Parse(args []string) (*jsvalue.Object, error) {
	original := append([]string(nil), args...)
	cooked := append([]string(nil), args...)
	data := jsvalue.NewObject()
	var remain []string
	cooked, remain, err := p.parse(cooked, data)
	if err != nil {
		return nil, err
	}
	if err := p.Clean(data); err != nil {
		return nil, err
	}
	argv := jsvalue.NewObject()
	argv.Set("remain", stringsToValues(remain))
	argv.Set("cooked", stringsToValues(cooked))
	argv.Set("original", stringsToValues(original))
	data.Set("argv", argv)
	return data, nil
}

func stringsToValues(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func splice(args []string, i, n int, items ...string) []string {
	out := make([]string, 0, len(args)-n+len(items))
	out = append(out, args[:i]...)
	out = append(out, items...)
	return append(out, args[i+n:]...)
}

func (p *Parser) parse(args []string, data *jsvalue.Object) ([]string, []string, error) {
	abbrevs := abbrev(keys(p.Types))
	shortAbbr := abbrev(keys(p.Shorthands))
	remain := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if allDashes.MatchString(arg) {
			// Done with options: the rest are arguments.
			remain = append(remain, args[i+1:]...)
			args[i] = "--"
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			remain = append(remain, arg)
			continue
		}
		hadEq := false
		if at := strings.IndexByte(arg, '='); at > -1 {
			hadEq = true
			v := arg[at+1:]
			arg = arg[:at]
			args = splice(args, i, 1, arg, v)
		}
		shRes, isShort, err := p.resolveShort(arg, abbrevs, shortAbbr)
		if err != nil {
			return nil, nil, err
		}
		if isShort {
			args = splice(args, i, 1, shRes...)
			if len(shRes) == 0 || arg != shRes[0] {
				i--
				continue
			}
		}
		arg = leadingDashes.ReplaceAllString(arg, "")
		no, hasNo := false, false
		for strings.HasPrefix(string(jsrt.FromUTF8(arg).ToLowerCase()), "no-") {
			no = !no
			hasNo = true
			arg = arg[3:]
		}
		if full, ok := abbrevs[arg]; ok && full != arg {
			arg = full
		} else if name, inherited := objectPrototypeName(arg); inherited {
			arg = name
		}

		spec, hasType := p.Types[arg]
		argType := spec
		isTypeArray := hasType && spec.list
		if isTypeArray && len(spec.types) == 1 {
			isTypeArray = false
			argType = One(spec.types[0])
		}
		single := hasType && !isTypeArray
		isArray := (single && argType.types[0] == Array) || (isTypeArray && argType.has(Array))
		existing, seen := data.Get(arg)
		if !hasType && seen {
			if _, ok := existing.([]any); !ok {
				data.Set(arg, []any{existing})
			}
			isArray = true
		}

		la, haveLa := "", i+1 < len(args)
		if haveLa {
			la = args[i+1]
		}
		isBool := hasNo ||
			(single && argType.types[0] == Boolean) ||
			(isTypeArray && argType.has(Boolean)) ||
			(!hasType && !hadEq)

		set := func(val any) {
			if !isArray {
				data.Set(arg, val)
				return
			}
			list, _ := data.Get(arg)
			arr, _ := list.([]any)
			data.Set(arg, append(arr, val))
		}

		if isBool {
			var val any = !no
			if haveLa && (la == "true" || la == "false") {
				val = la == "true"
				haveLa = false
				if no {
					val = la != "true"
				}
				i++
			}
			if isTypeArray && haveLa && la != "" {
				switch {
				case argType.hasLiteral(la):
					val = la
					i++
				case !doubleDashWord.MatchString(la) && !jsvalue.IsNaN(la) && argType.has(Number):
					val = jsvalue.ToNumber(la)
					i++
				case !singleDashWord.MatchString(la) && argType.has(String):
					val = la
					i++
				}
			}
			set(val)
			continue
		}

		if single && argType.types[0] == String {
			if !haveLa {
				la, haveLa = "", true
			} else if dashedWord.MatchString(la) {
				la = ""
				i--
			}
		}
		if haveLa && la != "" && allDashes.MatchString(la) {
			haveLa = false
			i--
		}
		if haveLa {
			set(la)
		} else {
			set(true)
		}
		i++
	}
	return args, remain, nil
}

// resolveShort returns the expansion of a shorthand argument, and false when arg is not one.
func (p *Parser) resolveShort(arg string, abbrevs, shortAbbr map[string]string) ([]string, bool, error) {
	arg = leadingDashes.ReplaceAllString(arg, "")
	if abbrevs[arg] == arg && arg != "" {
		return nil, false, nil
	}
	if _, inherited := objectPrototypeName(arg); inherited {
		// shorthands[arg] is the inherited function, and nopt calls .split on it.
		return nil, false, nodefmt.NewError("TypeError", "shorthands[arg].split is not a function")
	}
	if exp, ok := p.Shorthands[arg]; ok {
		return exp, true, nil
	}
	if chrs, ok := p.singleCharacters(arg); ok {
		var out []string
		for _, c := range chrs {
			out = append(out, p.Shorthands[c]...)
		}
		return out, true, nil
	}
	if _, ok := abbrevs[arg]; ok {
		return nil, false, nil
	}
	if full, ok := shortAbbr[arg]; ok {
		arg = full
	}
	exp, ok := p.Shorthands[arg]
	return exp, ok, nil
}

// singleCharacters splits arg into one-letter shorthands when every character of it is one.
func (p *Parser) singleCharacters(arg string) ([]string, bool) {
	units := jsrt.FromUTF8(arg).Units()
	out := make([]string, 0, len(units))
	for _, u := range units {
		c := string(jsrt.FromUnit(u).UTF8())
		if _, ok := p.Shorthands[c]; !ok {
			return nil, false
		}
		out = append(out, c)
	}
	return out, true
}

// Clean validates and coerces the values of data in place, as nopt.clean does: strings are
// trimmed and converted to booleans, null or numbers where the option's type allows it, values
// are checked against the types, and the first invalid value makes it return Invalid.
func (p *Parser) Clean(data *jsvalue.Object) error {
	for _, k := range data.Keys() {
		if k == "argv" {
			continue
		}
		raw, _ := data.Get(k)
		vals, isArray := raw.([]any)
		if !isArray {
			vals = []any{raw}
		}
		spec, hasType := p.Types[k]
		cleaned := make([]any, 0, len(vals))
		for _, v := range vals {
			if s, ok := v.(string); ok {
				s = string(jsrt.FromUTF8(s).Trim().UTF8())
				v = s
				switch {
				case !hasType:
					// nopt's default type list is [false, true, null, String, Array].
					switch s {
					case "null":
						v = nil
					case "true":
						v = true
					case "false":
						v = false
					}
				case (s == "true" || s == "false") && spec.has(Boolean):
					v = s == "true"
				case spec.has(Number) && !jsvalue.IsNaN(s):
					v = jsvalue.ToNumber(s)
				}
			}
			if !hasType {
				cleaned = append(cleaned, v)
				continue
			}
			out, ok, err := p.validate(v, spec)
			if err != nil {
				return err
			}
			if !ok {
				return Invalid(k, v)
			}
			cleaned = append(cleaned, out)
		}
		switch {
		case len(cleaned) == 0 && hasType && !spec.has(Array):
			data.Delete(k)
		case isArray:
			data.Set(k, cleaned)
		default:
			data.Set(k, cleaned[0])
		}
	}
	return nil
}

// validate is nopt's validate: a list is satisfied by any of its types (Array entries are
// skipped), Array accepts anything, a string type accepts the equal string, and other types are
// checked by the TypeDef registered for them.
func (p *Parser) validate(v any, spec TypeSpec) (any, bool, error) {
	if !spec.list {
		return p.validateOne(v, spec.types[0])
	}
	for _, t := range spec.types {
		if t == Array {
			continue
		}
		out, ok, err := p.validateOne(v, t)
		if err != nil || ok {
			return out, ok, err
		}
	}
	return nil, false, nil
}

func (p *Parser) validateOne(v any, t Type) (any, bool, error) {
	if t == Array {
		return v, true, nil
	}
	if s, ok := v.(string); ok && t.kind == kindLiteral && s == t.name {
		return v, true, nil
	}
	for _, td := range p.TypeDefs {
		if td.Type != t {
			continue
		}
		out, ok, err := td.Validate(v)
		if err != nil || ok {
			return out, ok, err
		}
	}
	return nil, false, nil
}
