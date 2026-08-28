package core

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

func decodeDurableCoreDelivery(delivery events.DurableDelivery) (*evtv1.Event, error) {
	var event evtv1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		return nil, events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	if event.GetEvent() == nil {
		return nil, events.TerminateDelivery("empty Chatto event envelope", fmt.Errorf("missing event payload"))
	}
	return &event, nil
}
