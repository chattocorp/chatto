package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type attachmentObjectPut func(context.Context, jetstream.ObjectMeta, io.Reader) (*jetstream.ObjectInfo, error)

// putAttachmentObject retries only explicit JetStream stream-ingest backpressure
// (429/10167). Put attempts to purge failed partial chunks before returning. Rewinding and
// reusing the object name is safe here, before upload offsets or asset facts are
// committed. Do not retry whole upload RPCs: their offsets can already advance.
// Non-seekable inputs get one attempt; never buffer an attachment to retry it.
func putAttachmentObject(ctx context.Context, put attachmentObjectPut, meta jetstream.ObjectMeta, reader io.Reader) (*jetstream.ObjectInfo, error) {
	seeker, seekable := reader.(io.Seeker)
	var offset int64
	if seekable {
		var err error
		offset, err = seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, fmt.Errorf("read attachment offset: %w", err)
		}
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			timer := time.NewTimer(250 * time.Millisecond * time.Duration(1<<(attempt-1)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			if _, err := seeker.Seek(offset, io.SeekStart); err != nil {
				return nil, fmt.Errorf("rewind attachment after backpressure: %w", err)
			}
		}
		info, err := put(ctx, meta, reader)
		apiErr, isAPIError := errors.AsType[*jetstream.APIError](err)
		if err == nil || !seekable || !isAPIError || apiErr.Code != 429 || apiErr.ErrorCode != 10167 || attempt == 4 {
			return info, err
		}
	}
}
