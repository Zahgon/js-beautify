// Package glob ports the synchronous file system walk of glob 13.0.6 (globSync and hasMagic)
// as the js-beautify command line uses it: absolute results, ignore patterns, and results in
// the order glob's walk produces them.
package glob

import (
	"github.com/beautifier/js-beautify-go/internal/minimatch"
	"github.com/beautifier/js-beautify-go/internal/nodefmt"
)

// Options are the glob options js-beautify passes.
type Options struct {
	// Cwd is the absolute working directory patterns are resolved against.
	Cwd string
	// Absolute is accepted for documentation: js-beautify always asks for absolute paths,
	// which is what Sync returns.
	Absolute bool
	// Ignore lists patterns whose matches (and, for patterns ending in "/**", whose contents)
	// are left out.
	Ignore []string
}

// patterns returns the pattern strings of a glob argument (a string or an array of strings),
// or minimatch's TypeError for anything else.
func patterns(glob any) ([]string, error) {
	invalid := nodefmt.NewError("TypeError", "invalid pattern")
	switch p := glob.(type) {
	case string:
		return []string{p}, nil
	case []any:
		out := make([]string, len(p))
		for i, e := range p {
			s, ok := e.(string)
			if !ok {
				return nil, invalid
			}
			out[i] = s
		}
		return out, nil
	}
	return nil, invalid
}

// HasMagic returns glob.hasMagic(glob): whether any of the patterns (a string or an array
// of strings) has characters that need matching rather than a plain path. Brace sets alone are
// not magic.
func HasMagic(glob any) (bool, error) {
	ps, err := patterns(glob)
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		mm, err := minimatch.New(p, minimatch.Options{})
		if err != nil {
			return false, err
		}
		if mm.HasMagic() {
			return true, nil
		}
	}
	return false, nil
}

// Sync returns globSync(glob, options) with absolute: true: the absolute paths of the
// entries the patterns match.
func Sync(glob any, opts Options) ([]string, error) {
	ps, err := patterns(glob)
	if err != nil {
		return nil, err
	}
	mmOpts := minimatch.Options{NoComment: true, NoNegate: true, OptimizationLevel: 2, BraceExpandMax: 10_000}
	var pats []*pattern
	for _, p := range ps {
		mm, err := minimatch.New(p, mmOpts)
		if err != nil {
			return nil, err
		}
		for i, set := range mm.Set {
			pats = append(pats, newPattern(set, mm.GlobParts[i]))
		}
	}
	w := &walker{seen: map[*pathEntry]bool{}, emitted: map[string]bool{}}
	if opts.Ignore != nil {
		if w.ignore, err = newIgnore(opts.Ignore); err != nil {
			return nil, err
		}
	}
	cwd := newCwd(opts.Cwd)
	if cwd.isUnknown() {
		cwd.lstat()
	}
	w.walk(cwd, pats, newProcessor(nil))
	return w.results, nil
}
