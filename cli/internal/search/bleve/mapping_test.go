package bleve

import (
	"testing"

	blevemapping "github.com/blevesearch/bleve/v2/mapping"
	bleveindex "github.com/blevesearch/bleve_index_api"
	"github.com/stretchr/testify/require"
)

func TestIndexMappingUsesBM25AndPurposeBuiltBodyFields(t *testing.T) {
	indexMapping := newIndexMapping()
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
