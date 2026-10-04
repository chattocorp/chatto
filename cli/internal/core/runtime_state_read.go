package core

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// readRuntimeStateLastMsg reads the latest RUNTIME_STATE message for key
// through the stream leader. key can be a wildcard filter.
//
// jetstream.KeyValue.Get and the newer Stream helpers use DirectGet when the
// stream allows it. Any replica can answer a direct get, and a follower can
// lag behind a write that already committed. A read that must observe a
// preceding write, or that decides an OCC update or a fencing check, must use
// this leader-routed management read instead.
func readRuntimeStateLastMsg(ctx context.Context, js jetstream.JetStream, key string) (*nats.RawStreamMsg, error) {
	opts := js.Options()
	options := []nats.JSOpt{nats.MaxWait(opts.DefaultTimeout)}
	if opts.Domain != "" {
		options = append(options, nats.Domain(opts.Domain))
	} else if opts.APIPrefix != "" {
		options = append(options, nats.APIPrefix(opts.APIPrefix))
	}
	reader, err := js.Conn().JetStream(options...)
	if err != nil {
		return nil, err
	}
	return reader.GetLastMsg("KV_RUNTIME_STATE", "$KV.RUNTIME_STATE."+key, nats.Context(ctx))
}

// isRuntimeStateTombstone reports whether msg removes its key: a KV delete or
// purge, or a server marker for a key that its TTL or a purge removed.
func isRuntimeStateTombstone(msg *nats.RawStreamMsg) bool {
	return msg.Header.Get("KV-Operation") != "" || msg.Header.Get(jetstream.MarkerReasonHeader) != ""
}

// getRuntimeStateLatest is a leader-routed jetstream.KeyValue.Get for one
// RUNTIME_STATE key. Like Get, it returns jetstream.ErrKeyNotFound for a
// missing or removed key. See readRuntimeStateLastMsg for when to use it.
func (c *ChattoCore) getRuntimeStateLatest(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	msg, err := readRuntimeStateLastMsg(ctx, c.js, key)
	if errors.Is(err, nats.ErrMsgNotFound) {
		return nil, jetstream.ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	if isRuntimeStateTombstone(msg) {
		return nil, jetstream.ErrKeyNotFound
	}
	return runtimeStateEntry{key: key, msg: msg}, nil
}

// getRuntimeStateConfirmingAbsence reads one RUNTIME_STATE key on a hot path.
// It accepts the fast DirectGet result when the key exists, but confirms a
// miss through the stream leader: a lagging replica must not reject a record
// that was just created. Use it only where an older revision of an existing
// record cannot lead to a wrong decision, or where the caller confirms that
// itself (see loadRenewableSessionAtLeast).
func (c *ChattoCore) getRuntimeStateConfirmingAbsence(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return c.getRuntimeStateLatest(ctx, key)
	}
	return entry, err
}

// runtimeStateEntry adapts a leader-routed stream message to the
// jetstream.KeyValueEntry interface. It describes a put; tombstones are
// reported as jetstream.ErrKeyNotFound before an entry is built.
type runtimeStateEntry struct {
	key string
	msg *nats.RawStreamMsg
}

func (e runtimeStateEntry) Bucket() string                  { return "RUNTIME_STATE" }
func (e runtimeStateEntry) Key() string                     { return e.key }
func (e runtimeStateEntry) Value() []byte                   { return e.msg.Data }
func (e runtimeStateEntry) Revision() uint64                { return e.msg.Sequence }
func (e runtimeStateEntry) Created() time.Time              { return e.msg.Time }
func (e runtimeStateEntry) Delta() uint64                   { return 0 }
func (e runtimeStateEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }
