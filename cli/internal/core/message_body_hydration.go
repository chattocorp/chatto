package core

import (
	"context"
	"fmt"
	"slices"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// assembleBodyRecords batches all distinct field sources across a page. It
// loads current values directly, without walking history or reading manifests.
func (h *RoomTimelineHydrator) assembleBodyRecords(ctx context.Context, references []TimelineBodyReference) ([]*evtv1.MessageBody, error) {
	var sequences []uint64
	seen := make(map[uint64]bool)
	for _, reference := range references {
		if reference.StreamSeq == 0 {
			return nil, ErrMessageBodyCorrupt
		}
		for _, seq := range append(reference.FieldSequences[:], reference.StreamSeq) {
			if seq != 0 && !seen[seq] {
				sequences = append(sequences, seq)
				seen[seq] = true
			}
		}
	}
	records, err := h.reader.EventsAt(ctx, sequences)
	if err != nil {
		return nil, fmt.Errorf("hydrate message bodies: %w", err)
	}
	if len(records) != len(sequences) {
		return nil, ErrMessageBodyCorrupt
	}
	bySequence := make(map[uint64]*evtstream.SubjectEvent, len(records))
	for i, record := range records {
		bySequence[sequences[i]] = record
	}
	bodies := make([]*evtv1.MessageBody, len(references))
	for i, reference := range references {
		head, err := validateTimelineBodyRecord(reference, bySequence[reference.StreamSeq])
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMessageBodyCorrupt, err)
		}
		if reference.FieldSequences == [4]uint64{} {
			bodies[i] = head
			continue
		}
		var fields [4]*evtv1.MessageBody
		for field, seq := range reference.FieldSequences {
			record := bySequence[seq]
			if seq == 0 || record == nil || record.Event == nil {
				return nil, ErrMessageBodyCorrupt
			}
			source := reference
			source.StreamSeq, source.BodyEventID = seq, record.Event.GetId()
			body, err := validateTimelineBodyRecord(source, record)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrMessageBodyCorrupt, err)
			}
			selected, err := evtstream.ParseMessageBodyMask(record.Event.GetMessageBody().GetUpdateMask())
			if err != nil || !([]bool{selected.Text, selected.Attachments, selected.LinkPreview, selected.Descriptions})[field] {
				return nil, ErrMessageBodyCorrupt
			}
			fields[field] = body
		}
		body := fields[0]
		body.CreatedAt, body.UpdatedAt = head.CreatedAt, head.UpdatedAt
		body.Attachments, body.AssetIds = fields[1].Attachments, fields[1].AssetIds
		body.LinkPreview = fields[2].LinkPreview
		body.AttachmentDescriptions = fields[3].AttachmentDescriptions
		if messageBodyAttachmentCount(body) != reference.AttachmentCount {
			return nil, ErrMessageBodyCorrupt
		}
		assets := messageBodyAttachmentIDs(body)
		described := make(map[string]bool)
		for _, description := range body.AttachmentDescriptions {
			if !slices.Contains(assets, description.GetAssetId()) || described[description.GetAssetId()] {
				return nil, ErrMessageBodyCorrupt
			}
			described[description.GetAssetId()] = true
			if description.SourceBodyEventId == "" {
				description.SourceBodyEventId = fields[3].GetBodyEventId()
			}
		}
		bodies[i] = body
	}
	return bodies, nil
}
