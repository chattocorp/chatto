package search

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// ParsedQuery is the provider-neutral meaning of Chatto's public message
// search syntax before room and author selectors are resolved to stable IDs.
type ParsedQuery struct {
	RequiredTerms   []string
	RequiredPhrases []string
	RoomSelectors   []string
	AuthorSelectors []string
	CreatedAfter    *time.Time
	CreatedBefore   *time.Time
	HasAttachments  bool
}

type queryToken struct {
	value  string
	quoted bool
}

type querySyntax struct {
	Parts []*querySyntaxPart `@@*`
}

// querySyntaxPart is one bare or quoted fragment. Participle positions let us
// join adjacent fragments into the whitespace-delimited tokens understood by
// Chatto's query semantics without hand-scanning the input.
type querySyntaxPart struct {
	Pos    lexer.Position
	Bare   *string `  @Bare`
	Quoted *string `| @Quoted`
	EndPos lexer.Position
}

var querySyntaxParser = participle.MustBuild[querySyntax](
	participle.Lexer(lexer.MustSimple([]lexer.SimpleRule{
		{Name: "Whitespace", Pattern: `[ \t\n\r]+`},
		{Name: "Quoted", Pattern: `"(?s:[^"\\]|\\.)*"`},
		{Name: "Bare", Pattern: `[^ \t\n\r"]+`},
	})),
	participle.Elide("Whitespace"),
)

var queryQuotedEscapes = strings.NewReplacer(`\"`, `"`, `\\`, `\`)

// ParseQuery parses Chatto's public message-search syntax. Unknown field-like
// tokens remain ordinary required terms so adding future filters does not make
// existing literal searches disappear silently.
func ParseQuery(input string) (ParsedQuery, error) {
	tokens, err := parseQueryTokens(strings.TrimSpace(input))
	if err != nil {
		return ParsedQuery{}, err
	}
	var parsed ParsedQuery
	for _, token := range tokens {
		value := strings.TrimSpace(token.value)
		if value == "" {
			return ParsedQuery{}, fmt.Errorf("search query contains an empty token")
		}
		if !containsSearchableRune(value) {
			return ParsedQuery{}, fmt.Errorf("search query contains a token without letters or numbers")
		}
		if token.quoted {
			parsed.RequiredPhrases = append(parsed.RequiredPhrases, value)
			if len(parsed.RequiredTerms)+len(parsed.RequiredPhrases) > maxQueryParts {
				return ParsedQuery{}, fmt.Errorf("search query exceeds %d terms and phrases", maxQueryParts)
			}
			continue
		}
		if strings.EqualFold(value, "AND") {
			continue
		}

		key, operand, hasKey := strings.Cut(value, ":")
		if hasKey {
			switch strings.ToLower(key) {
			case "in":
				if operand == "" {
					return ParsedQuery{}, fmt.Errorf("in filter requires a room")
				}
				parsed.RoomSelectors = appendUniqueFold(parsed.RoomSelectors, operand)
				if len(parsed.RoomSelectors) > maxFilterIDs {
					return ParsedQuery{}, fmt.Errorf("search query exceeds %d room filters", maxFilterIDs)
				}
				continue
			case "from":
				if operand == "" {
					return ParsedQuery{}, fmt.Errorf("from filter requires an author")
				}
				parsed.AuthorSelectors = appendUniqueFold(parsed.AuthorSelectors, operand)
				if len(parsed.AuthorSelectors) > maxFilterIDs {
					return ParsedQuery{}, fmt.Errorf("search query exceeds %d author filters", maxFilterIDs)
				}
				continue
			case "after":
				bound, err := parseQueryTime(operand)
				if err != nil {
					return ParsedQuery{}, fmt.Errorf("invalid after filter: %w", err)
				}
				if parsed.CreatedAfter == nil || bound.After(*parsed.CreatedAfter) {
					parsed.CreatedAfter = &bound
				}
				continue
			case "before":
				bound, err := parseQueryTime(operand)
				if err != nil {
					return ParsedQuery{}, fmt.Errorf("invalid before filter: %w", err)
				}
				if parsed.CreatedBefore == nil || bound.Before(*parsed.CreatedBefore) {
					parsed.CreatedBefore = &bound
				}
				continue
			case "has":
				if strings.EqualFold(operand, "attachment") || strings.EqualFold(operand, "attachments") {
					parsed.HasAttachments = true
					continue
				}
			}
		}
		parsed.RequiredTerms = append(parsed.RequiredTerms, value)
		if len(parsed.RequiredTerms)+len(parsed.RequiredPhrases) > maxQueryParts {
			return ParsedQuery{}, fmt.Errorf("search query exceeds %d terms and phrases", maxQueryParts)
		}
	}
	if len(parsed.RequiredTerms) == 0 && len(parsed.RequiredPhrases) == 0 {
		return ParsedQuery{}, fmt.Errorf("search query requires a term or quoted phrase")
	}
	if parsed.CreatedAfter != nil && parsed.CreatedBefore != nil && !parsed.CreatedAfter.Before(*parsed.CreatedBefore) {
		return ParsedQuery{}, fmt.Errorf("after filter must precede before filter")
	}
	return parsed, nil
}

func containsSearchableRune(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsNumber(r)
	}) >= 0
}

func parseQueryTokens(input string) ([]queryToken, error) {
	syntax, err := querySyntaxParser.ParseString("", input)
	if err != nil {
		// Bare tokens accept every non-whitespace rune except a quote, so the
		// only input-dependent lexer failure is an unclosed quoted fragment.
		// Do not return Participle's error because it includes query contents.
		return nil, fmt.Errorf("search query contains an unterminated quote")
	}

	tokens := make([]queryToken, 0, len(syntax.Parts))
	previousEnd := -1
	for _, part := range syntax.Parts {
		value, quoted := querySyntaxPartValue(part)
		if len(tokens) == 0 || part.Pos.Offset != previousEnd {
			tokens = append(tokens, queryToken{quoted: quoted})
		}
		tokens[len(tokens)-1].value += value
		previousEnd = part.EndPos.Offset
	}
	return tokens, nil
}

func querySyntaxPartValue(part *querySyntaxPart) (string, bool) {
	if part.Bare != nil {
		return *part.Bare, false
	}
	quoted := strings.TrimSuffix(strings.TrimPrefix(*part.Quoted, `"`), `"`)
	return queryQuotedEscapes.Replace(quoted), true
}

func parseQueryTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("use YYYY-MM-DD or RFC3339")
	}
	return parsed.UTC(), nil
}

func appendUniqueFold(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}
