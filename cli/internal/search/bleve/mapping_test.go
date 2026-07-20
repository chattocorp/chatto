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

	for _, field := range []string{bodyExactField, bodyEnglishField, bodyGermanField, bodyCJKField} {
		require.NotNil(t, indexMapping.AnalyzerNamed(indexMapping.AnalyzerNameForPath(field)), field)
	}
}
