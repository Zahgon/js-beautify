package glob

import "github.com/beautifier/js-beautify-go/internal/minimatch"

// This file ports glob's Processor (which decides, for one directory, which entries match and
// which must be walked further) and the synchronous GlobWalker.

// walkedCache records which patterns have been processed for which paths. Copies share the
// per-path sets, as glob's HasWalkedCache copies do.
type walkedCache map[string]map[string]bool

func (c walkedCache) copy() walkedCache {
	out := make(walkedCache, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}

func (c walkedCache) hasWalked(target *pathEntry, p *pattern) bool {
	return c[target.fullPath()][p.glob()]
}

func (c walkedCache) storeWalked(target *pathEntry, p *pattern) {
	full := target.fullPath()
	if set, ok := c[full]; ok {
		set[p.glob()] = true
	} else {
		c[full] = map[string]bool{p.glob(): true}
	}
}

// matchRecord collects matched entries in insertion order, with whether the absolute path is
// wanted and whether the entry only counts if it is a directory.
type matchRecord struct {
	order []*pathEntry
	flags map[*pathEntry]int
}

func (m *matchRecord) add(target *pathEntry, absolute, ifDir bool) {
	n := 0
	if absolute {
		n |= 2
	}
	if ifDir {
		n |= 1
	}
	if m.flags == nil {
		m.flags = map[*pathEntry]int{}
	}
	if current, ok := m.flags[target]; ok {
		m.flags[target] = n & current
		return
	}
	m.flags[target] = n
	m.order = append(m.order, target)
}

// subWalks collects, per directory, the patterns its entries must be tested against.
type subWalks struct {
	order []*pathEntry
	store map[*pathEntry][]*pattern
}

func (s *subWalks) add(target *pathEntry, p *pattern) {
	if !target.canReaddir() {
		return
	}
	if s.store == nil {
		s.store = map[*pathEntry][]*pattern{}
	}
	subs, ok := s.store[target]
	if !ok {
		s.store[target] = []*pattern{p}
		s.order = append(s.order, target)
		return
	}
	for _, q := range subs {
		if q.glob() == p.glob() {
			return
		}
	}
	s.store[target] = append(subs, p)
}

func (s *subWalks) keys() []*pathEntry {
	var out []*pathEntry
	for _, t := range s.order {
		if t.canReaddir() {
			out = append(out, t)
		}
	}
	return out
}

type processor struct {
	walked   walkedCache
	matches  matchRecord
	subwalks subWalks
}

func newProcessor(walked walkedCache) *processor {
	if walked == nil {
		return &processor{walked: walkedCache{}}
	}
	return &processor{walked: walked.copy()}
}

func (pr *processor) processPatterns(target *pathEntry, patterns []*pattern) {
	for _, pat := range patterns {
		t := target
		pr.walked.storeWalked(t, pat)
		absolute := pat.absolute
		if root := pat.root(); root != "" {
			t = t.resolve(root)
			rest := pat.rest()
			if rest == nil {
				pr.matches.add(t, true, false)
				continue
			}
			pat = rest
		}
		if t.isENOENT() {
			continue
		}
		changed := false
		for {
			p := pat.part()
			rest := pat.rest()
			if p.IsGlobstar() || p.IsRegexp() || rest == nil {
				break
			}
			t = t.resolve(p.Literal())
			pat = rest
			changed = true
		}
		p := pat.part()
		rest := pat.rest()
		if changed {
			if pr.walked.hasWalked(t, pat) {
				continue
			}
			pr.walked.storeWalked(t, pat)
		}
		switch {
		case p.IsGlobstar():
			if !t.isSymbolicLink() || pat.checkFollowGlobstar() {
				pr.subwalks.add(t, pat)
			}
			var rp string
			var rpString bool
			var rrest *pattern
			if rest != nil {
				part := rest.part()
				rpString = !part.IsGlobstar() && !part.IsRegexp()
				if rpString {
					rp = part.Literal()
				}
				rrest = rest.rest()
			}
			emptyOrDot := rpString && (rp == "" || rp == ".")
			if rest == nil || (emptyOrDot && rrest == nil) {
				// Only has to be a directory if it ends in **/ or **/.
				pr.matches.add(t, absolute, emptyOrDot)
			} else if rpString && rp == ".." {
				tp := t
				if t.parent != nil {
					tp = t.parent
				}
				if rrest == nil {
					pr.matches.add(tp, absolute, true)
				} else if !pr.walked.hasWalked(tp, rrest) {
					pr.subwalks.add(tp, rrest)
				}
			}
		case p.IsRegexp():
			pr.subwalks.add(t, pat)
		default:
			lit := p.Literal()
			ifDir := lit == ".." || lit == "" || lit == "."
			pr.matches.add(t.resolve(lit), absolute, ifDir)
		}
	}
}

func (pr *processor) child() *processor {
	return newProcessor(pr.walked)
}

// filterEntries tests the entries of parent against the patterns to walk there, and returns
// the processor holding the results.
func (pr *processor) filterEntries(parent *pathEntry, entries []*pathEntry) *processor {
	patterns := pr.subwalks.store[parent]
	results := pr.child()
	for _, e := range entries {
		for _, pat := range patterns {
			p := pat.part()
			rest := pat.rest()
			switch {
			case p.IsGlobstar():
				results.testGlobstar(e, pat, rest, pat.absolute)
			case p.IsRegexp():
				results.testRegExp(e, p, rest, pat.absolute)
			default:
				results.testString(e, p.Literal(), rest, pat.absolute)
			}
		}
	}
	return results
}

func (pr *processor) testGlobstar(e *pathEntry, pat, rest *pattern, absolute bool) {
	if e.name == "" || e.name[0] != '.' {
		if !pat.hasMore() {
			pr.matches.add(e, absolute, false)
		}
		if e.canReaddir() {
			switch {
			case !e.isSymbolicLink():
				pr.subwalks.add(e, pat)
			case rest != nil && pat.checkFollowGlobstar():
				pr.subwalks.add(e, rest)
			case pat.markFollowGlobstar():
				pr.subwalks.add(e, pat)
			}
		}
	}
	if rest == nil {
		return
	}
	rp := rest.part()
	switch {
	case rp.IsGlobstar():
	case rp.IsRegexp():
		pr.testRegExp(e, rp, rest.rest(), absolute)
	case rp.Literal() == "..":
		ep := e
		if e.parent != nil {
			ep = e.parent
		}
		pr.subwalks.add(ep, rest)
	case rp.Literal() != "" && rp.Literal() != ".":
		pr.testString(e, rp.Literal(), rest.rest(), absolute)
	}
}

func (pr *processor) testRegExp(e *pathEntry, p minimatch.Part, rest *pattern, absolute bool) {
	if !p.Test(e.name) {
		return
	}
	if rest == nil {
		pr.matches.add(e, absolute, false)
	} else {
		pr.subwalks.add(e, rest)
	}
}

func (pr *processor) testString(e *pathEntry, p string, rest *pattern, absolute bool) {
	if e.name != p {
		return
	}
	if rest == nil {
		pr.matches.add(e, absolute, false)
	} else {
		pr.subwalks.add(e, rest)
	}
}

// walker is glob's GlobWalker in synchronous mode.
type walker struct {
	ignore  *ignore
	seen    map[*pathEntry]bool
	results []string
	emitted map[string]bool
}

func (w *walker) ignored(e *pathEntry) bool {
	return w.seen[e] || (w.ignore != nil && w.ignore.ignored(e))
}

func (w *walker) match(e *pathEntry, ifDir bool) {
	s := e
	if e.isUnknown() {
		s = e.lstat()
	}
	if s == nil || (ifDir && !s.canReaddir()) || w.ignored(s) {
		return
	}
	w.seen[s] = true
	full := s.fullPath()
	if !w.emitted[full] {
		w.emitted[full] = true
		w.results = append(w.results, full)
	}
}

func (w *walker) walk(target *pathEntry, patterns []*pattern, pr *processor) {
	if w.ignore != nil && w.ignore.childrenIgnored(target) {
		return
	}
	pr.processPatterns(target, patterns)
	for _, m := range pr.matches.order {
		if !w.ignored(m) {
			w.match(m, pr.matches.flags[m]&1 != 0)
		}
	}
	for _, t := range pr.subwalks.keys() {
		w.walkEntries(t, t.readdir(), pr)
	}
}

func (w *walker) walkEntries(target *pathEntry, entries []*pathEntry, pr *processor) {
	pr = pr.filterEntries(target, entries)
	for _, m := range pr.matches.order {
		if !w.ignored(m) {
			w.match(m, pr.matches.flags[m]&1 != 0)
		}
	}
	for _, t := range pr.subwalks.keys() {
		w.walk(t, pr.subwalks.store[t], pr.child())
	}
}
