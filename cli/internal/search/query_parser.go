package search

import (
	"fmt"
	"strings"
	"time"
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

// ParseQuery parses Chatto's public message-search syntax. Unknown field-like
// tokens remain ordinary required terms so adding future filters does not make
// existing literal searches disappear silently.
func ParseQuery(input string) (ParsedQuery, error) {
	tokens, err := scanQueryTokens(strings.TrimSpace(input))
	if err != nil {
		return ParsedQuery{}, err
	}
	var parsed ParsedQuery
	for _, token := range tokens {
		value := strings.TrimSpace(token.value)
		if value == "" {
			return ParsedQuery{}, fmt.Errorf("search query contains an empty token")
		}
		if token.quoted {
			parsed.RequiredPhrases = append(parsed.RequiredPhrases, value)
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
				continue
			case "from":
				if operand == "" {
					return ParsedQuery{}, fmt.Errorf("from filter requires an author")
				}
				parsed.AuthorSelectors = appendUniqueFold(parsed.AuthorSelectors, operand)
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
	}
	if len(parsed.RequiredTerms) == 0 && len(parsed.RequiredPhrases) == 0 {
		return ParsedQuery{}, fmt.Errorf("search query requires a term or quoted phrase")
	}
	if parsed.CreatedAfter != nil && parsed.CreatedBefore != nil && !parsed.CreatedAfter.Before(*parsed.CreatedBefore) {
		return ParsedQuery{}, fmt.Errorf("after filter must precede before filter")
	}
	return parsed, nil
}

func scanQueryTokens(input string) ([]queryToken, error) {
	var tokens []queryToken
	for offset := 0; offset < len(input); {
		for offset < len(input) && isQuerySpace(input[offset]) {
			offset++
		}
		if offset == len(input) {
			break
		}

		var token strings.Builder
		quotedOnly := input[offset] == '"'
		for offset < len(input) && !isQuerySpace(input[offset]) {
			if input[offset] != '"' {
				token.WriteByte(input[offset])
				offset++
				continue
			}
			offset++
			closed := false
			for offset < len(input) {
				switch input[offset] {
				case '\\':
					if offset+1 < len(input) && (input[offset+1] == '"' || input[offset+1] == '\\') {
						token.WriteByte(input[offset+1])
						offset += 2
						continue
					}
					token.WriteByte(input[offset])
					offset++
				case '"':
					offset++
					closed = true
				default:
					token.WriteByte(input[offset])
					offset++
				}
				if closed {
					break
				}
			}
			if !closed {
				return nil, fmt.Errorf("search query contains an unterminated quote")
			}
		}
		tokens = append(tokens, queryToken{value: token.String(), quoted: quotedOnly})
	}
	return tokens, nil
}

func isQuerySpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
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
