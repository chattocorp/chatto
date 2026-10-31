package evtstream

import (
	"testing"

	corev1 "github.com/chattocorp/chatto/authling/internal/pb/authling/core/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAdministratorEventValidation(t *testing.T) {
	for _, id := range []string{"", "email@example.invalid", "acc.*", "acc/other"} {
		e := &corev1.Event{Id: "evt_admin", CreatedAt: timestamppb.Now(), Event: &corev1.Event_AdministratorChanged{AdministratorChanged: &corev1.AdministratorChangedEvent{AccountId: id}}}
		if err := validate(e); err == nil {
			t.Fatal("accepted invalid administrator event")
		}
	}
	e := &corev1.Event{Id: "evt_admin", CreatedAt: timestamppb.Now(), Event: &corev1.Event_AdministratorChanged{AdministratorChanged: &corev1.AdministratorChangedEvent{AccountId: "acc_valid"}}}
	if err := validate(e); err != nil {
		t.Fatal(err)
	}
	if EventAccountID(e) != "acc_valid" {
		t.Fatal("event missing from erasure boundary")
	}
}
