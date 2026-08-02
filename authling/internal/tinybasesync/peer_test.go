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
	newer := `[[{"servers":[{"first":[{"name":[{"__authling_tinybase_undefined":true},"0000000000000002"]},"0000000000000002"]},"0000000000000002"]},"0000000000000002"],[{}],1]`
	for _, body := range []string{older, newer, older} {
		if _, err := peer.Handle(t.Context(), Envelope{ClientID: "device", Message: MessageContentDiff, Body: json.RawMessage(body)}); err != nil {
			t.Fatal(err)
		}
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
