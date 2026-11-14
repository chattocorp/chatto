// Package jobs runs durable background jobs on NATS JetStream.
//
// A key-value bucket holds the current record of each job. A stream holds a
// replayable dispatch reference per job. Consumers claim a job by a
// revision-checked write with a renewed lease, so several processes can share
// one queue. Failed attempts wait with capped exponential backoff without
// holding a worker. Exhausted and permanently failed jobs stay for inspection
// and manual retry; succeeded records expire after a retention period.
//
// Effect turns an existing durable consumer into a job source: it accepts one
// reference job per relevant source message before it acknowledges that
// message, and each attempt runs the unchanged events.DurableDeliveryHandler.
//
// The application owns resource names and configurations, job types and
// policies, handlers, and every user interface or API over the queue.
package jobs
