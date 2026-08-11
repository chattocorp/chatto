package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	userKeyShreddingConsumerName       = "chatto-user-key-shredding-v1"
	userKeyShreddingConsumerAckWait    = 2 * time.Minute
	userKeyShreddingDeliveryHeartbeat  = 30 * time.Second
	userKeyShreddingRetryDelay         = 30 * time.Second
	userKeyShreddingAcknowledgeTimeout = 5 * time.Second
	userKeyShreddingMaxPending         = 16
)

var errUserKeyShreddingFactExists = errors.New("user key shredding fact already exists")

// UserKeyShreddingModel owns the request-before-destruction protocol. The
// command path attempts work synchronously; the shared durable consumer
// recovers requests after crashes and safely redelivers partial attempts.
type UserKeyShreddingModel struct {
	core               *ChattoCore
	worker             *events.DurableWorker
	appendRequestAtFn  func(context.Context, string, *corev1.Event, string, uint64) (uint64, error)
	appendOnceFn       func(context.Context, string, *corev1.Event, string) (uint64, error)
	shredContentKeyFn  func(context.Context, string) error
	shredWrappingKeyFn func(context.Context, string) error
}

func newUserKeyShreddingModel(ctx context.Context, core *ChattoCore, logger *log.Logger) (*UserKeyShreddingModel, error) {
	consumer, err := core.storage.serverEvtStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:            userKeyShreddingConsumerName,
		Durable:         userKeyShreddingConsumerName,
		Description:     "Shared durable queue for Chatto user-key shredding",
		DeliverPolicy:   jetstream.DeliverAllPolicy,
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         userKeyShreddingConsumerAckWait,
		MaxDeliver:      -1,
		FilterSubject:   evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   userKeyShreddingMaxPending,
		MaxRequestBatch: userKeyShreddingMaxPending,
	})
	if err != nil {
		return nil, fmt.Errorf("create user-key shredding consumer: %w", err)
	}
	m := &UserKeyShreddingModel{
		core:               core,
		appendRequestAtFn:  core.EventPublisher.AppendAtFilter,
		shredContentKeyFn:  core.encryption.contentKeys.Shred,
		shredWrappingKeyFn: core.encryption.keyWrapper.ShredKey,
	}
	m.appendOnceFn = m.appendOnce
	m.worker, err = events.NewDurableWorker(consumer, m.processDelivery, events.DurableWorkerOptions{
		MaxConcurrent:     userKeyShreddingMaxPending,
		FetchMaxWait:      time.Second,
		RetryDelay:        userKeyShreddingRetryDelay,
		AckTimeout:        userKeyShreddingAcknowledgeTimeout,
		HeartbeatInterval: userKeyShreddingDeliveryHeartbeat,
		Logger:            logger,
	})
	if err != nil {
		return nil, fmt.Errorf("configure user-key shredding worker: %w", err)
	}
	return m, nil
}

func (m *UserKeyShreddingModel) Run(ctx context.Context) error {
	return m.worker.Run(ctx)
}

func (m *UserKeyShreddingModel) Request(ctx context.Context, actorID, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is empty")
	}
	completionSubject := evtstream.UserAggregate(userID).Subject(evtstream.EventUserKeyShredded)
	completedSeq, err := m.core.EventPublisher.LastSubjectSeq(ctx, completionSubject)
	if err != nil {
		return err
	}
	if completedSeq > 0 {
		return m.waitForPrivacyBoundary(ctx, events.SubjectPosition(completionSubject, completedSeq))
	}
	requestEvent, seq, found, err := m.requestFact(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		requestEvent, seq, err = m.appendRequest(ctx, actorID, userID)
		if errors.Is(err, errUserKeyShreddingFactExists) {
			requestEvent, seq, found, err = m.requestFact(ctx, userID)
			if err == nil && !found {
				err = fmt.Errorf("user key shredding request disappeared")
			}
		}
		if err != nil {
			return fmt.Errorf("record user key shredding request: %w", err)
		}
	}
	if err := m.complete(ctx, requestEvent, evtstream.UserAggregate(userID).Subject(evtstream.EventUserKeyShreddingRequested), seq); err != nil {
		return fmt.Errorf("complete user key shredding: %w", err)
	}
	return nil
}

