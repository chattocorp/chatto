package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// DirectorySidebarLink contains one link and its owner from the same snapshot.
type DirectorySidebarLink struct {
	Link    *readmodel.SidebarLink
	GroupID string
}

// GetSidebarLink uses the same authenticated visibility as links in room groups.
func (s *RoomDirectoryReadModel) GetSidebarLink(ctx context.Context, actorID, linkID string) (*DirectorySidebarLink, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if linkID == "" {
		return nil, ErrInvalidArgument
	}
	if err := s.rooms.groupLayout.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	return s.sidebarLink(linkID)
}

// BatchGetSidebarLinks keeps first-seen order and omits unknown/deleted links.
// Each link and owner pair is consistent; the batch is a live read.
func (s *RoomDirectoryReadModel) BatchGetSidebarLinks(ctx context.Context, actorID string, linkIDs []string) ([]*DirectorySidebarLink, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.rooms.groupLayout.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(linkIDs))
	links := make([]*DirectorySidebarLink, 0, len(linkIDs))
	for _, id := range linkIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		link, err := s.sidebarLink(id)
		if err == ErrSidebarLinkNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

func (s *RoomDirectoryReadModel) sidebarLink(linkID string) (*DirectorySidebarLink, error) {
	snapshot := s.rooms.sidebarLinkMoveSnapshot(linkID, "")
	if snapshot.Link == nil {
		return nil, ErrSidebarLinkNotFound
	}
	return &DirectorySidebarLink{Link: snapshot.Link, GroupID: snapshot.SourceGroupID}, nil
}
