package events

import (
	"strings"
)

func subjectMatchesFilter(filter, subject string) bool {
	return compileSubjectFilter(filter).matches(subject)
}

type compiledSubjectFilter struct {
	raw    string
	tokens []string
}

func compileSubjectFilters(filters []string) []compiledSubjectFilter {
	compiled := make([]compiledSubjectFilter, 0, len(filters))
	for _, filter := range filters {
		compiled = append(compiled, compileSubjectFilter(filter))
	}
	return compiled
}

func compileSubjectFilter(filter string) compiledSubjectFilter {
	return compiledSubjectFilter{
		raw:    filter,
		tokens: splitSubjectTokens(filter),
	}
}

// splitSubjectTokens splits a NATS-style dotted subject into tokens.
// It tolerates empty tokens, including a trailing one ('a.b.' → ["a","b",""]);
// match semantics reject them later.
func splitSubjectTokens(subject string) []string {
	if subject == "" {
		return nil
	}
	tokens := make([]string, 0, strings.Count(subject, ".")+1)
	for {
		var token string
		var found bool
		token, subject, found = strings.Cut(subject, ".")
		tokens = append(tokens, token)
		if !found {
			return tokens
		}
	}
}

// matches reports whether one exact subject matches the compiled wildcard
// filter ('*' spans exactly one token; '>' is a terminal, nonempty-tail
// wildcard). It runs per delivered message and allocates nothing:
// the token scan works on subject in place with no copies.
func (f compiledSubjectFilter) matches(subject string) bool {
	if f.raw == "" || subject == "" {
		return false
	}
	// pos is the index of the next unconsumed subject byte, i.e. the start of
	// the subject token aligned with the current filter token. After consuming
	// a subject token through its delimiter, pos sits one past that delimiter,
	// which is why full consumption ends at len(subject)+1.
	pos := 0
	for i, filterToken := range f.tokens {
		if filterToken == ">" {
			// '>' matches everything remaining, but only as the last filter
			// token and only when a nonempty subject tail remains.
			return i == len(f.tokens)-1 && pos < len(subject)
		}
		if pos > len(subject) {
			// The previous subject token had no trailing delimiter, so the
			// filter has more tokens than the subject.
			return false
		}
		subjectEnd := pos
		for subjectEnd < len(subject) && subject[subjectEnd] != '.' {
			subjectEnd++
		}
		if subjectEnd == pos {
			return false // empty subject token ('..' or trailing '.')
		}
		if filterToken != "*" && filterToken != subject[pos:subjectEnd] {
			return false
		}
		pos = subjectEnd + 1
	}
	return pos == len(subject)+1
}

func matchesAnySubject(filters []compiledSubjectFilter, subject string) bool {
	for i := range filters {
		if filters[i].matches(subject) {
			return true
		}
	}
	return false
}

// subjectHasWildcard reports whether a subject filter has a '*' or '>' token.
// A filter without one matches exactly one subject.
func subjectHasWildcard(filter string) bool {
	for _, token := range splitSubjectTokens(filter) {
		if token == "*" || token == ">" {
			return true
		}
	}
	return false
}

// subjectFiltersOverlap reports whether at least one subject matches both
// filters. '*' matches exactly one token, and a terminal '>' matches one or
// more tokens. An empty filter matches no subject. A malformed filter, with an
// empty token or a '>' that is not the last token, gives true: the caller then
// does not reject a position on this check alone.
func subjectFiltersOverlap(a, b string) bool {
	at, bt := splitSubjectTokens(a), splitSubjectTokens(b)
	if len(at) == 0 || len(bt) == 0 {
		return false
	}
	if !wellFormedFilterTokens(at) || !wellFormedFilterTokens(bt) {
		return true
	}
	for i := 0; ; i++ {
		switch {
		case i == len(at) && i == len(bt):
			return true
		case i == len(at) || i == len(bt):
			return false
		}
		x, y := at[i], bt[i]
		if x == ">" || y == ">" {
			// The other filter has a token here. A subject can supply its
			// remaining tokens, because they are literal or wildcards.
			return true
		}
		if x != "*" && y != "*" && x != y {
			return false
		}
	}
}

// wellFormedFilterTokens reports whether filter tokens have no empty token and
// at most one '>', as the last token.
func wellFormedFilterTokens(tokens []string) bool {
	for i, token := range tokens {
		if token == "" || token == ">" && i != len(tokens)-1 {
			return false
		}
	}
	return true
}