func (m *UserKeyShreddingModel) appendRequest(ctx context.Context, actorID, userID string) (*corev1.Event, uint64, error) {
	aggregateFilter := evtstream.UserAggregate(userID).AllEventsFilter()
	subject := evtstream.UserAggregate(userID).Subject(evtstream.EventUserKeyShreddingRequested)
	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		aggregateSeq, err := m.core.EventPublisher.LastSubjectSeq(ctx, aggregateFilter)
		if err != nil {
			return nil, 0, fmt.Errorf("read user OCC filter seq: %w", err)
		}
		if exists, err := m.factExists(ctx, userID, evtstream.EventUserKeyShreddingRequested); err != nil {
			return nil, 0, err
		} else if exists {
			return nil, 0, errUserKeyShreddingFactExists
		}
		// Preflight recovery before making the fail-closed privacy boundary
		// durable. The worker repeats this lookup after publication and on every
		// retry rather than relying on coordinates copied into the request.
		if _, _, err := m.shreddingTargets(ctx, userID); err != nil {
			return nil, 0, err
		}
		requestEvent := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserKeyShreddingRequested{
			UserKeyShreddingRequested: &corev1.UserKeyShreddingRequestedEvent{UserId: userID},
		}})
		seq, err := m.appendRequestAtFn(ctx, subject, requestEvent, aggregateFilter, aggregateSeq)
		if err == nil {
			return requestEvent, seq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, 0, err
		}
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, 0, fmt.Errorf("user key shredding OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

// shreddingTargets reconstructs the deletion set from immutable per-user key
// facts. It also inspects surviving DEK records because their wrapping-key ref
// may have changed since the corresponding EVT fact was written.
func (m *UserKeyShreddingModel) shreddingTargets(ctx context.Context, userID string) ([]string, []string, error) {
	dekEvents, _, err := m.core.EventPublisher.SubjectEvents(ctx,
		evtstream.UserAggregate(userID).Subject(evtstream.EventUserDEKGenerated))
	if err != nil {
		return nil, nil, fmt.Errorf("load user DEK facts before shredding: %w", err)
	}
	if m.core.encryption.contentKeys == nil {
		return nil, nil, fmt.Errorf("content key store is not configured")
	}
	contentSet := make(map[string]struct{}, len(dekEvents))
	wrappingSet := map[string]struct{}{kms.LegacyUserKeyRef(userID): {}}
	for _, event := range dekEvents {
		dek := event.GetUserDekGenerated()
		if dek == nil || dek.GetUserId() != userID {
			return nil, nil, fmt.Errorf("invalid user DEK fact for %s", userID)
		}
		ref := dek.GetContentKeyRef()
		if err := dekstore.ValidateRef(ref); err != nil {
			return nil, nil, err
		}
		contentSet[ref] = struct{}{}
		wrappingRef := dek.GetWrappingKeyRef()
		if wrappingRef == "" {
			wrappingRef = kms.LegacyUserKeyRef(userID)
		}
		if err := kms.ValidateKeyRef(wrappingRef); err != nil {
			return nil, nil, err
		}
		wrappingSet[wrappingRef] = struct{}{}

		stored, err := m.core.encryption.contentKeys.Get(ctx, ref)
		if errors.Is(err, encryption.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("load DEK %s before shredding: %w", ref, err)
		}
		if err := kms.ValidateKeyRef(stored.GetWrappingKeyRef()); err != nil {
			return nil, nil, err
		}
		wrappingSet[stored.GetWrappingKeyRef()] = struct{}{}
	}
	return sortedSet(contentSet), sortedSet(wrappingSet), nil
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (m *UserKeyShreddingModel) processDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var event corev1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		return events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	request := event.GetUserKeyShreddingRequested()
	userID, ok := evtstream.ParseUserSubject(delivery.Subject)
	if !ok || request == nil || request.GetUserId() == "" || userID != request.GetUserId() {
		return events.TerminateDelivery("invalid user-key shredding request", errors.New("request subject and payload do not match"))
	}
	if err := validateUserKeyShreddingRequest(request); err != nil {
		return events.TerminateDelivery("invalid user-key shredding request payload", err)
	}
	return m.complete(ctx, &event, delivery.Subject, delivery.StreamSequence)
}

func validateUserKeyShreddingRequest(request *corev1.UserKeyShreddingRequestedEvent) error {
	if request == nil || request.GetUserId() == "" {
		return fmt.Errorf("missing user id")
	}
	return nil
}

func (m *UserKeyShreddingModel) complete(ctx context.Context, requestEvent *corev1.Event, subject string, seq uint64) error {
	request := requestEvent.GetUserKeyShreddingRequested()
	if request == nil {
		return fmt.Errorf("missing user-key shredding request")
	}
	if err := m.waitForPrivacyBoundary(ctx, events.SubjectPosition(subject, seq)); err != nil {
		return err
	}
	completionSubject := evtstream.UserAggregate(request.GetUserId()).Subject(evtstream.EventUserKeyShredded)
	completedSeq, err := m.core.EventPublisher.LastSubjectSeq(ctx, completionSubject)
	if err != nil {
		return err
	}
	if completedSeq > 0 {
		return m.waitForPrivacyBoundary(ctx, events.SubjectPosition(completionSubject, completedSeq))
	}
	contentRefs, wrappingRefs, err := m.shreddingTargets(ctx, request.GetUserId())
	if err != nil {
		return err
	}
	// Delete every KEK before any DEK record. Until the KEK phase succeeds,
	// all surviving DEKs remain available to rediscover newer wrapping refs on
	// redelivery. Once DEK deletion starts, every discovered KEK is already
	// irreversibly gone.
	for _, ref := range wrappingRefs {
		if err := m.shredWrappingKeyFn(ctx, ref); err != nil {
			return fmt.Errorf("shred wrapping key %s: %w", ref, err)
		}
	}
	for _, ref := range contentRefs {
		if err := m.shredContentKeyFn(ctx, ref); err != nil {
			return fmt.Errorf("shred content key %s: %w", ref, err)
		}
	}
	forgetDEKRequestCacheUser(ctx, request.GetUserId())
	actorID := requestEvent.GetActorId()
	if actorID == "" {
		actorID = request.GetUserId()
	}
	completedEvent := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserKeyShredded{
		UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: request.GetUserId()},
	}})
	completedSeq, err = m.appendOnceFn(ctx, request.GetUserId(), completedEvent, evtstream.EventUserKeyShredded)
	if errors.Is(err, errUserKeyShreddingFactExists) {
		completedSeq, err = m.core.EventPublisher.LastSubjectSeq(ctx, completionSubject)
		if err != nil {
			return err
		}
		if completedSeq == 0 {
			return fmt.Errorf("user key shredding completion disappeared")
		}
		return m.waitForPrivacyBoundary(ctx, events.SubjectPosition(completionSubject, completedSeq))
	}
	if err != nil {
		return fmt.Errorf("record user key shredding completion: %w", err)
	}
	return m.waitForPrivacyBoundary(ctx, events.SubjectPosition(completionSubject, completedSeq))
}

