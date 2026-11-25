package core

import (
	"cmp"
	"context"
	"slices"
	"time"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	projectionv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/projection/v1"
	"google.golang.org/protobuf/proto"
)

// countAttachmentUploadLocked records a creation once, independently of later
// attachment, processing, expiry or deletion. Caller holds the projection lock.
func (p *AssetProjection) countAttachmentUploadLocked(event *evtv1.Event) {
	created := event.GetAssetCreated()
	if created.GetRoomId() == "" || created.GetParentAssetId() != "" || !event.GetCreatedAt().IsValid() {
		return
	}
	month := event.GetCreatedAt().AsTime().UTC().Format("2006-01")
	if p.monthlyUploads == nil {
		p.monthlyUploads = make(map[string]*projectionv1.MonthlyAttachmentUploadsSnapshot)
	}
	usage := p.monthlyUploads[month]
	if usage == nil {
		usage = &projectionv1.MonthlyAttachmentUploadsSnapshot{
			Rooms: make(map[string]*projectionv1.UploadTotalsSnapshot),
			Users: make(map[string]*projectionv1.UploadTotalsSnapshot),
		}
		p.monthlyUploads[month] = usage
	}
	userID := cmp.Or(created.GetUserId(), event.GetActorId())
	if event.GetActorId() == SystemActorID {
		userID = ""
	}
	size := created.GetAsset().GetSize()
	countUpload(usage.Rooms, created.GetRoomId(), size)
	countUpload(usage.Users, userID, size)
}

func countUpload(rows map[string]*projectionv1.UploadTotalsSnapshot, id string, size int64) {
	if rows[id] == nil {
		rows[id] = &projectionv1.UploadTotalsSnapshot{}
	}
	rows[id].Bytes += size
	rows[id].Files++
}

// attachmentUploadMonth detaches both breakdowns from the same projection generation.
func (p *AssetProjection) attachmentUploadMonth(month string) *projectionv1.MonthlyAttachmentUploadsSnapshot {
	p.RLock()
	defer p.RUnlock()
	return proto.CloneOf(p.monthlyUploads[month])
}

// AttachmentUploadTotals counts completed files, including files later deleted.
type AttachmentUploadTotals struct{ Bytes, Files int64 }

// AttachmentUploadRow identifies a room or uploader. An empty ID groups
// private and unavailable rooms, or operator uploads in the user breakdown.
type AttachmentUploadRow struct {
	ID, Name string
	AttachmentUploadTotals
}

// AttachmentUploadReport contains the server total and independent breakdowns
// for one calendar month. Names reflect current catalog and profile data.
type AttachmentUploadReport struct {
	Total        AttachmentUploadTotals
	Rooms, Users []AttachmentUploadRow
}

// GetAttachmentUploads returns a server owner's monthly UTC upload report.
func (c *ChattoCore) GetAttachmentUploads(ctx context.Context, actorID, month string) (*AttachmentUploadReport, error) {
	if err := c.requireSystemDiagnostics(ctx, actorID); err != nil {
		return nil, err
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, invalidArgument("month must be in YYYY-MM format")
	}
	return c.assetModel.attachmentUploadReport(ctx, month)
}

func (m *AssetModel) attachmentUploadReport(ctx context.Context, month string) (*AttachmentUploadReport, error) {
	if err := m.waitForAssetsCurrent(ctx); err != nil {
		return nil, err
	}
	usage := m.assets.Projection().attachmentUploadMonth(month)
	report := &AttachmentUploadReport{}
	var privateRooms AttachmentUploadRow
	for id, totals := range usage.GetRooms() {
		report.Total.Bytes += totals.Bytes
		report.Total.Files += totals.Files
		room, ok := m.roomModel.room(id)
		if !ok || KindOfRoom(room) != KindChannel {
			privateRooms.Bytes += totals.Bytes
			privateRooms.Files += totals.Files
			continue
		}
		report.Rooms = append(report.Rooms, uploadRow(id, room.Name, totals))
	}
	if privateRooms.Files > 0 {
		report.Rooms = append(report.Rooms, privateRooms)
	}
	ids := sortedMapKeys(usage.GetUsers())
	users, err := m.GetUserReferences(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i, id := range ids {
		name := users[i].GetDisplayName()
		report.Users = append(report.Users, uploadRow(id, name, usage.Users[id]))
	}
	slices.SortFunc(report.Rooms, compareUploadRows)
	slices.SortFunc(report.Users, compareUploadRows)
	return report, nil
}

func uploadRow(id, name string, totals *projectionv1.UploadTotalsSnapshot) AttachmentUploadRow {
	return AttachmentUploadRow{ID: id, Name: name, AttachmentUploadTotals: AttachmentUploadTotals{Bytes: totals.Bytes, Files: totals.Files}}
}

func compareUploadRows(a, b AttachmentUploadRow) int {
	return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.ID, b.ID))
}
