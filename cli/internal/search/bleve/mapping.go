package bleve

import (
	blevesearch "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/lang/ar"
	"github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	"github.com/blevesearch/bleve/v2/analysis/lang/ckb"
	"github.com/blevesearch/bleve/v2/analysis/lang/da"
	"github.com/blevesearch/bleve/v2/analysis/lang/de"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/lang/es"
	"github.com/blevesearch/bleve/v2/analysis/lang/fa"
	"github.com/blevesearch/bleve/v2/analysis/lang/fi"
	"github.com/blevesearch/bleve/v2/analysis/lang/fr"
	"github.com/blevesearch/bleve/v2/analysis/lang/hi"
	"github.com/blevesearch/bleve/v2/analysis/lang/hr"
	"github.com/blevesearch/bleve/v2/analysis/lang/hu"
	"github.com/blevesearch/bleve/v2/analysis/lang/it"
	"github.com/blevesearch/bleve/v2/analysis/lang/nl"
	"github.com/blevesearch/bleve/v2/analysis/lang/no"
	"github.com/blevesearch/bleve/v2/analysis/lang/pl"
	"github.com/blevesearch/bleve/v2/analysis/lang/pt"
	"github.com/blevesearch/bleve/v2/analysis/lang/ro"
	"github.com/blevesearch/bleve/v2/analysis/lang/ru"
	"github.com/blevesearch/bleve/v2/analysis/lang/sv"
	"github.com/blevesearch/bleve/v2/analysis/lang/tr"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	bleveindex "github.com/blevesearch/bleve_index_api"
)

const (
	bodyExactField    = "body_exact"
	bodyCJKField      = "body_cjk"
	bodyExactAnalyzer = "chatto_exact"
	bodyCJKAnalyzer   = cjk.AnalyzerName
)

type languageAnalyzer struct {
	field    string
	analyzer string
}

// bodyLanguageAnalyzers contains every complete language analyzer shipped by
// Bleve. The neutral body_exact field remains authoritative for literal
// matching; these fields add lower-boost language-specific recall.
var bodyLanguageAnalyzers = []languageAnalyzer{
	{field: "body_ar", analyzer: ar.AnalyzerName},
	{field: bodyCJKField, analyzer: bodyCJKAnalyzer},
	{field: "body_ckb", analyzer: ckb.AnalyzerName},
	{field: "body_da", analyzer: da.AnalyzerName},
	{field: "body_de", analyzer: de.AnalyzerName},
	{field: "body_en", analyzer: en.AnalyzerName},
	{field: "body_es", analyzer: es.AnalyzerName},
	{field: "body_fa", analyzer: fa.AnalyzerName},
	{field: "body_fi", analyzer: fi.AnalyzerName},
	{field: "body_fr", analyzer: fr.AnalyzerName},
	{field: "body_hi", analyzer: hi.AnalyzerName},
	{field: "body_hr", analyzer: hr.AnalyzerName},
	{field: "body_hu", analyzer: hu.AnalyzerName},
	{field: "body_it", analyzer: it.AnalyzerName},
	{field: "body_nl", analyzer: nl.AnalyzerName},
	{field: "body_no", analyzer: no.AnalyzerName},
	{field: "body_pl", analyzer: pl.AnalyzerName},
	{field: "body_pt", analyzer: pt.AnalyzerName},
	{field: "body_ro", analyzer: ro.AnalyzerName},
	{field: "body_ru", analyzer: ru.AnalyzerName},
	{field: "body_sv", analyzer: sv.AnalyzerName},
	{field: "body_tr", analyzer: tr.AnalyzerName},
}

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
	document.AddFieldMappingsAt("body_event_id", keyword(true))
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
	fields := make([]*mapping.FieldMapping, 0, 1+len(bodyLanguageAnalyzers))
	fields = append(fields, field(bodyExactField, bodyExactAnalyzer, true))
	for _, language := range bodyLanguageAnalyzers {
		fields = append(fields, field(
			language.field,
			language.analyzer,
			language.field == bodyCJKField,
		))
	}
	return fields
}
