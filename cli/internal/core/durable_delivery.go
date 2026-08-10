package core

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

func decodeDurableCoreDelivery(delivery events.DurableDelivery) (*corev1.Event, error) {
	var event corev1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		return nil, events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	if event.GetEvent() == nil {
		return nil, events.TerminateDelivery("empty Chatto event envelope", fmt.Errorf("missing event payload"))
	}
	return &event, nil
}
