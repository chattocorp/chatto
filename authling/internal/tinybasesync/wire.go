package tinybasesync

import (
	"bytes"
	"encoding/json"
	"errors"
)

const (
	// MaxWireMessageSize bounds one TinyBase protocol message before parsing.
	MaxWireMessageSize = 288 << 10
)

// DecodeWireMessage decodes Authling's experimental three-item TinyBase wire
// envelope: [requestId, message, body].
func DecodeWireMessage(data []byte) (Envelope, error) {
	if len(data) == 0 || len(data) > MaxWireMessageSize {
		return Envelope{}, errors.New("invalid TinyBase wire message size")
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(data, &parts); err != nil || len(parts) != 3 {
		return Envelope{}, errors.New("invalid TinyBase wire envelope")
	}
	var requestID *string
	if !bytes.Equal(bytes.TrimSpace(parts[0]), []byte("null")) {
		var value string
		if err := json.Unmarshal(parts[0], &value); err != nil || value == "" || len(value) > 128 {
			return Envelope{}, errors.New("invalid TinyBase request ID")
		}
		requestID = &value
	}
	var message int
	if err := json.Unmarshal(parts[1], &message); err != nil || message < MessageResponse || message > MessageGetValueDiff {
		return Envelope{}, errors.New("invalid TinyBase message number")
	}
	if len(parts[2]) == 0 {
		return Envelope{}, errors.New("missing TinyBase message body")
	}
	return Envelope{RequestID: requestID, Message: message, Body: append(json.RawMessage(nil), parts[2]...)}, nil
}

// EncodeWireMessage encodes a peer response without its process-local target.
func EncodeWireMessage(message Outbound) ([]byte, error) {
	requestID := json.RawMessage("null")
	if message.RequestID != nil {
		encoded, err := json.Marshal(*message.RequestID)
		if err != nil {
			return nil, err
		}
		requestID = encoded
	}
	return json.Marshal([]json.RawMessage{requestID, json.RawMessage([]byte{byte('0' + message.Message)}), message.Body})
}
