package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	// MaxNeighbors bounds the public directory and its API payloads.
	MaxNeighbors = 100
	// MaxNeighborOriginLength bounds each stored canonical origin.
	MaxNeighborOriginLength    = 2048
	maxNeighborMutationRetries = 5
)

var (
	// ErrNeighborNotFound means that the requested Neighbor does not exist.
	ErrNeighborNotFound = errors.New("Neighbor not found")
	// ErrNeighborAlreadyExists means that the canonical origin is advertised.
	ErrNeighborAlreadyExists = errors.New("Neighbor origin already exists")
	// ErrNeighborRevisionChanged prevents a stale update or delete.
	ErrNeighborRevisionChanged = errors.New("Neighbor was changed by another request")
	// ErrNeighborLimitReached means that the directory is full.
	ErrNeighborLimitReached = errors.New("Neighbor limit reached")
)

// ListNeighbors returns the current Neighbor collection without an ordering
// contract.
func (cm *ConfigModel) ListNeighbors() []Neighbor {
	if cm == nil || cm.config.Projection() == nil {
		return nil
	}
	p := cm.config.Projection()
	p.RLock()
	defer p.RUnlock()
	neighbors := make([]Neighbor, 0, len(p.server.neighbors))
	// The opaque-ID order keeps repeated reads and cache validators stable. It
	// is not a ranking or a public ordering contract.
	for _, neighborID := range sortedMapKeys(p.server.neighbors) {
		neighbors = append(neighbors, p.server.neighbors[neighborID])
	}
	return neighbors
}

// GetNeighbor returns one projected Neighbor.
func (cm *ConfigModel) GetNeighbor(neighborID string) (Neighbor, bool) {
	if cm == nil || cm.config.Projection() == nil {
		return Neighbor{}, false
	}
	p := cm.config.Projection()
	p.RLock()
	defer p.RUnlock()
	neighbor, exists := p.server.neighbors[neighborID]
	return neighbor, exists
}

// ListManagedNeighbors returns the administrative Neighbor collection.
func (c *ChattoCore) ListManagedNeighbors(ctx context.Context, actorID string) ([]Neighbor, error) {
	if err := c.prepareNeighborRead(ctx, actorID); err != nil {
		return nil, err
	}
	return c.ConfigModel().ListNeighbors(), nil
}

// GetManagedNeighbor returns one Neighbor to an authorized administrator.
func (c *ChattoCore) GetManagedNeighbor(ctx context.Context, actorID, neighborID string) (Neighbor, error) {
	if err := c.prepareNeighborRead(ctx, actorID); err != nil {
		return Neighbor{}, err
	}
	neighbor, exists := c.ConfigModel().GetNeighbor(neighborID)
	if !exists {
		return Neighbor{}, ErrNeighborNotFound
	}
	return neighbor, nil
}

// CreateNeighbor adds one canonical server origin to the public directory.
func (c *ChattoCore) CreateNeighbor(ctx context.Context, actorID, rawOrigin string) (Neighbor, error) {
	origin, err := canonicalNeighborOrigin(rawOrigin)
	if err != nil {
		return Neighbor{}, err
	}
	neighborID := NewNeighborID()
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_ServerNeighborCreated{
		ServerNeighborCreated: &evtv1.ServerNeighborCreatedEvent{NeighborId: neighborID, Origin: origin},
	}})
	for attempt := 0; attempt < maxNeighborMutationRetries; attempt++ {
		prepared, err := c.prepareNeighborMutation(ctx, actorID)
		if err != nil {
			return Neighbor{}, err
		}
		neighbors := c.ConfigModel().ListNeighbors()
		if len(neighbors) >= MaxNeighbors {
			return Neighbor{}, ErrNeighborLimitReached
		}
		if neighborOriginExists(neighbors, origin, "") {
			return Neighbor{}, ErrNeighborAlreadyExists
		}
		if err := c.appendNeighborMutation(ctx, event, prepared); err == nil {
			return Neighbor{ID: neighborID, Origin: origin, Revision: event.GetId()}, nil
		} else if !errors.Is(err, events.ErrConflict) {
			return Neighbor{}, err
		}
		if err := waitNeighborRetry(ctx, attempt); err != nil {
			return Neighbor{}, err
		}
	}
	return Neighbor{}, fmt.Errorf("create Neighbor retry exhausted: %w", events.ErrConflict)
}

