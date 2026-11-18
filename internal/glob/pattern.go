package glob

import (
	"strings"

	"github.com/beautifier/js-beautify-go/internal/minimatch"
)

// pattern ports glob's Pattern: a view of one compiled pattern from a given portion on.
type pattern struct {
	parts      []minimatch.Part
	globs      []string
	index      int
	absolute   bool
	restCache  *pattern
	hasRest    bool
	globString string
	// followGlobstar is cleared once this globstar has followed a symbolic link.
	followGlobstar bool
}

// newPattern returns the pattern at index 0, with the root of an absolute pattern ("/") made
// its first portion.
func newPattern(parts []minimatch.Part, globs []string) *pattern {
	p := &pattern{parts: parts, globs: globs, followGlobstar: true}
	first := parts[0]
	p.absolute = len(parts) > 1 && !first.IsGlobstar() && !first.IsRegexp() && first.Literal() == ""
	if p.absolute {
		restParts, restGlobs := parts[1:], globs[1:]
		if len(restParts) > 0 && !restParts[0].IsGlobstar() && !restParts[0].IsRegexp() && restParts[0].Literal() == "" {
			// The pattern ends in /.
			restParts, restGlobs = restParts[1:], restGlobs[1:]
		}
		p.parts = append([]minimatch.Part{minimatch.LiteralPart("/")}, restParts...)
		p.globs = append([]string{globs[0] + "/"}, restGlobs...)
	}
	return p
}

func (p *pattern) part() minimatch.Part { return p.parts[p.index] }

func (p *pattern) hasMore() bool { return len(p.parts) > p.index+1 }

// rest returns the pattern after this portion, or nil at the end.
func (p *pattern) rest() *pattern {
	if p.hasRest {
		return p.restCache
	}
	p.hasRest = true
	if p.hasMore() {
		p.restCache = &pattern{parts: p.parts, globs: p.globs, index: p.index + 1, absolute: p.absolute, followGlobstar: true}
	}
	return p.restCache
}

// glob returns the /-joined glob portions from this one on.
func (p *pattern) glob() string {
	if p.globString == "" {
		switch {
		case p.index > 0:
			p.globString = strings.Join(p.globs[p.index:], "/")
		case p.absolute:
			p.globString = p.globs[0] + strings.Join(p.globs[1:], "/")
		default:
			p.globString = strings.Join(p.globs, "/")
		}
	}
	return p.globString
}

// root returns the root portion of an absolute pattern at index 0, else "".
func (p *pattern) root() string {
	if p.absolute && p.index == 0 {
		return p.parts[0].Literal()
	}
	return ""
}

func (p *pattern) checkFollowGlobstar() bool {
	return p.index != 0 && p.part().IsGlobstar() && p.followGlobstar
}

func (p *pattern) markFollowGlobstar() bool {
	if !p.checkFollowGlobstar() {
		return false
	}
	p.followGlobstar = false
	return true
}
