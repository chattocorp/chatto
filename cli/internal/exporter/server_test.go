package exporter

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUniqueListedKeysDeduplicatesStreamingKeys(t *testing.T) {
	lister := newFakeKeyLister("presence.U1", "presence.U2", "presence.U1", "presence.U3", "presence.U2")

	keys, err := uniqueListedKeys(lister)
	require.NoError(t, err)
	require.Equal(t, []string{"presence.U1", "presence.U2", "presence.U3"}, keys)
	require.True(t, lister.stopped)
}

func TestUniqueListedKeysReturnsListerError(t *testing.T) {
	wantErr := errors.New("stop failed")
	lister := newFakeKeyLister("presence.U1")
	lister.err = wantErr

	_, err := uniqueListedKeys(lister)
	require.ErrorIs(t, err, wantErr)
}

type fakeKeyLister struct {
	keys    chan string
	err     error
	stopped bool
}

func newFakeKeyLister(keys ...string) *fakeKeyLister {
	keyCh := make(chan string, len(keys))
	for _, key := range keys {
		keyCh <- key
	}
	close(keyCh)

	return &fakeKeyLister{
		keys: keyCh,
	}
}

func (l *fakeKeyLister) Keys() <-chan string {
	return l.keys
}

func (l *fakeKeyLister) Stop() error {
	l.stopped = true
	return l.err
}