// UpdateNeighbor changes one Neighbor origin if its revision is current.
func (c *ChattoCore) UpdateNeighbor(ctx context.Context, actorID, neighborID, rawOrigin, revision string) (Neighbor, error) {
	origin, err := canonicalNeighborOrigin(rawOrigin)
	if err != nil {
		return Neighbor{}, err
	}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_ServerNeighborOriginChanged{
		ServerNeighborOriginChanged: &evtv1.ServerNeighborOriginChangedEvent{NeighborId: neighborID, Origin: origin},
	}})
	for attempt := 0; attempt < maxNeighborMutationRetries; attempt++ {
		prepared, err := c.prepareNeighborMutation(ctx, actorID)
		if err != nil {
			return Neighbor{}, err
		}
		current, exists := c.ConfigModel().GetNeighbor(neighborID)
		if !exists {
			return Neighbor{}, ErrNeighborNotFound
		}
		if current.Revision != revision {
			return Neighbor{}, ErrNeighborRevisionChanged
		}
		if neighborOriginExists(c.ConfigModel().ListNeighbors(), origin, neighborID) {
			return Neighbor{}, ErrNeighborAlreadyExists
		}
		if current.Origin == origin {
			return current, nil
		}
		if err := c.appendNeighborMutation(ctx, event, prepared); err == nil {
			return Neighbor{ID: neighborID, Origin: origin, Revision: event.GetId()}, nil
		} else if !errors.Is(err, events.ErrConflict) {
			return Neighbor{}, err
		}
		if err := waitNeighborRetry(ctx, attempt); err != nil {
			return Neighbor{}, err
		}
	}
	return Neighbor{}, fmt.Errorf("update Neighbor retry exhausted: %w", events.ErrConflict)
}

// DeleteNeighbor removes one Neighbor if its revision is current.
func (c *ChattoCore) DeleteNeighbor(ctx context.Context, actorID, neighborID, revision string) error {
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_ServerNeighborDeleted{
		ServerNeighborDeleted: &evtv1.ServerNeighborDeletedEvent{NeighborId: neighborID},
	}})
	for attempt := 0; attempt < maxNeighborMutationRetries; attempt++ {
		prepared, err := c.prepareNeighborMutation(ctx, actorID)
		if err != nil {
			return err
		}
		current, exists := c.ConfigModel().GetNeighbor(neighborID)
		if !exists {
			return ErrNeighborNotFound
		}
		if current.Revision != revision {
			return ErrNeighborRevisionChanged
		}
		if err := c.appendNeighborMutation(ctx, event, prepared); err == nil {
			return nil
		} else if !errors.Is(err, events.ErrConflict) {
			return err
		}
		if err := waitNeighborRetry(ctx, attempt); err != nil {
			return err
		}
	}
	return fmt.Errorf("delete Neighbor retry exhausted: %w", events.ErrConflict)
}

type preparedNeighborMutation struct {
	configPosition   events.StreamPosition
	authorizationSeq uint64
}

func (c *ChattoCore) prepareNeighborRead(ctx context.Context, actorID string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	position, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.ConfigSubjectAggregate(ConfigSubjectServer).AllEventsFilter())
	if err != nil {
		return fmt.Errorf("read Neighbor directory position: %w", err)
	}
	if err := c.ConfigModel().waitFor(ctx, position); err != nil {
		return fmt.Errorf("wait for Neighbor directory: %w", err)
	}
	return c.requireServerPermission(ctx, actorID, PermServerManageNeighbors)
}

