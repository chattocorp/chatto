package http_server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
//   - Concurrent requests for the same derivative share one transform. Only
//     one of them reads the source.
//   - Transforms in progress hold at most assets.MaxDecodedImagePixels pixels
//     together. Small images run concurrently. A large image waits until the
//     transforms in progress release their pixels, and transforms that start
//     to wait later wait behind it.
//
// A transform reads its source before it waits for pixels, so a waiting
// transform holds its source bytes. An animated GIF counts as at most
// assets.MaxDecodedImagePixels pixels, although its frames together can have
// more.
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

var (
	// errTransformSourceNotFound marks a failure to open the source asset.
	errTransformSourceNotFound = errors.New("transform source not found")
	// errTransformSourceNotImage marks a source asset that is not an image.
	errTransformSourceNotImage = errors.New("transform source is not an image")
	// errTransformRequestEnded marks a failure after the context of the
	// request that did the work ended.
	errTransformRequestEnded = errors.New("transform request ended")
)

// do returns the transform result for key. If another request already
// transforms key, do waits for that result and does not call read. Otherwise,
// it calls read to get the source bytes, waits until enough pixels are free,
// and calls transform.
//
// The request that does the work uses its own context. If that context ends,
// one of the waiting requests whose context is still active does the work
// again with its own read. Other failures are shared with all waiting
// requests.
func (t *imageTransforms) do(
	ctx context.Context,
	key string,
	read func() ([]byte, error),
	transform func([]byte) (*transformedImage, error),
) (*transformedImage, error) {
	run := func() (any, error) {
		result, err := t.run(ctx, read, transform)
		if err != nil && ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", errTransformRequestEnded, err)
		}
		return result, err
	}
	for {
		result, err, _ := t.flight.Do(key, run)
		if err == nil {
			return result.(*transformedImage), nil
		}
		if !errors.Is(err, errTransformRequestEnded) || ctx.Err() != nil {
			return nil, err
		}
	}
}

func (t *imageTransforms) run(
	ctx context.Context,
	read func() ([]byte, error),
	transform func([]byte) (*transformedImage, error),
) (*transformedImage, error) {
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
