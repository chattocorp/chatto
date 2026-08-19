package core

import "hmans.de/chatto/internal/notificationstream"

const assetProcessingConsumerName = "chatto-asset-processing-v1"

// DurableWorkerHealth is broker-derived health for a known durable queue.
type DurableWorkerHealth int

const (
	DurableWorkerHealthInactive DurableWorkerHealth = iota
	DurableWorkerHealthHealthy
	DurableWorkerHealthWorking
	DurableWorkerHealthUnconfirmed
	DurableWorkerHealthStalled
	DurableWorkerHealthUnavailable
)

// DurableWorkerAdminStatus contains owner-only operational queue metadata.
type DurableWorkerAdminStatus struct {
	Key                   string
	Health                DurableWorkerHealth
	PendingCount          uint64
	AckPendingCount       uint64
	WaitingCount          int
	RedeliveredCount      int
	LastDeliveredSequence uint64
	AckFloorSequence      uint64
}

type durableWorkerDiagnosticSpec struct {
	key          string
	streamName   string
	consumerName string
	required     bool
}

func durableWorkerAdminStatuses(stats *JetStreamStats, videoUploadsEnabled bool) []DurableWorkerAdminStatus {
	specs := []durableWorkerDiagnosticSpec{
		{key: "asset_cleanup", streamName: "EVT", consumerName: assetCleanupConsumerName, required: true},
		{key: "call_key_cleanup", streamName: "EVT", consumerName: callKeyCleanupConsumerName, required: true},
		{key: "user_key_shredding", streamName: "EVT", consumerName: userKeyShreddingConsumerName, required: true},
		{key: "user_push_subscription_cleanup", streamName: "EVT", consumerName: pushSubscriptionCleanupConsumerName, required: true},
		{key: "notification_materializer", streamName: "EVT", consumerName: notificationWorkerConsumerName, required: true},
		{key: "notification_alert_delivery", streamName: notificationstream.StreamName, consumerName: notificationAlertConsumerName, required: true},
		{key: "asset_processing", streamName: "EVT", consumerName: assetProcessingConsumerName, required: videoUploadsEnabled},
	}
	type consumerCoordinate struct{ stream, name string }
	consumers := make(map[consumerCoordinate]ConsumerStats)
	if stats != nil {
		for _, consumer := range stats.Consumers {
			consumers[consumerCoordinate{stream: consumer.Stream, name: consumer.Name}] = consumer
		}
	}

	statuses := make([]DurableWorkerAdminStatus, 0, len(specs))
	for _, spec := range specs {
		consumer, found := consumers[consumerCoordinate{stream: spec.streamName, name: spec.consumerName}]
		if !found {
			health := DurableWorkerHealthInactive
			if spec.required {
				health = DurableWorkerHealthUnavailable
			}
			statuses = append(statuses, DurableWorkerAdminStatus{Key: spec.key, Health: health})
			continue
		}

		pendingCount := consumer.Pending
		hasUnacknowledgedWork := pendingCount > 0 || consumer.AckPending > 0
		health := DurableWorkerHealthInactive
		switch {
		case !spec.required:
		case consumer.Waiting > 0 && hasUnacknowledgedWork:
			health = DurableWorkerHealthWorking
		case consumer.Waiting > 0:
			health = DurableWorkerHealthHealthy
		case consumer.AckPending > 0:
			health = DurableWorkerHealthUnconfirmed
		default:
			health = DurableWorkerHealthStalled
		}
		statuses = append(statuses, DurableWorkerAdminStatus{
			Key:                   spec.key,
			Health:                health,
			PendingCount:          pendingCount,
			AckPendingCount:       uint64(max(consumer.AckPending, 0)),
			WaitingCount:          consumer.Waiting,
			RedeliveredCount:      consumer.Redelivered,
			LastDeliveredSequence: consumer.DeliveredStreamSeq,
			AckFloorSequence:      consumer.AckFloorStreamSeq,
		})
	}
	return statuses
}
