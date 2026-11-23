package core

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/chattocorp/chatto/cli/internal/core/linkpreview"
	"github.com/chattocorp/chatto/cli/internal/core/remoteimage"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
)

const (
	// remoteImageFetchTimeout bounds one remote image fetch, including redirects.
	remoteImageFetchTimeout = 10 * time.Second
	// remoteImageFailureKeyPrefix names the RUNTIME_STATE markers of remote
	// images that failed recently. The key ends in an HMAC of the source URL,
	// which can identify an account.
	remoteImageFailureKeyPrefix = "remote_image_failure."
	// remoteImageFailureTTL is how long all replicas skip a source after it
	// failed.
	remoteImageFailureTTL = 5 * time.Minute
)

// remoteImageClient fetches remote images. Tests replace it before they
// fetch; production code must not reassign it.
var remoteImageClient = linkpreview.NewSSRFSafeClient(remoteImageFetchTimeout)

// RemoteImagePath returns the signed, server-relative URL through which this
// server serves rawURL resized to preset. Sign only URLs that the server
// decided to show.
func (c *ChattoCore) RemoteImagePath(preset remoteimage.Preset, rawURL string) string {
	return remoteimage.Path(c.config.Assets.SigningSecret, preset, rawURL)
}

// VerifyRemoteImageSignature reports whether signature authorizes serving
// rawURL resized to preset.
func (c *ChattoCore) VerifyRemoteImageSignature(preset remoteimage.Preset, rawURL, signature string) bool {
	return remoteimage.Verify(c.config.Assets.SigningSecret, preset, rawURL, signature)
}

// FetchRemoteImage downloads the raw bytes of a remote image with the
// SSRF-safe client and the default size limit. It returns an error that wraps
// remoteimage.ErrRejected when the remote host does not supply an acceptable
// response. Errors do not contain the URL; callers must not log it.
//
// After a failure, every replica skips the source for remoteImageFailureTTL,
// so a broken or slow host does not cause a remote request for each client
// request.
func (c *ChattoCore) FetchRemoteImage(ctx context.Context, rawURL string) ([]byte, error) {
	failureKey := c.runtimeTokenKey(remoteImageFailureKeyPrefix, rawURL)
	// A lagging replica can miss a new marker. That costs one more fetch, so
	// the fast read is enough.
	if _, err := c.storage.runtimeStateKV.GetAnyReplica(ctx, failureKey); err == nil {
		return nil, fmt.Errorf("%w: failed recently", remoteimage.ErrRejected)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, remoteImageFetchTimeout)
	defer cancel()
	data, err := remoteimage.Fetch(fetchCtx, remoteImageClient, rawURL, remoteimage.MaxSourceBytes)
	if err != nil && ctx.Err() == nil {
		c.MarkRemoteImageFailure(ctx, rawURL)
	}
	return data, err
}

// MarkRemoteImageFailure makes every replica skip rawURL for
// remoteImageFailureTTL. FetchRemoteImage calls it for failed downloads;
// callers call it when a downloaded source turns out not to be a usable
// image. Concurrent first requests on several replicas can each fetch before
// a marker exists, so the marker bounds repeated failures, not the first one.
func (c *ChattoCore) MarkRemoteImageFailure(ctx context.Context, rawURL string) {
	failureKey := c.runtimeTokenKey(remoteImageFailureKeyPrefix, rawURL)
	// The marker's presence is the record, so it has no value.
	if _, err := c.storage.runtimeStateKV.Create(context.WithoutCancel(ctx), failureKey, nil, jetstream.KeyTTL(remoteImageFailureTTL)); err != nil && !jetstreamutil.IsSequenceConflict(err) {
		c.logger.Warn("Failed to record remote image failure", "error", err)
	}
}
