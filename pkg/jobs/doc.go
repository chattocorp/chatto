// Package jobs runs durable background jobs on NATS JetStream.
//
// One job stream is a small event log: each job is a sequence of facts, and
// the state of a job is the fold of its facts. Queue.Enqueue and Queue.Retry
// append facts under optimistic concurrency control. Queue.Work runs jobs from
// an application-owned durable consumer on the queued and retried facts: the
// acknowledgement wait is the claim, the delivery count is the attempt number,
// and a failed attempt returns with capped exponential backoff. Queue.Run keeps
// an in-memory view of all jobs for Queue.Get, Queue.List, Queue.Retry, and
// Queue.Delete. An unfinished job stays until it finishes. A succeeded job
// leaves one fact that expires after a retention period. A failed job leaves
// one fact that stays for inspection and manual retry until Queue.Delete, or
// until an optional retention period.
//
// Effect turns an existing durable consumer into a job source: it accepts one
// reference job per relevant source message before it acknowledges that
// message, and each attempt runs the unchanged events.DurableDeliveryHandler.
//
// The application owns resource names and configurations, job types and
// policies, handlers, and every user interface or API over the queue.
package jobs
