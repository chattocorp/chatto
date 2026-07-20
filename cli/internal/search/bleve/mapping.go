package bleve

import (
	blevesearch "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

func newIndexMapping() mapping.IndexMapping {
	indexMapping := blevesearch.NewIndexMapping()
	document := blevesearch.NewDocumentStaticMapping()

	keyword := func(stored bool) *mapping.FieldMapping {
		field := blevesearch.NewKeywordFieldMapping()
		field.Store = stored
		return field
	}
	text := blevesearch.NewTextFieldMapping()
	date := blevesearch.NewDateTimeFieldMapping()
	boolean := blevesearch.NewBooleanFieldMapping()

	document.AddFieldMappingsAt("message_id", keyword(false))
	document.AddFieldMappingsAt("room_id", keyword(true))
	document.AddFieldMappingsAt("author_id", keyword(false))
	document.AddFieldMappingsAt("body", text)
	document.AddFieldMappingsAt("created_at", date)
	document.AddFieldMappingsAt("updated_at", date)
	document.AddFieldMappingsAt("has_attachments", boolean)
	document.AddFieldMappingsAt("visible", boolean)
	indexMapping.DefaultMapping = document
	return indexMapping
}
