package search

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseQuery(t *testing.T) {
	parsed, err := ParseQuery(`motherfucking AND search "exact phrase" in:"Archived Room" from:alice after:2025-01-01 before:2025-02-01 has:attachments`)
	require.NoError(t, err)
	require.Equal(t, []string{"motherfucking", "search"}, parsed.RequiredTerms)
	require.Equal(t, []string{"exact phrase"}, parsed.RequiredPhrases)
	require.Equal(t, []string{"Archived Room"}, parsed.RoomSelectors)
	require.Equal(t, []string{"alice"}, parsed.AuthorSelectors)
	require.Equal(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), *parsed.CreatedAfter)
	require.Equal(t, time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), *parsed.CreatedBefore)
	require.True(t, parsed.HasAttachments)
}

func TestParseQueryUsesStrictestRepeatedDateBounds(t *testing.T) {
	parsed, err := ParseQuery("search after:2025-01-01 after:2025-01-03 before:2025-02-01 before:2025-01-20")
	require.NoError(t, err)
	require.Equal(t, time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), *parsed.CreatedAfter)
	require.Equal(t, time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC), *parsed.CreatedBefore)
}

func TestParseQueryRejectsInvalidSyntax(t *testing.T) {
	tests := []string{
		`"unterminated`,
		`"!!!"`,
		"in:room has:attachments",
		"search in:",
		"search after:not-a-date",
		"search after:2025-02-01 before:2025-01-01",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseQuery(input)
			require.Error(t, err)
		})
	}
}

func TestParseQueryKeepsUnknownFiltersAsTerms(t *testing.T) {
	parsed, err := ParseQuery("search future:value")
	require.NoError(t, err)
	require.Equal(t, []string{"search", "future:value"}, parsed.RequiredTerms)
}

func TestParseQueryRejectsLimitsBeforeScopeResolution(t *testing.T) {
	_, err := ParseQuery(strings.Repeat("term ", maxQueryParts+1))
	require.ErrorContains(t, err, "terms and phrases")

	filters := make([]string, maxFilterIDs+1)
	for index := range filters {
		filters[index] = fmt.Sprintf("in:room-%d", index)
	}
	_, err = ParseQuery("search " + strings.Join(filters, " "))
	require.ErrorContains(t, err, "room filters")
}
