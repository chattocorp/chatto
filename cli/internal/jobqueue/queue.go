// Package jobqueue owns Chatto's shared pending-job stream. Features own job
// payloads, durable consumers, handlers, and retry policy. Job handlers can use
// the existing events.DurableWorker for delayed retries and double acknowledgement.
package jobqueue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// StreamName is the file-backed stream for all job types. Backups include it.
const StreamName = "JOBS"

// Queue publishes opaque feature-owned jobs into the shared work queue.
// JetStream owns pending state. Acknowledged jobs are removed, and MaxAge
// deletes outstanding jobs even if no worker has processed them.
type Queue struct {
	js     jetstream.JetStream
	stream jetstream.Stream
}

// New creates or updates the shared stream. maxAge must be positive. There
// are no byte or message-count limits. Replicas follows the server's NATS policy.
func New(ctx context.Context, js jetstream.JetStream, maxAge time.Duration, replicas int) (*Queue, error) {
	if maxAge <= 0 {
		return nil, fmt.Errorf("job queue max age must be positive")
	}
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: StreamName, Description: "Pending background jobs",
		Subjects: []string{"jobs.>"}, Retention: jetstream.WorkQueuePolicy,
		Storage: jetstream.FileStorage, Replicas: replicas, MaxAge: maxAge,
		MaxBytes: -1, MaxMsgs: -1, MaxMsgsPerSubject: -1,
		Duplicates: min(2*time.Minute, maxAge),
	})
	if err != nil {
		return nil, err
	}
	return &Queue{js: js, stream: stream}, nil
}

// Stream returns the shared stream for application-owned durable consumers.
// Each job type must use a non-overlapping filter such as jobs.bot_webhook.deliver.
// Keep named consumers across normal process restarts; do not delete on shutdown.
func (q *Queue) Stream() jetstream.Stream { return q.stream }

// Subject returns the exact subject for a feature-owned job type. Types use
// dot-separated lowercase letters, digits, or underscores, never wildcards.
func Subject(jobType string) (string, error) {
	for _, token := range strings.Split(jobType, ".") {
		if token == "" {
			return "", fmt.Errorf("job type must contain non-empty tokens")
		}
		for _, c := range token {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
				return "", fmt.Errorf("job type contains an invalid character")
			}
		}
	}
	return "jobs." + jobType, nil
}

// Enqueue confirms a job publication before returning. id is a stable opaque
// identifier within the job type. Deduplication lasts up to two minutes (or
// MaxAge if shorter), so handlers and receivers must tolerate later duplicates.
// The payload is stored unchanged; callers own its encoding and privacy policy.
func (q *Queue) Enqueue(ctx context.Context, jobType, id string, payload []byte) error {
	subject, err := Subject(jobType)
	if err != nil {
		return err
	}
	if id == "" || strings.ContainsAny(id, "\r\n") {
		return fmt.Errorf("job ID must be non-empty and contain no line breaks")
	}
	_, err = q.js.Publish(ctx, subject, payload, jetstream.WithMsgID(jobType+":"+id))
	return err
}
