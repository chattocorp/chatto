package http_server

import (
	"bytes"
	"context"
	"sync"

	"github.com/chattocorp/chatto/cli/internal/assets"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

// imageTransforms bounds the memory that on-demand image transforms use in
// one process. A decoder holds all pixels of an image in memory, so a 40
// megapixel image needs approximately 160 MB. The limits are per process
// because memory is per process.
//
//   - Concurrent requests for the same derivative share one transform.
//   - Transforms in progress hold at most assets.MaxDecodedImagePixels pixels
//     together. Small images run concurrently. A large image waits until the
//     transforms in progress release their pixels.
//
// The zero value is ready for use.
type imageTransforms struct {
	flight     singleflight.Group
	pixelsOnce sync.Once
	pixels     *semaphore.Weighted
}

// transformedImage is the shared result of a transform. Callers must not
// modify Data.
type transformedImage struct {
	Data        []byte
	ContentType string
}

// do returns the transform result for key. If another request already
// transforms key, do waits for that result and does not call read. Otherwise,
// it calls read to get the source bytes, waits until enough pixels are free,
// and calls transform.
//
// The first request does the work with its own context. If that work fails,
// each waiting request whose context is still active does the work again
// with its own read, so a cancelled first request does not fail the others.
func (t *imageTransforms) do(
	ctx context.Context,
	key string,
	read func() ([]byte, error),
	transform func([]byte) (*transformedImage, error),
) (*transformedImage, error) {
	run := func() (*transformedImage, error) {
		data, err := read()
		if err != nil {
			return nil, err
		}
		t.pixelsOnce.Do(func() { t.pixels = semaphore.NewWeighted(assets.MaxDecodedImagePixels) })
		weight := transformPixelWeight(data)
		if err := t.pixels.Acquire(ctx, weight); err != nil {
			return nil, err
		}
		defer t.pixels.Release(weight)
		return transform(data)
	}

	ran := false
	result, err, _ := t.flight.Do(key, func() (any, error) {
		ran = true
		return run()
	})
	if err != nil && !ran && ctx.Err() == nil {
		return run()
	}
	if err != nil {
		return nil, err
	}
	return result.(*transformedImage), nil
}

// transformPixelWeight returns the number of pixels that a decoder can hold
// in memory for data, from 1 to assets.MaxDecodedImagePixels. An animated GIF
// counts all frames. Data that InspectImage rejects gets weight 1, because the
// transform rejects it before it decodes pixels.
func transformPixelWeight(data []byte) int64 {
	info, err := assets.InspectImage(bytes.NewReader(data))
	if err != nil {
		return 1
	}
	pixels := int64(info.Width) * int64(info.Height) * int64(max(info.Frames, 1))
	return min(max(pixels, 1), assets.MaxDecodedImagePixels)
}
