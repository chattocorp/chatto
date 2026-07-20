package bleve

import (
	blevesearch "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	"github.com/blevesearch/bleve/v2/analysis/lang/de"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	bleveindex "github.com/blevesearch/bleve_index_api"
)

const (
	bodyExactField    = "body_exact"
	bodyEnglishField  = "body_en"
	bodyGermanField   = "body_de"
	bodyCJKField      = "body_cjk"
	bodyExactAnalyzer = "chatto_exact"
)

func newIndexMapping() mapping.IndexMapping {
	indexMapping := blevesearch.NewIndexMapping()
	indexMapping.ScoringModel = bleveindex.BM25Scoring
	if err := indexMapping.AddCustomAnalyzer(bodyExactAnalyzer, map[string]interface{}{
		"type": custom.Name, "tokenizer": unicode.Name,
		"token_filters": []string{lowercase.Name},
	}); err != nil {
		panic("register static Chatto search analyzer: " + err.Error())
	}
	document := blevesearch.NewDocumentStaticMapping()

	keyword := func(stored bool) *mapping.FieldMapping {
		field := blevesearch.NewKeywordFieldMapping()
		field.Store = stored
		return field
	}
	date := blevesearch.NewDateTimeFieldMapping()
	boolean := blevesearch.NewBooleanFieldMapping()

	document.AddFieldMappingsAt("message_id", keyword(false))
	document.AddFieldMappingsAt("room_id", keyword(true))
	document.AddFieldMappingsAt("author_id", keyword(false))
	document.AddFieldMappingsAt("body", searchBodyFields()...)
	document.AddFieldMappingsAt("created_at", date)
	document.AddFieldMappingsAt("updated_at", date)
	document.AddFieldMappingsAt("has_attachments", boolean)
	document.AddFieldMappingsAt("visible", boolean)
	indexMapping.DefaultMapping = document
	return indexMapping
}

// searchBodyFields keep a language-neutral representation authoritative while
// adding lower-boost recall fields for the languages we can tune confidently.
// Multiple mappings index the same source body without storing duplicate
// plaintext values or doc values.
func searchBodyFields() []*mapping.FieldMapping {
	field := func(name, analyzer string, termVectors bool) *mapping.FieldMapping {
		mapped := blevesearch.NewTextFieldMapping()
		mapped.Name = name
		mapped.Analyzer = analyzer
		mapped.Store = false
		mapped.DocValues = false
		mapped.IncludeInAll = false
		mapped.IncludeTermVectors = termVectors
		return mapped
	}
	return []*mapping.FieldMapping{
		field(bodyExactField, bodyExactAnalyzer, true),
		field(bodyEnglishField, en.AnalyzerName, false),
		field(bodyGermanField, de.AnalyzerName, false),
		field(bodyCJKField, cjk.AnalyzerName, true),
	}
}
