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

// maxConcurrentImageSources bounds source reads and buffered originals through
// the end of a transform. Two slots allow small transforms to run concurrently
// or one source to load while a large transform holds the pixel budget.
const maxConcurrentImageSources = 2

// imageTransforms limits concurrent on-demand image work in one process.
// A decoder holds all pixels of an image in memory, so a 40
// megapixel image needs approximately 160 MB. The limits are per process
// because memory is per process.
//
//   - Concurrent requests for the same derivative share one transform. Only
//     one of them reads the source.
//   - At most maxConcurrentImageSources operations read or hold source bytes.
//     Other operations wait before opening their source. A source slot stays
//     held while waiting for pixels and through the end of the transform.
//   - Transforms acquire a shared assets.MaxDecodedImagePixels budget before
//     decoding. Small images run concurrently. A large image waits until the
//     transforms in progress release their pixels, and transforms that start
//     to wait later wait behind it.
//
// These limits do not bound total bytes: encoded sources, decoded pixel
// formats, transform output, and buffers retained by callers vary in size.
// An animated GIF counts as at most
// assets.MaxDecodedImagePixels pixels, although its frames together can have
// more.
//
// The zero value is ready for use.
type imageTransforms struct {
	flight     singleflight.Group
	limitsOnce sync.Once
	sources    *semaphore.Weighted
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
// it waits for a source slot, calls read to get the source bytes, waits until
// enough pixels are free, and calls transform. Same-key followers do not hold
// source slots.
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
	t.limitsOnce.Do(func() {
		t.sources = semaphore.NewWeighted(maxConcurrentImageSources)
		t.pixels = semaphore.NewWeighted(assets.MaxDecodedImagePixels)
	})
	if err := t.sources.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer t.sources.Release(1)

	data, err := read()
	if err != nil {
		return nil, err
	}
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
