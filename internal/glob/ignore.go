package glob

import "github.com/beautifier/js-beautify-go/internal/minimatch"

// ignore ports glob's Ignore: patterns whose matches are left out of the results, and whose
// "/**" forms stop the walk from descending.
type ignore struct {
	relative, relativeChildren, absolute, absoluteChildren []*minimatch.Minimatch
}

var ignoreOptions = minimatch.Options{Dot: true, NoComment: true, NoNegate: true, OptimizationLevel: 2}

func newIgnore(patterns []string) (*ignore, error) {
	ig := &ignore{}
	for _, ign := range patterns {
		mm, err := minimatch.New(ign, ignoreOptions)
		if err != nil {
			return nil, err
		}
		for i, parsed := range mm.Set {
			globParts := mm.GlobParts[i]
			// Strip leading ./ portions.
			for len(parsed) > 0 && len(globParts) > 0 && !parsed[0].IsGlobstar() && !parsed[0].IsRegexp() &&
				parsed[0].Literal() == "." && globParts[0] == "." {
				parsed, globParts = parsed[1:], globParts[1:]
			}
			p := newPattern(parsed, globParts)
			m, err := minimatch.New(p.glob(), ignoreOptions)
			if err != nil {
				return nil, err
			}
			children := globParts[len(globParts)-1] == "**"
			if p.absolute {
				ig.absolute = append(ig.absolute, m)
				if children {
					ig.absoluteChildren = append(ig.absoluteChildren, m)
				}
			} else {
				ig.relative = append(ig.relative, m)
				if children {
					ig.relativeChildren = append(ig.relativeChildren, m)
				}
			}
		}
	}
	return ig, nil
}

func relativeOrDot(p *pathEntry) string {
	if rel := p.relativePath(); rel != "" {
		return rel
	}
	return "."
}

func (ig *ignore) ignored(p *pathEntry) bool {
	full := p.fullPath()
	rel := relativeOrDot(p)
	for _, m := range ig.relative {
		if m.Match(rel) || m.Match(rel+"/") {
			return true
		}
	}
	for _, m := range ig.absolute {
		if m.Match(full) || m.Match(full+"/") {
			return true
		}
	}
	return false
}

func (ig *ignore) childrenIgnored(p *pathEntry) bool {
	full := p.fullPath() + "/"
	rel := relativeOrDot(p) + "/"
	for _, m := range ig.relativeChildren {
		if m.Match(rel) {
			return true
		}
	}
	for _, m := range ig.absoluteChildren {
		if m.Match(full) {
			return true
		}
	}
	return false
}
