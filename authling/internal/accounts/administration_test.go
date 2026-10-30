package accounts

import (
	"context"
	"errors"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/evtstream"
	corev1 "github.com/chattocorp/chatto/authling/internal/pb/authling/core/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type administratorConflict struct {
	*evtstream.Publisher
	conflicted bool
}

func (p *administratorConflict) AppendAdministratorChanged(ctx context.Context, event *corev1.Event, tail uint64) (events.StreamPosition, error) {
	if !p.conflicted {
		p.conflicted = true
		other := &corev1.Event{Id: "evt_concurrent", CreatedAt: timestamppb.Now(), Event: &corev1.Event_AdministratorChanged{AdministratorChanged: &corev1.AdministratorChangedEvent{AccountId: event.GetAdministratorChanged().GetAccountId()}}}
		if _, err := p.Publisher.AppendAdministratorChanged(ctx, other, tail); err != nil {
			return events.StreamPosition{}, err
		}
		return events.StreamPosition{}, events.ErrConflict
	}
	return p.Publisher.AppendAdministratorChanged(ctx, event, tail)
}

func TestAdministratorConflictAndUnavailableState(t *testing.T) {
	f := newSafetyFixture(t, t.TempDir(), nil)
	defer f.stop()
	a, err := f.service.CreateLocal(t.Context(), "admin@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	publisher := &administratorConflict{Publisher: f.publisher}
	f.service.publisher = publisher
	if err := f.service.SetAdministrator(t.Context(), a.ID, true); err != nil {
		t.Fatal(err)
	}
	if !publisher.conflicted {
		t.Fatal("conflict not exercised")
	}
	access, err := f.service.Administrator(t.Context(), a.ID, nil)
	if err != nil || !access.Stored {
		t.Fatalf("administrator after conflict: %v", err)
	}
	// A stored flag must never authorize through an unavailable durable boundary.
	f.service.publisher = unavailableAdministratorState{f.publisher}
	access, err = f.service.Administrator(t.Context(), a.ID, []string{"admin@example.invalid"})
	if err == nil || access.Allowed() {
		t.Fatal("unavailable state granted access")
	}
}

type unavailableAdministratorState struct{ *evtstream.Publisher }

func (p unavailableAdministratorState) AccountTail(context.Context, string) (uint64, error) {
	return 0, errors.New("injected storage outage")
}

func TestAdministratorProjectionRejectsAbsentAccount(t *testing.T) {
	p := NewProjection(nil, nil)
	e := &corev1.Event{Id: "evt_admin", CreatedAt: timestamppb.Now(), Event: &corev1.Event_AdministratorChanged{AdministratorChanged: &corev1.AdministratorChangedEvent{AccountId: "acc_missing", IsAdministrator: true}}}
	if err := p.Apply(e, 1); err == nil {
		t.Fatal("accepted administrator for absent account")
	}
	if p.Count() != 0 {
		t.Fatal("invalid event changed accounts")
	}
}
