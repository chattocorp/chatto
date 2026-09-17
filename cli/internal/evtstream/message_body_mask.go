package evtstream

import (
	"fmt"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// MessageBodyFields identifies the field groups selected by a body update.
// Encryption fields and the two attachment representations change together.
type MessageBodyFields struct {
	Text, Attachments, LinkPreview, Descriptions bool
}

// ParseMessageBodyMask validates stored update paths. A nil mask is a legacy
// complete replacement; an empty non-nil mask preserves every field. Unknown
// paths must stop replay so readers cannot silently discard newer content.
func ParseMessageBodyMask(mask *fieldmaskpb.FieldMask) (MessageBodyFields, error) {
	if mask == nil {
		return MessageBodyFields{true, true, true, true}, nil
	}
	paths := make(map[string]bool)
	for _, path := range mask.GetPaths() {
		switch path {
		case "encrypted_body", "encryption_nonce", "encryption_version", "content_key_epoch", "attachments", "asset_ids", "link_preview", "attachment_descriptions":
			paths[path] = true
		default:
			return MessageBodyFields{}, fmt.Errorf("unsupported message body update path")
		}
	}
	fields := MessageBodyFields{Text: paths["encrypted_body"], Attachments: paths["asset_ids"], LinkPreview: paths["link_preview"], Descriptions: paths["attachment_descriptions"]}
	if fields.Text != paths["encryption_nonce"] || fields.Text != paths["encryption_version"] || fields.Text != paths["content_key_epoch"] || fields.Attachments != paths["attachments"] {
		return MessageBodyFields{}, fmt.Errorf("incomplete message body update field group")
	}
	return fields, nil
}

// Mask returns an explicit mask, including for an update with no changed fields.
func (f MessageBodyFields) Mask() *fieldmaskpb.FieldMask {
	mask := &fieldmaskpb.FieldMask{}
	if f.Text {
		mask.Paths = append(mask.Paths, "encrypted_body", "encryption_nonce", "encryption_version", "content_key_epoch")
	}
	if f.Attachments {
		mask.Paths = append(mask.Paths, "attachments", "asset_ids")
	}
	if f.LinkPreview {
		mask.Paths = append(mask.Paths, "link_preview")
	}
	if f.Descriptions {
		mask.Paths = append(mask.Paths, "attachment_descriptions")
	}
	return mask
}
