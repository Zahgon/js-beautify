package jsrt

// The matcher is a backtracking interpreter over the parsed pattern, written in the
// continuation-passing style of the ECMAScript specification (each matcher receives the
// continuation that must succeed after it), so that alternation order, greedy and lazy
// quantifiers, the empty-iteration check and the capture resets of repeated groups behave
// exactly as in JavaScript.

type matcher struct {
	in   []uint16
	caps []int // start and end of each group (group 0 is the match); -1 when unset
}

type cont func(pos int) bool

func (m *matcher) match(n *node, i int, k cont) bool {
	switch n.kind {
	case nEmpty:
		return k(i)
	case nLiteral:
		if i+len(n.lit) > len(m.in) {
			return false
		}
		for j, c := range n.lit {
			u := m.in[i+j]
			if u != c && !(n.icase && canonicalize(u) == canonicalize(c)) {
				return false
			}
		}
		return k(i + len(n.lit))
	case nSet:
		if i < len(m.in) && n.set.matches(m.in[i], n.icase) {
			return k(i + 1)
		}
		return false
	case nSeq:
		return m.seq(n.subs, i, k)
	case nAlt:
		for _, s := range n.subs {
			if m.match(s, i, k) {
				return true
			}
		}
		return false
	case nGroup:
		return m.match(n.sub, i, func(j int) bool {
			s, e := m.caps[2*n.n], m.caps[2*n.n+1]
			m.caps[2*n.n], m.caps[2*n.n+1] = i, j
			if k(j) {
				return true
			}
			m.caps[2*n.n], m.caps[2*n.n+1] = s, e
			return false
		})
	case nLook:
		return m.look(n, i, k)
	case nBackref:
		s, e := m.caps[2*n.n], m.caps[2*n.n+1]
		if s < 0 || e < 0 {
			return k(i)
		}
		l := e - s
		if i+l > len(m.in) {
			return false
		}
		for j := 0; j < l; j++ {
			a, b := m.in[s+j], m.in[i+j]
			if a != b && !(n.icase && canonicalize(a) == canonicalize(b)) {
				return false
			}
		}
		return k(i + l)
	case nLineStart:
		if i == 0 || (n.multi && IsLineTerminator(m.in[i-1])) {
			return k(i)
		}
		return false
	case nLineEnd:
		if i == len(m.in) || (n.multi && IsLineTerminator(m.in[i])) {
			return k(i)
		}
		return false
	case nWordBoundary:
		a := i > 0 && IsWordChar(m.in[i-1])
		b := i < len(m.in) && IsWordChar(m.in[i])
		if (a != b) != n.negate {
			return k(i)
		}
		return false
	case nRepeat:
		if n.single != nil {
			return m.repeatSingle(n, i, k)
		}
		return m.repeat(n, n.min, n.max, i, k)
	}
	return false
}

func (m *matcher) seq(subs []*node, i int, k cont) bool {
	if len(subs) == 0 {
		return k(i)
	}
	if len(subs) == 1 {
		return m.match(subs[0], i, k)
	}
	return m.match(subs[0], i, func(j int) bool { return m.seq(subs[1:], j, k) })
}

// look matches a lookaround. A successful positive lookaround keeps the captures it made; the
// continuation cannot backtrack into it.
func (m *matcher) look(n *node, i int, k cont) bool {
	saved := m.saveCaps(n.capLo, n.capHi)
	var ok bool
	if n.behind {
		// Some start j <= i from which the sub-pattern ends exactly at i.
		for j := 0; j <= i && !ok; j++ {
			ok = m.match(n.sub, j, func(e int) bool { return e == i })
		}
	} else {
		ok = m.match(n.sub, i, func(int) bool { return true })
	}
	if n.negate {
		m.restoreCaps(n.capLo, saved)
		if ok {
			return false
		}
		return k(i)
	}
	if ok && k(i) {
		return true
	}
	m.restoreCaps(n.capLo, saved)
	return false
}

