package tinybasesync

import (
	"encoding/json"
	"testing"
)

func TestWireMessageRoundTrip(t *testing.T) {
	requestID := "request.1"
	encoded, err := EncodeWireMessage(Outbound{RequestID: &requestID, Message: MessageContentDiff, Body: json.RawMessage(`[[{}],[{}],1]`)})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeWireMessage(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.RequestID == nil || *decoded.RequestID != requestID || decoded.Message != MessageContentDiff || string(decoded.Body) != `[[{}],[{}],1]` {
		t.Fatalf("decoded wire message = %+v", decoded)
	}
}

func TestWireMessageRejectsMalformedAndOversizeInput(t *testing.T) {
	for _, input := range [][]byte{
		nil,
		[]byte(`{}`),
		[]byte(`[null,8,{}]`),
		[]byte(`["",1,""]`),
		make([]byte, MaxWireMessageSize+1),
	} {
		if _, err := DecodeWireMessage(input); err == nil {
			t.Fatalf("accepted invalid wire input of length %d: %q", len(input), input)
		}
	}
}