func (c *ChattoCore) prepareNeighborMutation(ctx context.Context, actorID string) (preparedNeighborMutation, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return preparedNeighborMutation{}, err
	}
	aggregate := evtstream.ConfigSubjectAggregate(ConfigSubjectServer)
	position, err := c.EventPublisher.LastSubjectPosition(ctx, aggregate.AllEventsFilter())
	if err != nil {
		return preparedNeighborMutation{}, fmt.Errorf("read Neighbor directory OCC position: %w", err)
	}
	if err := c.ConfigModel().waitFor(ctx, position); err != nil {
		return preparedNeighborMutation{}, fmt.Errorf("wait for Neighbor directory: %w", err)
	}
	authorizationSeq, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		return preparedNeighborMutation{}, fmt.Errorf("read authorization fence: %w", err)
	}
	rbacPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return preparedNeighborMutation{}, fmt.Errorf("read RBAC position: %w", err)
	}
	if err := c.rbacModel.waitFor(ctx, rbacPosition); err != nil {
		return preparedNeighborMutation{}, fmt.Errorf("wait for RBAC projection: %w", err)
	}
	if err := c.requireServerPermission(ctx, actorID, PermServerManageNeighbors); err != nil {
		return preparedNeighborMutation{}, err
	}
	return preparedNeighborMutation{configPosition: position, authorizationSeq: authorizationSeq}, nil
}

func (c *ChattoCore) appendNeighborMutation(ctx context.Context, event *evtv1.Event, prepared preparedNeighborMutation) error {
	aggregate := evtstream.ConfigSubjectAggregate(ConfigSubjectServer)
	subject := aggregate.SubjectFor(event)
	seqs, err := c.appendAuthorizationFencedBatch(ctx, event.GetActorId(), []evtstream.BatchEntry{{
		Subject: subject, Event: event, HasOCC: true,
		ExpectedSeq: prepared.configPosition.Seq, FilterSubject: aggregate.AllEventsFilter(),
	}}, prepared.authorizationSeq)
	if err != nil {
		return err
	}
	if err := c.ConfigModel().waitFor(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
		return fmt.Errorf("wait for Neighbor mutation: %w", err)
	}
	return nil
}

func neighborOriginExists(neighbors []Neighbor, origin, exceptID string) bool {
	for _, neighbor := range neighbors {
		if neighbor.ID != exceptID && neighbor.Origin == origin {
			return true
		}
	}
	return false
}

func waitNeighborRetry(ctx context.Context, attempt int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		return nil
	}
}

func canonicalNeighborOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", invalidNeighborOrigin()
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", invalidNeighborOrigin()
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", invalidNeighborOrigin()
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", invalidNeighborOrigin()
	}
	if ip := net.ParseIP(hostname); ip != nil {
		hostname = ip.String()
	} else {
		hostname, err = idna.Lookup.ToASCII(hostname)
		if err != nil || hostname == "" {
			return "", invalidNeighborOrigin()
		}
		hostname = strings.ToLower(hostname)
	}
	port := parsed.Port()
	if port == "" && strings.HasSuffix(parsed.Host, ":") {
		return "", invalidNeighborOrigin()
	}
	if port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", invalidNeighborOrigin()
		}
		if (scheme == "http" && portNumber == 80) || (scheme == "https" && portNumber == 443) {
			port = ""
		} else {
			port = strconv.Itoa(portNumber)
		}
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	origin := scheme + "://" + host
	if len(origin) > MaxNeighborOriginLength {
		return "", invalidNeighborOrigin()
	}
	return origin, nil
}

func invalidNeighborOrigin() error {
	return fmt.Errorf("%w: Neighbor origin must be an HTTP or HTTPS origin without a path, query, fragment, or credentials", ErrInvalidArgument)
}
