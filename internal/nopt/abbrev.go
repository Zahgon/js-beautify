package nopt

import "sort"

// abbrev is a port of the abbrev package: it maps every unambiguous prefix of each word (and
// the word itself) to the word.
func abbrev(words []string) map[string]string {
	list := append([]string(nil), words...)
	sort.Strings(list)
	abbrevs := map[string]string{}
	prev := ""
	for i, current := range list {
		next := ""
		if i+1 < len(list) {
			next = list[i+1]
		}
		if current == next {
			continue
		}
		nextMatches, prevMatches := true, true
		j := 0
		for ; j < len(current); j++ {
			c := current[j]
			nextMatches = nextMatches && j < len(next) && c == next[j]
			prevMatches = prevMatches && j < len(prev) && c == prev[j]
			if !nextMatches && !prevMatches {
				j++
				break
			}
		}
		prev = current
		if j == len(current) {
			abbrevs[current] = current
			continue
		}
		for a := current[:j]; j <= len(current); j++ {
			abbrevs[a] = current
			if j < len(current) {
				a += current[j : j+1]
			}
		}
	}
	return abbrevs
}
