package model

import corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"

type MessagePostedEvent struct {
	Envelope *corev1.Event
	Payload  *corev1.MessagePostedEvent
	RoomID   string
}

func (*MessagePostedEvent) IsEventType() {}
