package sitesettings

import (
	"errors"
	"strings"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/evtstream"
	corev1 "github.com/chattocorp/chatto/authling/internal/pb/authling/core/v1"
	"google.golang.org/protobuf/proto"
)

func TestProjectionReplayAndAudit(t *testing.T) {
	first, err := newEvent("acc_admin", Snapshot{Site: config.SiteConfig{Name: "First name", Introduction: "Welcome"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := newEvent("acc_other", Snapshot{Revision: 4, Site: config.SiteConfig{Name: "Second name"}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		p := NewProjection()
		for i, event := range []*corev1.Event{first, second} {
			data, err := proto.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := evtstream.Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Apply(decoded.Event, uint64(4+i)); err != nil {
				t.Fatal(err)
			}
		}
		state := p.Get()
		if state.Site.Name != "Second name" || state.ActorAccountID != "acc_other" || state.Revision != 5 || !state.ChangedAt.Equal(second.CreatedAt.AsTime()) {
			t.Fatal("audit state not replayed")
		}
		if err := p.Apply(first, 6); err == nil {
			t.Fatal("stale predecessor accepted")
		}
	}
}

func TestProjectionRejectsMalformedEvents(t *testing.T) {
	valid, err := newEvent("acc_admin", Snapshot{Site: config.SiteConfig{Name: "Valid"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*corev1.Event){
		"actor":       func(e *corev1.Event) { e.GetSiteSettingsChanged().ActorAccountId = "" },
		"id":          func(e *corev1.Event) { e.Id = "" },
		"predecessor": func(e *corev1.Event) { e.GetSiteSettingsChanged().PreviousRevision = 3 },
		"settings":    func(e *corev1.Event) { e.GetSiteSettingsChanged().Settings = nil },
		"text":        func(e *corev1.Event) { e.GetSiteSettingsChanged().Settings.Name = "line\nbreak" },
		"timestamp":   func(e *corev1.Event) { e.CreatedAt = nil },
		"payload":     func(e *corev1.Event) { e.Event = nil },
	} {
		t.Run(name, func(t *testing.T) {
			e := proto.Clone(valid).(*corev1.Event)
			mutate(e)
			p := NewProjection()
			if p.Apply(e, 1) == nil || p.Get().Revision != 0 {
				t.Fatal("invalid event changed state")
			}
		})
	}
}

func TestValidateRejectsUnsafeOrExcessiveText(t *testing.T) {
	for _, site := range []config.SiteConfig{
		{Name: " "}, {Name: "name\n"}, {Name: strings.Repeat("n", 121)},
		{Name: "Valid", Description: strings.Repeat("d", 501)},
		{Name: "Valid", Introduction: strings.Repeat("i", 501)},
		{Name: "Valid", Introduction: "line\nbreak"},
		{Name: "Valid", Description: string([]byte{0xff})},
	} {
		if !errors.Is(Validate(site), ErrInvalid) {
			t.Fatal("invalid text accepted")
		}
	}
	if err := (&Service{}).Save(t.Context(), "acc_admin", Snapshot{Site: config.SiteConfig{Name: "Valid"}}); !errors.Is(err, ErrForbidden) {
		t.Fatal("missing authorization allowed a save")
	}
}
