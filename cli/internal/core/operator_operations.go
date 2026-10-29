package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/google/uuid"
)

// operatorOperation fences one root-equivalent operation across replicas. A
// receipt commits with its facts; it is not an import session or archive cursor.
type operatorOperation struct {
	core                           *ChattoCore
	subject, fingerprint, resource string
}

func (c *ChattoCore) operatorOperation(ctx context.Context, key, method string, input any) (*operatorOperation, error) {
	id, err := uuid.Parse(key)
	if err != nil || len(key) != 36 || id == uuid.Nil {
		return nil, invalidArgument("operation_key must be a non-zero UUID")
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	o := &operatorOperation{core: c, subject: evtstream.Aggregate{Type: "operator_operation", ID: id.String()}.Subject("operator_operation_completed"),
		fingerprint: c.runtimeTokenHash("operator_operation_v1", method+"\x00"+string(encoded))}
	return o, o.load(ctx)
}

func (o *operatorOperation) load(ctx context.Context) error {
	facts, _, err := o.core.EventPublisher.SubjectEvents(ctx, o.subject)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return nil
	}
	if len(facts) != 1 || facts[0].GetOperatorOperationCompleted().GetFingerprint() != o.fingerprint {
		return invalidArgument("operation_key already belongs to different arguments")
	}
	o.resource = facts[0].GetOperatorOperationCompleted().GetResourceId()
	if o.resource == "" {
		return fmt.Errorf("operator receipt has no result")
	}
	return nil
}

func (o *operatorOperation) receipt(id string) evtstream.BatchEntry {
	e := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_OperatorOperationCompleted{OperatorOperationCompleted: &evtv1.OperatorOperationCompletedEvent{Fingerprint: o.fingerprint, ResourceId: id}}})
	return evtstream.BatchEntry{Subject: o.subject, Event: e, Expect: events.ExpectSubjectSeq(0)}
}

// CreateHistoricalOperatorUser creates one active human account with permanent
// retry protection. Email ownership remains enforced by ordinary account creation.
func (c *ChattoCore) CreateHistoricalOperatorUser(ctx context.Context, key, login, display, email string) (*readmodel.User, error) {
	o, err := c.operatorOperation(ctx, key, "CreateUser", []string{login, display, email})
	if err != nil {
		return nil, err
	}
	if o.resource != "" {
		return c.GetUser(ctx, o.resource)
	}
	user, err := c.createUserWithOptions(ctx, SystemActorID, login, display, "", userCreationOptions{
		importEmails: nonemptyOperatorEmail(email), importReceipt: o.receipt, importContext: &evtv1.ImportContext{HiddenTimeline: true, UnknownTime: true},
	})
	if err != nil {
		if loadErr := o.load(ctx); loadErr != nil {
			return nil, loadErr
		}
		if o.resource != "" {
			return c.GetUser(ctx, o.resource)
		}
	}
	return user, err
}

// CreateHistoricalOperatorRoom creates one channel, independently of archives.
func (c *ChattoCore) CreateHistoricalOperatorRoom(ctx context.Context, key, group, name, description string) (*readmodel.Room, error) {
	o, err := c.operatorOperation(ctx, key, "CreateRoom", []string{group, name, description})
	if err != nil {
		return nil, err
	}
	if o.resource != "" {
		return c.GetRoom(ctx, KindChannel, o.resource)
	}
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, group, name, description, func(options *createRoomOptions) {
		options.importReceipt = o.receipt
		options.importContext = &evtv1.ImportContext{HiddenTimeline: true, UnknownTime: true}
	})
	if err != nil {
		if loadErr := o.load(ctx); loadErr != nil {
			return nil, loadErr
		}
		if o.resource != "" {
			return c.GetRoom(ctx, KindChannel, o.resource)
		}
	}
	return room, err
}

func appendOperatorReceipt(entries []evtstream.BatchEntry, claim *messagePostClaim) []evtstream.BatchEntry {
	if claim != nil && claim.operation != nil {
		return append(entries, claim.operation.receipt(claim.messageID))
	}
	return entries
}

func nonemptyOperatorEmail(email string) []string {
	if email == "" {
		return nil
	}
	return []string{email}
}