func (m *UserKeyShreddingModel) waitForPrivacyBoundary(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos,
		waitForProjection("users", m.core.userModel.users.Projector()),
		waitForProjection("user auth", m.core.userModel.auth.Projector()),
		waitForProjection("content key", m.core.userModel.contentKeys.Projector()),
		waitForProjection("mentionables", m.core.mentionables.mentionables.Projector()),
		waitForProjection("room timeline", m.core.roomModel.timeline.Projector()),
		waitForProjection("threads", m.core.roomModel.threads.Projector()),
	)
}

func (m *UserKeyShreddingModel) appendOnce(ctx context.Context, userID string, event *corev1.Event, eventType string) (uint64, error) {
	return m.core.appendUserEvent(ctx, userID, event, "", func() error {
		exists, err := m.factExists(ctx, userID, eventType)
		if err != nil {
			return err
		}
		if exists {
			return errUserKeyShreddingFactExists
		}
		return nil
	})
}

func (m *UserKeyShreddingModel) factExists(ctx context.Context, userID, eventType string) (bool, error) {
	seq, err := m.core.EventPublisher.LastSubjectSeq(ctx, evtstream.UserAggregate(userID).Subject(eventType))
	return seq > 0, err
}

func (m *UserKeyShreddingModel) requestFact(ctx context.Context, userID string) (*corev1.Event, uint64, bool, error) {
	subject := evtstream.UserAggregate(userID).Subject(evtstream.EventUserKeyShreddingRequested)
	eventsOnSubject, seq, err := m.core.EventPublisher.SubjectEvents(ctx, subject)
	if err != nil {
		return nil, 0, false, err
	}
	if len(eventsOnSubject) == 0 {
		return nil, 0, false, nil
	}
	requestEvent := eventsOnSubject[len(eventsOnSubject)-1]
	request := requestEvent.GetUserKeyShreddingRequested()
	if err := validateUserKeyShreddingRequest(request); err != nil {
		return nil, 0, false, fmt.Errorf("invalid persisted user-key shredding request: %w", err)
	}
	return proto.Clone(requestEvent).(*corev1.Event), seq, true, nil
}