// repeat is the RepeatMatcher of the specification.
func (m *matcher) repeat(n *node, min, max, i int, k cont) bool {
	if max == 0 {
		return k(i)
	}
	d := func(j int) bool {
		if min == 0 && j == i {
			// An iteration that matched the empty string ends the repetition with a failure.
			return false
		}
		nmin, nmax := min, max
		if nmin > 0 {
			nmin--
		}
		if nmax > 0 {
			nmax--
		}
		return m.repeat(n, nmin, nmax, j, k)
	}
	saved := m.saveCaps(n.capLo, n.capHi)
	clear := func() {
		for g := n.capLo + 1; g <= n.capHi; g++ {
			m.caps[2*g], m.caps[2*g+1] = -1, -1
		}
	}
	if min > 0 {
		clear()
		if m.match(n.sub, i, d) {
			return true
		}
		m.restoreCaps(n.capLo, saved)
		return false
	}
	if !n.greedy {
		if k(i) {
			return true
		}
		clear()
		if m.match(n.sub, i, d) {
			return true
		}
		m.restoreCaps(n.capLo, saved)
		return false
	}
	clear()
	if m.match(n.sub, i, d) {
		return true
	}
	m.restoreCaps(n.capLo, saved)
	return k(i)
}

// repeatSingle repeats a sub-pattern that always matches exactly one code unit and captures
// nothing, iteratively; it behaves exactly as repeat does for such a sub-pattern.
func (m *matcher) repeatSingle(n *node, i int, k cont) bool {
	single := n.single
	limit := len(m.in) - i
	if n.max >= 0 && n.max < limit {
		limit = n.max
	}
	if n.greedy {
		count := 0
		for count < limit && single(m.in[i+count]) {
			count++
		}
		for c := count; c >= n.min; c-- {
			if k(i + c) {
				return true
			}
		}
		return false
	}
	for c := 0; c <= limit; c++ {
		if c >= n.min && k(i+c) {
			return true
		}
		if c == limit || !single(m.in[i+c]) {
			return false
		}
	}
	return false
}

// singleUnit returns a predicate when n matches exactly one code unit without side effects.
func singleUnit(n *node) func(uint16) bool {
	switch n.kind {
	case nSet:
		set, icase := n.set, n.icase
		return func(u uint16) bool { return set.matches(u, icase) }
	case nLiteral:
		if len(n.lit) != 1 {
			return nil
		}
		c, icase := n.lit[0], n.icase
		if !icase {
			return func(u uint16) bool { return u == c }
		}
		cc := canonicalize(c)
		return func(u uint16) bool { return u == c || canonicalize(u) == cc }
	case nAlt:
		preds := make([]func(uint16) bool, len(n.subs))
		for i, s := range n.subs {
			if preds[i] = singleUnit(s); preds[i] == nil {
				return nil
			}
		}
		return func(u uint16) bool {
			for _, p := range preds {
				if p(u) {
					return true
				}
			}
			return false
		}
	}
	return nil
}

func (m *matcher) saveCaps(lo, hi int) []int {
	if hi <= lo {
		return nil
	}
	return append([]int(nil), m.caps[2*(lo+1):2*(hi+1)]...)
}

func (m *matcher) restoreCaps(lo int, saved []int) {
	copy(m.caps[2*(lo+1):], saved)
}

// matchAt tries to match re at exactly position i; it returns the capture array on success.
func (re *Regexp) matchAt(in []uint16, i int) []int {
	m := &matcher{in: in, caps: make([]int, 2*(re.ncap+1))}
	for j := range m.caps {
		m.caps[j] = -1
	}
	end := -1
	if !m.match(re.root, i, func(e int) bool { end = e; return true }) {
		return nil
	}
	m.caps[0], m.caps[1] = i, end
	return m.caps
}

// search finds the leftmost match starting at or after from.
func (re *Regexp) search(in []uint16, from int) []int {
	for i := from; i <= len(in); i++ {
		if re.prefix >= 0 {
			// Skip to the next occurrence of the pattern's first code unit.
			for i < len(in) && in[i] != uint16(re.prefix) {
				i++
			}
			if i == len(in) {
				return nil
			}
		}
		if caps := re.matchAt(in, i); caps != nil {
			return caps
		}
	}
	return nil
}
