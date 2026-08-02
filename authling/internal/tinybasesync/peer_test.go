package tinybasesync

import (
	"context"
	"encoding/json"
	"testing"
)

type memoryStore struct {
	content  []byte
	revision uint64
}

func (store *memoryStore) Load(context.Context) ([]byte, uint64, error) {
	return store.content, store.revision, nil
}
func (store *memoryStore) Save(_ context.Context, content []byte, expected uint64) (uint64, error) {
	if expected != store.revision {
		return 0, ErrConflict
	}
	store.content = append(store.content[:0], content...)
	store.revision++
	return store.revision, nil
}

func TestPeerPersistsLastWriterWinsStateAndTombstones(t *testing.T) {
	storage := &memoryStore{}
	peer, err := NewPeer(t.Context(), storage)
	if err != nil {
		t.Fatal(err)
	}

	older := `[[{"servers":[{"first":[{"name":["One","0000000000000001"]},"0000000000000001"]},"0000000000000001"]},"0000000000000001"],[{}],1]`
	newer := `[[{"servers":[{"first":[{"name":["\ufffc","0000000000000002"]},"0000000000000002"]},"0000000000000002"]},"0000000000000002"],[{}],1]`
	var finalOutbound []Outbound
	for _, body := range []string{older, newer, older} {
		finalOutbound, err = peer.Handle(t.Context(), Envelope{ClientID: "device", Message: MessageContentDiff, Body: json.RawMessage(body)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(finalOutbound) != 1 || finalOutbound[0].Message != MessageContentHashes {
		t.Fatalf("stale writer notification = %+v, want durable content hashes", finalOutbound)
	}

	restarted, err := NewPeer(t.Context(), storage)
	if err != nil {
		t.Fatal(err)
	}
	body, err := restarted.tableDiff()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(body) {
		t.Fatalf("invalid response: %s", body)
	}
	if string(body) == "" || !containsJSON(body, UndefinedJSON) {
		t.Fatalf("tombstone was not retained: %s", body)
	}
}

func TestPeerPersistsParentClockChangesAndNotifiesEveryClient(t *testing.T) {
	storage := &memoryStore{}
	peer, err := NewPeer(t.Context(), storage)
	if err != nil {
		t.Fatal(err)
	}
	for _, clientID := range []string{"device-a", "device-b"} {
		if _, err := peer.Handle(t.Context(), Envelope{ClientID: clientID, Message: MessageGetContentHashes, Body: json.RawMessage(`""`)}); err != nil {
			t.Fatal(err)
		}
	}
	body := json.RawMessage(`[[{"table":[{"row":[{},"0000000000000002"]}]}],[{}],1]`)
	outbound, err := peer.Handle(t.Context(), Envelope{ClientID: "device-a", Message: MessageContentDiff, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if storage.revision != 1 {
		t.Fatalf("parent-only change revision = %d, want 1", storage.revision)
	}
	counts := map[string]int{}
	for _, message := range outbound {
		if message.Message == MessageContentHashes {
			counts[message.ClientID]++
		}
	}
	if counts["device-a"] != 1 || counts["device-b"] != 1 {
		t.Fatalf("hash notifications = %v, want both clients", counts)
	}
	restarted, err := NewPeer(t.Context(), storage)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.state.Tables["table"].Rows["row"].HLC != "0000000000000002" {
		t.Fatal("parent HLC was not retained")
	}
}

func TestPeerValidatesDurableStateVersionAndContents(t *testing.T) {
	invalidVersion := newState()
	invalidVersion.FormatVersion++
	encodedVersion, _ := json.Marshal(invalidVersion)
	if _, err := NewPeer(t.Context(), &memoryStore{content: encodedVersion, revision: 1}); err == nil {
		t.Fatal("unsupported durable state version was accepted")
	}

	invalidClock := newState()
	invalidClock.Values["bad"] = leaf{Value: json.RawMessage(`true`), HLC: "invalid"}
	encodedClock, _ := json.Marshal(invalidClock)
	if _, err := NewPeer(t.Context(), &memoryStore{content: encodedClock, revision: 1}); err == nil {
		t.Fatal("invalid durable state clock was accepted")
	}
}

func TestLegacyUndefinedMarkerIsOrdinaryJSON(t *testing.T) {
	peer, err := NewPeer(t.Context(), &memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	body := json.RawMessage(`[[{}],[{"value":[{"__authling_tinybase_undefined":true},"0000000000000001"]}],1]`)
	if _, err := peer.Handle(t.Context(), Envelope{ClientID: "device", Message: MessageContentDiff, Body: body}); err != nil {
		t.Fatal(err)
	}
	if isUndefined(peer.state.Values["value"].Value) {
		t.Fatal("ordinary JSON object was mistaken for undefined")
	}
}

func TestContentHashesMatchTinyBaseNinePointThree(t *testing.T) {
	peer, err := NewPeer(t.Context(), &memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	body := `[[{"servers":[{"one":[{"name":["First server","NjEtLV-----OVUT0"],"url":["https://one.example","NjEtLV----0OVUT0"]}]}]}],[{"theme":["light","NjEtLV----1OVUT0"]}],1]`
	if _, err := peer.Handle(t.Context(), Envelope{ClientID: "device-a", Message: MessageContentDiff, Body: json.RawMessage(body)}); err != nil {
		t.Fatal(err)
	}
	if got, want := string(peer.contentHashes()), "[2190076735,3515047040]"; got != want {
		t.Fatalf("content hashes = %s, want TinyBase 9.3 fixture %s", got, want)
	}
	objectPeer, err := NewPeer(t.Context(), &memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	objectBody := `[[{}],[{"preferences":["\ufffd{\"nested\":{\"enabled\":true}}","0000000000000001"]}],1]`
	if _, err := objectPeer.Handle(t.Context(), Envelope{ClientID: "device-a", Message: MessageContentDiff, Body: json.RawMessage(objectBody)}); err != nil {
		t.Fatal(err)
	}
	if got, want := valuesHash(objectPeer.state.Values, objectPeer.state.ValuesHLC), uint32(3618592637); got != want {
		t.Fatalf("JSON value hash = %d, want TinyBase 9.3 fixture %d", got, want)
	}
}

func TestPeerRejectsFutureClocksAndPendingRequestFloods(t *testing.T) {
	storage := &memoryStore{}
	peer, err := NewPeer(t.Context(), storage)
	if err != nil {
		t.Fatal(err)
	}
	future := json.RawMessage(`[[{"table":[{"row":[{"cell":[true,"zzzzzzzzzzzzzzzz"]}]}]}],[{}],1]`)
	if _, err := peer.Handle(t.Context(), Envelope{ClientID: "device", Message: MessageContentDiff, Body: future}); err == nil {
		t.Fatal("future HLC was accepted")
	}
	if storage.revision != 0 {
		t.Fatalf("invalid message changed durable revision to %d", storage.revision)
	}
	for attempt := 0; attempt < 32; attempt++ {
		if _, err := peer.Handle(t.Context(), Envelope{ClientID: "device", Message: MessageContentHashes, Body: json.RawMessage(`[1,1]`)}); err != nil {
			t.Fatalf("pending request %d: %v", attempt, err)
		}
	}
	if _, err := peer.Handle(t.Context(), Envelope{ClientID: "device", Message: MessageContentHashes, Body: json.RawMessage(`[1,1]`)}); err == nil {
		t.Fatal("pending request flood was accepted")
	}
}

func containsJSON(haystack, needle []byte) bool {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if string(haystack[index:index+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
