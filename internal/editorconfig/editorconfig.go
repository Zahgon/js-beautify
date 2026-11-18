// Package editorconfig ports editorconfig 3.0.2's parseSync: it finds the .editorconfig files
// from a file's directory up to the root (stopping at one with root = true), and combines the
// properties of the sections whose glob matches the file.
package editorconfig

import (
	"os"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
	"github.com/beautifier/js-beautify-go/internal/jsvalue"
	"github.com/beautifier/js-beautify-go/internal/minimatch"
	"github.com/beautifier/js-beautify-go/internal/nodepath"
)

// knownProps are the properties whose values editorconfig lowercases.
var knownProps = map[string]bool{
	"charset": true, "end_of_line": true, "indent_size": true, "indent_style": true,
	"insert_final_newline": true, "trim_trailing_whitespace": true,
}

var matchOptions = minimatch.Options{MatchBase: true, Dot: true}

// configSection is a parsed section: its normalized properties and the glob it applies to (nil
// for the preamble).
type configSection struct {
	props *jsvalue.Object
	glob  *minimatch.Minimatch
}

type configFile struct {
	root     bool
	sections []configSection
}

// ParseSync returns the EditorConfig properties of the file at filepath (an absolute path).
// Keys are lowercased; values are parsed as JSON when they can be (2 becomes a number, true a
// boolean) and are strings otherwise. An error means a configuration file could not be parsed,
// in which case editorconfig throws.
func ParseSync(filepath string) (*jsvalue.Object, error) {
	var configs []*configFile
	dir := filepath
	for {
		dir = nodepath.Dirname(dir)
		cfg, err := loadConfig(nodepath.Join(dir, ".editorconfig"))
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			configs = append(configs, cfg)
			if cfg.root {
				break
			}
		}
		if dir == "/" {
			break
		}
	}
	props := jsvalue.NewObject()
	for i := len(configs) - 1; i >= 0; i-- {
		for _, s := range configs[i].sections {
			if s.glob == nil || !s.glob.Match(filepath) {
				continue
			}
			for _, k := range s.props.Keys() {
				v, _ := s.props.Get(k)
				props.Set(k, v)
			}
		}
	}
	processMatches(props)
	return props, nil
}

// loadConfig reads and parses one .editorconfig file; it returns nil when the file cannot be
// read.
func loadConfig(path string) (*configFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	sections, err := parseINI(data)
	if err != nil {
		return nil, err
	}
	prefix := minimatch.Escape(nodepath.Dirname(path))
	if strings.HasPrefix(prefix, "#") {
		prefix = "[#]" + prefix[1:]
	}
	cfg := &configFile{}
	for i, s := range sections {
		cs := configSection{props: normalizeProps(s.pairs)}
		if i > 0 {
			if cs.glob, err = buildFullGlob(prefix, s.name); err != nil {
				return nil, err
			}
		}
		cfg.sections = append(cfg.sections, cs)
	}
	root, _ := cfg.sections[0].props.Get("root")
	cfg.root = jsvalue.Truthy(root)
	return cfg, nil
}

// buildFullGlob turns a section name into a pattern rooted at the directory of its file: a name
// without "/" matches at any depth, a leading "/" anchors it, and "**" also matches nothing.
func buildFullGlob(pathPrefix, glob string) (*minimatch.Minimatch, error) {
	switch strings.IndexByte(glob, '/') {
	case -1:
		glob = "**/" + glob
	case 0:
		glob = glob[1:]
	}
	glob = strings.ReplaceAll(glob, `\\`, `\\\\`)
	glob = strings.ReplaceAll(glob, "**", "{*,**/**/**}")
	return minimatch.New(pathPrefix+"/"+glob, matchOptions)
}

// normalizeProps lowercases the keys (and the values of the known properties) of a section and
// parses each value as JSON when it is valid JSON other than null.
func normalizeProps(pairs [][2]string) *jsvalue.Object {
	// The section's pairs as editorconfig stores them in an object: a repeated key keeps its
	// first position and its last value, and "__proto__" is not an own property.
	options := jsvalue.NewObject()
	for _, kv := range pairs {
		if kv[0] != "__proto__" {
			options.Set(kv[0], kv[1])
		}
	}
	props := jsvalue.NewObject()
	for _, key := range options.Keys() {
		raw, _ := options.Get(key)
		value := raw.(string)
		key2 := string(jsrt.FromUTF8(key).ToLowerCase().UTF8())
		var value2 any = value
		if knownProps[key2] {
			value2 = string(jsrt.FromUTF8(value).ToLowerCase().UTF8())
		}
		if parsed, err := jsvalue.ParseJSON(value); err == nil {
			value2 = parsed
		}
		if value2 == nil {
			value2 = value
		}
		if key2 == "__proto__" {
			// Assigning __proto__ on the props object replaces its prototype instead of adding
			// a property that later lookups would see by name.
			continue
		}
		props.Set(key2, value2)
	}
	return props
}

// processMatches applies EditorConfig's defaults between indent_style, indent_size and
// tab_width.
func processMatches(m *jsvalue.Object) {
	style, hasStyle := m.Get("indent_style")
	size, hasSize := m.Get("indent_size")
	if hasStyle && style == "tab" && !hasSize {
		m.Set("indent_size", "tab")
		size, hasSize = "tab", true
	}
	_, hasTabWidth := m.Get("tab_width")
	if hasSize && !hasTabWidth && size != "tab" {
		m.Set("tab_width", size)
		hasTabWidth = true
	}
	if hasSize && hasTabWidth && size == "tab" {
		tw, _ := m.Get("tab_width")
		m.Set("indent_size", tw)
	}
}
