package bleve

import (
	"slices"
	"testing"

	blevemapping "github.com/blevesearch/bleve/v2/mapping"
	bleveindex "github.com/blevesearch/bleve_index_api"
	"github.com/stretchr/testify/require"
)

func TestIndexMappingUsesBM25AndPurposeBuiltBodyFields(t *testing.T) {
	languages, err := resolveLanguageAnalyzers(nil)
	require.NoError(t, err)
	indexMapping := newIndexMapping(languages)
	implementation, ok := indexMapping.(*blevemapping.IndexMappingImpl)
	require.True(t, ok)
	require.Equal(t, bleveindex.BM25Scoring, implementation.ScoringModel)

	bodyMapping := implementation.DefaultMapping.Properties["body"]
	require.NotNil(t, bodyMapping)
	require.Len(t, bodyMapping.Fields, 1+len(bodyLanguageAnalyzers))
	fields := make(map[string]string, len(bodyMapping.Fields))
	for _, field := range bodyMapping.Fields {
		fields[field.Name] = field.Analyzer
	}
	require.Equal(t, bodyExactAnalyzer, fields[bodyExactField])
	require.Len(t, bodyLanguageAnalyzers, 22)
	for _, language := range bodyLanguageAnalyzers {
		require.Equal(t, language.analyzer, fields[language.field], language.field)
		require.NotNil(t, indexMapping.AnalyzerNamed(language.analyzer), language.field)
	}
}

func TestConfiguredLanguageAnalyzersAreCanonicalAndSelective(t *testing.T) {
	languages, err := resolveLanguageAnalyzers([]string{"fr", "en"})
	require.NoError(t, err)
	require.Equal(t, []string{"en", "fr"}, []string{languages[0].code, languages[1].code})

	indexMapping := newIndexMapping(languages).(*blevemapping.IndexMappingImpl)
	bodyFields := indexMapping.DefaultMapping.Properties["body"].Fields
	require.Len(t, bodyFields, 3)
	require.True(t, slices.ContainsFunc(bodyFields, func(field *blevemapping.FieldMapping) bool {
		return field.Name == "body_en"
	}))
	require.True(t, slices.ContainsFunc(bodyFields, func(field *blevemapping.FieldMapping) bool {
		return field.Name == "body_fr"
	}))
	require.False(t, slices.ContainsFunc(bodyFields, func(field *blevemapping.FieldMapping) bool {
		return field.Name == "body_de"
	}))
}

func TestLanguageAnalyzerConfigurationRejectsUnknownAndDuplicateCodes(t *testing.T) {
	_, err := resolveLanguageAnalyzers([]string{"en", "xx"})
	require.ErrorContains(t, err, `unsupported Bleve language analyzer "xx"`)
	_, err = resolveLanguageAnalyzers([]string{"en", "en"})
	require.ErrorContains(t, err, `duplicate Bleve language analyzer "en"`)
}
